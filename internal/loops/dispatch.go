package loops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gotodo/internal/task"
)

// HookSet is every process boundary the engine crosses. Tests replace
// Hooks so no real runner or agent CLI ever starts.
type HookSet struct {
	// StartRunner starts `gotodo -file <board> loops run <task> <stage>`
	// detached, with stdout/stderr appended to logPath, and returns its pid.
	StartRunner func(boardPath, taskID, stage, logPath string) (int, error)
	// ExecAgent runs the agent CLI to completion, writing its output to
	// out. It calls started with the agent's pid once it is running and
	// stops the agent's process group when ctx ends. exit is -1 when the
	// agent did not exit normally.
	ExecAgent func(ctx context.Context, c Command, out io.Writer, started func(pid int)) (exit int, err error)
	// Alive reports whether a pid is still running.
	Alive func(pid int) bool
	// KillGroup sends SIGTERM to a process group.
	KillGroup func(pgid int) error
	// HasSkill reports whether agent can load skill.
	HasSkill func(agent, skill string) bool
	// LookPath finds an agent CLI on PATH.
	LookPath func(name string) (string, error)
}

// DefaultHooks are the real process boundaries.
func DefaultHooks() HookSet {
	return HookSet{
		StartRunner: startRunner,
		ExecAgent:   execAgent,
		Alive:       processAlive,
		KillGroup:   func(pgid int) error { return killGroup(pgid, syscall.SIGTERM) },
		HasSkill:    HasSkill,
		LookPath:    exec.LookPath,
	}
}

// Hooks is what Dispatch, Run and Stop use.
var Hooks = DefaultHooks()

// AgentName is the GOTODO_AGENT a stage's runs claim and act as.
func AgentName(stage string) string { return "loop:" + stage }

// LoopEnv is set to 1 in every agent a loop starts.
const LoopEnv = "GOTODO_LOOP"

// BoardEnv carries the absolute board path into the agent a loop starts.
const BoardEnv = "GOTODO_BOARD"

// InLoop reports whether this process was started by a loop.
func InLoop() bool { return os.Getenv(LoopEnv) == "1" }

// Launch is one runner Dispatch started.
type Launch struct {
	TaskID  string
	Stage   string
	Agent   string
	N       int // run number for (task, stage), 1-based
	PID     int
	Log     string
	Started time.Time
}

// Dispatch starts loop runs for cards that entered a configured column
// since the last call. It is idempotent and safe to call from any process
// after any board save: a lock serialises dispatchers, and per-task cursors
// in state.json make each transition count once. It also drains queued runs
// when slots free up and marks runs whose runner died as lost.
//
// With no loops config and no state it touches nothing. When loops are off
// it only advances the cursor, so turning them on never replays history.
func Dispatch(boardPath string, now time.Time) ([]Launch, error) {
	cfg, cfgPath, err := Load(boardPath)
	if err != nil {
		return nil, err
	}
	dir := StateDir(boardPath)
	if cfgPath == "" {
		if _, err := os.Stat(statePath(dir)); err != nil {
			return nil, nil
		}
	}
	var launches []Launch
	var notes []skillNote
	err = withState(boardPath, func(s *state) (bool, error) {
		b, err := task.Load(boardPath)
		if err != nil {
			return false, err
		}
		launches, notes = dispatchLocked(boardPath, dir, cfg, b, s, now)
		return true, nil
	})
	for _, n := range notes {
		if err := addTaskNote(boardPath, n.taskID, n.text, now); err != nil {
			eventLog(dir, now, "could not note %s: %v", shortID(n.taskID), err)
		}
	}
	return launches, err
}

type skillNote struct{ taskID, text string }

func dispatchLocked(boardPath, dir string, cfg Config, b *task.Board, s *state, now time.Time) ([]Launch, []skillNote) {
	if s.Primed.IsZero() {
		// First enable: everything already on the board is history.
		primeState(s, b, now)
	}
	reapRuns(dir, cfg, s, now)
	active := cfg.Enabled && len(cfg.Stages) > 0

	// New transitions into configured, enabled stages.
	var fresh []Queued
	onBoard := make(map[string]bool, len(b.Tasks))
	for _, t := range b.Tasks {
		onBoard[t.ID] = true
		start := s.Processed[t.ID]
		if start > len(t.History) {
			start = len(t.History) // history was rewritten by a merge
		}
		for _, tr := range t.History[start:] {
			if !active || t.Archived || !tr.At.After(s.Primed) {
				continue
			}
			if tr.Agent == task.DepsAgent {
				continue // auto-block/unblock is bookkeeping, not work arriving
			}
			st, ok := cfg.Stages[string(tr.To)]
			if !ok || !st.On() {
				continue
			}
			fresh = append(fresh, Queued{TaskID: t.ID, Stage: string(tr.To), QueuedAt: now})
		}
		s.Processed[t.ID] = len(t.History)
	}
	for id := range s.Processed {
		if !onBoard[id] {
			delete(s.Processed, id) // deleted tasks
		}
	}

	if !active {
		for _, q := range s.Queue {
			eventLog(dir, now, "dropped queued %s %s: loops are off", shortID(q.TaskID), q.Stage)
		}
		s.Queue = nil
		return nil, nil
	}

	oldQueue := s.Queue
	pending := append([]Queued(nil), oldQueue...)
	for _, q := range fresh {
		if !containsQueued(pending, q) {
			pending = append(pending, q)
		}
	}
	parallel, perTask := cfg.Limits()
	var launches []Launch
	var notes []skillNote
	s.Queue = nil
	for _, q := range pending {
		wasQueued := containsQueued(oldQueue, q)
		st, ok := cfg.Stages[q.Stage]
		if !ok || !st.On() {
			eventLog(dir, now, "dropped %s %s: stage no longer configured", shortID(q.TaskID), q.Stage)
			continue
		}
		t, ok := b.TaskByID(q.TaskID)
		if !ok || t.Archived {
			eventLog(dir, now, "dropped %s %s: task deleted or archived", shortID(q.TaskID), q.Stage)
			continue
		}
		if string(t.Status) != q.Stage {
			eventLog(dir, now, "dropped %s %s: card moved on to %s", shortID(q.TaskID), q.Stage, t.Status)
			continue
		}
		key := countKey(t.ID, q.Stage)
		if s.Counts[key] >= perTask {
			eventLog(dir, now, "loop skipped: %s %s already ran %d times (max_runs_per_task)", shortID(t.ID), q.Stage, s.Counts[key])
			continue
		}
		if st.Skill != "" && !skillFound(boardPath, t, st) {
			msg := fmt.Sprintf("loop skipped: skill %s not found for %s (run gotodo setup)", st.Skill, st.Agent)
			eventLog(dir, now, "%s %s: %s", shortID(t.ID), q.Stage, msg)
			noteKey := key + "|" + st.Skill
			if !s.Noted[noteKey] {
				if s.Noted == nil {
					s.Noted = map[string]bool{}
				}
				s.Noted[noteKey] = true
				notes = append(notes, skillNote{t.ID, msg})
			}
			continue
		}
		if s.taskRunning(t.ID) || len(s.running()) >= parallel {
			// Wait for this card's current run, or for a free slot.
			s.Queue = append(s.Queue, q)
			if !wasQueued {
				eventLog(dir, now, "queued %s %s", shortID(t.ID), q.Stage)
			}
			continue
		}
		if t.ClaimHeld(now) && t.ClaimedBy != AgentName(q.Stage) {
			eventLog(dir, now, "loop skipped: %s %s is claimed by %s", shortID(t.ID), q.Stage, t.ClaimedBy)
			continue
		}
		if s.Counts == nil {
			s.Counts = map[string]int{}
		}
		s.Counts[key]++
		n := s.Counts[key]
		logPath := filepath.Join(dir, fmt.Sprintf("%s-%s-%d.log", shortID(t.ID), q.Stage, n))
		rec := RunRecord{N: n, TaskID: t.ID, Stage: q.Stage, Agent: st.Agent, Model: st.Model,
			Started: now, Status: RunRunning, Log: logPath}
		pid, err := Hooks.StartRunner(boardPath, t.ID, q.Stage, logPath)
		if err != nil {
			exit := -1
			s.finish(&rec, RunFailed, &exit, "start runner: "+err.Error(), now)
			s.Runs = append(s.Runs, rec)
			eventLog(dir, now, "could not start %s %s: %v", shortID(t.ID), q.Stage, err)
			continue
		}
		rec.PID = pid
		s.Runs = append(s.Runs, rec)
		eventLog(dir, now, "started %s %s run %d (%s, pid %d) → %s", shortID(t.ID), q.Stage, n, st.Agent, pid, logPath)
		launches = append(launches, Launch{TaskID: t.ID, Stage: q.Stage, Agent: st.Agent, N: n,
			PID: pid, Log: logPath, Started: now})
	}
	return launches, notes
}

// StageSkillFound reports whether a stage's agent can load its skill from
// the user's skill directories or the project at boardPath's root. Status
// output uses it so ✓/✗ agrees with what Dispatch will do.
func StageSkillFound(boardPath string, st Stage) bool {
	return Hooks.HasSkill(st.Agent, st.Skill) || hasSkillName(ProjectSkills(st.Agent, ProjectRoot(boardPath)), st.Skill)
}

// skillFound reports whether a stage's agent can load its skill: from the
// user's skill directories (Hooks.HasSkill), or from the project skills of
// the project root or the task's worktree, where the run starts.
func skillFound(boardPath string, t task.Task, st Stage) bool {
	if Hooks.HasSkill(st.Agent, st.Skill) {
		return true
	}
	root := ProjectRoot(boardPath)
	cwd, _ := workDir(boardPath, t)
	for _, dir := range []string{root, cwd} {
		if hasSkillName(ProjectSkills(st.Agent, dir), st.Skill) {
			return true
		}
		if cwd == root {
			break
		}
	}
	return false
}

func containsQueued(list []Queued, q Queued) bool {
	for _, x := range list {
		if x.TaskID == q.TaskID && x.Stage == q.Stage {
			return true
		}
	}
	return false
}

// reapRuns marks runs lost when their runner died without recording an
// end, or when they outlived the timeout by a wide margin.
func reapRuns(dir string, cfg Config, s *state, now time.Time) {
	limit := cfg.Timeout() + 15*time.Minute
	for _, r := range s.running() {
		switch {
		case r.PID > 0 && !Hooks.Alive(r.PID):
			s.finish(r, RunLost, nil, "runner exited without recording an end", now)
			eventLog(dir, now, "lost %s %s run %d: runner pid %d is gone", shortID(r.TaskID), r.Stage, r.N, r.PID)
		case now.Sub(r.Started) > limit:
			Hooks.KillGroup(r.AgentPID)
			Hooks.KillGroup(r.PID)
			s.finish(r, RunLost, nil, "overran the timeout; killed", now)
			eventLog(dir, now, "killed %s %s run %d: overran the timeout", shortID(r.TaskID), r.Stage, r.N)
		}
	}
}

func primeState(s *state, b *task.Board, now time.Time) {
	s.Primed = now
	s.Processed = make(map[string]int, len(b.Tasks))
	for _, t := range b.Tasks {
		s.Processed[t.ID] = len(t.History)
	}
	s.Queue = nil
}

// Prime moves the cursor to now: every transition already on the board is
// treated as history. Call it when loops are switched on.
func Prime(boardPath string, now time.Time) error {
	return withState(boardPath, func(s *state) (bool, error) {
		b, err := task.Load(boardPath)
		if err != nil {
			return false, err
		}
		primeState(s, b, now)
		return true, nil
	})
}

// addTaskNote appends text to a task's description unless it is already
// there, keeping every other field.
func addTaskNote(boardPath, taskID, text string, now time.Time) error {
	return task.UpdateLocked(boardPath, "", func(b *task.Board) error {
		t, ok := b.TaskByID(taskID)
		if !ok || strings.Contains(t.Description, text) {
			return nil
		}
		desc := strings.TrimSpace(t.Description)
		if desc != "" {
			desc += "\n\n"
		}
		return b.EditWithParent(t.ID, t.ParentID, t.Title, desc+text, t.Deadline, now)
	})
}

// Report is what `gotodo loops` shows about runs.
type Report struct {
	Running []RunRecord
	Queued  []Queued
	Recent  []RunRecord // finished runs, newest first
}

// Status reads the loop state without changing it. Running runs whose
// runner is gone are shown as lost.
func Status(boardPath string) (Report, error) {
	s, err := readState(StateDir(boardPath))
	if err != nil {
		return Report{}, err
	}
	var r Report
	for _, run := range s.Runs {
		if run.Status == RunRunning {
			if run.PID > 0 && !Hooks.Alive(run.PID) {
				run.Status = RunLost
				r.Recent = append(r.Recent, run)
				continue
			}
			r.Running = append(r.Running, run)
			continue
		}
		r.Recent = append(r.Recent, run)
	}
	for i, j := 0, len(r.Recent)-1; i < j; i, j = i+1, j-1 {
		r.Recent[i], r.Recent[j] = r.Recent[j], r.Recent[i]
	}
	r.Queued = s.Queue
	return r, nil
}

// LatestLog is the newest run log for a task, optionally for one stage.
func LatestLog(boardPath, taskID, stage string) (RunRecord, error) {
	s, err := readState(StateDir(boardPath))
	if err != nil {
		return RunRecord{}, err
	}
	var best *RunRecord
	for i := range s.Runs {
		r := &s.Runs[i]
		if r.TaskID != taskID || (stage != "" && r.Stage != stage) {
			continue
		}
		if best == nil || !r.Started.Before(best.Started) {
			best = r
		}
	}
	if best == nil {
		return RunRecord{}, errors.New("no loop runs for this task")
	}
	return *best, nil
}

// Stop ends a task's live runs: it kills the agent's and the runner's
// process groups, marks the runs stopped, drops the task's queued runs,
// releases the loop claims and drains the queue.
func Stop(boardPath, taskID string, now time.Time) ([]RunRecord, error) {
	dir := StateDir(boardPath)
	var stopped []RunRecord
	err := withState(boardPath, func(s *state) (bool, error) {
		changed := false
		for _, r := range s.running() {
			if r.TaskID != taskID {
				continue
			}
			Hooks.KillGroup(r.AgentPID)
			Hooks.KillGroup(r.PID)
			s.finish(r, RunStopped, nil, "stopped by gotodo loops stop", now)
			stopped = append(stopped, *r)
			eventLog(dir, now, "stopped %s %s run %d", shortID(r.TaskID), r.Stage, r.N)
			changed = true
		}
		kept := s.Queue[:0]
		for _, q := range s.Queue {
			if q.TaskID == taskID {
				eventLog(dir, now, "dropped queued %s %s: stopped", shortID(q.TaskID), q.Stage)
				changed = true
				continue
			}
			kept = append(kept, q)
		}
		s.Queue = kept
		return changed, nil
	})
	if err != nil {
		return nil, err
	}
	for _, r := range stopped {
		releaseClaim(boardPath, taskID, r.Stage, now)
	}
	if len(stopped) > 0 {
		Dispatch(boardPath, now)
	}
	return stopped, nil
}

func releaseClaim(boardPath, taskID, stage string, now time.Time) error {
	agent := AgentName(stage)
	return task.UpdateLocked(boardPath, agent, func(b *task.Board) error {
		t, ok := b.TaskByID(taskID)
		if !ok || t.ClaimedBy != agent {
			return nil
		}
		_, err := b.Release(taskID, agent, now)
		return err
	})
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// startRunner is the real StartRunner: the current gotodo binary, detached
// into its own session so it survives the dispatching command.
func startRunner(boardPath, taskID, stage, logPath string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	if strings.HasSuffix(filepath.Base(exe), ".test") {
		return 0, errors.New("refusing to start a loop runner from a test binary")
	}
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	cmd := exec.Command(exe, "-file", abs, "loops", "run", taskID, stage)
	cmd.Dir = ProjectRoot(abs)
	cmd.Stdout, cmd.Stderr = f, f
	cmd.SysProcAttr = detachedAttr()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	// Reap it when it ends, so a long-lived dispatcher (the TUI) never
	// keeps a zombie that still looks alive.
	go cmd.Wait()
	return cmd.Process.Pid, nil
}

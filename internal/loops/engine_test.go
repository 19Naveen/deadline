package loops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gotodo/internal/task"
)

// fakes records every process the engine would have started.
type fakes struct {
	mu       sync.Mutex
	runners  []string // "taskID|stage"
	nextPID  int
	dead     map[int]bool
	killed   []int
	skills   map[string]bool // "agent|skill"
	agentRun func(c Command, out io.Writer) int
	commands []Command
}

func installFakes(t *testing.T) *fakes {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	f := &fakes{nextPID: 1000, dead: map[int]bool{}, skills: map[string]bool{}}
	old := Hooks
	t.Cleanup(func() { Hooks = old })
	Hooks = HookSet{
		StartRunner: func(boardPath, taskID, stage, logPath string) (int, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.runners = append(f.runners, taskID+"|"+stage)
			f.nextPID++
			return f.nextPID, nil
		},
		ExecAgent: func(ctx context.Context, c Command, out io.Writer, started func(int)) (int, error) {
			f.mu.Lock()
			f.commands = append(f.commands, c)
			f.mu.Unlock()
			started(4242)
			fmt.Fprintln(out, "fake agent output")
			if f.agentRun != nil {
				return f.agentRun(c, out), nil
			}
			return 0, nil
		},
		Alive: func(pid int) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return !f.dead[pid]
		},
		KillGroup: func(pgid int) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.killed = append(f.killed, pgid)
			f.dead[pgid] = true
			return nil
		},
		HasSkill: func(agent, skill string) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.skills[agent+"|"+skill]
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
	}
	return f
}

func (f *fakes) launched() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.runners...)
}

// newBoard makes a dev project board with n todo tasks and returns the
// board path and task IDs.
func newBoard(t *testing.T, n int) (string, []string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".deadline", "board.json")
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	var ids []string
	for i := 0; i < n; i++ {
		ids = append(ids, b.Add(fmt.Sprintf("Task %d", i), "", nil, base).ID)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	return path, ids
}

var base = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

func move(t *testing.T, path, id string, to task.Status, actor string, at time.Time) {
	t.Helper()
	err := task.UpdateLocked(path, actor, func(b *task.Board) error { return b.Move(id, to, at) })
	if err != nil {
		t.Fatal(err)
	}
}

func writeCfg(t *testing.T, path string, c Config) {
	t.Helper()
	if err := WriteFile(ProjectPath(path), c); err != nil {
		t.Fatal(err)
	}
}

func enabledCfg() Config {
	return Config{Enabled: true, Stages: map[string]Stage{
		"indev":          {Agent: "claude", Skill: "implement"},
		"testing-review": {Agent: "codex", Skill: "verify"},
	}}
}

func readStateT(t *testing.T, path string) *state {
	t.Helper()
	s, err := readState(StateDir(path))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDispatchTriggersOnlyOnEntryIntoConfiguredStages(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"], f.skills["codex|verify"] = true, true
	path, ids := newBoard(t, 3)
	cfg := enabledCfg()
	off := false
	cfg.Stages["blocked"] = Stage{Agent: "claude", Skill: "implement", Enabled: &off}
	writeCfg(t, path, cfg)
	if err := Prime(path, base); err != nil {
		t.Fatal(err)
	}

	move(t, path, ids[0], task.StatusInDev, "", base.Add(time.Minute))             // configured
	move(t, path, ids[1], task.StatusBlocked, "", base.Add(time.Minute))           // disabled stage
	move(t, path, ids[2], task.StatusInDev, task.DepsAgent, base.Add(time.Minute)) // deps bookkeeping
	got, err := Dispatch(path, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TaskID != ids[0] || got[0].Stage != "indev" || got[0].N != 1 {
		t.Fatalf("launches = %+v, want one indev run for task 0", got)
	}
	if !strings.HasSuffix(got[0].Log, shortID(ids[0])+"-indev-1.log") {
		t.Errorf("log path = %s", got[0].Log)
	}
	if l := f.launched(); !slices.Equal(l, []string{ids[0] + "|indev"}) {
		t.Errorf("runners = %v", l)
	}
	// Idempotent: nothing new, nothing launched.
	if again, _ := Dispatch(path, base.Add(3*time.Minute)); len(again) != 0 {
		t.Errorf("second dispatch launched %+v", again)
	}
	if l := f.launched(); len(l) != 1 {
		t.Errorf("runners after second dispatch = %v", l)
	}
}

func TestDispatchIgnoresHistoryBeforeFirstEnable(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 1)
	move(t, path, ids[0], task.StatusInDev, "", base.Add(time.Minute)) // before loops exist
	writeCfg(t, path, enabledCfg())
	// No Prime: the first dispatch primes itself.
	if got, _ := Dispatch(path, base.Add(2*time.Minute)); len(got) != 0 {
		t.Fatalf("old history launched %+v", got)
	}
	move(t, path, ids[0], task.StatusTodo, "", base.Add(3*time.Minute))
	move(t, path, ids[0], task.StatusInDev, "", base.Add(4*time.Minute))
	if got, _ := Dispatch(path, base.Add(5*time.Minute)); len(got) != 1 {
		t.Fatalf("new entry launched %+v, want 1", got)
	}
}

func TestDispatchWithoutConfigTouchesNothing(t *testing.T) {
	installFakes(t)
	path, ids := newBoard(t, 1)
	move(t, path, ids[0], task.StatusInDev, "", base.Add(time.Minute))
	if got, err := Dispatch(path, base); err != nil || len(got) != 0 {
		t.Fatalf("dispatch = %+v %v", got, err)
	}
	if _, err := os.Stat(StateDir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state dir created without a config: %v", err)
	}
}

func TestDispatchWhileOffAdvancesCursor(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 1)
	cfg := enabledCfg()
	cfg.Enabled = false
	writeCfg(t, path, cfg)
	Dispatch(path, base)
	move(t, path, ids[0], task.StatusInDev, "", base.Add(time.Minute))
	if got, _ := Dispatch(path, base.Add(2*time.Minute)); len(got) != 0 {
		t.Fatalf("off launched %+v", got)
	}
	cfg.Enabled = true
	writeCfg(t, path, cfg)
	if got, _ := Dispatch(path, base.Add(3*time.Minute)); len(got) != 0 {
		t.Fatalf("turning on replayed a move made while off: %+v", got)
	}
}

func TestDispatchMaxRunsPerTask(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 1)
	cfg := enabledCfg()
	cfg.MaxRunsPerTask = 2
	writeCfg(t, path, cfg)
	Prime(path, base)
	at := base
	total := 0
	for i := 0; i < 4; i++ {
		at = at.Add(time.Minute)
		move(t, path, ids[0], task.StatusInDev, "", at)
		got, _ := Dispatch(path, at)
		total += len(got)
		// Finish the run so the next entry is not just queued behind it.
		for _, l := range got {
			f.dead[l.PID] = true
		}
		at = at.Add(time.Minute)
		move(t, path, ids[0], task.StatusTodo, "", at)
		Dispatch(path, at)
	}
	if total != 2 {
		t.Errorf("runs launched = %d, want max_runs_per_task 2", total)
	}
	data, _ := os.ReadFile(EventLogPath(path))
	if !strings.Contains(string(data), "already ran 2 times") {
		t.Errorf("event log missing max-runs skip:\n%s", data)
	}
}

func TestDispatchMaxParallelQueuesThenDrains(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 3)
	cfg := enabledCfg()
	cfg.MaxParallel = 1
	writeCfg(t, path, cfg)
	Prime(path, base)
	for _, id := range ids {
		move(t, path, id, task.StatusInDev, "", base.Add(time.Minute))
	}
	got, _ := Dispatch(path, base.Add(2*time.Minute))
	if len(got) != 1 {
		t.Fatalf("launched %d, want 1 (max_parallel)", len(got))
	}
	s := readStateT(t, path)
	if len(s.Queue) != 2 {
		t.Fatalf("queue = %+v, want 2", s.Queue)
	}
	// Still running: nothing drains.
	if again, _ := Dispatch(path, base.Add(3*time.Minute)); len(again) != 0 {
		t.Fatalf("drained while slot busy: %+v", again)
	}
	f.dead[got[0].PID] = true // runner gone → slot frees
	next, _ := Dispatch(path, base.Add(4*time.Minute))
	if len(next) != 1 || next[0].TaskID != ids[1] {
		t.Fatalf("drain = %+v, want task 1 (FIFO)", next)
	}
	if s := readStateT(t, path); len(s.Queue) != 1 {
		t.Errorf("queue after drain = %+v", s.Queue)
	}
	// A queued card that moves on is dropped, not run.
	move(t, path, ids[2], task.StatusTodo, "", base.Add(5*time.Minute))
	f.dead[next[0].PID] = true
	if last, _ := Dispatch(path, base.Add(6*time.Minute)); len(last) != 0 {
		t.Errorf("stale queued run launched: %+v", last)
	}
}

func TestDispatchMissingSkillNotesOnce(t *testing.T) {
	installFakes(t) // no skills installed
	path, ids := newBoard(t, 1)
	writeCfg(t, path, enabledCfg())
	Prime(path, base)
	for i := 0; i < 2; i++ {
		at := base.Add(time.Duration(2*i+1) * time.Minute)
		move(t, path, ids[0], task.StatusInDev, "", at)
		if got, _ := Dispatch(path, at); len(got) != 0 {
			t.Fatalf("launched without skill: %+v", got)
		}
		move(t, path, ids[0], task.StatusTodo, "", at.Add(time.Minute))
	}
	b, _ := task.Load(path)
	tk, _ := b.TaskByID(ids[0])
	want := "loop skipped: skill implement not found for claude (run gotodo setup)"
	if strings.Count(tk.Description, want) != 1 {
		t.Errorf("description = %q, want the note once", tk.Description)
	}
	if tk.Title != "Task 0" || tk.Status != task.StatusTodo {
		t.Errorf("note changed other fields: %+v", tk)
	}
	data, _ := os.ReadFile(EventLogPath(path))
	if strings.Count(string(data), want) != 2 {
		t.Errorf("event log should record each skip:\n%s", data)
	}
}

func TestDispatchSkipsCardsClaimedByOthers(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 1)
	writeCfg(t, path, enabledCfg())
	Prime(path, base)
	now := time.Now()
	task.UpdateLocked(path, "claude", func(b *task.Board) error { return b.Claim(ids[0], "claude", time.Hour, now) })
	move(t, path, ids[0], task.StatusInDev, "claude", now)
	if got, _ := Dispatch(path, now); len(got) != 0 {
		t.Errorf("launched on a card another agent claimed: %+v", got)
	}
}

func TestConcurrentDispatchLaunchesOnce(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 1)
	writeCfg(t, path, enabledCfg())
	Prime(path, base)
	move(t, path, ids[0], task.StatusInDev, "", base.Add(time.Minute))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Dispatch(path, base.Add(2*time.Minute)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if l := f.launched(); len(l) != 1 {
		t.Errorf("concurrent dispatch launched %v, want once", l)
	}
}

func TestBuildCommandPerAgentAndPermission(t *testing.T) {
	board := filepath.Join(t.TempDir(), "proj", ".deadline", "board.json")
	dir := filepath.Join(t.TempDir(), "wt")
	cases := []struct {
		st   Stage
		want []string
		not  []string
	}{
		{Stage{Agent: "claude", Model: "claude-opus-5-5", Permissions: PermReadOnly},
			[]string{"claude", "-p", "--allowedTools", "Bash(gotodo *)", "--permission-mode", "plan", "--model", "claude-opus-5-5"}, []string{"--dangerously-skip-permissions"}},
		{Stage{Agent: "claude"}, []string{"--permission-mode", "acceptEdits"}, []string{"--model"}},
		{Stage{Agent: "claude", Permissions: PermFull}, []string{"-p", "--dangerously-skip-permissions"}, []string{"--permission-mode", "--allowedTools"}},
		{Stage{Agent: "codex", Model: "gpt-5", Permissions: PermReadOnly}, []string{"codex", "exec", "--sandbox", "read-only", "-m", "gpt-5"}, nil},
		{Stage{Agent: "codex"}, []string{"--sandbox", "workspace-write", "--add-dir"}, []string{"--dangerously-bypass-approvals-and-sandbox"}},
		{Stage{Agent: "codex", Permissions: PermFull}, []string{"--dangerously-bypass-approvals-and-sandbox"}, []string{"--sandbox"}},
		{Stage{Agent: "opencode", Model: "anthropic/claude-sonnet-5-5"}, []string{"opencode", "run", "-m", "anthropic/claude-sonnet-5-5"}, []string{"--auto"}},
		{Stage{Agent: "opencode", Permissions: PermFull}, []string{"--auto"}, nil},
	}
	for _, c := range cases {
		cmd := BuildCommand("indev", c.st, "do the work", dir, board)
		argv := append([]string{cmd.Name}, cmd.Args...)
		if argv[len(argv)-1] != "do the work" {
			t.Errorf("%+v: prompt not last: %v", c.st, argv)
		}
		for _, w := range c.want {
			if !slices.Contains(argv, w) {
				t.Errorf("%+v: argv %v missing %q", c.st, argv, w)
			}
		}
		for _, n := range c.not {
			if slices.Contains(argv, n) {
				t.Errorf("%+v: argv %v has %q", c.st, argv, n)
			}
		}
		if cmd.Dir != dir || !slices.Contains(cmd.Env, "GOTODO_AGENT=loop:indev") || !slices.Contains(cmd.Env, "GOTODO_LOOP=1") {
			t.Errorf("%+v: dir/env wrong: %s %v", c.st, cmd.Dir, cmd.Env)
		}
	}
	// A prompt that looks like a flag stays a positional argument.
	cmd := BuildCommand("indev", Stage{Agent: "claude"}, "--help me", dir, board)
	if last := cmd.Args[len(cmd.Args)-1]; strings.HasPrefix(last, "-") {
		t.Errorf("flag-like prompt passed as %q", last)
	}
	// opencode cannot express readonly: the log says so.
	if cmd := BuildCommand("indev", Stage{Agent: "opencode", Permissions: PermReadOnly}, "p", dir, board); len(cmd.Notes) == 0 {
		t.Error("opencode readonly has no note")
	}
}

func TestAgentEnvDropsNestedSessionMarkers(t *testing.T) {
	env := agentEnv([]string{"PATH=/bin", "CLAUDECODE=1", "GOTODO_AGENT=claude", "GOTODO_LOOP=0"}, "indev", "/b.json")
	want := []string{"PATH=/bin", "GOTODO_AGENT=loop:indev", "GOTODO_LOOP=1", "GOTODO_BOARD=/b.json"}
	if !slices.Equal(env, want) {
		t.Errorf("env = %v, want %v", env, want)
	}
}

func TestRunClaimsLogsAndReleases(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 1)
	writeCfg(t, path, enabledCfg())
	wt := filepath.Join(filepath.Dir(filepath.Dir(path)), "wt")
	os.MkdirAll(wt, 0o755)
	task.UpdateLocked(path, "", func(b *task.Board) error { return b.SetWorktree(ids[0], "wt", base) })
	var claimedDuring string
	f.agentRun = func(c Command, out io.Writer) int {
		b, _ := task.Load(path)
		tk, _ := b.TaskByID(ids[0])
		claimedDuring = tk.ClaimedBy
		return 3
	}
	var out strings.Builder
	if err := Run(path, ids[0], "indev", &out); err != nil {
		t.Fatal(err)
	}
	if claimedDuring != "loop:indev" {
		t.Errorf("claim during run = %q, want loop:indev", claimedDuring)
	}
	b, _ := task.Load(path)
	if tk, _ := b.TaskByID(ids[0]); tk.ClaimedBy != "" {
		t.Errorf("claim not released: %q", tk.ClaimedBy)
	}
	if len(f.commands) != 1 || f.commands[0].Dir != wt {
		t.Fatalf("agent cwd = %+v, want worktree %s", f.commands, wt)
	}
	rec, err := LatestLog(path, ids[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != RunFailed || rec.Exit == nil || *rec.Exit != 3 || rec.AgentPID != 4242 || rec.Ended == nil {
		t.Errorf("record = %+v", rec)
	}
	data, _ := os.ReadFile(rec.Log)
	for _, want := range []string{"started:", "cwd:      " + wt, "command:  claude -p", "fake agent output", "· failed · exit 3"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log missing %q:\n%s", want, data)
		}
	}
	if !strings.Contains(out.String(), "failed") {
		t.Errorf("summary = %q", out.String())
	}
}

func TestRunExitsOnClaimConflict(t *testing.T) {
	f := installFakes(t)
	path, ids := newBoard(t, 1)
	writeCfg(t, path, enabledCfg())
	now := time.Now()
	task.UpdateLocked(path, "claude", func(b *task.Board) error { return b.Claim(ids[0], "claude", time.Hour, now) })
	err := Run(path, ids[0], "indev", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "claimed by claude") {
		t.Fatalf("run = %v, want a claim conflict", err)
	}
	if len(f.commands) != 0 {
		t.Error("agent started despite the conflict")
	}
	b, _ := task.Load(path)
	if tk, _ := b.TaskByID(ids[0]); tk.ClaimedBy != "claude" {
		t.Errorf("other agent's claim touched: %q", tk.ClaimedBy)
	}
	if rec, _ := LatestLog(path, ids[0], "indev"); rec.Status != RunSkipped {
		t.Errorf("record status = %q, want skipped", rec.Status)
	}
}

func TestRunUsesDispatchedRecordAndDrainsQueue(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 2)
	cfg := enabledCfg()
	cfg.MaxParallel = 1
	writeCfg(t, path, cfg)
	Prime(path, base)
	move(t, path, ids[0], task.StatusInDev, "", base.Add(time.Minute))
	move(t, path, ids[1], task.StatusInDev, "", base.Add(time.Minute))
	got, _ := Dispatch(path, base.Add(2*time.Minute))
	if len(got) != 1 {
		t.Fatalf("launched %+v", got)
	}
	// Pretend this process is the launched runner.
	withState(path, func(s *state) (bool, error) {
		s.Runs[0].PID = os.Getpid()
		return true, nil
	})
	f.dead[got[0].PID] = true
	if err := Run(path, got[0].TaskID, "indev", io.Discard); err != nil {
		t.Fatal(err)
	}
	s := readStateT(t, path)
	if s.Counts[countKey(got[0].TaskID, "indev")] != 1 {
		t.Errorf("runner made a second record: %+v", s.Runs)
	}
	if l := f.launched(); len(l) != 2 || !strings.HasPrefix(l[1], ids[1]) {
		t.Errorf("queue not drained after run: %v", l)
	}
}

func TestStopKillsAndReleases(t *testing.T) {
	f := installFakes(t)
	f.skills["claude|implement"] = true
	path, ids := newBoard(t, 1)
	writeCfg(t, path, enabledCfg())
	Prime(path, base)
	move(t, path, ids[0], task.StatusInDev, "", base.Add(time.Minute))
	got, _ := Dispatch(path, base.Add(2*time.Minute))
	now := time.Now()
	task.UpdateLocked(path, "loop:indev", func(b *task.Board) error { return b.Claim(ids[0], "loop:indev", time.Hour, now) })
	stopped, err := Stop(path, ids[0], now)
	if err != nil || len(stopped) != 1 {
		t.Fatalf("stop = %+v %v", stopped, err)
	}
	if !slices.Contains(f.killed, got[0].PID) {
		t.Errorf("runner group not killed: %v", f.killed)
	}
	b, _ := task.Load(path)
	if tk, _ := b.TaskByID(ids[0]); tk.ClaimedBy != "" {
		t.Errorf("claim not released: %q", tk.ClaimedBy)
	}
	r, _ := Status(path)
	if len(r.Running) != 0 || len(r.Recent) != 1 || r.Recent[0].Status != RunStopped {
		t.Errorf("status after stop = %+v", r)
	}
}

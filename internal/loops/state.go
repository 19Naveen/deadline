package loops

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Run statuses.
const (
	RunRunning = "running"
	RunDone    = "done"    // agent exited 0
	RunFailed  = "failed"  // agent exited non-zero or could not start
	RunTimeout = "timeout" // stopped after Config.Timeout
	RunStopped = "stopped" // stopped by gotodo loops stop
	RunLost    = "lost"    // runner died without recording an end
	RunSkipped = "skipped" // runner exited before starting the agent (e.g. claim conflict)
)

// stampLayout is how loops print times: DD/MM/YYYY, 24-hour.
const stampLayout = "02/01/2006 15:04"

// keepFinished caps finished run records kept in state.json.
const keepFinished = 200

// RunRecord is one loop run, live or finished.
type RunRecord struct {
	N        int        `json:"n"` // 1-based run number for (task, stage)
	TaskID   string     `json:"task_id"`
	Stage    string     `json:"stage"`
	Agent    string     `json:"agent"`
	Model    string     `json:"model,omitempty"`
	PID      int        `json:"pid,omitempty"`       // the gotodo loops run process (session leader)
	AgentPID int        `json:"agent_pid,omitempty"` // the agent CLI (its own process group)
	Started  time.Time  `json:"started"`
	Ended    *time.Time `json:"ended,omitempty"`
	Exit     *int       `json:"exit,omitempty"`
	Status   string     `json:"status"`
	Log      string     `json:"log"`
	Note     string     `json:"note,omitempty"`
}

// Queued is a run waiting for a free slot.
type Queued struct {
	TaskID   string    `json:"task_id"`
	Stage    string    `json:"stage"`
	QueuedAt time.Time `json:"queued_at"`
}

// state is .deadline/loops/state.json.
type state struct {
	// Primed is when the cursor was first set; history before it never runs.
	Primed time.Time `json:"primed"`
	// Processed is, per task ID, how many History entries were handled.
	// History only grows, so new entries are History[Processed[id]:].
	Processed map[string]int `json:"processed"`
	// Counts is runs launched per "taskID|stage", for MaxRunsPerTask.
	Counts map[string]int `json:"counts,omitempty"`
	// Noted marks "taskID|stage|skill" missing-skill notes already written.
	Noted map[string]bool `json:"noted,omitempty"`
	Runs  []RunRecord     `json:"runs,omitempty"`
	Queue []Queued        `json:"queue,omitempty"`
}

func countKey(taskID, stage string) string { return taskID + "|" + stage }

// StateDir holds loop state, locks and logs: <root>/.deadline/loops for a
// project board, else <board dir>/.gotodo-loops.
func StateDir(boardPath string) string {
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		abs = boardPath
	}
	if ProjectPath(abs) != "" {
		return filepath.Join(filepath.Dir(abs), "loops")
	}
	return filepath.Join(filepath.Dir(abs), ".gotodo-loops")
}

// ProjectRoot is the directory runs start in when the task has no
// worktree: the folder holding .deadline, or the board's own folder.
func ProjectRoot(boardPath string) string {
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		abs = boardPath
	}
	if ProjectPath(abs) != "" {
		return filepath.Dir(filepath.Dir(abs))
	}
	return filepath.Dir(abs)
}

func statePath(dir string) string { return filepath.Join(dir, "state.json") }

func readState(dir string) (*state, error) {
	s := &state{}
	data, err := os.ReadFile(statePath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		s.Processed = map[string]int{}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", statePath(dir), err)
	}
	if s.Processed == nil {
		s.Processed = map[string]int{}
	}
	return s, nil
}

func writeState(dir string, s *state) error {
	s.trim()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, statePath(dir)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// trim drops the oldest finished runs beyond keepFinished.
func (s *state) trim() {
	finished := 0
	for _, r := range s.Runs {
		if r.Status != RunRunning {
			finished++
		}
	}
	if finished <= keepFinished {
		return
	}
	sort.SliceStable(s.Runs, func(i, j int) bool { return s.Runs[i].Started.Before(s.Runs[j].Started) })
	drop := finished - keepFinished
	kept := s.Runs[:0]
	for _, r := range s.Runs {
		if r.Status != RunRunning && drop > 0 {
			drop--
			continue
		}
		kept = append(kept, r)
	}
	s.Runs = kept
}

func (s *state) running() []*RunRecord {
	var out []*RunRecord
	for i := range s.Runs {
		if s.Runs[i].Status == RunRunning {
			out = append(out, &s.Runs[i])
		}
	}
	return out
}

func (s *state) taskRunning(taskID string) bool {
	for _, r := range s.running() {
		if r.TaskID == taskID {
			return true
		}
	}
	return false
}

func (s *state) finish(r *RunRecord, status string, exit *int, note string, now time.Time) {
	r.Status, r.Exit = status, exit
	end := now
	r.Ended = &end
	if note != "" {
		r.Note = note
	}
}

// withState runs fn on the board's loop state under the dispatch lock and
// writes it back when fn reports a change.
func withState(boardPath string, fn func(s *state) (bool, error)) error {
	dir := StateDir(boardPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(dir, "dispatch.lock"), lockWait)
	if err != nil {
		return err
	}
	defer unlock()
	s, err := readState(dir)
	if err != nil {
		return err
	}
	changed, err := fn(s)
	if err != nil || !changed {
		return err
	}
	return writeState(dir, s)
}

// lockWait bounds how long a caller queues behind another dispatcher.
const lockWait = 15 * time.Second

// eventLog appends one stamped line to <state dir>/loops.log, the log for
// loop decisions that have no run of their own (skips, queueing, stops).
func eventLog(dir string, now time.Time, format string, args ...any) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "loops.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", now.In(time.Local).Format(stampLayout), fmt.Sprintf(format, args...))
}

// EventLogPath is where loop decisions without a run are logged.
func EventLogPath(boardPath string) string {
	return filepath.Join(StateDir(boardPath), "loops.log")
}

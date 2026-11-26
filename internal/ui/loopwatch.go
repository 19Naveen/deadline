package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/loops"
)

// loopWatch is the app's cached read of the board's stage loops. The app
// refreshes it on the live-reload tick and after each save; the board and
// the Agents page share one pointer and read it on every View, so a View
// never touches the disk for loop state.
type loopWatch struct {
	on     bool // a config exists, is enabled and has stages
	report loops.Report
	err    string
}

// readLoopWatch reads the loop config and state for a board. A board with
// no path (tests, scratch boards) has no loops.
func readLoopWatch(boardPath string) loopWatch {
	if boardPath == "" {
		return loopWatch{}
	}
	var w loopWatch
	cfg, from, err := loops.Load(boardPath)
	if err != nil {
		w.err = err.Error()
	}
	w.on = err == nil && from != "" && cfg.Enabled && len(cfg.Stages) > 0
	r, err := loops.Status(boardPath)
	if err != nil {
		w.err = err.Error()
		return w
	}
	w.report = r
	return w
}

// running is the task's live run, if any.
func (w *loopWatch) running(taskID string) (loops.RunRecord, bool) {
	if w == nil {
		return loops.RunRecord{}, false
	}
	for _, r := range w.report.Running {
		if r.TaskID == taskID {
			return r, true
		}
	}
	return loops.RunRecord{}, false
}

// latest is the task's newest run, live or finished, by start time.
func (w *loopWatch) latest(taskID string) (loops.RunRecord, bool) {
	if w == nil {
		return loops.RunRecord{}, false
	}
	var best loops.RunRecord
	found := false
	for _, list := range [][]loops.RunRecord{w.report.Running, w.report.Recent} {
		for _, r := range list {
			if r.TaskID == taskID && (!found || r.Started.After(best.Started)) {
				best, found = r, true
			}
		}
	}
	return best, found
}

// find is the current record of one run (same task, stage and number), so
// an open log viewer sees its run finish.
func (w *loopWatch) find(run loops.RunRecord) (loops.RunRecord, bool) {
	if w == nil {
		return loops.RunRecord{}, false
	}
	for _, list := range [][]loops.RunRecord{w.report.Running, w.report.Recent} {
		for _, r := range list {
			if r.TaskID == run.TaskID && r.Stage == run.Stage && r.N == run.N {
				return r, true
			}
		}
	}
	return loops.RunRecord{}, false
}

// loopStyle marks live loop runs: amber, like work in progress.
var loopStyle = lipgloss.NewStyle().Foreground(colDoing)

// runMarker is the glyph for a run's status: ⟳ running, ✓ done, ✗ failed
// (or timed out, or lost), ■ stopped, – skipped.
func runMarker(status string) string {
	switch status {
	case loops.RunRunning:
		return loopStyle.Render("⟳")
	case loops.RunDone:
		return lipgloss.NewStyle().Foreground(colDone).Render("✓")
	case loops.RunFailed, loops.RunTimeout, loops.RunLost:
		return lipgloss.NewStyle().Foreground(colBlocked).Render("✗")
	case loops.RunStopped:
		return MutedStyle.Render("■")
	}
	return MutedStyle.Render("–")
}

// runDuration is a run's length: seconds under a minute, else FormatDuration.
func runDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return FormatDuration(d)
}

// runTook is how long a run took, or has taken so far.
func runTook(r loops.RunRecord, now time.Time) time.Duration {
	if r.Ended != nil {
		return r.Ended.Sub(r.Started)
	}
	return now.Sub(r.Started)
}

// runAgent is "claude · model", or just the agent without a model.
func runAgent(r loops.RunRecord) string {
	if r.Model == "" {
		return r.Agent
	}
	return r.Agent + " · " + r.Model
}

// loopSummary is the detail popup's Loop value: "indev · claude · running
// 12m" for a live run, "indev · done 26/11 13:11" for a finished one.
func loopSummary(r loops.RunRecord, now time.Time) string {
	if r.Status == loops.RunRunning {
		return runMarker(r.Status) + " " + r.Stage + MutedStyle.Render(" · "+r.Agent+" · ") +
			loopStyle.Render("running "+runDuration(runTook(r, now)))
	}
	end := r.Started
	if r.Ended != nil {
		end = *r.Ended
	}
	return runMarker(r.Status) + " " + r.Stage + MutedStyle.Render(" · "+r.Status+" "+end.In(now.Location()).Format("02/01 15:04"))
}

// cardLoop fits the "⟳ loop:stage" marker into budget cells, like
// cardClaim does for "◆ agent": the full name, else the bare stage, else
// shortened, else the glyph alone.
func cardLoop(r loops.RunRecord, budget int) string {
	if budget < 1 {
		return ""
	}
	if budget < 4 {
		return loopStyle.Render("⟳")
	}
	name := loops.AgentName(r.Stage)
	if lipgloss.Width(name) > budget-2 {
		name = r.Stage
	}
	return loopStyle.Render("⟳ " + truncate(name, budget-2))
}

// fileExists reports whether path names a readable regular file.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

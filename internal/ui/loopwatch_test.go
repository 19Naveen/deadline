package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/loops"
	"gotodo/internal/task"
)

// loopFixture is a dev project board on disk with loop config and state
// written by hand: no dispatcher, runner or agent ever starts.
type loopFixture struct {
	t     *testing.T
	path  string // board.json
	board *task.Board
	ids   map[string]string // title → task ID
}

func newLoopFixture(t *testing.T, on bool, titles ...string) *loopFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "proj", ".deadline", "board.json")
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	f := &loopFixture{t: t, path: path, board: b, ids: map[string]string{}}
	for _, title := range titles {
		id := b.Add(title, "", nil, ref.Add(-time.Hour)).ID
		if err := b.Move(id, task.StatusInDev, ref.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
		f.ids[title] = id
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if on {
		cfg := loops.Config{Enabled: true, Stages: map[string]loops.Stage{
			"indev": {Agent: "claude", Skill: "tdd"}, "testing-review": {Agent: "codex", Skill: "review"}}}
		if err := loops.WriteFile(loops.ProjectPath(path), cfg); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// run builds a record for title; ended is nil for a live run.
func (f *loopFixture) run(title, stage, status string, n int, started time.Time, ended *time.Time) loops.RunRecord {
	id := f.ids[title]
	return loops.RunRecord{N: n, TaskID: id, Stage: stage, Agent: "claude", Model: "opus", Started: started,
		Ended: ended, Status: status, Log: filepath.Join(loops.StateDir(f.path), fmt.Sprintf("%s-%s-%d.log", shortTaskID(id), stage, n))}
}

// state writes .deadline/loops/state.json the way the engine would.
func (f *loopFixture) state(runs []loops.RunRecord, queue []loops.Queued) {
	f.t.Helper()
	data, err := json.Marshal(map[string]any{"primed": ref.Add(-24 * time.Hour), "processed": map[string]int{}, "runs": runs, "queue": queue})
	if err != nil {
		f.t.Fatal(err)
	}
	dir := loops.StateDir(f.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *loopFixture) writeLog(r loops.RunRecord, text string) {
	f.t.Helper()
	if err := os.WriteFile(r.Log, []byte(text), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *loopFixture) appendLog(r loops.RunRecord, text string) {
	f.t.Helper()
	fh, err := os.OpenFile(r.Log, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		f.t.Fatal(err)
	}
	defer fh.Close()
	if _, err := fh.WriteString(text); err != nil {
		f.t.Fatal(err)
	}
}

// app opens the TUI on the fixture board, sized and pinned to ref.
func (f *loopFixture) app(w, h int) AppModel {
	f.t.Helper()
	a := NewApp(f.board)
	clock := func() time.Time { return ref }
	a.now, a.board.now, a.agents.now = clock, clock, clock
	m, _ := a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m.(AppModel)
}

func at(d time.Duration) *time.Time { t := ref.Add(d); return &t }

func send(a AppModel, msgs ...tea.Msg) AppModel {
	var m tea.Model = a
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(AppModel)
}

func toAgents(a AppModel) AppModel {
	for a.page != pageAgents {
		a = send(a, tea.KeyMsg{Type: tea.KeyTab})
	}
	return a
}

// openDetail selects the task and opens its detail popup.
func openDetail(t *testing.T, a AppModel, id string) AppModel {
	t.Helper()
	if !a.board.selectTaskID(id) {
		t.Fatalf("task %s not on the board", id)
	}
	a = send(a, tea.KeyMsg{Type: tea.KeyEnter})
	if a.board.mode != modeDetail {
		t.Fatalf("mode = %v, want detail", a.board.mode)
	}
	return a
}

// fits reports any frame that overflows the terminal.
func fits(t *testing.T, name string, out string, w, h int) {
	t.Helper()
	if got := lipgloss.Height(out); got > h {
		t.Errorf("%dx%d %s: %d rows tall:\n%s", w, h, name, got, stripANSI(out))
	}
	for i, line := range strings.Split(out, "\n") {
		if lw := lipgloss.Width(line); lw > w {
			t.Errorf("%dx%d %s: line %d is %d wide: %q", w, h, name, i, lw, stripANSI(line))
			break
		}
	}
}

// --- Agents tab ---

func TestAgentsTabListsLoopRuns(t *testing.T) {
	f := newLoopFixture(t, true, "[Live]", "[Waiting]", "[Shipped]", "[Broken]", "[Halted]", "[Passed]")
	f.state([]loops.RunRecord{
		f.run("[Shipped]", "indev", loops.RunDone, 1, ref.Add(-3*time.Hour), at(-2*time.Hour-46*time.Minute)),
		f.run("[Broken]", "indev", loops.RunFailed, 1, ref.Add(-2*time.Hour), at(-time.Hour)),
		f.run("[Halted]", "testing-review", loops.RunStopped, 2, ref.Add(-90*time.Minute), at(-80*time.Minute)),
		f.run("[Passed]", "indev", loops.RunSkipped, 1, ref.Add(-70*time.Minute), at(-70*time.Minute)),
		f.run("[Live]", "indev", loops.RunRunning, 1, ref.Add(-12*time.Minute), nil),
	}, []loops.Queued{{TaskID: f.ids["[Waiting]"], Stage: "testing-review", QueuedAt: ref.Add(-5 * time.Minute)}})

	a := toAgents(f.app(120, 44))
	out := stripANSI(a.View())
	for _, want := range []string{
		"Loops (1 running · 1 queued)",
		"⟳ indev", shortTaskID(f.ids["[Live]"]) + "  [Live]", "claude · opus  12m",
		"… testing-review", "[Waiting]", "queued 5m ago",
		"✓ indev", "[Shipped]", "done 30/07 09:14  14m",
		"✗ indev", "[Broken]", "failed 30/07 11:00  1h",
		"■ testing-review", "stopped 30/07 10:40  10m",
		"– indev", "skipped",
		"Agents (", "Activity",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Agents tab is missing %q:\n%s", want, out)
		}
	}
	// Loops come first, newest finished run first.
	if strings.Index(out, "Loops") > strings.Index(out, "Agents (") {
		t.Errorf("Loops section is not at the top:\n%s", out)
	}
	if strings.Index(out, "[Passed]") > strings.Index(out, "[Shipped]") {
		t.Errorf("finished runs are not newest first:\n%s", out)
	}
}

func TestAgentsTabCapsFinishedRuns(t *testing.T) {
	titles := []string{"[R0]", "[R1]", "[R2]", "[R3]", "[R4]", "[R5]", "[R6]"}
	f := newLoopFixture(t, true, titles...)
	var runs []loops.RunRecord
	for i, title := range titles {
		runs = append(runs, f.run(title, "indev", loops.RunDone, 1, ref.Add(time.Duration(i-10)*time.Hour), at(time.Duration(i-10)*time.Hour+time.Minute)))
	}
	f.state(runs, nil)
	out := stripANSI(toAgents(f.app(120, 44)).View())
	out = out[:strings.Index(out, "Agents (")] // the feed lists every card
	if strings.Count(out, "✓ indev") != loopRecentRows {
		t.Errorf("want the last %d finished runs:\n%s", loopRecentRows, out)
	}
	if strings.Contains(out, "[R0]") || !strings.Contains(out, "[R6]") {
		t.Errorf("want the newest runs, not the oldest:\n%s", out)
	}
}

func TestAgentsTabSaysWhenLoopsAreOff(t *testing.T) {
	for _, on := range []bool{false, true} {
		f := newLoopFixture(t, false, "[Card]")
		if on { // configured but switched off
			cfg := loops.Config{Enabled: false, Stages: map[string]loops.Stage{"indev": {Agent: "claude", Skill: "tdd"}}}
			if err := loops.WriteFile(loops.ProjectPath(f.path), cfg); err != nil {
				t.Fatal(err)
			}
		}
		out := stripANSI(toAgents(f.app(100, 30)).View())
		if !strings.Contains(out, "Loops are off · gotodo setup or gotodo loops on") {
			t.Errorf("configured=%v: missing the loops-off line:\n%s", on, out)
		}
		if strings.Contains(out, "running ·") {
			t.Errorf("configured=%v: loops off shows run counts:\n%s", on, out)
		}
	}
	// A board with no file (scratch, tests) has no loops either.
	out := stripANSI(toAgents(send(NewApp(&task.Board{}), tea.WindowSizeMsg{Width: 100, Height: 30})).View())
	if !strings.Contains(out, "Loops are off") {
		t.Errorf("pathless board:\n%s", out)
	}
}

// A run started after the app opened shows up on the next refresh tick.
func TestAgentsTabRefreshesOnTick(t *testing.T) {
	f := newLoopFixture(t, true, "[Late]")
	a := toAgents(f.app(120, 40))
	if out := stripANSI(a.View()); strings.Contains(out, "⟳") {
		t.Fatalf("run shown before it exists:\n%s", out)
	}
	f.state([]loops.RunRecord{f.run("[Late]", "indev", loops.RunRunning, 1, ref.Add(-2*time.Minute), nil)}, nil)
	a = send(a, refreshTickMsg(ref))
	if out := stripANSI(a.View()); !strings.Contains(out, "⟳ indev") || !strings.Contains(out, "[Late]") {
		t.Errorf("tick did not pick up the run:\n%s", out)
	}
}

// j/k scroll the tab when it outgrows the terminal; the footer says so.
func TestAgentsTabScrolls(t *testing.T) {
	titles := make([]string, 12)
	for i := range titles {
		titles[i] = fmt.Sprintf("[Card %02d]", i)
	}
	f := newLoopFixture(t, true, titles...)
	a := toAgents(f.app(80, 15))
	top := stripANSI(a.View())
	if !strings.Contains(top, "j/k Scroll") {
		t.Fatalf("overflowing tab has no scroll hint:\n%s", top)
	}
	if !strings.Contains(strings.Split(top, "\n")[1], "Loops") {
		t.Fatalf("tab does not start at the top:\n%s", top)
	}
	a = send(a, runes("j"))
	if a.agents.scroll != 1 || strings.Contains(strings.Split(stripANSI(a.View()), "\n")[1], "Loops") {
		t.Errorf("j did not scroll one row (scroll=%d):\n%s", a.agents.scroll, stripANSI(a.View()))
	}
	a = send(a, runes("G"))
	bottom := stripANSI(a.View())
	if !strings.Contains(bottom, "[Card ") || a.agents.scroll == 0 {
		t.Errorf("G did not reach the feed:\n%s", bottom)
	}
	last := a.agents.scroll
	a = send(a, runes("j"))
	if a.agents.scroll != last {
		t.Errorf("scrolled past the end: %d → %d", last, a.agents.scroll)
	}
	a = send(a, runes("g"))
	if a.agents.scroll != 0 {
		t.Errorf("g did not return to the top: %d", a.agents.scroll)
	}
	// A page that fits keeps the read-only footer.
	if out := stripANSI(toAgents(f.app(120, 60)).View()); strings.Contains(out, "j/k Scroll") || !strings.Contains(out, "read-only") {
		t.Errorf("fitting tab offers scrolling:\n%s", out)
	}
}

func TestAgentsTabFitsWithLoops(t *testing.T) {
	withColor(t)
	f := newLoopFixture(t, true, "[A task whose title is long enough to be cut on narrow terminals]", "[B]", "[C]")
	f.state([]loops.RunRecord{
		f.run("[B]", "indev", loops.RunDone, 1, ref.Add(-3*time.Hour), at(-2*time.Hour)),
		f.run("[A task whose title is long enough to be cut on narrow terminals]", "testing-review", loops.RunRunning, 3, ref.Add(-12*time.Minute), nil),
	}, []loops.Queued{{TaskID: f.ids["[C]"], Stage: "testing-review", QueuedAt: ref}})
	for _, sz := range [][2]int{{80, 15}, {80, 24}, {100, 30}, {120, 44}} {
		a := toAgents(f.app(sz[0], sz[1]))
		fits(t, "agents", a.View(), sz[0], sz[1])
		a = send(a, runes("G"))
		fits(t, "agents scrolled", a.View(), sz[0], sz[1])
	}
}

// --- cards and detail ---

func TestCardShowsRunningLoopMarker(t *testing.T) {
	f := newLoopFixture(t, true, "[Looping]", "[Claimed]", "[Idle]")
	if err := f.board.Claim(f.ids["[Looping]"], "loop:indev", time.Hour, ref); err != nil {
		t.Fatal(err)
	}
	if err := f.board.Claim(f.ids["[Claimed]"], "codex", time.Hour, ref); err != nil {
		t.Fatal(err)
	}
	before := f.app(140, 40)
	beforeH := lipgloss.Height(before.board.renderCard(1, 0, mustTask(t, f.board, f.ids["[Looping]"]), before.board.columnWidth()))
	f.state([]loops.RunRecord{f.run("[Looping]", "indev", loops.RunRunning, 1, ref.Add(-time.Minute), nil)}, nil)

	a := f.app(140, 40)
	out := stripANSI(a.View())
	for _, want := range []string{"⟳ loop:indev", "◆ codex"} {
		if !strings.Contains(out, want) {
			t.Errorf("board is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "◆ loop:indev") {
		t.Errorf("a looping card shows its claim instead of the loop marker:\n%s", out)
	}
	card := a.board.renderCard(1, 0, mustTask(t, f.board, f.ids["[Looping]"]), a.board.columnWidth())
	if got := lipgloss.Height(card); got != beforeH {
		t.Errorf("loop marker grew the card from %d to %d rows", beforeH, got)
	}
	// A narrow column drops "loop:", then shortens, then keeps the glyph.
	for budget, want := range map[int]string{12: "⟳ loop:indev", 9: "⟳ indev", 5: "⟳ in…", 3: "⟳"} {
		if got := stripANSI(cardLoop(loops.RunRecord{Stage: "indev"}, budget)); got != want {
			t.Errorf("marker in %d cells = %q, want %q", budget, got, want)
		}
	}
}

func mustTask(t *testing.T, b *task.Board, id string) task.Task {
	t.Helper()
	tk, ok := b.TaskByID(id)
	if !ok {
		t.Fatalf("no task %s", id)
	}
	return tk
}

func TestDetailShowsLoopLineAndLogHint(t *testing.T) {
	f := newLoopFixture(t, true, "[Live]", "[Finished]", "[NoLog]", "[Never]")
	live := f.run("[Live]", "indev", loops.RunRunning, 1, ref.Add(-12*time.Minute), nil)
	done := f.run("[Finished]", "indev", loops.RunDone, 2, ref.Add(-time.Hour), at(-49*time.Minute))
	nolog := f.run("[NoLog]", "indev", loops.RunFailed, 1, ref.Add(-time.Hour), at(-50*time.Minute))
	f.state([]loops.RunRecord{done, nolog, live}, nil)
	f.writeLog(live, "starting\n")
	f.writeLog(done, "finished\n")
	a := f.app(120, 40)

	for _, c := range []struct {
		title, loop string
		hint        bool
	}{
		{"[Live]", "indev · claude · running 12m", true},
		{"[Finished]", "indev · done 30/07 11:11", true},
		{"[NoLog]", "indev · failed 30/07 11:10", false},
		{"[Never]", "", false},
	} {
		out := stripANSI(openDetail(t, a, f.ids[c.title]).View())
		if c.loop == "" {
			if strings.Contains(out, "Loop ") {
				t.Errorf("%s: Loop line without a run:\n%s", c.title, out)
			}
		} else if !strings.Contains(out, "Loop") || !strings.Contains(out, c.loop) {
			t.Errorf("%s: missing Loop line %q:\n%s", c.title, c.loop, out)
		}
		if got := strings.Contains(out, "L Loop log"); got != c.hint {
			t.Errorf("%s: L hint shown = %v, want %v:\n%s", c.title, got, c.hint, out)
		}
	}
}

// --- log viewer ---

func TestLogViewerOpensAtBottomScrollsAndFollows(t *testing.T) {
	f := newLoopFixture(t, true, "[Live]")
	run := f.run("[Live]", "indev", loops.RunRunning, 4, ref.Add(-12*time.Minute), nil)
	f.state([]loops.RunRecord{run}, nil)
	var log strings.Builder
	log.WriteString("== gotodo loop · indev · run 4\n")
	for i := 1; i <= 120; i++ {
		fmt.Fprintf(&log, "line %03d \x1b[1;32mgreen\x1b[0m\n", i)
	}
	f.writeLog(run, log.String())

	a := openDetail(t, f.app(100, 30), f.ids["[Live]"])
	a = send(a, runes("L"))
	if a.board.mode != modeLog {
		t.Fatalf("L did not open the log viewer (mode %v)", a.board.mode)
	}
	out := stripANSI(a.View())
	for _, want := range []string{"Loop log", "[Live]", "indev · run 4 · ⟳ running 12m · claude · opus", filepath.Base(run.Log), "line 120 green", "following", "x Stop run", "esc Back"} {
		if !strings.Contains(out, want) {
			t.Errorf("log viewer is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "line 001") {
		t.Errorf("log viewer did not open at the bottom:\n%s", out)
	}
	if strings.Contains(a.View(), "\x1b[1;32m") || strings.Contains(strings.Join(a.board.logView.lines, ""), "\x1b") {
		t.Error("agent ANSI codes reached the screen")
	}

	// k leaves the bottom; new lines arrive but the view stays put.
	a = send(a, runes("k"))
	if a.board.logView.follow {
		t.Fatal("k still following")
	}
	off := a.board.logView.offset
	f.appendLog(run, "line 121 late\n")
	a = send(a, refreshTickMsg(ref))
	if a.board.logView.offset != off || strings.Contains(stripANSI(a.View()), "line 121") {
		t.Errorf("scrolled-up viewer jumped to new output (offset %d → %d)", off, a.board.logView.offset)
	}
	// G returns to the bottom and follows again.
	a = send(a, runes("G"))
	f.appendLog(run, "line 122 later\n")
	a = send(a, refreshTickMsg(ref))
	if out := stripANSI(a.View()); !strings.Contains(out, "line 122 later") {
		t.Errorf("following viewer missed appended output:\n%s", out)
	}
	// g goes to the top, pgdn moves a page.
	a = send(a, runes("g"))
	if out := stripANSI(a.View()); !strings.Contains(out, "== gotodo loop") {
		t.Errorf("g did not reach the top:\n%s", out)
	}
	a = send(a, tea.KeyMsg{Type: tea.KeyPgDown})
	if a.board.logView.offset != a.board.logWindow()-1 {
		t.Errorf("pgdn offset = %d, want %d", a.board.logView.offset, a.board.logWindow()-1)
	}
	a = send(a, tea.KeyMsg{Type: tea.KeyPgUp})
	if a.board.logView.offset != 0 {
		t.Errorf("pgup offset = %d, want 0", a.board.logView.offset)
	}
	// esc goes back to the detail popup.
	a = send(a, tea.KeyMsg{Type: tea.KeyEsc})
	if a.board.mode != modeDetail {
		t.Errorf("esc went to mode %v, want detail", a.board.mode)
	}
}

func TestLogViewerFromBoardReturnsToBoard(t *testing.T) {
	f := newLoopFixture(t, true, "[Live]", "[Never]")
	run := f.run("[Live]", "indev", loops.RunRunning, 1, ref, nil)
	f.state([]loops.RunRecord{run}, nil)
	f.writeLog(run, "hello\n")
	a := f.app(100, 30)
	a.board.selectTaskID(f.ids["[Live]"])
	a = send(a, runes("L"))
	if a.board.mode != modeLog {
		t.Fatalf("L on the board did not open the log (mode %v)", a.board.mode)
	}
	a = send(a, tea.KeyMsg{Type: tea.KeyEsc})
	if a.board.mode != modeNormal {
		t.Errorf("esc went to mode %v, want the board", a.board.mode)
	}
	a.board.selectTaskID(f.ids["[Never]"])
	a = send(a, runes("L"))
	if a.board.mode != modeNormal || !strings.Contains(stripANSI(a.View()), "no loop log for this card yet") {
		t.Errorf("L without a log: mode %v\n%s", a.board.mode, stripANSI(a.View()))
	}
}

func TestLogViewerStopsARunAfterConfirm(t *testing.T) {
	f := newLoopFixture(t, true, "[Live]")
	run := f.run("[Live]", "indev", loops.RunRunning, 1, ref.Add(-time.Minute), nil)
	f.state([]loops.RunRecord{run}, nil)
	f.writeLog(run, "working\n")
	a := openDetail(t, f.app(100, 30), f.ids["[Live]"])
	a = send(a, runes("L"))

	var stopped []string
	a.board.stopLoop = func(boardPath, taskID string, now time.Time) ([]loops.RunRecord, error) {
		stopped = append(stopped, taskID)
		r := run
		r.Status, r.Ended = loops.RunStopped, &now
		f.state([]loops.RunRecord{r}, nil) // what loops.Stop would record
		return []loops.RunRecord{r}, nil
	}
	// n cancels.
	a = send(a, runes("x"))
	if !strings.Contains(stripANSI(a.View()), "Stop this run?") {
		t.Fatalf("x did not ask first:\n%s", stripANSI(a.View()))
	}
	a = send(a, runes("n"))
	if len(stopped) != 0 || a.board.logView.confirm {
		t.Fatalf("n stopped the run or kept asking: %v", stopped)
	}
	// x then y stops it.
	a = send(a, runes("x"), runes("y"))
	if len(stopped) != 1 || stopped[0] != f.ids["[Live]"] {
		t.Fatalf("stopper calls = %v", stopped)
	}
	out := stripANSI(a.View())
	if !strings.Contains(out, "stopped") || strings.Contains(out, "x Stop run") {
		t.Errorf("viewer does not show the run stopped:\n%s", out)
	}
	if _, ok := a.watch.running(f.ids["[Live]"]); ok {
		t.Error("shared loop state still has the run live")
	}
	// x on a finished run does nothing.
	a = send(a, runes("x"))
	if a.board.logView.confirm {
		t.Error("x offered to stop a finished run")
	}
}

func TestLogViewerReportsStopErrors(t *testing.T) {
	f := newLoopFixture(t, true, "[Live]")
	run := f.run("[Live]", "indev", loops.RunRunning, 1, ref, nil)
	f.state([]loops.RunRecord{run}, nil)
	f.writeLog(run, "working\n")
	a := openDetail(t, f.app(100, 30), f.ids["[Live]"])
	a = send(a, runes("L"))
	a.board.stopLoop = func(string, string, time.Time) ([]loops.RunRecord, error) {
		return nil, errors.New("lock busy")
	}
	a = send(a, runes("x"), runes("y"))
	if out := stripANSI(a.View()); !strings.Contains(out, "stop: lock busy") {
		t.Errorf("stop error not shown:\n%s", out)
	}
}

func TestLogViewerWrapsLongLinesAndFits(t *testing.T) {
	withColor(t)
	f := newLoopFixture(t, true, "[Live]")
	run := f.run("[Live]", "indev", loops.RunRunning, 1, ref, nil)
	f.state([]loops.RunRecord{run}, nil)
	long := strings.Repeat("abcdefghij", 40)
	f.writeLog(run, "short\n"+long+"\n漢字のログ行"+strings.Repeat("字", 100)+"\n\tTabbed\n")
	for _, sz := range [][2]int{{80, 15}, {80, 24}, {110, 40}, {120, 44}} {
		a := openDetail(t, f.app(sz[0], sz[1]), f.ids["[Live]"])
		fits(t, "detail", a.View(), sz[0], sz[1])
		a = send(a, runes("L"))
		fits(t, "log", a.View(), sz[0], sz[1])
		a = send(a, runes("g"))
		fits(t, "log top", a.View(), sz[0], sz[1])
		a = send(a, runes("x"))
		fits(t, "log confirm", a.View(), sz[0], sz[1])
		a = send(a, runes("n"))
		for _, row := range a.board.logRows() {
			if lipgloss.Width(row) > a.board.logWidth()-2 {
				t.Errorf("%dx%d: wrapped row %d wide: %q", sz[0], sz[1], lipgloss.Width(row), row)
			}
		}
		if got := strings.Join(a.board.logRows(), ""); !strings.Contains(got, long) {
			t.Errorf("%dx%d: wrapping lost text", sz[0], sz[1])
		}
	}
	// A resize while open rewraps.
	a := send(openDetail(t, f.app(120, 40), f.ids["[Live]"]), runes("L"))
	a = send(a, tea.WindowSizeMsg{Width: 80, Height: 20})
	if a.board.logView.wrapW != a.board.logWidth()-2 {
		t.Errorf("resize did not rewrap: wrapW %d, width %d", a.board.logView.wrapW, a.board.logWidth()-2)
	}
	fits(t, "log resized", a.View(), 80, 20)
}

// The help still fits the smallest supported terminal with the new keys.
func TestEveryLoopScreenFitsSmallTerminals(t *testing.T) {
	withColor(t)
	f := newLoopFixture(t, true, "[Live]")
	run := f.run("[Live]", "indev", loops.RunRunning, 1, ref, nil)
	f.state([]loops.RunRecord{run}, nil)
	f.writeLog(run, strings.Repeat("output\n", 50))
	for _, h := range []int{15, 18, 20, 23, 24, 40, 50} {
		a := f.app(110, h)
		fits(t, "board", a.View(), 110, h)
		a = openDetail(t, a, f.ids["[Live]"])
		fits(t, "detail", a.View(), 110, h)
		a = send(a, runes("L"))
		fits(t, "log", a.View(), 110, h)
	}
}

// --- log text ---

func TestCleanLogStripsEscapesAndControls(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"\x1b[1;31mred\x1b[0m plain", "red plain"},
		{"\x1b]0;window title\x07after", "after"},
		{"\x1b]8;;https://x.y\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"\x1b(Bascii", "ascii"},
		{"10%\r50%\r100%\ndone", "100%\ndone"},
		{"crlf\r\nline", "crlf\nline"},
		{"a\tb", "a    b"},
		{"bell\x07 back\x08", "bell back"},
		{"\x1b[?25lhidden cursor\x1b[?25h", "hidden cursor"},
		{"bad \xff byte", "bad � byte"},
		{"trailing esc \x1b", "trailing esc "},
	} {
		if got := cleanLog(c.in); got != c.want {
			t.Errorf("cleanLog(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReadLogTailReadsOnlyTheEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&b, "row %04d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, size, clipped, err := readLogTail(path, 95) // 10.5 rows
	if err != nil {
		t.Fatal(err)
	}
	if !clipped || size != int64(b.Len()) {
		t.Errorf("clipped=%v size=%d, want true %d", clipped, size, b.Len())
	}
	if len(lines) != 10 || lines[0] != "row 0990" || lines[9] != "row 0999" {
		t.Errorf("tail = %q", lines)
	}
	lines, _, clipped, _ = readLogTail(path, 1<<20)
	if clipped || len(lines) != 1000 {
		t.Errorf("whole file: clipped=%v rows=%d", clipped, len(lines))
	}
}

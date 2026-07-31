package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

// seeded returns a board with one task per column.
func seeded(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	b.SetPath("")
	for _, s := range task.Statuses {
		id := b.Add("["+string(s)+" task]", "", nil, ref).ID
		if err := b.Move(id, s, ref); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	return b
}

func TestNewBoardModelStartsInItemFocusOnTodo(t *testing.T) {
	m := NewBoardModel(seeded(t))
	if m.focus != focusItem {
		t.Errorf("focus = %v, want focusItem", m.focus)
	}
	if m.col != 0 {
		t.Errorf("col = %d, want 0", m.col)
	}
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
}

func TestViewRendersEveryColumnHeader(t *testing.T) {
	m := NewBoardModel(seeded(t))
	m.SetSize(120, 30)
	out := m.View()
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("View missing header %q", s.Label())
		}
	}
}

func TestViewRendersTaskTitles(t *testing.T) {
	m := NewBoardModel(seeded(t))
	m.SetSize(160, 30)
	out := m.View()
	if !strings.Contains(out, "todo task") {
		t.Errorf("View missing the todo task title:\n%s", out)
	}
	if !strings.Contains(out, "blocked task") {
		t.Errorf("View missing the blocked task title:\n%s", out)
	}
}

func TestViewShowsEmptyPlaceholder(t *testing.T) {
	m := NewBoardModel(&task.Board{})
	m.SetSize(120, 30)
	if !strings.Contains(m.View(), "empty") {
		t.Errorf("View of an empty board should say 'empty':\n%s", m.View())
	}
}

func TestSelectedTask(t *testing.T) {
	m := NewBoardModel(seeded(t))
	got, ok := m.selectedTask()
	if !ok {
		t.Fatal("selectedTask returned ok=false, want true")
	}
	if got.Status != task.StatusTodo {
		t.Errorf("selected status = %q, want todo", got.Status)
	}
}

func TestSelectedTaskEmptyColumn(t *testing.T) {
	m := NewBoardModel(&task.Board{})
	if _, ok := m.selectedTask(); ok {
		t.Error("selectedTask on an empty board returned ok=true, want false")
	}
}

func TestClampSelectionAfterShrink(t *testing.T) {
	b := seeded(t)
	m := NewBoardModel(b)
	m.sel[0] = 5 // stale index, e.g. after a delete
	m.clampSelection()
	if m.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (clamped to the single task)", m.sel[0])
	}
}

func key(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "ctrl+t":
		return tea.KeyMsg{Type: tea.KeyCtrlT}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	panic("unhandled key in test helper: " + s)
}

// press feeds a sequence of keys through Update.
func press(m BoardModel, keys ...string) BoardModel {
	for _, k := range keys {
		m, _ = m.Update(key(k))
	}
	return m
}

func TestCtrlTTogglesFocus(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "ctrl+t")
	if m.focus != focusColumn {
		t.Fatalf("focus = %v, want focusColumn", m.focus)
	}
	m = press(m, "ctrl+t")
	if m.focus != focusItem {
		t.Errorf("focus = %v, want focusItem", m.focus)
	}
}

func TestHLMoveBetweenColumns(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "l", "l")
	if m.col != 2 {
		t.Errorf("col = %d, want 2", m.col)
	}
	m = press(m, "h")
	if m.col != 1 {
		t.Errorf("col = %d, want 1", m.col)
	}
}

func TestColumnNavigationClampsAtEdges(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "h", "h")
	if m.col != 0 {
		t.Errorf("col = %d, want 0 (clamped at the left edge)", m.col)
	}
	m = press(m, "l", "l", "l", "l", "l")
	if m.col != len(task.Statuses)-1 {
		t.Errorf("col = %d, want %d (clamped at the right edge)", m.col, len(task.Statuses)-1)
	}
}

func TestJKMoveBetweenItemsInItemFocus(t *testing.T) {
	b := &task.Board{}
	b.Add("[One]", "", nil, ref)
	b.Add("[Two]", "", nil, ref)
	b.Add("[Three]", "", nil, ref)
	m := press(NewBoardModel(b), "j", "j")
	if m.sel[0] != 2 {
		t.Errorf("sel[0] = %d, want 2", m.sel[0])
	}
	m = press(m, "j") // clamp at the bottom
	if m.sel[0] != 2 {
		t.Errorf("sel[0] = %d, want 2 (clamped at the last item)", m.sel[0])
	}
	m = press(m, "k", "k", "k")
	if m.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (clamped at the first item)", m.sel[0])
	}
}

func TestJKDoNotMoveItemsInColumnFocus(t *testing.T) {
	b := &task.Board{}
	b.Add("[One]", "", nil, ref)
	b.Add("[Two]", "", nil, ref)
	m := press(NewBoardModel(b), "ctrl+t", "j")
	if m.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (column focus must not move the item cursor)", m.sel[0])
	}
}

func TestGAndShiftGJumpToEnds(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 4; i++ {
		b.Add("[Task title]", "", nil, ref)
	}
	m := press(NewBoardModel(b), "G")
	if m.sel[0] != 3 {
		t.Errorf("sel[0] after G = %d, want 3", m.sel[0])
	}
	m = press(m, "g")
	if m.sel[0] != 0 {
		t.Errorf("sel[0] after g = %d, want 0", m.sel[0])
	}
}

func TestArrowKeysMirrorHJKL(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "right", "right")
	if m.col != 2 {
		t.Errorf("col = %d, want 2", m.col)
	}
}

// fixedClock pins the model's clock so transitions are deterministic.
func fixedClock(m BoardModel) BoardModel {
	m.now = func() time.Time { return ref }
	return m
}

func TestAddOpensInputAndCommitsOnEnter(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want modeInput", m.mode)
	}
	m = press(m, "b", "u", "y", " ", "m", "i", "l", "k")
	m = press(m, "enter")

	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal after enter", m.mode)
	}
	todo := m.board.ByStatus(task.StatusTodo)
	if len(todo) != 1 {
		t.Fatalf("todo tasks = %d, want 1", len(todo))
	}
	if todo[0].Title != "buy milk" {
		t.Errorf("Title = %q, want %q", todo[0].Title, "buy milk")
	}
}

func TestAddEscCancels(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a", "x", "esc")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 after cancel", len(m.board.Tasks))
	}
}

func TestAddBlankTitleIsRejected(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a", " ", "enter")
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 for a blank title", len(m.board.Tasks))
	}
	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput (stay open on a blank title)", m.mode)
	}
}

func TestEditReplacesTitle(t *testing.T) {
	b := &task.Board{}
	b.Add("[Old title]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "e")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want modeInput", m.mode)
	}
	if m.inputs[fieldTitle].Value() != "[Old title]" {
		t.Errorf("input prefilled with %q, want the existing title", m.inputs[fieldTitle].Value())
	}
	// clear then type
	m.inputs[fieldTitle].SetValue("new")
	m = press(m, "enter")
	if m.board.Tasks[0].Title != "new" {
		t.Errorf("Title = %q, want %q", m.board.Tasks[0].Title, "new")
	}
}

func TestDeleteAsksThenRemoves(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "d")
	if m.mode != modeConfirm {
		t.Fatalf("mode = %v, want modeConfirm", m.mode)
	}
	if len(m.board.Tasks) != 1 {
		t.Fatalf("Tasks = %d, want 1 before confirming", len(m.board.Tasks))
	}
	m = press(m, "y")
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 after confirming", len(m.board.Tasks))
	}
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
}

func TestDeleteCancelledByN(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "d", "n")
	if len(m.board.Tasks) != 1 {
		t.Errorf("Tasks = %d, want 1 after cancelling", len(m.board.Tasks))
	}
}

func TestGrabMoveAndDrop(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))

	m = press(m, "m")
	if m.mode != modeMove {
		t.Fatalf("mode = %v, want modeMove", m.mode)
	}
	m = press(m, "l", "l") // todo -> doing -> blocked
	if got := m.board.Tasks[0].Status; got != task.StatusBlocked {
		t.Errorf("Status during move = %q, want blocked", got)
	}
	if m.col != 2 {
		t.Errorf("col = %d, want 2 (focus follows the grabbed task)", m.col)
	}
	m = press(m, "enter")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal after drop", m.mode)
	}
	if got := m.board.Tasks[0].Status; got != task.StatusBlocked {
		t.Errorf("Status after drop = %q, want blocked", got)
	}
}

func TestGrabEscRestoresOriginalColumn(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "m", "l", "l", "esc")
	if got := m.board.Tasks[0].Status; got != task.StatusTodo {
		t.Errorf("Status after esc = %q, want todo", got)
	}
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
}

func TestMutationEmitsDirtyCmd(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a", "x")
	m, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("commit returned a nil cmd, want a dirtyMsg cmd")
	}
	if _, ok := cmd().(dirtyMsg); !ok {
		t.Errorf("cmd produced %T, want dirtyMsg", cmd())
	}
	_ = m
}

func TestMoveKeysIgnoredOnEmptyColumn(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "m")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (nothing to grab)", m.mode)
	}
	m = press(m, "e")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (nothing to edit)", m.mode)
	}
	m = press(m, "d")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (nothing to delete)", m.mode)
	}
}

func TestGrabTracksTaskByIDNotLastSlot(t *testing.T) {
	b := &task.Board{}
	t1 := b.Add("[T1]", "", nil, ref).ID
	b.Add("[T2]", "", nil, ref)
	b.Add("[T3]", "", nil, ref)
	// Move T2 to doing first, so doing's insertion order is [T2].
	if err := b.Move(b.Tasks[1].ID, task.StatusDoing, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	m := fixedClock(NewBoardModel(b))
	// Grab T1 (still in todo, at sel[0] = 0).
	m = press(m, "m")
	if m.grabID != t1 {
		t.Fatalf("grabID = %q, want %q (T1)", m.grabID, t1)
	}
	m = press(m, "l") // todo -> doing; doing now holds [T2, T1] in insertion order
	got, ok := m.selectedTask()
	if !ok {
		t.Fatal("selectedTask returned ok=false after shift")
	}
	if got.ID != m.grabID {
		t.Errorf("selectedTask = %q, want the grabbed task %q", got.ID, m.grabID)
	}
}

func TestGrabShiftEmitsDirtyCmd(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "m")
	m, cmd := m.Update(key("l"))
	if cmd == nil {
		t.Fatal("shift returned a nil cmd, want a dirtyMsg cmd")
	}
	if _, ok := cmd().(dirtyMsg); !ok {
		t.Errorf("cmd produced %T, want dirtyMsg", cmd())
	}
	_ = m
}

// TestGrabShiftAtRightEdgeIsNoOpAndSkipsDirty pins the fix for a no-op
// keypress triggering a disk write: grabbing the rightmost column's task and
// pressing "l" again has nowhere to go, so it must not emit a dirtyMsg cmd
// (which would otherwise trigger a full, identical Save).
func TestGrabShiftAtRightEdgeIsNoOpAndSkipsDirty(t *testing.T) {
	b := &task.Board{}
	id := b.Add("[Task title]", "", nil, ref).ID
	if err := b.Move(id, task.StatusDone, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	m := fixedClock(NewBoardModel(b))
	m = press(m, "m") // grab the task in the rightmost (done) column
	m, cmd := m.Update(key("l"))
	if cmd != nil {
		t.Errorf("shift past the right edge produced %T, want a nil cmd (no-op, no save)", cmd())
	}
	if got := m.board.Tasks[0].Status; got != task.StatusDone {
		t.Errorf("Status after no-op shift = %q, want done (unchanged)", got)
	}
}

// withFields returns a board holding one fully-populated todo task.
func withFields(t *testing.T, desc string, deadline *time.Time) *task.Board {
	t.Helper()
	b := &task.Board{}
	b.Add("[Ship the report]", desc, deadline, ref)
	return b
}

func TestCardShowsTitleDescriptionAndDeadline(t *testing.T) {
	due := ref.AddDate(0, 0, 5)
	m := NewBoardModel(withFields(t, "[draft, review, send]", &due))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	out := stripANSI(m.View())
	for _, want := range []string{"Ship the report", "draft, review, send", "04/08/2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
}

func TestCardOmitsDescriptionLineWhenEmpty(t *testing.T) {
	m := NewBoardModel(withFields(t, "", nil))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	card := stripANSI(m.renderCard(0, 0, m.board.ByStatus(task.StatusTodo)[0], 30))
	if got := len(strings.Split(card, "\n")); got != 1 {
		t.Errorf("card has %d lines, want 1 for a task with no description and no deadline:\n%q", got, card)
	}
}

func TestCardHasThreeLinesWhenFullyPopulated(t *testing.T) {
	due := ref.AddDate(0, 0, 5)
	m := NewBoardModel(withFields(t, "[a description]", &due))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	card := stripANSI(m.renderCard(0, 0, m.board.ByStatus(task.StatusTodo)[0], 30))
	if got := len(strings.Split(card, "\n")); got != 3 {
		t.Errorf("card has %d lines, want 3:\n%q", got, card)
	}
}

func TestCardMarksOverdueDeadline(t *testing.T) {
	due := ref.AddDate(0, 0, -3)
	m := NewBoardModel(withFields(t, "", &due))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	out := stripANSI(m.View())
	if !strings.Contains(out, "27/07/2026 ✗") {
		t.Errorf("View missing the overdue marker:\n%s", out)
	}
}

func TestCardTruncatesLongDescription(t *testing.T) {
	long := strings.Repeat("x", 200)
	m := NewBoardModel(withFields(t, long, nil))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	const width = 30
	const inner = width - 4 // renderCard's truncation budget
	// renderCard(0, 0, ...) is selected by default (colIdx/itemIdx match the
	// model's defaults), so CardSelectedStyle's left border + padding add 2
	// more columns on top of the truncated content.
	const maxLineWidth = inner + 2
	card := stripANSI(m.renderCard(0, 0, m.board.ByStatus(task.StatusTodo)[0], width))
	for _, line := range strings.Split(card, "\n") {
		if got := lipgloss.Width(line); got > maxLineWidth {
			t.Errorf("line is %d columns wide, wider than the %d-wide truncation budget: %q", got, maxLineWidth, line)
		}
	}
}

// TestCardDeadlineFitsAtMinimumColumnWidth pins the enforced minimum column
// width (18, set by columnWidth) as wide enough for the worst-case deadline
// line: "● DD/MM/YYYY ✗" (14 display columns) plus the card's own padding.
// Checked in all three render states because each uses a different style.
func TestCardDeadlineFitsAtMinimumColumnWidth(t *testing.T) {
	overdue := ref.AddDate(0, 0, -1)
	b := &task.Board{}
	b.Add("[T]", "", &overdue, ref)
	m := NewBoardModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(4, 40) // forces columnWidth() down to its floor
	w := m.columnWidth()
	tt := m.board.ByStatus(task.StatusTodo)[0]

	check := func(label, card string) {
		for _, line := range strings.Split(stripANSI(card), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("%s: line is %d columns wide, wider than the %d-wide column: %q", label, got, w, line)
			}
		}
	}

	// normal (not selected): use a column index that never matches m.col.
	check("normal", m.renderCard(1, 0, tt, w))

	// selected
	check("selected", m.renderCard(0, 0, tt, w))

	// grabbed
	grabbed := m
	grabbed.mode = modeMove
	grabbed.grabID = tt.ID
	check("grabbed", grabbed.renderCard(0, 0, tt, w))
}

func TestCardCJKDescriptionFitsColumn(t *testing.T) {
	long := strings.Repeat("日本語", 20)
	m := NewBoardModel(withFields(t, long, nil))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	const width = 30
	card := stripANSI(m.renderCard(0, 0, m.board.ByStatus(task.StatusTodo)[0], width))
	for _, line := range strings.Split(card, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Errorf("line is %d columns wide, wider than the %d-wide column: %q", got, width, line)
		}
	}
}

// TestColumnClipsToHeight ensures a column with many cards never renders
// taller than columnHeight, in a terminal too short to show them all.
func TestColumnClipsToHeight(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 20; i++ {
		b.Add("[Task title]", "", nil, ref)
	}
	m := NewBoardModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(160, 20)

	out := stripANSI(m.renderColumn(0, task.StatusTodo, m.columnWidth()))
	got := len(strings.Split(out, "\n"))
	if want := m.columnHeight() + 2; got > want { // +2 for the column's own top/bottom border
		t.Errorf("column is %d lines tall, want at most %d", got, want)
	}
}

// TestColumnShowsMoreCount checks the "+N more" line appears with the
// correct count when a column's cards do not all fit.
func TestColumnShowsMoreCount(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 20; i++ {
		b.Add("[Task title]", "", nil, ref)
	}
	m := NewBoardModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(160, 20)

	out := stripANSI(m.renderColumn(0, task.StatusTodo, m.columnWidth()))
	if !strings.Contains(out, "more") {
		t.Errorf("column with 20 cards in a short terminal should show a '+N more' line:\n%s", out)
	}
}

// TestColumnKeepsSelectedCardVisibleBeyondFold ensures selecting the last
// card of a long column still renders that card, even though it would fall
// past the fold in a naive top-down render.
func TestColumnKeepsSelectedCardVisibleBeyondFold(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 20; i++ {
		b.Add("[Task title]", "", nil, ref)
	}
	m := NewBoardModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(160, 20)
	m = press(m, "G") // select the last card

	out := stripANSI(m.renderColumn(0, task.StatusTodo, m.columnWidth()))
	if !strings.Contains(out, "Task title") {
		t.Errorf("selected last card should still be rendered:\n%s", out)
	}
}

func TestViewFitsWithinTerminalWidth(t *testing.T) {
	m := NewBoardModel(seeded(t))
	m.SetSize(160, 40)
	out := stripANSI(m.View())
	for _, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got > 160 {
			t.Errorf("line is %d columns wide, wider than the 160-wide terminal: %q", got, line)
		}
	}
}

func TestViewRendersColumnsAtComfortableWidth(t *testing.T) {
	m := NewBoardModel(seeded(t))
	m.SetSize(160, 40)
	out := m.View()
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("View missing header %q at a comfortable width", s.Label())
		}
	}
	if strings.Contains(out, "too narrow") {
		t.Errorf("View should not warn at a comfortable width:\n%s", out)
	}
}

func TestViewWarnsWhenTerminalTooNarrow(t *testing.T) {
	for _, w := range []int{60, 72} {
		m := NewBoardModel(seeded(t))
		m.SetSize(w, 40)
		out := m.View()
		if !strings.Contains(out, "too narrow") {
			t.Errorf("width=%d: View should warn that the terminal is too narrow:\n%s", w, out)
		}
		for _, s := range task.Statuses {
			if strings.Contains(out, s.Label()) {
				t.Errorf("width=%d: View should not render the four-column layout while too narrow", w)
			}
		}
	}
}

func TestViewRendersNormallyAtTheWidthThreshold(t *testing.T) {
	m := NewBoardModel(seeded(t))
	need := m.minBoardWidth()
	m.SetSize(need, 40)
	out := m.View()
	if strings.Contains(out, "too narrow") {
		t.Errorf("width=%d (exactly minBoardWidth): should not warn, an off-by-one snuck in:\n%s", need, out)
	}
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("width=%d: View missing header %q at the exact threshold", need, s.Label())
		}
	}
}

func TestViewRendersNormallyBeforeFirstWindowSizeMsg(t *testing.T) {
	m := NewBoardModel(seeded(t)) // width is 0 until SetSize is called
	out := m.View()
	if strings.Contains(out, "too narrow") {
		t.Errorf("width=0 (pre-WindowSizeMsg) should fall through to the normal path, not warn:\n%s", out)
	}
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("width=0: View missing header %q", s.Label())
		}
	}
}

func TestParseDeadline(t *testing.T) {
	got, err := parseDeadline("02/08/2026")
	if err != nil {
		t.Fatalf("parseDeadline returned %v", err)
	}
	if got == nil {
		t.Fatal("parseDeadline returned nil for a valid date")
	}
	// Local, not UTC: DaysUntilDeadline re-anchors the deadline to now's
	// location before taking its calendar day, so parsing into UTC would
	// shift the calendar day back for anyone west of UTC (see
	// TestParseDeadlineThenUrgencyIsNotOffByOneWestOfUTC below).
	want := time.Date(2026, 8, 2, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("parseDeadline = %v, want %v", *got, want)
	}
}

// TestParseDeadlineThenUrgencyIsNotOffByOneWestOfUTC pins the bug where a
// deadline typed for today rendered overdue on the day it was due. The
// symptom only showed up for negative UTC offsets: parseDeadline used to
// parse into UTC, then DaysUntilDeadline re-anchored that instant to the
// local zone before taking its calendar day, which rolled the deadline back
// a day for anyone west of UTC. Singapore (+8) never saw it.
func TestParseDeadlineThenUrgencyIsNotOffByOneWestOfUTC(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	orig := time.Local
	time.Local = loc
	defer func() { time.Local = orig }()

	now := time.Date(2026, 8, 2, 10, 0, 0, 0, loc)
	deadline, err := parseDeadline("02/08/2026")
	if err != nil {
		t.Fatalf("parseDeadline returned %v", err)
	}
	tt := task.Task{Title: "[Task title]", Status: task.StatusTodo, CreatedAt: now, Deadline: deadline}
	if got := task.DeadlineUrgency(tt, now); got != task.UrgencyUrgent {
		t.Errorf("urgency = %v, want UrgencyUrgent for a deadline due today in a negative-offset zone", got)
	}
}

func TestParseDeadlineBlankIsNoDeadline(t *testing.T) {
	got, err := parseDeadline("   ")
	if err != nil {
		t.Fatalf("parseDeadline returned %v", err)
	}
	if got != nil {
		t.Errorf("parseDeadline = %v, want nil for blank input", got)
	}
}

func TestParseDeadlineRejectsUSFormatAndGarbage(t *testing.T) {
	for _, in := range []string{"2026-08-02", "notadate", "13/13/2026"} {
		if _, err := parseDeadline(in); err == nil {
			t.Errorf("parseDeadline(%q) returned nil error, want a rejection", in)
		}
	}
}

func TestFormTabCyclesFields(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	if m.field != fieldTitle {
		t.Fatalf("field = %d, want fieldTitle on open", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.field != fieldDesc {
		t.Errorf("field = %d, want fieldDesc after tab", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.field != fieldDeadline {
		t.Errorf("field = %d, want fieldDeadline after two tabs", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.field != fieldTitle {
		t.Errorf("field = %d, want it to wrap back to fieldTitle", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.field != fieldDeadline {
		t.Errorf("field = %d, want fieldDeadline after shift+tab from the first field", m.field)
	}
}

func TestFormSavesAllThreeFields(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldTitle].SetValue("[Ship the report]")
	m.inputs[fieldDesc].SetValue("[draft, review, send]")
	m.inputs[fieldDeadline].SetValue("02/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal after save", m.mode)
	}
	todo := m.board.ByStatus(task.StatusTodo)
	if len(todo) != 1 {
		t.Fatalf("todo tasks = %d, want 1", len(todo))
	}
	got := todo[0]
	if got.Title != "[Ship the report]" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Description != "[draft, review, send]" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Deadline == nil || !got.Deadline.Equal(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Deadline = %v, want 02/08/2026", got.Deadline)
	}
}

func TestFormRejectsBadDeadlineAndStaysOpen(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldTitle].SetValue("[Ship the report]")
	m.inputs[fieldDeadline].SetValue("tomorrow")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput — the form must stay open on a bad date", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 — nothing should be saved", len(m.board.Tasks))
	}
	if m.err == "" {
		t.Error("err is empty, want a message explaining the date format")
	}
}

func TestFormBlankTitleStillRejected(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldDesc].SetValue("[a description]")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0", len(m.board.Tasks))
	}
}

func TestEditPrefillsAllThreeFields(t *testing.T) {
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	b := &task.Board{}
	b.Add("[Old title]", "[old description]", &due, ref)

	m := fixedClock(NewBoardModel(b))
	m = press(m, "e")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want modeInput", m.mode)
	}
	if got := m.inputs[fieldTitle].Value(); got != "[Old title]" {
		t.Errorf("title field = %q", got)
	}
	if got := m.inputs[fieldDesc].Value(); got != "[old description]" {
		t.Errorf("description field = %q", got)
	}
	if got := m.inputs[fieldDeadline].Value(); got != "02/08/2026" {
		t.Errorf("deadline field = %q, want 02/08/2026", got)
	}
}

func TestEditWithClearedDeadlineRemovesIt(t *testing.T) {
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	b := &task.Board{}
	b.Add("[Task title]", "", &due, ref)

	m := fixedClock(NewBoardModel(b))
	m = press(m, "e")
	m.inputs[fieldDeadline].SetValue("")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.board.Tasks[0].Deadline != nil {
		t.Errorf("Deadline = %v, want nil after clearing the field", m.board.Tasks[0].Deadline)
	}
}

func TestFormErrorClearsAfterSuccessfulSave(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // blank title, rejected
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want modeInput after blank submit", m.mode)
	}
	if m.err == "" {
		t.Fatal("err is empty, want a message after a blank title submit")
	}

	m.inputs[fieldTitle].SetValue("[Task title]")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.board.Tasks) != 1 {
		t.Fatalf("Tasks = %d, want 1 after a valid save", len(m.board.Tasks))
	}
	if m.err != "" {
		t.Errorf("err = %q, want empty after a successful save", m.err)
	}
}

func TestFormEscCancels(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldTitle].SetValue("[Task title]")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 after cancel", len(m.board.Tasks))
	}
}

func TestCtrlDOpensThePickerOnlyOnTheDeadlineField(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.picker.open {
		t.Error("picker opened from the Title field, want it to stay closed")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // now on Deadline
	if m.field != fieldDeadline {
		t.Fatalf("field = %d, want fieldDeadline", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !m.picker.open {
		t.Error("picker did not open on the Deadline field")
	}
}

func TestPickerOpensOnTodayWhenTheFieldIsEmpty(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if !sameDay(m.picker.cursor, ref) {
		t.Errorf("cursor = %v, want today (%v)", m.picker.cursor, ref)
	}
}

func TestPickerOpensOnTheDateAlreadyTyped(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m.inputs[fieldDeadline].SetValue("09/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if m.picker.cursor.Day() != 9 || m.picker.cursor.Month() != time.August {
		t.Errorf("cursor = %v, want 09/08/2026 from the field", m.picker.cursor)
	}
}

func TestPickerEnterWritesTheDateAndCloses(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	// ref is 30/07/2026; two days forward is 01/08/2026.
	m, _ = m.Update(key("l"))
	m, _ = m.Update(key("l"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.picker.open {
		t.Error("picker still open after enter")
	}
	if got := m.inputs[fieldDeadline].Value(); got != "01/08/2026" {
		t.Errorf("deadline field = %q, want %q", got, "01/08/2026")
	}
	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput — enter picks a date, it must not save the task", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 — enter in the picker must not save", len(m.board.Tasks))
	}
}

func TestPickerEscClosesAndChangesNothing(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m.inputs[fieldDeadline].SetValue("09/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = m.Update(key("l"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.picker.open {
		t.Error("picker still open after esc")
	}
	if got := m.inputs[fieldDeadline].Value(); got != "09/08/2026" {
		t.Errorf("deadline field = %q, want it untouched", got)
	}
	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput — esc closes the picker, not the form", m.mode)
	}
}

func TestPickerXClearsTheField(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m.inputs[fieldDeadline].SetValue("09/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = m.Update(key("x"))

	if m.picker.open {
		t.Error("picker still open after x")
	}
	if got := m.inputs[fieldDeadline].Value(); got != "" {
		t.Errorf("deadline field = %q, want empty after clearing", got)
	}
}

func TestPickerNavigationKeys(t *testing.T) {
	// ref is Thursday 30/07/2026.
	cases := []struct {
		keys  []string
		want  time.Time
		label string
	}{
		{[]string{"l"}, time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC), "l is one day forward"},
		{[]string{"h"}, time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC), "h is one day back"},
		{[]string{"j"}, time.Date(2026, time.August, 6, 0, 0, 0, 0, time.UTC), "j is one week forward"},
		{[]string{"k"}, time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC), "k is one week back"},
		{[]string{"]"}, time.Date(2026, time.August, 30, 0, 0, 0, 0, time.UTC), "] is one month forward"},
		{[]string{"["}, time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC), "[ is one month back"},
		{[]string{"]", "t"}, time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC), "t returns to today"},
	}
	for _, c := range cases {
		m := fixedClock(NewBoardModel(&task.Board{}))
		m = press(m, "a")
		m.field = fieldDeadline
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		for _, k := range c.keys {
			m, _ = m.Update(key(k))
		}
		if !sameDay(m.picker.cursor, c.want) {
			t.Errorf("%s: cursor = %v, want %v", c.label, m.picker.cursor, c.want)
		}
	}
}

func TestPickerAppearsInTheFormView(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m.SetSize(120, 40)
	m = press(m, "a")
	m.field = fieldDeadline

	before := stripANSI(m.View())
	if strings.Contains(before, "Mo Tu We Th Fr Sa Su") {
		t.Error("the calendar is visible before ctrl+d")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	after := stripANSI(m.View())
	if !strings.Contains(after, "Mo Tu We Th Fr Sa Su") {
		t.Errorf("the calendar did not appear after ctrl+d:\n%s", after)
	}
	if !strings.Contains(after, "July 2026") {
		t.Errorf("the calendar is not showing the current month:\n%s", after)
	}
}

func TestClosingTheFormAlsoClosesThePicker(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // closes the picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // closes the form

	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal", m.mode)
	}
	m = press(m, "a")
	if m.picker.open {
		t.Error("the picker is open on a freshly opened form")
	}
}

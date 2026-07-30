package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

// seeded returns a board with one task per column.
func seeded(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	b.SetPath("")
	for _, s := range task.Statuses {
		id := b.Add("["+string(s)+" task]", ref).ID
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
	b.Add("[One]", ref)
	b.Add("[Two]", ref)
	b.Add("[Three]", ref)
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
	b.Add("[One]", ref)
	b.Add("[Two]", ref)
	m := press(NewBoardModel(b), "ctrl+t", "j")
	if m.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (column focus must not move the item cursor)", m.sel[0])
	}
}

func TestGAndShiftGJumpToEnds(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 4; i++ {
		b.Add("[Task title]", ref)
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

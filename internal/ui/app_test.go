package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
)

func app(t *testing.T) AppModel {
	t.Helper()
	a := NewApp(seeded(t))
	a.board.now = fixedClock(a.board).now
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m.(AppModel)
}

func TestTabSwitchesPages(t *testing.T) {
	a := app(t)
	if a.page != pageBoard {
		t.Fatalf("page = %v, want pageBoard", a.page)
	}
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.page != pageAnalytics {
		t.Fatalf("page = %v, want pageAnalytics", a.page)
	}
	if !strings.Contains(a.View(), "THROUGHPUT") {
		t.Errorf("analytics page view missing THROUGHPUT:\n%s", a.View())
	}

	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.page != pageBoard {
		t.Errorf("page = %v, want pageBoard after a second tab", a.page)
	}
}

func TestQuestionMarkTogglesHelp(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	if !a.showHelp {
		t.Fatal("showHelp = false, want true")
	}
	if !strings.Contains(a.View(), "ctrl+t") {
		t.Errorf("help overlay missing the ctrl+t binding:\n%s", a.View())
	}
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if m.(AppModel).showHelp {
		t.Error("showHelp = true, want false after a second ?")
	}
}

func TestQQuits(t *testing.T) {
	a := app(t)
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q returned a nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q produced %T, want tea.QuitMsg", cmd())
	}
}

func TestQIsTypableWhileAddingATask(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	a = m.(AppModel)
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	a = m.(AppModel)
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("q quit the app while the input was open")
		}
	}
	if a.board.input.Value() != "q" {
		t.Errorf("input value = %q, want %q", a.board.input.Value(), "q")
	}
}

func TestTabIgnoredWhileAddingATask(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	a = m.(AppModel)
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.(AppModel).page != pageBoard {
		t.Error("tab switched pages while the input was open, want it ignored")
	}
}

func TestHelpOpenTabDismissesOnly(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.showHelp {
		t.Error("showHelp = true, want false after tab while help open")
	}
	if a.page != pageBoard {
		t.Errorf("page = %v, want pageBoard unchanged", a.page)
	}
}

func TestHelpOpenQDismissesOnly(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	a = m.(AppModel)
	if a.showHelp {
		t.Error("showHelp = true, want false after q while help open")
	}
	if cmd != nil {
		t.Error("q while help open returned a non-nil cmd, want nil (must not quit)")
	}
}

func TestHelpOpenCtrlCStillQuits(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c while help open returned a nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c while help open produced %T, want tea.QuitMsg", cmd())
	}
}

func TestCoalescedMultiRuneMovesGrabbedTaskTwoColumns(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", ref)
	a := NewApp(b)
	a.board = fixedClock(a.board)

	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	a = m.(AppModel)
	if a.board.mode != modeMove {
		t.Fatalf("mode = %v, want modeMove", a.board.mode)
	}

	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ll")})
	a = m.(AppModel)
	if got := a.board.board.Tasks[0].Status; got != task.StatusBlocked {
		t.Errorf("status after coalesced \"ll\" = %q, want blocked (todo -> doing -> blocked)", got)
	}
}

func TestCoalescedMultiRuneMovesSelectionDownTwo(t *testing.T) {
	b := &task.Board{}
	b.Add("[One]", ref)
	b.Add("[Two]", ref)
	b.Add("[Three]", ref)
	a := NewApp(b)
	a.board = fixedClock(a.board)

	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	a = m.(AppModel)
	if a.board.sel[0] != 2 {
		t.Errorf("sel[0] = %d, want 2 after coalesced \"jj\"", a.board.sel[0])
	}
}

func TestDirtyMsgTriggersSave(t *testing.T) {
	b := &task.Board{}
	dir := t.TempDir()
	b.SetPath(dir + "/tasks.json")
	b.Add("[Task title]", ref)

	a := NewApp(b)
	if _, cmd := a.Update(dirtyMsg{}); cmd != nil {
		cmd() // drain any follow-up
	}
	if _, err := task.Load(dir + "/tasks.json"); err != nil {
		t.Fatalf("Load after dirtyMsg returned %v", err)
	}
	reloaded, _ := task.Load(dir + "/tasks.json")
	if len(reloaded.Tasks) != 1 {
		t.Errorf("saved tasks = %d, want 1", len(reloaded.Tasks))
	}
}

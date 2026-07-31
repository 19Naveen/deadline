package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
)

// archivedBoard returns a board with two archived tasks and one live one.
func archivedBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	due := ref.AddDate(0, 0, -20)

	older := b.Add("[Older archived]", "[first description]", &due, ref.Add(-40*24*time.Hour)).ID
	newer := b.Add("[Newer archived]", "", nil, ref.Add(-30*24*time.Hour)).ID
	for _, id := range []string{older, newer} {
		if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	if n := b.SweepArchive(ref); n != 2 {
		t.Fatalf("SweepArchive = %d, want 2", n)
	}
	b.Add("[Still on the board]", "", nil, ref)
	return b
}

func fixedArchive(b *task.Board) ArchiveModel {
	m := NewArchiveModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(100, 40)
	return m
}

func TestArchiveViewListsArchivedTasksOnly(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())

	for _, want := range []string{"Older archived", "Newer archived"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Still on the board") {
		t.Errorf("View shows a live task; the archive must list archived tasks only:\n%s", out)
	}
}

func TestArchiveViewShowsDescriptionAndDeadline(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, "first description") {
		t.Errorf("View missing the description:\n%s", out)
	}
	if !strings.Contains(out, "10/07/2026") {
		t.Errorf("View missing the deadline date:\n%s", out)
	}
}

func TestArchiveViewShowsArchivedDate(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, FormatDate(ref)) {
		t.Errorf("View missing the archived-on date %s:\n%s", FormatDate(ref), out)
	}
}

func TestArchiveViewCountsEntries(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, "2") || !strings.Contains(out, "ARCHIVE") {
		t.Errorf("View missing the ARCHIVE heading and its count:\n%s", out)
	}
}

func TestArchiveViewEmptyState(t *testing.T) {
	out := stripANSI(fixedArchive(&task.Board{}).View())
	if !strings.Contains(out, "ARCHIVE") {
		t.Errorf("empty archive lost its heading:\n%s", out)
	}
	if !strings.Contains(out, "nothing archived yet") {
		t.Errorf("empty archive missing its placeholder:\n%s", out)
	}
}

func TestArchiveJKMovesSelection(t *testing.T) {
	m := fixedArchive(archivedBoard(t))
	if m.sel != 0 {
		t.Fatalf("sel = %d, want 0", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 1 {
		t.Errorf("sel = %d, want 1 after j", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 1 {
		t.Errorf("sel = %d, want it clamped at the last entry", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0 after k", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want it clamped at the first entry", m.sel)
	}
}

func TestArchiveSelectionSurvivesEmptyBoard(t *testing.T) {
	m := fixedArchive(&task.Board{})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0 on an empty archive", m.sel)
	}
}

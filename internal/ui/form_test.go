package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// editForm opens the edit form on a long-titled subtask with a long parent.
func editForm(t *testing.T, w, h int) (BoardModel, string, string) {
	t.Helper()
	b := &task.Board{}
	parent := b.Add("Hawthorne: tax rates from Xero and one tracking dropdown per category (UAT 25/11 #2, #7)", "", nil, ref).ID
	child, err := b.AddChild(parent, "Hawthorne #7 backend: GET /xero/tax-rates; publish sends the picked TaxType, refuses unknown codes", "", nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	childID := child.ID
	m := fixedClock(NewBoardModel(b))
	m.SetSize(w, h)
	if !m.selectTaskID(childID) {
		t.Fatal("cannot select the child")
	}
	return press(m, "e"), parent, childID
}

// Every gap is present or every gap is gone: never the uneven mix where one
// section sits flush under another.
func TestFormGapsAreAllOrNothing(t *testing.T) {
	for h := 15; h <= 44; h++ {
		for field := 0; field < fieldCount; field++ {
			m, _, _ := editForm(t, 120, h)
			m.field = field
			m.applyFieldFocus()
			l := m.computeFormLayout()
			lines := strings.Split(stripANSI(m.renderForm()), "\n")
			for i, line := range lines {
				for _, label := range []string{"Description", "Deadline", "Parent", "Attachments"} {
					if !strings.HasPrefix(strings.Trim(line, "│ "), label) {
						continue
					}
					if label == "Deadline" && l.showDesc {
						continue // the row above is the editor's last (often empty) row
					}
					blank := strings.Trim(lines[i-1], "│ ") == ""
					if blank != l.gaps {
						t.Errorf("h=%d field=%d: gap before %s = %v, layout gaps = %v:\n%s", h, field, label, blank, l.gaps, strings.Join(lines, "\n"))
					}
				}
			}
			if got := lipgloss.Height(m.View()) + 1; got > h { // +1 for the app tab row
				t.Errorf("h=%d field=%d: form needs %d rows", h, field, got)
			}
		}
	}
}

// Unfocused long values show their start, cut with an ellipsis; the parent
// shows its title and short ID.
func TestFormShowsStartOfLongValues(t *testing.T) {
	m, parent, _ := editForm(t, 120, 40)
	m.field = fieldDeadline
	m.applyFieldFocus()
	out := stripANSI(m.renderForm())
	if !strings.Contains(out, "› Hawthorne #7 backend: GET") || !strings.Contains(out, "…") {
		t.Errorf("unfocused title does not show its start:\n%s", out)
	}
	if !strings.Contains(out, "› Hawthorne: tax rates from Xero") || !strings.Contains(out, shortTaskID(parent)) {
		t.Errorf("parent does not show its title start and short ID:\n%s", out)
	}
}

// Only the focused field carries a hint; the footer follows the field.
func TestFormHintsFollowFocus(t *testing.T) {
	m, _, child := editForm(t, 120, 40)
	out := stripANSI(m.renderForm()) // Title focused
	if !strings.Contains(out, "required") || strings.Contains(out, "dd/mm/yyyy") || strings.Contains(out, "type to search") {
		t.Errorf("hints shown on unfocused fields:\n%s", out)
	}
	if strings.Contains(out, "ctrl+j") {
		t.Errorf("ctrl+j hint outside the description:\n%s", out)
	}
	if !strings.Contains(out, "Edit task") || !strings.Contains(out, shortTaskID(child)) {
		t.Errorf("edit header lacks the short ID:\n%s", out)
	}
	m = press(m, "tab")
	if out := stripANSI(m.renderForm()); !strings.Contains(out, "ctrl+j New line") || strings.Contains(out, "required") {
		t.Errorf("description focus: want ctrl+j hint and no title hint:\n%s", out)
	}
	m.field = fieldAttachments
	m.applyFieldFocus()
	if out := stripANSI(m.renderForm()); strings.Count(out, "type to search") != 1 || !strings.Contains(out, "⌫ Remove") {
		t.Errorf("want one search hint, on the focused field, and the chip keys:\n%s", out)
	}
}

// Attachments and Working notes share the row evenly.
func TestFormDocumentRowIsEvenGrid(t *testing.T) {
	m, _, _ := editForm(t, 120, 40)
	for _, line := range strings.Split(stripANSI(m.renderForm()), "\n") {
		if strings.Contains(line, "Attachments") {
			inner := strings.Trim(line, "│")
			left, _ := formPairWidths(m.popupWidth() - 2)
			if idx := strings.Index(inner, "Working notes"); idx != left+3 { // " " padding + left + 2 gap
				t.Errorf("Working notes starts at %d, want %d:\n%s", idx, left+3, line)
			}
		}
	}
}

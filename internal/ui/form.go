package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// formLayout is what the add/edit form shows at the current terminal size.
// computeFormLayout measures the real sections and gives space back in a
// fixed order, so spacing degrades evenly instead of one gap at a time.
type formLayout struct {
	descRows int  // description editor height
	showDesc bool // description section drawn
	showDocs bool // Attachments / Working notes row drawn
	gaps     bool // one blank row between every section
	calendar bool // deadline calendar under the fields
	dropdown int  // parent dropdown height, 0 when closed
}

// formBudget is the content rows the form may use: the terminal minus the
// tab row and the panel's two border rows. Unbounded until sized.
func (m BoardModel) formBudget() int {
	if m.height <= 0 {
		return 1 << 30
	}
	return m.height - 3
}

// descWantRows is the description height its content asks for: four rows
// when empty, one more than the wrapped text otherwise, capped at descMaxRows.
func (m BoardModel) descWantRows() int {
	inner := max(m.popupWidth()-2-lipgloss.Width(m.desc.Prompt), 1)
	rows := 0
	for _, line := range strings.Split(m.desc.Value(), "\n") {
		rows += max((lipgloss.Width(line)+inner-1)/inner, 1)
	}
	rows++ // room for the cursor's next line
	return min(max(rows, descEmptyRows), descMaxRows)
}

// computeFormLayout picks the richest layout that fits the terminal. Order
// of sacrifice: description shrinks to three rows, the calendar goes, all
// gaps go together, unfocused document fields hide, the description shrinks
// to one row and then hides (unless focused), then the dropdown shrinks.
func (m BoardModel) computeFormLayout() formLayout {
	l := formLayout{
		descRows: m.descWantRows(),
		showDesc: true,
		showDocs: true,
		gaps:     true,
		calendar: m.picker.open,
		dropdown: m.parentDropdownWant(),
	}
	budget := m.formBudget()
	fits := func() bool { return m.formHeight(l) <= budget }
	if fits() {
		return l
	}
	for l.descRows > descMinRows && !fits() {
		l.descRows--
	}
	if !fits() {
		l.calendar = false
	}
	if !fits() {
		l.gaps = false
	}
	if !fits() && m.field != fieldAttachments && m.field != fieldWorkingNotes {
		l.showDocs = false
	}
	for l.descRows > 1 && !fits() {
		l.descRows--
	}
	if !fits() && m.field != fieldDesc {
		l.showDesc = false
	}
	for l.dropdown > 0 && !fits() {
		l.dropdown--
	}
	return l
}

// formHeight is the rendered content height of the form under l.
func (m BoardModel) formHeight(l formLayout) int {
	return lipgloss.Height(strings.Join(m.formRowsFor(l, strings.Repeat("\n", max(l.descRows-1, 0))), "\n"))
}

// parentDropdownWant is the dropdown's natural height while Parent has
// focus: up to parentDropdownRows options plus a heading and two borders.
func (m BoardModel) parentDropdownWant() int {
	if m.field != fieldParent {
		return 0
	}
	return min(max(len(m.filteredParentOptions()), 1), parentDropdownRows) + 3
}

// parentDropdownHeight is the dropdown height the current layout allows.
func (m BoardModel) parentDropdownHeight() int { return m.computeFormLayout().dropdown }

// formPairWidths splits a two-field row evenly, two columns apart.
func formPairWidths(width int) (left, right int) {
	left = (width - 2) / 2
	return left, width - left - 2
}

// sizeForm fits the inputs to the popup and the description to its layout.
func (m *BoardModel) sizeForm() {
	inner := max(m.popupWidth()-2, 10) // panel's one-column padding each side
	// textinput draws "› " plus Width+1 cells (a cursor cell), so each input
	// gets its column minus 3 or it wraps onto a ghost blank row.
	left, right := formPairWidths(inner)
	m.title.Width = inner - 3
	m.deadline.Width = left - 3
	m.worktree.Width = right - 3
	m.parent.Width = inner - 3
	m.attachments.Width = left - 3
	m.workingNotes.Width = right - 3
	m.desc.SetWidth(inner)
	m.desc.SetHeight(max(m.computeFormLayout().descRows, 1))
}

// renderForm draws the add/edit form: title, description, deadline and
// worktree, parent, documents, then the relationships summary.
func (m BoardModel) renderForm() string {
	l := m.computeFormLayout()
	// Pin the editor to the layout height even if sizeForm has not run since
	// the last change, so the measured and drawn heights always agree.
	lines := strings.Split(m.desc.View(), "\n")
	for len(lines) < l.descRows {
		lines = append(lines, "")
	}
	desc := strings.Join(lines[:l.descRows], "\n")
	return ColumnStyle.Copy().Width(m.popupWidth()).Render(strings.Join(m.formRowsFor(l, desc), "\n"))
}

// formRowsFor builds the form's rows under l with the given description
// block, so measuring and drawing share one code path.
func (m BoardModel) formRowsFor(l formLayout, desc string) []string {
	width := m.popupWidth() - 2
	heading := TitleStyle.Render("New task")
	if m.editID != "" {
		id := MutedStyle.Render(shortTaskID(m.editID))
		heading = TitleStyle.Render("Edit task")
		heading += strings.Repeat(" ", max(width-lipgloss.Width(heading)-lipgloss.Width(id), 1)) + id
	}
	var sections [][]string
	add := func(rows ...string) { sections = append(sections, rows) }

	add(m.formLabelFor(fieldTitle, "Title", "required", width), m.formInput(m.title, fieldTitle, width))
	if l.showDesc {
		add(m.formLabelFor(fieldDesc, "Description", "Markdown", width), desc)
	}
	left, right := formPairWidths(width)
	add(m.formPair(fieldDeadline, "Deadline", "dd/mm/yyyy · hjkl calendar", m.deadline,
		fieldWorktree, "Worktree", "path or branch", m.worktree, left, right))
	parent := []string{m.formLabelFor(fieldParent, "Parent", "type to filter", width), m.parentView(width)}
	if l.dropdown > 0 {
		if dropdown := m.renderParentDropdownSized(width, l.dropdown); dropdown != "" {
			parent = append(parent, dropdown)
		}
	}
	add(parent...)
	if l.showDocs {
		add(m.formPair(fieldAttachments, "Attachments", "comma-separated", m.attachments,
			fieldWorkingNotes, "Working notes", "comma-separated", m.workingNotes, left, right))
	}
	if rel := m.formRelationships(width); len(rel) > 0 {
		add(rel[1:]...) // drop its own leading blank; the gap rule spaces it
	}

	rows := []string{heading, MutedStyle.Render(strings.Repeat("─", width))}
	for i, sec := range sections {
		if i > 0 && l.gaps {
			rows = append(rows, "")
		}
		rows = append(rows, sec...)
	}
	if m.err != "" {
		rows = append(rows, errorLine(m.err))
	}
	if l.calendar {
		rows = append(rows, "", m.picker.View(m.now()),
			keyHint("hjkl", "Move")+"  "+keyHint("t", "Today")+MutedStyle.Render("  or type a date"))
	}
	return append(rows, popupFoot(width, m.formHints()...)...)
}

// formLabelFor is a form label whose hint shows only while it has focus, so
// the form carries one hint at a time instead of a hint on every field.
func (m BoardModel) formLabelFor(field int, label, hint string, width int) string {
	if field != m.field {
		hint = ""
	}
	return formLabel(field, m.field, label, hint, width)
}

// formPair renders two fields side by side on an even grid.
func (m BoardModel) formPair(lf int, ll, lh string, lin textinput.Model, rf int, rl, rh string, rin textinput.Model, lw, rw int) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(lw).Render(m.formLabelFor(lf, ll, lh, lw)+"\n"+m.formInput(lin, lf, lw)),
		"  ",
		lipgloss.NewStyle().Width(rw).Render(m.formLabelFor(rf, rl, rh, rw)+"\n"+m.formInput(rin, rf, rw)))
}

// formInput is the live input while focused. Unfocused, it shows the start
// of the value (cut with …) rather than wherever the cursor left it
// scrolled, or the placeholder when empty.
func (m BoardModel) formInput(in textinput.Model, field, width int) string {
	if field == m.field {
		return in.View()
	}
	avail := max(width-lipgloss.Width(in.Prompt), 1)
	if v := in.Value(); v != "" {
		return in.Prompt + truncate(v, avail)
	}
	return in.Prompt + MutedStyle.Render(truncate(in.Placeholder, avail))
}

// parentView shows the chosen parent as its title plus muted short ID, like
// the detail popup; while focused it is the live search input.
func (m BoardModel) parentView(width int) string {
	if m.field == fieldParent || m.formParentID == "" {
		return m.formInput(m.parent, fieldParent, width)
	}
	id := MutedStyle.Render(shortTaskID(m.formParentID))
	title := m.parent.Value()
	if parent, ok := m.board.TaskByID(m.formParentID); ok {
		title = parent.Title
	}
	avail := max(width-lipgloss.Width(m.parent.Prompt)-lipgloss.Width(id)-2, 1)
	title = truncate(strings.Join(strings.Fields(title), " "), avail)
	gap := max(width-lipgloss.Width(m.parent.Prompt)-lipgloss.Width(title)-lipgloss.Width(id), 2)
	return m.parent.Prompt + title + strings.Repeat(" ", gap) + id
}

// formHints is the footer: field-specific keys first, then the common ones.
func (m BoardModel) formHints() []string {
	var hints []string
	switch m.field {
	case fieldDesc:
		hints = append(hints, keyHint("ctrl+j", "New line"))
	case fieldParent:
		hints = append(hints, keyHint("↑↓", "Choose"))
	}
	return append(hints, keyHint("tab", "Next"), keyHint("shift+tab", "Back"),
		keyHint("enter", "Save"), keyHint("esc", "Cancel"))
}

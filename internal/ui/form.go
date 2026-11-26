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
	showDate bool // Deadline / Worktree row drawn
	showPar  bool // Parent row drawn
	showRel  bool // relationships summary drawn
	gaps     bool // one blank row between every section
	calendar bool // deadline calendar under the fields
	dropdown int  // focused field's dropdown height, 0 when closed
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
// to one row and then hides (unless focused), the relationships summary
// goes, the unfocused Deadline/Worktree and Parent rows hide, then the
// focused field's dropdown shrinks.
func (m BoardModel) computeFormLayout() formLayout {
	l := formLayout{
		descRows: m.descWantRows(),
		showDesc: true,
		showDocs: true,
		showDate: true,
		showPar:  true,
		showRel:  true,
		gaps:     true,
		calendar: m.picker.open,
		dropdown: m.dropdownWant(),
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
	if !fits() {
		l.showRel = false
	}
	if !fits() && m.field != fieldDeadline && m.field != fieldWorktree {
		l.showDate = false
	}
	if !fits() && m.field != fieldParent {
		l.showPar = false
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
	m.attachments.Width = m.docInputWidth(fieldAttachments, left)
	m.workingNotes.Width = m.docInputWidth(fieldWorkingNotes, right)
	m.attachments.Placeholder = docPlaceholder(m.attachments.Width)
	m.workingNotes.Placeholder = docPlaceholder(m.workingNotes.Width)
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
	// withDropdown hangs the focused field's dropdown under its section.
	withDropdown := func(field int, rows ...string) []string {
		if m.field == field || (isDocField(field) && isDocField(m.field)) {
			if dd := m.renderFieldDropdown(width, l.dropdown); l.dropdown > 0 && dd != "" {
				rows = append(rows, dd)
			}
		}
		return rows
	}
	wtHint := "path or branch"
	if m.wt.repo {
		wtHint = "type to search"
	}
	if l.showDate {
		add(withDropdown(fieldWorktree, m.formPair(fieldDeadline, "Deadline", "dd/mm/yyyy · hjkl calendar", m.formInput(m.deadline, fieldDeadline, left),
			fieldWorktree, "Worktree", wtHint, m.formInput(m.worktree, fieldWorktree, right), left, right))...)
	}
	if l.showPar {
		add(withDropdown(fieldParent, m.formLabelFor(fieldParent, "Parent", "type to filter", width), m.parentView(width))...)
	}
	if l.showDocs {
		add(withDropdown(fieldAttachments, m.formPair(fieldAttachments, "Attachments", "type to search", m.docFieldView(fieldAttachments, left),
			fieldWorkingNotes, "Working notes", "type to search", m.docFieldView(fieldWorkingNotes, right), left, right))...)
	}
	if rel := m.formRelationships(width); l.showRel && len(rel) > 0 {
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

// formPair renders two fields side by side on an even grid; lv and rv are
// the rendered values under each label.
func (m BoardModel) formPair(lf int, ll, lh, lv string, rf int, rl, rh, rv string, lw, rw int) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(lw).MaxWidth(lw).Render(m.formLabelFor(lf, ll, lh, lw)+"\n"+lv),
		"  ",
		lipgloss.NewStyle().Width(rw).MaxWidth(rw).Render(m.formLabelFor(rf, rl, rh, rw)+"\n"+rv))
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
	case fieldWorktree:
		if m.wt.repo {
			hints = append(hints, keyHint("↑↓", "Choose"))
		}
	case fieldAttachments, fieldWorkingNotes:
		hints = append(hints, keyHint("↑↓", "Choose"), keyHint("enter", "Add"), keyHint("⌫", "Remove"))
	}
	return append(hints, keyHint("tab", "Next"), keyHint("shift+tab", "Back"),
		keyHint("enter", "Save"), keyHint("esc", "Cancel"))
}

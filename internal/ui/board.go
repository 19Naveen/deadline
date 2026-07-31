package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

type focusMode int

const (
	focusItem focusMode = iota
	focusColumn
)

type boardMode int

const (
	modeNormal boardMode = iota
	modeInput
	modeMove
	modeConfirm
)

// Fields of the add/edit form, in tab order.
const (
	fieldTitle = iota
	fieldDesc
	fieldDeadline
)

// BoardModel is the kanban page.
type BoardModel struct {
	board *task.Board

	col   int    // focused column, index into task.Statuses
	sel   [4]int // selected item per column
	focus focusMode
	mode  boardMode

	inputs [3]textinput.Model
	field  int        // which of inputs has focus
	picker datePicker // the deadline calendar, when open

	editID   string      // set while editing an existing task
	grabID   string      // set while in modeMove
	grabFrom task.Status // original column of the grabbed task

	now func() time.Time // injectable clock; tests pin it

	width      int
	height     int
	err        string
	footerRows int // height of the last-rendered footer; set by View, read by columnHeight
}

// NewBoardModel wires a board into a fresh page model.
func NewBoardModel(b *task.Board) BoardModel {
	m := BoardModel{board: b, focus: focusItem, mode: modeNormal, now: time.Now}
	placeholders := [3]string{"task title", "description (optional)", "DD/MM/YYYY (optional)"}
	limits := [3]int{200, 500, 10}
	for i := range m.inputs {
		in := textinput.New()
		in.Placeholder = placeholders[i]
		in.CharLimit = limits[i]
		in.Prompt = "› "
		m.inputs[i] = in
	}
	return m
}

// SetSize records the terminal size for layout.
func (m *BoardModel) SetSize(w, h int) { m.width, m.height = w, h }

func (m BoardModel) currentStatus() task.Status { return task.Statuses[m.col] }

// selectedTask is the card under the cursor, if the column is not empty.
func (m BoardModel) selectedTask() (task.Task, bool) {
	items := m.board.ByStatus(m.currentStatus())
	i := m.sel[m.col]
	if i < 0 || i >= len(items) {
		return task.Task{}, false
	}
	return items[i], true
}

// clampSelection keeps every column's cursor inside its item range.
func (m *BoardModel) clampSelection() {
	for i, s := range task.Statuses {
		n := len(m.board.ByStatus(s))
		if m.sel[i] >= n {
			m.sel[i] = n - 1
		}
		if m.sel[i] < 0 {
			m.sel[i] = 0
		}
	}
}

const (
	// minColumnWidth is the floor set by the widest deadline line
	// ("● DD/MM/YYYY ✗", 14 columns) plus the card's own border and padding.
	// Do not lower this without re-checking
	// TestCardDeadlineFitsAtMinimumColumnWidth.
	minColumnWidth = 18
	// columnChrome is how much wider than its content each rendered column
	// is: ColumnStyle's one-column border plus one-column padding, on each
	// side that columnWidth's "w/n - columnChrome" already subtracts for.
	columnChrome = 2
)

// columnWidth splits the terminal evenly across the four columns, leaving
// room for each column's border and padding.
func (m BoardModel) columnWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	per := w/len(task.Statuses) - columnChrome
	if per < minColumnWidth {
		per = minColumnWidth
	}
	return per
}

// minBoardWidth is the narrowest terminal that can show all four columns at
// minColumnWidth without wrapping. View falls back to a warning below this.
func (m BoardModel) minBoardWidth() int {
	return len(task.Statuses) * (minColumnWidth + columnChrome)
}

// minColumnBlockHeight is the least columnHeight() will ever return: header,
// blank line, one card row, plus the column's own top+bottom border. Below
// it a column shows nothing worth looking at, so View bails out of drawing
// the board entirely rather than forcing this floor and overflowing.
const minColumnBlockHeight = 5

// rawColumnHeight is columnHeight() before its floor is applied — what the
// terminal actually has room for, which can go negative on a short terminal
// with a tall footer (the form, or the form with the calendar open).
// Reserves one row for the tab bar (AppModel joins that on above this
// model's own View), two for the column block's border, and one spare row
// so the board isn't rendered flush against the very last line.
func (m BoardModel) rawColumnHeight() int {
	return m.height - 4 - m.footerRows
}

func (m BoardModel) columnHeight() int {
	h := m.rawColumnHeight()
	if h < minColumnBlockHeight {
		h = minColumnBlockHeight
	}
	return h
}

// View renders the four columns side by side plus the footer line. Below
// minBoardWidth the columns would overflow and wrap into a scrambled mess,
// so it renders a short warning instead. m.width == 0 (before the first
// WindowSizeMsg) falls through to the normal path, which defaults to 80.
//
// The footer is measured before the columns are laid out: in modeInput the
// footer is the whole bordered form panel (and grows further when the date
// picker is open), not the fixed one-line hint of normal mode. columnHeight
// needs that real height, not a hard-coded guess, or the calendar pushes the
// board off the top of the screen. m has a value receiver, so the recorded
// height is stashed on this local copy before it is used below.
//
// When the form footer alone leaves no room for even a floored column block,
// clamping to the floor would still overflow the terminal. The user has the
// form open and is looking at it, so below that point View shows just the
// footer instead of a board squeezed to nothing and scrolled off screen.
func (m BoardModel) View() string {
	if need := m.minBoardWidth(); m.width > 0 && m.width < need {
		return MutedStyle.Render(fmt.Sprintf(
			"terminal too narrow\n\ngotodo needs at least %d columns for the four-column board.\nThis terminal is %d. Widen it, or press tab for Analytics or Archive.",
			need, m.width))
	}
	footer := m.renderFooter()
	m.footerRows = lipgloss.Height(footer)

	if m.mode == modeInput && m.rawColumnHeight() < minColumnBlockHeight {
		return footer
	}

	cw := m.columnWidth()
	cols := make([]string, 0, len(task.Statuses))
	for i, s := range task.Statuses {
		cols = append(cols, m.renderColumn(i, s, cw))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (m BoardModel) renderColumn(idx int, s task.Status, width int) string {
	items := m.board.ByStatus(s)

	header := HeaderStyle(s).Render(s.Label()) +
		MutedStyle.Render(" ("+strconv.Itoa(len(items))+")")

	var lines []string
	lines = append(lines, header, "")

	if len(items) == 0 {
		lines = append(lines, MutedStyle.Render("empty"))
	}
	lines = append(lines, m.renderCardWindow(idx, items, width)...)

	style := ColumnStyle.Copy()
	if idx == m.col {
		style = ColumnFocusedStyle.Copy()
		if m.focus == focusColumn {
			style = style.BorderForeground(AccentFor(s))
		}
	}
	return style.Width(width).Height(m.columnHeight()).
		Render(strings.Join(lines, "\n"))
}

// renderCardWindow renders as many cards as fit within the column's height
// (lipgloss.Style.Height only pads, it does not clip), separated by a blank
// line. When some cards do not fit it emits a trailing "+N more" line, and
// scrolls the window down so the currently selected card is always shown
// rather than clipped away.
func (m BoardModel) renderCardWindow(idx int, items []task.Task, width int) []string {
	avail := m.columnHeight() - 2 // header + blank line already emitted

	sel := -1
	if idx == m.col {
		sel = m.sel[m.col]
	}

	rendered := make([]string, len(items))
	heights := make([]int, len(items))
	for i, t := range items {
		rendered[i] = m.renderCard(idx, i, t, width)
		heights[i] = strings.Count(rendered[i], "\n") + 1
	}

	start, end, more := fitWindow(heights, avail, sel)

	var lines []string
	for i := start; i < end; i++ {
		if i > start {
			lines = append(lines, "")
		}
		lines = append(lines, rendered[i])
	}
	if more > 0 {
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("+%d more", more)))
	}
	return lines
}

// fitWindow picks the run of entries [start, end) that fits within avail
// rows — heights[i] gives entry i's own row count, and every entry after
// the first in the window costs one more row for its blank separator — while
// scrolling so the entry at sel (if any, sel < 0 means nothing is selected)
// stays inside the window rather than falling off past the fold. more
// reports how many trailing entries were left out, after reserving a row
// for the "+N more" line the caller appends when it is non-zero.
//
// Shared by BoardModel's card columns and ArchiveModel's single list: both
// need the same variable-height, keep-the-selection-visible windowing.
func fitWindow(heights []int, avail, sel int) (start, end, more int) {
	if avail < 1 {
		avail = 1
	}
	n := len(heights)

	// windowRows is the row count of [s,e), including the blank separator
	// before every entry after the first in the window.
	windowRows := func(s, e int) int {
		rows := 0
		for i := s; i < e; i++ {
			if i > s {
				rows++
			}
			rows += heights[i]
		}
		return rows
	}

	// fit grows e greedily from s while the window still fits in avail.
	fit := func(s int) int {
		e := s
		for e < n && windowRows(s, e+1) <= avail {
			e++
		}
		return e
	}

	start = 0
	end = fit(start)
	// Scroll down until the selected entry is inside the window.
	for sel >= end && start < n-1 {
		start++
		end = fit(start)
	}

	more = n - end
	if more > 0 {
		// Reserve one row for the "+N more" line.
		for end > start && windowRows(start, end)+1 > avail {
			end--
		}
		more = n - end
	}
	return start, end, more
}

// renderCard draws one card: title, optional description, optional deadline.
// A card is 1 to 3 lines tall depending on which fields are set.
func (m BoardModel) renderCard(colIdx, itemIdx int, t task.Task, width int) string {
	inner := width - 4
	selected := colIdx == m.col && itemIdx == m.sel[m.col]
	grabbed := selected && m.mode == modeMove && t.ID == m.grabID

	title := truncate(t.Title, inner)
	if grabbed {
		title = "⇄ " + truncate(t.Title, inner-2)
	}

	lines := []string{title}
	if t.Description != "" {
		lines = append(lines, MutedStyle.Render(truncate(t.Description, inner)))
	}
	if dl := RenderDeadline(t, m.now()); dl != "" {
		lines = append(lines, dl)
	}
	body := strings.Join(lines, "\n")

	switch {
	case grabbed:
		return CardGrabbedStyle.Render(body)
	case selected && m.focus == focusItem:
		return CardSelectedStyle.Render(body)
	default:
		return CardStyle.Render(body)
	}
}

// parseDeadline reads a DD/MM/YYYY date. Blank means "no deadline", which is
// not an error. The layout matches FormatDate, so what the card shows is
// exactly what you type back in.
//
// Parsed in the local zone, not UTC: DaysUntilDeadline re-anchors the
// deadline to now's location before taking its calendar day. Parsing into
// UTC would shift that calendar day back for anyone west of UTC, making a
// deadline typed for "today" read as overdue on the day it's due.
func parseDeadline(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	d, err := time.ParseInLocation("02/01/2006", s, time.Local)
	if err != nil {
		return nil, errors.New("deadline must look like 02/08/2026, or be left blank")
	}
	return &d, nil
}

// openForm puts the model into the add/edit form, seeded with the given
// values and focused on the title.
func (m *BoardModel) openForm(editID, title, desc string, deadline *time.Time) {
	m.mode = modeInput
	m.editID = editID
	m.field = fieldTitle
	m.err = ""

	dl := ""
	if deadline != nil {
		dl = FormatDate(*deadline)
	}
	for i, v := range [3]string{title, desc, dl} {
		m.inputs[i].SetValue(v)
		m.inputs[i].CursorEnd()
		m.inputs[i].Blur()
	}
	m.inputs[fieldTitle].Focus()
}

// closeForm returns to normal mode and drops focus from every field.
func (m *BoardModel) closeForm() {
	m.mode = modeNormal
	m.editID = ""
	m.err = ""
	m.picker = datePicker{}
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

// focusField moves form focus by delta, wrapping at both ends.
func (m *BoardModel) focusField(delta int) {
	m.inputs[m.field].Blur()
	m.field = (m.field + delta + len(m.inputs)) % len(m.inputs)
	m.inputs[m.field].Focus()
	m.inputs[m.field].CursorEnd()
}

// renderForm draws the three-field entry panel shown at the bottom.
func (m BoardModel) renderForm() string {
	heading := "new task"
	if m.editID != "" {
		heading = "edit task"
	}
	labels := [3]string{"Title", "Description", "Deadline"}

	rows := []string{TitleStyle.Render(heading)}
	for i, label := range labels {
		name := MutedStyle.Render(fmt.Sprintf("%-12s", label))
		if i == m.field {
			name = lipgloss.NewStyle().Foreground(ColAccent).Bold(true).
				Render(fmt.Sprintf("%-12s", label))
		}
		rows = append(rows, name+m.inputs[i].View())
	}
	if m.err != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(AccentFor(task.StatusBlocked)).
			Render("! "+m.err))
	}
	if m.picker.open {
		rows = append(rows, "", m.picker.View(m.now()))
		rows = append(rows, MutedStyle.Render(
			"hjkl day/week · [ ] month · t today · enter pick · x clear · esc close"))
	} else {
		hint := "tab/shift+tab field · enter save · esc cancel"
		if m.field == fieldDeadline {
			hint = "ctrl+d calendar · " + hint
		}
		rows = append(rows, MutedStyle.Render(hint))
	}
	return ColumnStyle.Render(strings.Join(rows, "\n"))
}

func (m BoardModel) renderFooter() string {
	switch m.mode {
	case modeInput:
		return "\n" + m.renderForm()
	case modeMove:
		return HelpStyle.Render("move: h/l reposition · enter drop · esc cancel")
	case modeConfirm:
		return HelpStyle.Render("delete this task? y / n")
	}
	if m.err != "" {
		return HelpStyle.Render("! " + m.err)
	}
	focusLabel := "item"
	if m.focus == focusColumn {
		focusLabel = "column"
	}
	return HelpStyle.Render(
		"focus: " + focusLabel + " (ctrl+t) · hjkl move · a add · e edit · d delete · m grab · tab analytics · ? help · q quit")
}

// truncate shortens s to fit n display columns, appending an ellipsis when
// it has to cut. It measures with lipgloss.Width, not rune count: CJK and
// emoji occupy two columns each, so a rune budget silently overflows.
// ponytail: O(n^2) shrink loop on short strings (card titles/descriptions);
// fine at this size, revisit if it ever runs on long text.
func truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)+"…") > n {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// Update dispatches on the current mode: modeInput handles add/edit text
// entry, modeMove handles grab-to-move, modeConfirm handles the delete
// prompt, and everything else falls through to normal navigation.
func (m BoardModel) Update(msg tea.Msg) (BoardModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch m.mode {
	case modeInput:
		return m.updateInput(keyMsg)
	case modeMove:
		return m.updateMove(keyMsg)
	case modeConfirm:
		return m.updateConfirm(keyMsg)
	}
	return m.updateNormal(keyMsg)
}

// updateNormal handles navigation and focus in the default mode.
func (m BoardModel) updateNormal(keyMsg tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.err = ""

	if keyMsg.Type == tea.KeyCtrlT {
		if m.focus == focusItem {
			m.focus = focusColumn
		} else {
			m.focus = focusItem
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "h", "left":
		m.moveColumn(-1)
	case "l", "right":
		m.moveColumn(1)
	case "j", "down":
		if m.focus == focusItem {
			m.moveItem(1)
		}
	case "k", "up":
		if m.focus == focusItem {
			m.moveItem(-1)
		}
	case "g":
		if m.focus == focusItem {
			m.sel[m.col] = 0
		}
	case "G":
		if m.focus == focusItem {
			m.sel[m.col] = len(m.board.ByStatus(m.currentStatus())) - 1
		}
	case "a":
		m.openForm("", "", "", nil)
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.openForm(t.ID, t.Title, t.Description, t.Deadline)
	case "d":
		if _, ok := m.selectedTask(); !ok {
			return m, nil
		}
		m.mode = modeConfirm
	case "m":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.mode = modeMove
		m.focus = focusItem
		m.grabID = t.ID
		m.grabFrom = t.Status
	}
	m.clampSelection()
	return m, nil
}

// dirtyMsg signals that the board changed and should be persisted.
type dirtyMsg struct{}

func dirty() tea.Cmd { return func() tea.Msg { return dirtyMsg{} } }

func (m BoardModel) updateInput(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	// While the calendar is up it owns every key — otherwise enter would
	// save the task instead of picking a date.
	if m.picker.open {
		return m.updatePicker(k)
	}

	switch k.Type {
	case tea.KeyEsc:
		m.closeForm()
		return m, nil

	case tea.KeyTab:
		m.focusField(1)
		return m, nil

	case tea.KeyShiftTab:
		m.focusField(-1)
		return m, nil

	case tea.KeyCtrlD:
		if m.field != fieldDeadline {
			return m, nil
		}
		seed := m.now()
		if d, err := parseDeadline(m.inputs[fieldDeadline].Value()); err == nil && d != nil {
			seed = *d
		}
		m.picker = newDatePicker(seed)
		m.err = ""
		return m, nil

	case tea.KeyEnter:
		title := strings.TrimSpace(m.inputs[fieldTitle].Value())
		if title == "" {
			m.err = "title must not be blank"
			return m, nil // stay in the form
		}
		deadline, err := parseDeadline(m.inputs[fieldDeadline].Value())
		if err != nil {
			m.err = err.Error()
			return m, nil // stay in the form
		}
		desc := strings.TrimSpace(m.inputs[fieldDesc].Value())

		if m.editID != "" {
			if err := m.board.Edit(m.editID, title, desc, deadline, m.now()); err != nil {
				m.err = err.Error()
				return m, nil
			}
		} else {
			m.board.Add(title, desc, deadline, m.now())
			m.col = 0
			m.sel[0] = len(m.board.ByStatus(task.StatusTodo)) - 1
		}
		m.closeForm()
		m.clampSelection()
		return m, dirty()
	}

	var cmd tea.Cmd
	m.inputs[m.field], cmd = m.inputs[m.field].Update(k)
	return m, cmd
}

// updatePicker handles keys while the deadline calendar is open. It never
// saves the task: enter picks a date and hands control back to the form.
func (m BoardModel) updatePicker(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		m.picker = datePicker{}
		return m, nil

	case tea.KeyEnter:
		m.inputs[fieldDeadline].SetValue(FormatDate(m.picker.cursor))
		m.inputs[fieldDeadline].CursorEnd()
		m.picker = datePicker{}
		return m, nil

	case tea.KeyLeft:
		m.picker = m.picker.move(-1)
		return m, nil
	case tea.KeyRight:
		m.picker = m.picker.move(1)
		return m, nil
	case tea.KeyUp:
		m.picker = m.picker.move(-7)
		return m, nil
	case tea.KeyDown:
		m.picker = m.picker.move(7)
		return m, nil
	}

	switch k.String() {
	case "h":
		m.picker = m.picker.move(-1)
	case "l":
		m.picker = m.picker.move(1)
	case "k":
		m.picker = m.picker.move(-7)
	case "j":
		m.picker = m.picker.move(7)
	case "[":
		m.picker = m.picker.addMonths(-1)
	case "]":
		m.picker = m.picker.addMonths(1)
	case "t":
		m.picker = newDatePicker(m.now())
	case "x":
		m.inputs[fieldDeadline].SetValue("")
		m.picker = datePicker{}
	}
	return m, nil
}

func (m BoardModel) updateMove(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.String() {
	case "h", "left":
		if m.shiftGrabbed(-1) {
			return m, dirty()
		}
		return m, nil
	case "l", "right":
		if m.shiftGrabbed(1) {
			return m, dirty()
		}
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.grabID = ""
		m.clampSelection()
		return m, dirty()
	case "esc":
		if err := m.board.Move(m.grabID, m.grabFrom, m.now()); err != nil {
			m.err = err.Error()
		}
		m.col = m.grabFrom.Index()
		m.mode = modeNormal
		m.grabID = ""
		m.clampSelection()
		return m, dirty()
	}
	return m, nil
}

// shiftGrabbed moves the grabbed task one column over and follows it. It
// reports whether anything actually moved, so a no-op press (at either end
// of the board, or on a Move error) doesn't trigger a spurious dirty save.
func (m *BoardModel) shiftGrabbed(delta int) bool {
	next := m.col + delta
	if next < 0 || next >= len(task.Statuses) {
		return false
	}
	if err := m.board.Move(m.grabID, task.Statuses[next], m.now()); err != nil {
		m.err = err.Error()
		return false
	}
	m.col = next
	items := m.board.ByStatus(task.Statuses[next])
	for i, t := range items {
		if t.ID == m.grabID {
			m.sel[next] = i
			break
		}
	}
	m.clampSelection()
	return true
}

func (m BoardModel) updateConfirm(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.mode = modeNormal
	if k.String() != "y" {
		return m, nil
	}
	t, ok := m.selectedTask()
	if !ok {
		return m, nil
	}
	if err := m.board.Delete(t.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.clampSelection()
	return m, dirty()
}

// moveColumn shifts the focused column, clamped at both edges.
func (m *BoardModel) moveColumn(delta int) {
	next := m.col + delta
	if next < 0 {
		next = 0
	}
	if next >= len(task.Statuses) {
		next = len(task.Statuses) - 1
	}
	m.col = next
}

// moveItem shifts the cursor within the focused column, clamped at both ends.
func (m *BoardModel) moveItem(delta int) {
	n := len(m.board.ByStatus(m.currentStatus()))
	if n == 0 {
		m.sel[m.col] = 0
		return
	}
	next := m.sel[m.col] + delta
	if next < 0 {
		next = 0
	}
	if next >= n {
		next = n - 1
	}
	m.sel[m.col] = next
}

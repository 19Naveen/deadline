package ui

import (
	"encoding/csv"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
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
	modeDetail
	modeSearch // typing a / filter in the footer
	modeOpen   // choosing a document from the detail popup
)

// Fields of the add/edit form, in tab order.
const (
	fieldTitle = iota
	fieldDesc
	fieldDeadline
	fieldWorktree
	fieldParent
	fieldAttachments
	fieldWorkingNotes
	fieldCount
)

// BoardModel is the kanban page.
type BoardModel struct {
	board *task.Board

	col   int   // focused column, index into the board's columns
	sel   []int // selected item per column, same length as the board's columns
	focus focusMode
	mode  boardMode

	title        textinput.Model
	desc         textarea.Model // multi-line: ctrl+j inserts a newline, enter saves
	deadline     textinput.Model
	worktree     textinput.Model
	parent       textinput.Model
	attachments  textinput.Model
	workingNotes textinput.Model
	field        int        // which form field has focus
	picker       datePicker // the deadline calendar, when open
	// The parent field filters eligible roots; formParentID is set only once
	// Enter confirms the highlighted option.
	formParentID   string
	parentIndex    int
	parentChoosing bool

	editID   string      // set while editing an existing task
	grabID   string      // set while in modeMove
	grabFrom task.Status // original column of the grabbed task

	detailScroll int    // detail popup body offset in wrapped rows
	familyID     string // root task shown by the optional family filter

	search textinput.Model // the / filter while typing
	query  string          // the active / filter; empty shows every card
	choice int             // highlighted row in the document chooser

	// opener opens one resolved document path; tests swap it out.
	opener func(path string) tea.Cmd

	undo     []undoEntry // newest last, at most undoLimit
	grabSnap []task.Task // the grabbed task before its first shift
	info     string      // one-shot footer notice, cleared by the next key

	now func() time.Time // injectable clock; tests pin it

	width      int
	height     int
	err        string
	footerRows int // height of the last-rendered footer; set by View, read by columnHeight
}

// NewBoardModel wires a board into a fresh page model.
func NewBoardModel(b *task.Board) BoardModel {
	m := BoardModel{board: b, focus: focusItem, mode: modeNormal, now: time.Now, opener: openDocument}
	m.sel = make([]int, len(b.Statuses()))

	line := func(placeholder string, limit int) textinput.Model {
		in := textinput.New()
		in.Placeholder = placeholder
		in.CharLimit = limit
		in.Prompt = "› "
		return in
	}
	m.title = line("task title", 200)
	m.deadline = line("DD/MM/YYYY", 10)
	m.worktree = line("path or branch", 500)
	m.parent = line("none: type a title or ID", 200)
	m.attachments = line("none", 4000)
	m.workingNotes = line("none", 4000)
	m.search = line("title, #label, @agent or ID", 200)
	m.search.Prompt = "/ "

	m.desc = textarea.New()
	m.desc.Placeholder = "Write details, context, or Markdown…"
	m.desc.CharLimit = 10000
	// The field label already marks focus; a plain gutter beats a per-row arrow.
	m.desc.Prompt = "  "
	m.desc.ShowLineNumbers = false
	// Enter saves the task, so newline moves to ctrl+j (0x0A vs enter's 0x0D).
	m.desc.KeyMap.InsertNewline.SetKeys("ctrl+j")
	m.sizeForm()
	return m
}

// SetSize records the terminal size for layout.
func (m *BoardModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.search.Width = max(min(w-40, 48), 10)
	m.sizeForm()
}

const (
	// popupMaxWidth caps the centred panels at a comfortable line length.
	popupMaxWidth = 72
	// The description editor is bounded so the other fields stay visible.
	// It opens at descEmptyRows, grows with its content up to descMaxRows,
	// and gives rows back (down to one) only when the terminal is short.
	descEmptyRows = 4
	descMaxRows   = 15
	descMinRows   = 3
)

// popupWidth is the content width of the centred panels, with a side margin.
func (m BoardModel) popupWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	w -= 8
	if w > popupMaxWidth {
		w = popupMaxWidth
	}
	if w < minColumnWidth {
		w = minColumnWidth
	}
	return w
}

// overlay centres a panel on the page. lipgloss has no compositing at this
// version, so the popup replaces the board rather than floating over it.
// ponytail: no true overlay, revisit if the board behind is worth the
// ANSI-aware line surgery it would take.
func (m BoardModel) overlay(panel string) string {
	w, h := m.width, m.height-2 // AppModel joins the tab bar on above us
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 22
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, panel)
}

func (m BoardModel) currentStatus() task.Status { return m.board.Statuses()[m.col] }

// visibleByStatus applies the optional family filter without changing the
// board's domain ordering or hiding tasks from other pages.
func (m BoardModel) visibleByStatus(s task.Status) []task.Task {
	items := m.board.ByStatus(s)
	if !m.filtering() {
		return items
	}
	out := make([]task.Task, 0, len(items))
	for _, t := range items {
		if m.familyID != "" && t.ID != m.familyID && t.ParentID != m.familyID {
			continue
		}
		if !matchesQuery(t, m.query) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// selectedTask is the card under the cursor, if the column is not empty.
func (m BoardModel) selectedTask() (task.Task, bool) {
	items := m.visibleByStatus(m.currentStatus())
	i := m.sel[m.col]
	if i < 0 || i >= len(items) {
		return task.Task{}, false
	}
	return items[i], true
}

// clampSelection keeps every column's cursor inside its item range.
func (m *BoardModel) clampSelection() {
	for i, s := range m.board.Statuses() {
		n := len(m.visibleByStatus(s))
		if m.sel[i] >= n {
			m.sel[i] = n - 1
		}
		if m.sel[i] < 0 {
			m.sel[i] = 0
		}
	}
}

// selectTaskID restores cursor identity after filtering or mutation.
func (m *BoardModel) selectTaskID(id string) bool {
	for col, s := range m.board.Statuses() {
		for i, t := range m.visibleByStatus(s) {
			if t.ID == id {
				m.col, m.sel[col] = col, i
				return true
			}
		}
	}
	return false
}

// reconcileFamily clears a stale family filter and restores selection by ID
// after live reload or archival changes.
func (m *BoardModel) reconcileFamily(selectedID string) {
	if m.familyID != "" {
		if selected, ok := m.board.TaskByID(selectedID); ok && !selected.Archived {
			m.familyID = m.board.FamilyRootID(selected)
		}
		root, ok := m.board.TaskByID(m.familyID)
		if !ok || root.Archived || root.ParentID != "" {
			m.familyID = ""
		}
	}
	if selectedID == "" || !m.selectTaskID(selectedID) {
		m.clampSelection()
	}
}

const (
	// minColumnWidth fits the widest deadline line ("● DD/MM/YYYY ✗", 14
	// columns) plus the card's border and padding.
	minColumnWidth = 18
	// columnChrome is each column's extra width beyond its content (border
	// plus padding).
	columnChrome = 2
)

// columnWidth splits the terminal evenly across the board's columns.
func (m BoardModel) columnWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	per := w/len(m.board.Statuses()) - columnChrome
	if per < minColumnWidth {
		per = minColumnWidth
	}
	return per
}

// minBoardWidth is the narrowest terminal that can show every column at
// minColumnWidth without wrapping. View falls back to a warning below this.
func (m BoardModel) minBoardWidth() int {
	return len(m.board.Statuses()) * (minColumnWidth + columnChrome)
}

// minColumnBlockHeight is columnHeight's floor: header, blank, one card row,
// and the column's top+bottom border.
const minColumnBlockHeight = 5

// rawColumnHeight is columnHeight without its floor; it can go negative.
// Reserves the tab bar row and the column border's two rows, so the footer
// lands on the last row like every other page's.
func (m BoardModel) rawColumnHeight() int {
	return m.height - 3 - m.footerRows
}

func (m BoardModel) columnHeight() int {
	h := m.rawColumnHeight()
	if h < minColumnBlockHeight {
		h = minColumnBlockHeight
	}
	return h
}

// View renders the columns plus footer, or a centred popup. Below
// minBoardWidth it shows a warning instead of wrapped columns (popups are
// exempt). The footer is measured first so columnHeight sees its real height.
func (m BoardModel) View() string {
	switch m.mode {
	case modeDetail:
		return m.overlay(m.renderDetail())
	case modeInput:
		return m.overlay(m.renderForm())
	case modeConfirm:
		return m.overlay(m.renderConfirm())
	case modeOpen:
		return m.overlay(m.renderOpen())
	}
	if need := m.minBoardWidth(); m.width > 0 && m.width < need {
		msg := fmt.Sprintf("gotodo needs at least %d columns for the %d-column board. This terminal is %d. Widen it, or press tab for the other pages.",
			need, len(m.board.Statuses()), m.width)
		w := m.popupWidth()
		return m.overlay(ColumnStyle.Copy().Width(w).Render(strings.Join([]string{
			ErrorStyle.Render("Terminal too narrow"),
			MutedStyle.Render(strings.Repeat("─", w-2)),
			MutedStyle.Copy().Width(w - 2).Render(msg),
		}, "\n")))
	}
	footer := m.renderFooter()
	m.footerRows = lipgloss.Height(footer)

	cw := m.columnWidth()
	cols := make([]string, 0, len(m.board.Statuses()))
	for i, s := range m.board.Statuses() {
		cols = append(cols, m.renderColumn(i, s, cw))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (m BoardModel) renderColumn(idx int, s task.Status, width int) string {
	items := m.visibleByStatus(s)
	header := m.columnHeader(s, len(items), width)

	var lines []string
	lines = append(lines, header, "")

	if len(items) == 0 {
		lines = append(lines, MutedStyle.Render("  empty"))
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

// renderCardWindow renders the cards that fit the column height, keeping the
// selected card visible and ending with "+N more" when some are cut.
// lipgloss Height only pads, it never clips, so the windowing happens here.
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
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("  +%d more", more)))
	}
	return lines
}

// fitWindow picks the entries [start, end) that fit in avail rows, counting a
// blank separator between entries and scrolling so sel (if >= 0) stays
// visible. more is the count left out after reserving a "+N more" row.
// Shared by the board columns and the archive list.
func fitWindow(heights []int, avail, sel int) (start, end, more int) {
	if avail < 1 {
		avail = 1
	}
	n := len(heights)

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

// renderCard draws one card with compact relationship context.
func (m BoardModel) renderCard(colIdx, itemIdx int, t task.Task, width int) string {
	inner := width - 4
	selected := colIdx == m.col && itemIdx == m.sel[m.col]
	grabbed := selected && m.mode == modeMove && t.ID == m.grabID

	// The countdown sits right-aligned on the title row: the one thing on a
	// card allowed to be loud.
	// The priority marker leads the title; an agent's claim sits beside the
	// countdown, shortened before the title gives up its last 8 cells.
	prefix := ""
	if grabbed {
		prefix = "⇄ "
	}
	if mark := priorityMarker(t.Priority); mark != "" {
		prefix += mark + " "
	}
	right := Countdown(t, m.now(), m.board.DoneStatus())
	room := inner - lipgloss.Width(prefix)
	if right != "" {
		room -= lipgloss.Width(right) + 1
	}
	if claim := cardClaim(t, m.now(), room-8-1); claim != "" {
		room -= lipgloss.Width(claim) + 1
		if right != "" {
			claim += " "
		}
		right = claim + right
	}
	title := prefix + truncate(t.Title, max(room, 1))
	if right != "" {
		title += strings.Repeat(" ", max(inner-lipgloss.Width(title)-lipgloss.Width(right), 1)) + right
	}

	lines := []string{title}
	if t.ParentID != "" {
		if parent, ok := m.board.TaskByID(t.ParentID); ok {
			lines = append(lines, MutedStyle.Render(truncate("↳ "+parent.Title, inner)))
		}
	} else if done, total := m.board.ChildProgress(t.ID); total > 0 {
		lines = append(lines, progressBar(done, total)+MutedStyle.Render(fmt.Sprintf(" %d of %d subtasks", done, total)))
	}
	if t.Description != "" {
		lines = append(lines, descLine(t.Description, inner))
	}
	if t.Worktree != "" {
		lines = append(lines, MutedStyle.Render(truncate("⎇ "+filepath.Base(t.Worktree), inner)))
	}
	body := strings.Join(lines, "\n")

	switch {
	case grabbed:
		return CardGrabbedStyle.Render(body)
	case selected && m.focus == focusItem:
		return CardSelectedStyle.Render(body)
	case m.inSelectedFamily(t):
		return CardRelatedStyle.Render(body)
	default:
		return CardStyle.Render(body)
	}
}

// inSelectedFamily reports whether t shares a family with the selected card
// (and is not that card), so related cards can be marked across columns.
func (m BoardModel) inSelectedFamily(t task.Task) bool {
	if m.mode != modeNormal || m.focus != focusItem {
		return false
	}
	sel, ok := m.selectedTask()
	if !ok || sel.ID == t.ID {
		return false
	}
	root := m.board.FamilyRootID(sel)
	return m.board.FamilyRootID(t) == root && (t.ParentID != "" || sel.ParentID != "")
}

// progressBar draws subtask completion: one cell per subtask up to five,
// scaled beyond that, green for done.
func progressBar(done, total int) string {
	cells, filled := total, done
	if total > 5 {
		cells, filled = 5, done*5/total
	}
	return lipgloss.NewStyle().Foreground(colDone).Render(strings.Repeat("▰", filled)) +
		MutedStyle.Render(strings.Repeat("▱", cells-filled))
}

// descLine is the one-line description shown on cards and archive entries;
// multi-line text collapses to its first line plus an ellipsis.
func descLine(desc string, width int) string {
	first, rest, _ := strings.Cut(desc, "\n")
	if strings.TrimSpace(rest) != "" {
		first += " …"
	}
	return MutedStyle.Render(truncate(first, width))
}

// rowsHeight is the terminal height of the joined rows; empty is zero, where
// lipgloss.Height("") would report one.
func rowsHeight(rows []string) int {
	if len(rows) == 0 {
		return 0
	}
	return lipgloss.Height(strings.Join(rows, "\n"))
}

// detailBudget is the content rows the detail panel may use: the terminal
// minus the tab row and the panel's two border rows. Unbounded until sized.
func (m BoardModel) detailBudget() int {
	if m.height <= 0 {
		return 1 << 30
	}
	return m.height - 3
}

// detailMaxBodyRows caps the scrolling body even on a tall terminal.
const detailMaxBodyRows = 15
const detailMinBodyRows = 10

// detailLayout splits the detail popup into fixed parts (head, props) and the
// body (subtasks, description), which scrolls once it outgrows window rows.
type detailLayout struct {
	head, props, body []string
	window            int
}

func (m BoardModel) detailLayout(t task.Task, w int) detailLayout {
	l := detailLayout{head: m.detailHead(t, w), props: m.detailProps(t, w), body: m.detailBodyLines(t, w)}
	// Budget the scrolling footer: with its counter it can wrap a row taller.
	foot := rowsHeight(m.detailFoot(t, "999–999 / 999", w))
	// On a very short terminal the fixed props give way before the frame.
	if room := max(m.detailBudget()-rowsHeight(l.head)-foot, 0); len(l.props) > room {
		l.props = l.props[:room]
	}
	avail := m.detailBudget() - rowsHeight(l.head) - rowsHeight(l.props) - foot
	if len(l.props) > 0 {
		avail-- // blank row between props and body
	}
	if l.window = min(max(len(l.body), detailMinBodyRows), detailMaxBodyRows, avail); l.window < 2 {
		l.window = 0 // not even a label and one row: drop the body
	}
	return l
}

// renderDetail is the centred popup for the selected task, laid out like the
// form: label rows with a hint on the right, indented values, one blank row
// between sections.
func (m BoardModel) renderDetail() string {
	t, ok := m.selectedTask()
	if !ok {
		return ""
	}
	w := m.popupWidth()
	cw := max(w-2, 1) // ColumnStyle's padding comes out of Width
	l := m.detailLayout(t, cw)

	rows := append(append([]string{}, l.head...), l.props...)
	counter := ""
	if l.window > 0 {
		start := min(max(m.detailScroll, 0), max(len(l.body)-l.window, 0))
		if l.window < len(l.body) {
			counter = fmt.Sprintf("%d–%d / %d", start+1, start+l.window, len(l.body))
		}
		if len(l.props) > 0 {
			rows = append(rows, "")
		}
		end := min(start+l.window, len(l.body))
		rows = append(rows, l.body[start:end]...)
		for i := end - start; i < l.window; i++ {
			rows = append(rows, "")
		}
	}
	rows = append(rows, m.detailFoot(t, counter, cw)...)
	return ColumnStyle.Copy().Width(w).Render(strings.Join(rows, "\n"))
}

// detailLabel is a read-only section label, styled like an unfocused form
// label so the two popups read as one design.
func detailLabel(label, hint string, width int) string {
	return formLabel(-1, 0, label, hint, width)
}

// indent puts a value under its label, aligned with the form's input text.
func indent(s string) string { return "  " + s }

// detailHead is the title, wrapped on words, and the status/meta line.
func (m BoardModel) detailHead(t task.Task, width int) []string {
	title := strings.Join(strings.Fields(t.Title), " ")
	return []string{
		lipgloss.NewStyle().Width(width).Bold(true).Foreground(ColText).Render(title),
		lipgloss.NewStyle().MaxWidth(width).Render(detailMetaLine(t, m.now())),
		MutedStyle.Render(strings.Repeat("─", width)),
	}
}

// detailProps are the fixed sections: Deadline and Worktree (side by side
// when both are set, as in the form), then Parent.
func (m BoardModel) detailProps(t task.Task, width int) []string {
	var sections []string
	due := Countdown(t, m.now(), m.board.DoneStatus())
	date := ""
	if t.Deadline != nil {
		date = indent(FormatDate(t.Deadline.In(m.now().Location())))
	}
	switch {
	case t.Deadline != nil && t.Worktree != "":
		dlW, wtW := formDateWidths(width)
		sections = append(sections, lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(dlW).Render(detailLabel("Deadline", due, dlW)+"\n"+date),
			"  ",
			lipgloss.NewStyle().Width(wtW).Render(detailLabel("Worktree", "", wtW)+"\n"+indent(truncate(t.Worktree, wtW-2)))))
	case t.Deadline != nil:
		sections = append(sections, detailLabel("Deadline", due, width)+"\n"+date)
	case t.Worktree != "":
		sections = append(sections, detailLabel("Worktree", "", width)+"\n"+indent(truncate(t.Worktree, width-2)))
	}
	if t.ParentID != "" {
		if parent, ok := m.board.TaskByID(t.ParentID); ok {
			id := MutedStyle.Render("  " + shortTaskID(parent.ID))
			sections = append(sections, detailLabel("Parent", statusWord(parent.Status), width)+"\n"+
				indent(truncate(parent.Title, max(width-2-lipgloss.Width(id), 1))+id))
		} else {
			sections = append(sections, detailLabel("Parent", "missing", width)+"\n"+indent(MutedStyle.Render(shortTaskID(t.ParentID))))
		}
	}
	if agent := m.agentProps(t, width); len(agent) > 0 {
		sections = append(sections, strings.Join(agent, "\n"))
	}
	var rows []string
	for i, sec := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, strings.Split(sec, "\n")...)
	}
	return rows
}

// detailFoot is the popup's hint footer, led by any error. A non-empty
// counter means the body scrolls: it adds the j/k hint and the window
// position. o and y appear only when they would do something.
func (m BoardModel) detailFoot(t task.Task, counter string, width int) []string {
	var hints []string
	if m.err != "" {
		hints = append(hints, errorLine(m.err))
	}
	if counter != "" {
		hints = append(hints, keyHint("j/k", "Scroll"))
	}
	if m.awaitingApproval(t) {
		hints = append(hints, keyHint("y", "Approve"))
	}
	if len(taskDocuments(t)) > 0 {
		hints = append(hints, keyHint("o", "Open"))
	}
	hints = append(hints, keyHint("s", "Subtask"), keyHint("e", "Edit"), keyHint("d", "Delete"), keyHint("esc", "Close"))
	if counter != "" {
		hints = append(hints, MutedStyle.Render(counter))
	}
	return popupFoot(width, hints...)
}

// popupFoot is the rule and hint bar closing every centred panel.
func popupFoot(width int, hints ...string) []string {
	return []string{MutedStyle.Render(strings.Repeat("─", width)), hintBar(width, hints...)}
}

// detailMetaLine is the line under the title: status, ages and moves.
// Zero-value parts are left out, so a fresh card reads `TODO  created 0m ago`.
func detailMetaLine(t task.Task, now time.Time) string {
	parts := []string{shortTaskID(t.ID)}
	if age := now.Sub(t.CreatedAt); age >= 0 {
		parts = append(parts, "created "+ago(age))
	}
	if t.UpdatedAt.After(t.CreatedAt.Add(time.Minute)) {
		if age := now.Sub(t.UpdatedAt); age >= 0 {
			parts = append(parts, "updated "+ago(age))
		}
	}
	if n := len(t.History); n == 1 {
		parts = append(parts, "1 move")
	} else if n > 1 {
		parts = append(parts, fmt.Sprintf("%d moves", n))
	}
	meta := ""
	if len(parts) > 0 {
		meta = MutedStyle.Render("   " + strings.Join(parts, "   "))
	}
	return HeaderStyle(t.Status).Render(statusWord(t.Status)) + meta
}

// ago is an age at reading precision: whole days once past a day.
func ago(d time.Duration) string {
	if d >= 24*time.Hour {
		d = d.Truncate(24 * time.Hour)
	}
	return FormatDuration(d) + " ago"
}

// statusWord is a column name as it reads inside a sentence: "testing/review".
func statusWord(s task.Status) string { return strings.ToLower(s.Label()) }

func matchCount(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

// detailBodyLines is the scrollable part: the Subtasks checklist, then the
// Description, each under a form-style label. Empty when there is neither.
func (m BoardModel) detailBodyLines(t task.Task, w int) []string {
	var rows []string
	if children := m.board.Children(t.ID); len(children) > 0 {
		done, total := m.board.ChildProgress(t.ID)
		rows = append(rows, detailLabel("Subtasks", fmt.Sprintf("%d of %d complete", done, total), w))
		for _, child := range children {
			rows = append(rows, indent(m.childDetailLine(child, w-2)))
		}
	}
	if t.Description != "" {
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, detailLabel("Description", "", w))
		for _, line := range strings.Split(renderMarkdown(t.Description, w-2), "\n") {
			rows = append(rows, indent(line))
		}
	}
	for _, section := range []struct {
		label string
		paths []string
	}{{"Attachments", t.Attachments}, {"Working notes", t.WorkingNotes}} {
		if len(section.paths) == 0 {
			continue
		}
		if len(rows) > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, detailLabel(section.label, "", w))
		for _, path := range section.paths {
			for _, line := range strings.Split(lipgloss.NewStyle().Width(max(w-2, 1)).Render(path), "\n") {
				rows = append(rows, indent(line))
			}
		}
	}
	return rows
}

// Document paths use CSV quoting, allowing commas and spaces in file names.
func formatDocumentPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	var out strings.Builder
	writer := csv.NewWriter(&out)
	_ = writer.Write(paths)
	writer.Flush()
	return strings.TrimSuffix(out.String(), "\n")
}

func parseDocumentPaths(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	r := csv.NewReader(strings.NewReader(value))
	r.TrimLeadingSpace = true
	return r.Read()
}

// childDetailLine is one checklist row: a tick or circle, the title, and
// the subtask's column on the right.
func (m BoardModel) childDetailLine(child task.Task, w int) string {
	mark := lipgloss.NewStyle().Foreground(AccentFor(child.Status)).Render("○")
	if child.Status == m.board.DoneStatus() {
		mark = lipgloss.NewStyle().Foreground(colDone).Render("✓")
	}
	status := statusWord(child.Status)
	if child.Archived {
		status += ", archived"
	}
	right := MutedStyle.Render(status)
	title := truncate(child.Title, max(w-lipgloss.Width(right)-4, 1))
	gap := max(w-2-lipgloss.Width(title)-lipgloss.Width(right), 1) // 2 = mark and space
	return mark + " " + title + strings.Repeat(" ", gap) + right
}

// parseDeadline reads a DD/MM/YYYY date (FormatDate's layout); blank means no
// deadline. Parsed in local time: UTC would shift the calendar day back west
// of UTC and make today's deadline read as overdue.
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

func shortTaskID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// openForm puts the model into the add/edit form, seeded with the given
// values and focused on the title.
func (m *BoardModel) openForm(editID, title, desc string, deadline *time.Time, worktree, parentID string) {
	m.mode = modeInput
	m.editID = editID
	m.field = fieldTitle
	m.err = ""

	dl := ""
	if deadline != nil {
		dl = FormatDate(*deadline)
	}
	m.title.SetValue(title)
	m.desc.SetValue(desc)
	m.deadline.SetValue(dl)
	m.worktree.SetValue(worktree)
	m.attachments.SetValue("")
	m.workingNotes.SetValue("")
	if t, ok := m.board.TaskByID(editID); ok {
		m.attachments.SetValue(formatDocumentPaths(t.Attachments))
		m.workingNotes.SetValue(formatDocumentPaths(t.WorkingNotes))
	}
	m.formParentID = parentID
	m.parentIndex = 0
	m.parentChoosing = false
	parentText := ""
	if parent, ok := m.board.TaskByID(parentID); ok {
		parentText = parent.Title
	}
	m.parent.SetValue(parentText)
	m.applyFieldFocus() // sizes the fields too
}

// openSubtaskForm opens a new task under t's family root, starting in the
// root's worktree since subtasks usually share it.
func (m *BoardModel) openSubtaskForm(t task.Task) {
	rootID := m.board.FamilyRootID(t)
	root, _ := m.board.TaskByID(rootID)
	m.openForm("", "", "", nil, root.Worktree, rootID)
}

// closeForm returns to normal mode and drops focus from every field.
func (m *BoardModel) closeForm() {
	m.mode = modeNormal
	m.editID = ""
	m.err = ""
	m.picker = datePicker{}
	m.title.Blur()
	m.desc.Blur()
	m.deadline.Blur()
	m.worktree.Blur()
	m.parent.Blur()
	m.attachments.Blur()
	m.workingNotes.Blur()
	m.formParentID = ""
	m.parent.SetValue("")
	m.parentIndex = 0
	m.parentChoosing = false
}

// applyFieldFocus focuses m.field's widget, blurs the rest, and opens the
// calendar only on the Deadline field.
func (m *BoardModel) applyFieldFocus() {
	m.title.Blur()
	m.desc.Blur()
	m.deadline.Blur()
	m.worktree.Blur()
	m.parent.Blur()
	m.attachments.Blur()
	m.workingNotes.Blur()
	switch m.field {
	case fieldTitle:
		m.title.Focus()
		m.title.CursorEnd()
	case fieldDesc:
		m.desc.Focus()
	case fieldDeadline:
		m.deadline.Focus()
		m.deadline.CursorEnd()
	case fieldWorktree:
		m.worktree.Focus()
		m.worktree.CursorEnd()
	case fieldParent:
		m.parent.Focus()
		m.parent.CursorEnd()
		m.parentIndex = 0
	case fieldAttachments:
		m.attachments.Focus()
		m.attachments.CursorEnd()
	case fieldWorkingNotes:
		m.workingNotes.Focus()
		m.workingNotes.CursorEnd()
	}
	if m.field == fieldDeadline {
		m.seedPicker()
	} else {
		m.picker = datePicker{}
	}
	m.sizeForm() // the calendar's rows come out of the description box
}

// focusField moves form focus by delta, wrapping at both ends.
func (m *BoardModel) focusField(delta int) {
	m.field = (m.field + delta + fieldCount) % fieldCount
	m.applyFieldFocus()
}

// formRelationships is the read-only links block under the Parent selector:
// a new subtask names its parent, an edited parent lists its subtasks.
func (m BoardModel) formRelationships(width int) []string {
	if width < 1 {
		width = 1
	}
	if m.editID == "" {
		if m.formParentID == "" {
			return nil
		}
		parent, ok := m.board.TaskByID(m.formParentID)
		if !ok {
			return nil
		}
		return []string{"", MutedStyle.Render("Subtask of  ") + truncate(parent.Title+"  "+shortTaskID(parent.ID), width-12)}
	}
	t, ok := m.board.TaskByID(m.editID)
	if !ok {
		return nil
	}
	children := m.board.Children(t.ID)
	if len(children) == 0 {
		return nil
	}
	done, total := m.board.ChildProgress(t.ID)
	rows := []string{"", MutedStyle.Copy().Bold(true).Render(formatSubtaskHeader(done, total))}
	for i, child := range children {
		if i >= 3 {
			rows = append(rows, MutedStyle.Render(truncate(fmt.Sprintf("  +%d more", len(children)-3), width)))
			break
		}
		rows = append(rows, MutedStyle.Render(truncate("  • "+child.Title, width)))
	}
	return rows
}

func formatSubtaskHeader(done, total int) string {
	return fmt.Sprintf("Subtasks  %d of %d", done, total)
}

// parentOptions returns assignable top-level tasks in board order. Done tasks
// are excluded except the current parent, so an edit keeps a done family.
func (m BoardModel) parentOptions() []task.Task {
	if m.editID != "" && len(m.board.Children(m.editID)) > 0 {
		return nil // a parent task cannot itself become a subtask
	}
	var options []task.Task
	for _, candidate := range m.board.Active() {
		if candidate.ID == m.editID || candidate.ParentID != "" {
			continue
		}
		if candidate.Status == m.board.DoneStatus() && candidate.ID != m.formParentID {
			continue
		}
		options = append(options, candidate)
	}
	return options
}

const parentDropdownRows = 4

func (m BoardModel) filteredParentOptions() []task.Task {
	query := strings.ToLower(strings.TrimSpace(m.parent.Value()))
	options := m.parentOptions()
	if query == "" {
		return options
	}
	filtered := make([]task.Task, 0, len(options))
	for _, option := range options {
		if strings.Contains(strings.ToLower(option.Title), query) || strings.HasPrefix(strings.ToLower(option.ID), query) {
			filtered = append(filtered, option)
		}
	}
	return filtered
}

func (m *BoardModel) moveParentHighlight(delta int) {
	options := m.filteredParentOptions()
	if len(options) == 0 {
		m.parentIndex = 0
		return
	}
	m.parentIndex = (m.parentIndex + delta + len(options)) % len(options)
	m.parentChoosing = true
}

func (m *BoardModel) chooseHighlightedParent() bool {
	options := m.filteredParentOptions()
	if len(options) == 0 {
		return false
	}
	if m.parentIndex >= len(options) {
		m.parentIndex = len(options) - 1
	}
	chosen := options[m.parentIndex]
	m.formParentID = chosen.ID
	m.parent.SetValue(chosen.Title)
	m.parent.CursorEnd()
	m.parentIndex = 0
	m.parentChoosing = false
	m.sizeForm()
	return true
}

func (m BoardModel) renderParentDropdown(width int) string {
	return m.renderParentDropdownSized(width, m.parentDropdownHeight())
}

// renderParentDropdownSized draws the dropdown in at most height rows.
func (m BoardModel) renderParentDropdownSized(width, height int) string {
	if height == 0 {
		return ""
	}
	options := m.filteredParentOptions()
	index := m.parentIndex
	if index >= len(options) {
		index = max(0, len(options)-1)
	}
	heading := truncate(matchCount(len(options)), width-6)
	lines := []string{MutedStyle.Copy().Bold(true).Render(heading)}
	if len(options) == 0 {
		lines = append(lines, MutedStyle.Render(truncate("No matching parent tasks", width-6)))
	} else {
		visibleRows := parentDropdownRows
		if height >= 4 && visibleRows > height-3 {
			visibleRows = height - 3
		}
		if height < 4 {
			visibleRows = max(1, height-1)
		}
		start := 0
		if index >= visibleRows {
			start = index - visibleRows + 1
		}
		end := start + visibleRows
		if end > len(options) {
			end = len(options)
		}
		for i := start; i < end; i++ {
			prefix := "  "
			style := MutedStyle
			if i == index {
				prefix = "› "
				style = lipgloss.NewStyle().Foreground(ColAccent).Bold(true)
			}
			id := shortTaskID(options[i].ID)
			avail := width - 8 - len(id) // border, padding, "› " and "  "
			if avail < 1 {
				avail = 1
			}
			lines = append(lines, style.Render(prefix+truncate(options[i].Title, avail)+"  "+id))
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	if height < 4 {
		return strings.Join(lines, "\n")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(ColMuted).Padding(0, 1).Width(width - 2).Render(strings.Join(lines, "\n"))
}

// formDateWidths splits a form row between Deadline and Worktree, two apart.
func formDateWidths(width int) (deadline, worktree int) {
	deadline = min(24, width/2-1)
	return deadline, width - deadline - 2
}

// formLabel renders a section name with its hint right-aligned; the focused
// field uses the accent colour.
func formLabel(field, focused int, label, hint string, width int) string {
	style := MutedStyle.Copy().Bold(true)
	if field == focused {
		style = lipgloss.NewStyle().Foreground(ColAccent).Bold(true)
	}
	left := style.Render(label)
	right := MutedStyle.Render(hint)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderFooter is the hint bar: error or state, mode keys, then global keys.
func (m BoardModel) renderFooter() string {
	var items []string
	if m.err != "" {
		items = append(items, errorLine(m.err))
	}
	if m.info != "" {
		items = append(items, noticeLine(m.info))
	}
	switch {
	case m.mode == modeSearch:
		items = append(items, m.search.View(), keyHint("enter", "Keep"), keyHint("esc", "Clear"))
		return HelpStyle.Render(hintBar(m.width, items...))
	case m.query != "" && m.mode == modeNormal:
		items = append(items, statusTag("Search")+" "+MutedStyle.Render(truncate(m.query, 30)),
			keyHint("esc", "Clear"), keyHint("/", "Edit"))
		if m.familyID != "" {
			items = append(items, keyHint("f", "Show all"))
		}
		items = append(items, keyHint("enter", "Open"), keyHint("m", "Grab"))
	case m.mode == modeMove:
		if m.err == "" {
			items = append(items, statusTag("Moving"))
		}
		items = append(items, keyHint("h/l", "Reposition"), keyHint("enter", "Drop"), keyHint("esc", "Cancel"))
		return HelpStyle.Render(hintBar(m.width, items...))
	case m.familyID != "":
		name := shortTaskID(m.familyID)
		if parent, ok := m.board.TaskByID(m.familyID); ok {
			name = parent.Title
		}
		items = append(items, statusTag("Family")+" "+MutedStyle.Render(truncate(name, 30)),
			keyHint("f", "Show all"), keyHint("s", "Subtask"), keyHint("enter", "Open"), keyHint("m", "Grab"))
	default:
		// hjkl and s live in ? help and the detail popup, so the bar still
		// fits one row at 120 columns with undo and search added.
		items = append(items, keyHint("enter", "Open"), keyHint("a", "Add"),
			keyHint("e", "Edit"), keyHint("d", "Delete"), keyHint("u", "Undo"),
			keyHint("/", "Search"), keyHint("m", "Grab"), keyHint("f", "Family"), keyHint("ctrl+t", "Focus"))
	}
	return HelpStyle.Render(hintBar(m.width, append(items, globalHints()...)...))
}

// renderConfirm is the delete prompt; it names the task so a stray d never
// deletes blind.
func (m BoardModel) renderConfirm() string {
	t, ok := m.selectedTask()
	if !ok {
		return ""
	}
	w := m.popupWidth()
	cw := w - 2
	rows := []string{
		ErrorStyle.Render("Delete task"),
		MutedStyle.Render(strings.Repeat("─", cw)),
		lipgloss.NewStyle().Width(cw).Bold(true).Foreground(ColText).Render(t.Title),
		"",
	}
	if n := len(m.board.Children(t.ID)); n > 0 {
		rows = append(rows, MutedStyle.Copy().Width(cw).Render(plural(n, "subtask")+" will stay on the board without a parent."))
	}
	rows = append(rows, MutedStyle.Render("Press u on the board to bring it back."))
	rows = append(rows, popupFoot(cw, keyHint("y", "Delete"), keyHint("n", "Cancel"))...)
	return ColumnStyle.Copy().Width(w).Render(strings.Join(rows, "\n"))
}

// truncate shortens s to n display columns with an ellipsis. It measures
// display width, not runes, since CJK and emoji take two columns.
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

// Update dispatches key messages to the handler for the current mode.
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
	case modeDetail:
		return m.updateDetail(keyMsg)
	case modeSearch:
		return m.updateSearch(keyMsg)
	case modeOpen:
		return m.updateOpen(keyMsg)
	}
	return m.updateNormal(keyMsg)
}

// updateNormal handles navigation and focus in the default mode.
func (m BoardModel) updateNormal(keyMsg tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.err = ""
	m.info = ""

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
			m.sel[m.col] = len(m.visibleByStatus(m.currentStatus())) - 1
		}
	case "enter":
		if _, ok := m.selectedTask(); !ok {
			return m, nil
		}
		m.focus = focusItem
		m.mode = modeDetail
		m.detailScroll = 0
	case "a":
		m.openForm("", "", "", nil, "", "")
	case "s":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.openSubtaskForm(t)
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.openForm(t.ID, t.Title, t.Description, t.Deadline, t.Worktree, t.ParentID)
	case "f":
		if m.familyID != "" {
			selectedID := ""
			if t, ok := m.selectedTask(); ok {
				selectedID = t.ID
			}
			m.familyID = ""
			if !m.selectTaskID(selectedID) {
				m.clampSelection()
			}
			break
		}
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.familyID = m.board.FamilyRootID(t)
		m.selectTaskID(t.ID)
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
		m.grabSnap = m.snapshotTasks(t.ID)
	case "/":
		m.openSearch()
		return m, nil
	case "esc":
		if m.query != "" {
			m.search.SetValue("")
			m.setQuery("")
			return m, nil
		}
	case "u":
		if m.undoLast() {
			return m, dirty()
		}
		return m, nil
	case "A":
		on := !m.board.RequireApproval
		m.board.SetRequireApproval(on)
		if on {
			m.info = "approval on: only you can move cards to " + m.board.DoneStatus().Label()
		} else {
			m.info = "approval off: agents may ship on their own"
		}
		return m, dirty()
	}
	m.clampSelection()
	return m, nil
}

// approve ships a card waiting for review, as the human at the keyboard.
func (m BoardModel) approve(t task.Task) (BoardModel, tea.Cmd) {
	before := m.snapshotTasks(t.ID)
	actor := m.board.Actor
	m.board.Actor = "" // the TUI is always a human, whatever the environment says
	err := m.board.Move(t.ID, m.board.DoneStatus(), m.now())
	m.board.Actor = actor
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.reconcile()
	m.pushUndo(undoEntry{what: "approve " + t.Title, restore: before, selectID: t.ID})
	m.mode = modeNormal
	m.info = "approved: " + t.Title
	if !m.selectTaskID(t.ID) {
		m.clampSelection()
	}
	return m, dirty()
}

// dirtyMsg signals that the board changed and should be persisted.
type dirtyMsg struct{}

func dirty() tea.Cmd { return func() tea.Msg { return dirtyMsg{} } }

func (m BoardModel) updateInput(k tea.KeyMsg) (BoardModel, tea.Cmd) {
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

	case tea.KeyEnter:
		if m.field == fieldParent && m.parentChoosing {
			if m.chooseHighlightedParent() {
				return m, nil
			}
		}
		title := strings.TrimSpace(m.title.Value())
		if title == "" {
			m.err = "title must not be blank"
			m.sizeForm()
			return m, nil
		}
		deadline, err := parseDeadline(m.deadline.Value())
		if err != nil {
			m.err = err.Error()
			m.sizeForm()
			return m, nil
		}
		desc := strings.TrimSpace(m.desc.Value())
		if strings.TrimSpace(m.parent.Value()) != "" && m.formParentID == "" {
			m.err = "choose a parent from the dropdown, or clear the search"
			m.field = fieldParent
			m.applyFieldFocus()
			return m, nil
		}
		parentID := m.formParentID
		worktree := m.worktree.Value()
		attachments, err := parseDocumentPaths(m.attachments.Value())
		if err != nil {
			m.err = "invalid attachments: use CSV-quoted paths"
			m.sizeForm()
			return m, nil
		}
		notes, err := parseDocumentPaths(m.workingNotes.Value())
		if err != nil {
			m.err = "invalid working notes: use CSV-quoted paths"
			m.sizeForm()
			return m, nil
		}

		selectedID := m.editID
		before := m.snapshotTasks(m.editID)
		if m.editID != "" {
			if err := m.board.EditWithParent(m.editID, parentID, title, desc, deadline, m.now()); err != nil {
				m.err = err.Error()
				m.sizeForm()
				return m, nil
			}
		} else {
			if parentID == "" {
				selectedID = m.board.Add(title, desc, deadline, m.now()).ID
			} else {
				added, err := m.board.AddChild(parentID, title, desc, deadline, m.now())
				if err != nil {
					m.err = err.Error()
					m.sizeForm()
					return m, nil
				}
				selectedID = added.ID
			}
		}
		if err := m.board.SetWorktree(selectedID, worktree, m.now()); err != nil {
			m.err = err.Error()
			m.sizeForm()
			return m, nil
		}
		if err := m.board.SetDocuments(selectedID, attachments, notes, m.now()); err != nil {
			m.err = err.Error()
			m.sizeForm()
			return m, nil
		}
		if m.editID != "" {
			m.pushUndo(undoEntry{what: "edit " + title, restore: before, selectID: selectedID})
		} else {
			m.pushUndo(undoEntry{what: "add " + title, removeID: selectedID})
		}
		if m.familyID != "" {
			if saved, ok := m.board.TaskByID(selectedID); ok {
				m.familyID = m.board.FamilyRootID(saved)
			}
		}
		m.closeForm()
		if !m.selectTaskID(selectedID) {
			m.clampSelection()
		}
		return m, dirty()
	}

	if m.field == fieldParent {
		switch k.String() {
		case "up", "ctrl+p":
			m.moveParentHighlight(-1)
			return m, nil
		case "down", "ctrl+n":
			m.moveParentHighlight(1)
			return m, nil
		}
		before := m.parent.Value()
		var cmd tea.Cmd
		m.parent, cmd = m.parent.Update(k)
		if m.parent.Value() != before {
			m.formParentID = ""
			m.parentIndex = 0
			m.parentChoosing = strings.TrimSpace(m.parent.Value()) != ""
			m.sizeForm()
		}
		m.err = ""
		return m, cmd
	}

	// On the Deadline field hjkl and t steer the calendar; those letters are
	// never valid in a DD/MM/YYYY date, so both can share the keyboard.
	if m.field == fieldDeadline {
		moved := true
		switch k.String() {
		case "h":
			m.picker = m.picker.move(-1)
		case "l":
			m.picker = m.picker.move(1)
		case "k":
			m.picker = m.picker.move(-7)
		case "j":
			m.picker = m.picker.move(7)
		case "t":
			m.picker = newDatePicker(m.now())
		default:
			moved = false
		}
		if moved {
			m.deadline.SetValue(FormatDate(m.picker.cursor))
			m.deadline.CursorEnd()
			m.err = ""
			m.sizeForm() // a month with six week rows makes the calendar taller
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.field {
	case fieldTitle:
		m.title, cmd = m.title.Update(k)
	case fieldDesc:
		m.desc, cmd = m.desc.Update(k)
		m.sizeForm() // the editor grows with its content
	case fieldWorktree:
		m.worktree, cmd = m.worktree.Update(k)
	case fieldAttachments:
		m.attachments, cmd = m.attachments.Update(k)
	case fieldWorkingNotes:
		m.workingNotes, cmd = m.workingNotes.Update(k)
	case fieldDeadline:
		m.deadline, cmd = m.deadline.Update(k)
		m.seedPicker()
		m.sizeForm()
	}
	return m, cmd
}

// updateDetail handles the detail popup: j/k/g/G scroll, and s/e/d act as on
// the board.
func (m BoardModel) updateDetail(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.err = ""
	switch k.String() {
	case "o":
		if t, ok := m.selectedTask(); ok {
			return m, m.startOpen(t)
		}
	case "y":
		if t, ok := m.selectedTask(); ok && m.awaitingApproval(t) {
			return m.approve(t)
		}
	case "esc", "enter", "q":
		m.mode = modeNormal
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			m.mode = modeNormal
			return m, nil
		}
		m.openForm(t.ID, t.Title, t.Description, t.Deadline, t.Worktree, t.ParentID)
	case "s":
		t, ok := m.selectedTask()
		if !ok {
			m.mode = modeNormal
			return m, nil
		}
		m.openSubtaskForm(t)
	case "d":
		if _, ok := m.selectedTask(); !ok {
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeConfirm
	case "j", "down":
		m.detailScroll = m.clampDetailScroll(m.detailScroll + 1)
	case "k", "up":
		m.detailScroll = m.clampDetailScroll(m.detailScroll - 1)
	case "g":
		m.detailScroll = 0
	case "G":
		m.detailScroll = 1 << 30 // clamped to the last window below
		m.detailScroll = m.clampDetailScroll(m.detailScroll)
	}
	return m, nil
}

// clampDetailScroll keeps the detail body offset within [0, last window].
func (m BoardModel) clampDetailScroll(offset int) int {
	t, ok := m.selectedTask()
	if !ok {
		return 0
	}
	l := m.detailLayout(t, max(m.popupWidth()-2, 1))
	return min(max(offset, 0), max(len(l.body)-l.window, 0))
}

// seedPicker points the calendar at the Deadline field's date, or today when
// empty. A half-typed date leaves the cursor alone so it doesn't jump while
// typing. parseDeadline returns (nil, nil) for empty, hence the text check.
func (m *BoardModel) seedPicker() {
	text := strings.TrimSpace(m.deadline.Value())
	d, err := parseDeadline(text)
	switch {
	case err == nil && d != nil:
		m.picker = newDatePicker(*d)
	case text == "" || !m.picker.open:
		m.picker = newDatePicker(m.now())
	}
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
		if t, ok := m.board.TaskByID(m.grabID); ok && len(m.grabSnap) > 0 && t.Status != m.grabSnap[0].Status {
			m.pushUndo(undoEntry{what: "move " + t.Title, restore: m.grabSnap, selectID: t.ID})
		}
		m.mode = modeNormal
		m.grabID = ""
		m.grabSnap = nil
		m.reconcile()
		m.clampSelection()
		return m, dirty()
	case "esc":
		if err := m.board.Move(m.grabID, m.grabFrom, m.now()); err != nil {
			m.err = err.Error()
		}
		m.col = m.board.ColumnIndex(m.grabFrom)
		m.mode = modeNormal
		m.grabID = ""
		m.grabSnap = nil
		m.clampSelection()
		return m, dirty()
	}
	return m, nil
}

// shiftGrabbed moves the grabbed task one column over and follows it,
// reporting whether it moved so a no-op press skips the save.
func (m *BoardModel) shiftGrabbed(delta int) bool {
	next := m.col + delta
	if next < 0 || next >= len(m.board.Statuses()) {
		return false
	}
	if err := m.board.Move(m.grabID, m.board.Statuses()[next], m.now()); err != nil {
		m.err = err.Error()
		return false
	}
	m.col = next
	items := m.visibleByStatus(m.board.Statuses()[next])
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
	ids := []string{t.ID}
	for _, c := range m.board.Children(t.ID) {
		ids = append(ids, c.ID)
	}
	before := m.snapshotTasks(ids...)
	if err := m.board.Delete(t.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.pushUndo(undoEntry{what: "delete " + t.Title, restore: before, selectID: t.ID})
	if m.familyID == t.ID {
		m.familyID = ""
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
	if next >= len(m.board.Statuses()) {
		next = len(m.board.Statuses()) - 1
	}
	m.col = next
}

// moveItem shifts the cursor within the focused column, clamped at both ends.
func (m *BoardModel) moveItem(delta int) {
	n := len(m.visibleByStatus(m.currentStatus()))
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

// reconcile applies the dependency and claim rules after a dropped move:
// dependants of a shipped card unblock, and its claim is released. Cards it
// moves are named in the footer.
func (m *BoardModel) reconcile() {
	moved := m.board.Reconcile(m.now())
	if len(moved) == 0 {
		return
	}
	var names []string
	for _, id := range moved {
		if t, ok := m.board.TaskByID(id); ok {
			names = append(names, shortTaskID(t.ID)+" → "+statusWord(t.Status))
		}
	}
	m.info = "dependencies: " + strings.Join(names, ", ")
}

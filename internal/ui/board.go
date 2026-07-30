package ui

import (
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

// BoardModel is the kanban page.
type BoardModel struct {
	board *task.Board

	col   int    // focused column, index into task.Statuses
	sel   [4]int // selected item per column
	focus focusMode
	mode  boardMode

	input    textinput.Model
	editID   string      // set while editing an existing task
	grabID   string      // set while in modeMove
	grabFrom task.Status // original column of the grabbed task

	now func() time.Time // injectable clock; tests pin it

	width  int
	height int
	err    string
}

// NewBoardModel wires a board into a fresh page model.
func NewBoardModel(b *task.Board) BoardModel {
	in := textinput.New()
	in.Placeholder = "task title"
	in.CharLimit = 200
	in.Prompt = "› "
	return BoardModel{board: b, focus: focusItem, mode: modeNormal, input: in, now: time.Now}
}

// Now returns the model's injectable clock.
func (m *BoardModel) Now() time.Time { return m.now() }

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

// columnWidth splits the terminal evenly across the four columns, leaving
// room for each column's border and padding.
func (m BoardModel) columnWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	per := w/len(task.Statuses) - 2
	if per < 12 {
		per = 12
	}
	return per
}

func (m BoardModel) columnHeight() int {
	h := m.height - 6 // header + footer + borders
	if h < 5 {
		h = 5
	}
	return h
}

// View renders the four columns side by side plus the footer line.
func (m BoardModel) View() string {
	cw := m.columnWidth()
	cols := make([]string, 0, len(task.Statuses))
	for i, s := range task.Statuses {
		cols = append(cols, m.renderColumn(i, s, cw))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderFooter())
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
	for i, t := range items {
		lines = append(lines, m.renderCard(idx, i, t, width))
	}

	style := ColumnStyle
	if idx == m.col {
		style = ColumnFocusedStyle
		if m.focus == focusColumn {
			style = style.Copy().BorderForeground(AccentFor(s))
		}
	}
	return style.Width(width).Height(m.columnHeight()).
		Render(strings.Join(lines, "\n"))
}

func (m BoardModel) renderCard(colIdx, itemIdx int, t task.Task, width int) string {
	title := truncate(t.Title, width-4)
	selected := colIdx == m.col && itemIdx == m.sel[m.col]

	switch {
	case selected && m.mode == modeMove && t.ID == m.grabID:
		return CardGrabbedStyle.Render("⇄ " + title)
	case selected && m.focus == focusItem:
		return CardSelectedStyle.Render(title)
	default:
		return CardStyle.Render(title)
	}
}

func (m BoardModel) renderFooter() string {
	switch m.mode {
	case modeInput:
		return "\n" + m.input.View()
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

func truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// Update dispatches on the current mode. The non-Normal branches are stubs
// until the next task fills them in.
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
		m.mode = modeInput
		m.editID = ""
		m.input.SetValue("")
		m.input.Focus()
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.mode = modeInput
		m.editID = t.ID
		m.input.SetValue(t.Title)
		m.input.CursorEnd()
		m.input.Focus()
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
	switch k.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		m.editID = ""
		m.input.Blur()
		return m, nil

	case tea.KeyEnter:
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			m.err = "title must not be blank"
			return m, nil // stay in input mode
		}
		if m.editID != "" {
			if err := m.board.Edit(m.editID, title, m.now()); err != nil {
				m.err = err.Error()
			}
		} else {
			m.board.Add(title, m.now())
			m.col = 0
			m.sel[0] = len(m.board.ByStatus(task.StatusTodo)) - 1
		}
		m.mode = modeNormal
		m.editID = ""
		m.input.Blur()
		m.clampSelection()
		return m, dirty()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

func (m BoardModel) updateMove(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.String() {
	case "h", "left":
		m.shiftGrabbed(-1)
	case "l", "right":
		m.shiftGrabbed(1)
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

// shiftGrabbed moves the grabbed task one column over and follows it.
func (m *BoardModel) shiftGrabbed(delta int) {
	next := m.col + delta
	if next < 0 || next >= len(task.Statuses) {
		return
	}
	if err := m.board.Move(m.grabID, task.Statuses[next], m.now()); err != nil {
		m.err = err.Error()
		return
	}
	m.col = next
	m.sel[m.col] = len(m.board.ByStatus(task.Statuses[next])) - 1
	m.clampSelection()
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

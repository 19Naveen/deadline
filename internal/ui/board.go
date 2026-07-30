package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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
	return BoardModel{board: b, focus: focusItem, mode: modeNormal, input: in}
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

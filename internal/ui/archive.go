package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// ArchiveModel is the third page: done tasks that have aged off the board.
// It is view-only — nothing here mutates the board.
type ArchiveModel struct {
	board *task.Board
	now   func() time.Time

	sel    int
	width  int
	height int
}

// NewArchiveModel wires a board into the archive page.
func NewArchiveModel(b *task.Board) ArchiveModel {
	return ArchiveModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *ArchiveModel) SetSize(w, h int) { m.width, m.height = w, h }

// Update handles selection movement. The archive is read-only, so no key
// here changes any task.
func (m ArchiveModel) Update(msg tea.Msg) (ArchiveModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	n := len(m.board.ArchivedTasks())
	switch k.String() {
	case "j", "down":
		m.sel++
	case "k", "up":
		m.sel--
	case "g":
		m.sel = 0
	case "G":
		m.sel = n - 1
	}
	if m.sel >= n {
		m.sel = n - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	return m, nil
}

// View renders the archive list.
func (m ArchiveModel) View() string {
	items := m.board.ArchivedTasks()
	width := m.width
	if width <= 0 {
		width = 80
	}

	head := TitleStyle.Render("ARCHIVE") +
		MutedStyle.Render(" ("+strconv.Itoa(len(items))+")")
	lines := []string{head, ""}

	if len(items) == 0 {
		lines = append(lines,
			MutedStyle.Render("nothing archived yet"),
			"",
			MutedStyle.Render(fmt.Sprintf(
				"done tasks move here %d days after you finish them",
				int(task.ArchiveAfter.Hours()/24))))
		return strings.Join(lines, "\n")
	}

	for i, t := range items {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderEntry(i, t, width))
	}
	lines = append(lines, "",
		MutedStyle.Render("j/k move · tab back to the board · read-only"))
	return strings.Join(lines, "\n")
}

func (m ArchiveModel) renderEntry(i int, t task.Task, width int) string {
	inner := width - 6

	rows := []string{truncate(t.Title, inner)}
	if t.Description != "" {
		rows = append(rows, MutedStyle.Render(truncate(t.Description, inner)))
	}

	meta := MutedStyle.Render("archived " + FormatDate(archivedOn(t)))
	if dl := RenderDeadline(t, m.now()); dl != "" {
		meta = dl + MutedStyle.Render("  ·  archived "+FormatDate(archivedOn(t)))
	}
	rows = append(rows, meta)

	body := strings.Join(rows, "\n")
	if i == m.sel {
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColAccent).
			PaddingLeft(1).Bold(true).Render(body)
	}
	return CardStyle.Render(body)
}

// archivedOn falls back to UpdatedAt for entries archived by an older build
// that did not stamp ArchivedAt.
func archivedOn(t task.Task) time.Time {
	if t.ArchivedAt != nil {
		return *t.ArchivedAt
	}
	return t.UpdatedAt
}

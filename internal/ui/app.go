package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

type page int

const (
	pageBoard page = iota
	pageAnalytics
)

// AppModel is the root Bubble Tea model: it owns page switching, the help
// overlay, and persistence.
type AppModel struct {
	board     BoardModel
	analytics AnalyticsModel
	store     *task.Board

	page     page
	showHelp bool
	saveErr  string

	width  int
	height int
}

// NewApp builds the root model around a loaded board.
func NewApp(b *task.Board) AppModel {
	return AppModel{
		board:     NewBoardModel(b),
		analytics: NewAnalyticsModel(b),
		store:     b,
	}
}

// Init satisfies tea.Model; nothing to do at startup.
func (m AppModel) Init() tea.Cmd { return nil }

// Update routes global keys itself and forwards the rest to the active page.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.board.SetSize(msg.Width, msg.Height)
		m.analytics.SetSize(msg.Width, msg.Height)
		return m, nil

	case dirtyMsg:
		if err := m.store.Save(); err != nil {
			m.saveErr = err.Error()
		} else {
			m.saveErr = ""
		}
		return m, nil

	case tea.KeyMsg:
		// While the board is capturing text, only ctrl+c is global —
		// everything else, including q and tab, belongs to the input.
		typing := m.page == pageBoard && m.board.mode == modeInput
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if !typing {
			switch msg.String() {
			case "tab":
				if m.page == pageBoard {
					m.page = pageAnalytics
				} else {
					m.page = pageBoard
				}
				return m, nil
			case "?":
				m.showHelp = !m.showHelp
				return m, nil
			case "q":
				return m, tea.Quit
			}
			if m.showHelp {
				m.showHelp = false // any other key dismisses the overlay
				return m, nil
			}
		}
		if m.page == pageBoard {
			var cmd tea.Cmd
			m.board, cmd = m.board.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

// View renders the tab bar plus the active page, or the help overlay.
func (m AppModel) View() string {
	if m.showHelp {
		return m.renderHelp()
	}
	body := m.board.View()
	if m.page == pageAnalytics {
		body = m.analytics.View()
	}
	parts := []string{m.renderTabs(), body}
	if m.saveErr != "" {
		parts = append(parts, HelpStyle.Render("save failed: "+m.saveErr))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m AppModel) renderTabs() string {
	active := lipgloss.NewStyle().Bold(true).Foreground(ColAccent).Padding(0, 2)
	inactive := MutedStyle.Copy().Padding(0, 2)

	board, analytics := active.Render("BOARD"), inactive.Render("ANALYTICS")
	if m.page == pageAnalytics {
		board, analytics = inactive.Render("BOARD"), active.Render("ANALYTICS")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, board, analytics,
		MutedStyle.Render("  tab to switch"))
}

func (m AppModel) renderHelp() string {
	rows := [][2]string{
		{"tab", "switch board ↔ analytics"},
		{"ctrl+t", "toggle column / item focus"},
		{"h l", "previous / next column"},
		{"j k", "previous / next task (item focus)"},
		{"g G", "first / last task in column"},
		{"a", "add a task"},
		{"e", "edit the selected task"},
		{"d", "delete the selected task (confirms)"},
		{"m", "grab the task, then h/l to move, enter to drop, esc to cancel"},
		{"?", "toggle this help"},
		{"q", "quit"},
	}
	lines := []string{TitleStyle.Render("KEYS"), ""}
	for _, r := range rows {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(ColAccent).Width(10).Render(r[0])+
				MutedStyle.Render(r[1]))
	}
	lines = append(lines, "", MutedStyle.Render("any key to close"))
	return ColumnStyle.Render(strings.Join(lines, "\n"))
}

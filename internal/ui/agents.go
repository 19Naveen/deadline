package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

const (
	// activityFeedSize is how many recent transitions the feed lists.
	activityFeedSize = 20
	// agentTaskRows caps the task lines listed under each agent.
	agentTaskRows = 3
	// agentWeek is the window for the per-agent move count.
	agentWeek = 7 * 24 * time.Hour
)

// AgentsModel is the read-only page of agent activity. It reads the shared
// board on every View, so a live reload shows up with no extra wiring.
type AgentsModel struct {
	board *task.Board
	now   func() time.Time

	width  int
	height int
}

// NewAgentsModel wires a board into the agents page.
func NewAgentsModel(b *task.Board) AgentsModel {
	return AgentsModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *AgentsModel) SetSize(w, h int) { m.width, m.height = w, h }

// agentSummary is one agent's row on the page.
type agentSummary struct {
	name   string
	active []task.Task // claimed or in progress under this agent
	last   time.Time   // latest move, or update to a task it touched
	moves  int         // moves in the last agentWeek
}

// activity is one transition in the feed.
type activity struct {
	task task.Task
	tr   task.Transition
}

// inProgress reports whether s is a working column: neither the first, the
// terminal one, nor blocked.
func inProgress(b *task.Board, s task.Status) bool {
	cols := b.Statuses()
	return s != cols[0] && s != b.DoneStatus() && s != task.StatusBlocked
}

// summarizeAgents collects every agent named in history, claims or
// LastAgent, most recently active first.
func summarizeAgents(b *task.Board, now time.Time) []agentSummary {
	byName := map[string]*agentSummary{}
	get := func(name string) *agentSummary {
		s, ok := byName[name]
		if !ok {
			s = &agentSummary{name: name}
			byName[name] = s
		}
		return s
	}
	bump := func(s *agentSummary, at time.Time) {
		if at.After(s.last) {
			s.last = at
		}
	}
	for _, t := range b.Tasks {
		for _, tr := range t.History {
			if tr.Agent == "" {
				continue
			}
			s := get(tr.Agent)
			bump(s, tr.At)
			if d := now.Sub(tr.At); d >= 0 && d <= agentWeek {
				s.moves++
			}
		}
		claimed := activeClaim(t, now)
		if t.ClaimedBy != "" {
			bump(get(t.ClaimedBy), t.UpdatedAt)
		}
		if t.LastAgent != "" {
			bump(get(t.LastAgent), t.UpdatedAt)
		}
		if t.Archived || t.Status == b.DoneStatus() {
			continue
		}
		switch {
		case claimed:
			s := get(t.ClaimedBy)
			s.active = append(s.active, t)
		case t.LastAgent != "" && inProgress(b, t.Status):
			s := get(t.LastAgent)
			s.active = append(s.active, t)
		}
	}
	out := make([]agentSummary, 0, len(byName))
	for _, s := range byName {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b agentSummary) int {
		if c := b.last.Compare(a.last); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	return out
}

// recentActivity is the newest transitions across all tasks, archived ones
// included, newest first.
func recentActivity(b *task.Board, n int) []activity {
	var all []activity
	for _, t := range b.Tasks {
		for _, tr := range t.History {
			all = append(all, activity{task: t, tr: tr})
		}
	}
	slices.SortStableFunc(all, func(a, b activity) int { return b.tr.At.Compare(a.tr.At) })
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// View renders the agents list above the activity feed.
func (m AgentsModel) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	now := m.now()
	agents := summarizeAgents(m.board, now)
	feed := recentActivity(m.board, activityFeedSize)

	lines := []string{TitleStyle.Render("Agents") + MutedStyle.Render(" ("+strconv.Itoa(len(agents))+")")}
	if len(agents) == 0 {
		lines = append(lines, MutedStyle.Render(truncate("no agent activity yet; agents name themselves with "+task.AgentEnv, width)))
	}
	for _, a := range agents {
		lines = append(lines, m.agentLines(a, now, width)...)
	}

	lines = append(lines, "", TitleStyle.Render("Activity"))
	if len(feed) == 0 {
		lines = append(lines, MutedStyle.Render("no moves yet"))
	}
	for _, f := range feed {
		lines = append(lines, m.feedLine(f, now, width))
	}
	hints := append([]string{MutedStyle.Render("read-only")}, globalHints()...)
	return pageFrame(m.width, m.height, strings.Join(lines, "\n"), hints...)
}

// agentLines is an agent's heading row and its current tasks.
func (m AgentsModel) agentLines(a agentSummary, now time.Time, width int) []string {
	last := "never"
	if !a.last.IsZero() {
		if d := now.Sub(a.last); d >= 0 {
			last = ago(d)
		} else {
			last = "just now"
		}
	}
	stats := MutedStyle.Render(fmt.Sprintf("  %s active   last %s   %s this week",
		strconv.Itoa(len(a.active)), last, plural(a.moves, "move")))
	name := claimStyle.Copy().Bold(true).Render("◆ " + truncate(a.name, max(width-lipgloss.Width(stats)-4, 4)))
	rows := []string{lipgloss.NewStyle().MaxWidth(width).Render("  " + name + stats)}
	for i, t := range a.active {
		if i == agentTaskRows {
			rows = append(rows, MutedStyle.Render(fmt.Sprintf("      +%d more", len(a.active)-i)))
			break
		}
		status := HeaderStyle(t.Status).Render(statusWord(t.Status))
		head := "    " + MutedStyle.Render(shortTaskID(t.ID)) + "  "
		title := truncate(t.Title, max(width-lipgloss.Width(head)-lipgloss.Width(status)-2, 1))
		rows = append(rows, head+title+"  "+status)
	}
	return rows
}

// feedLine is one transition: DD/MM HH:MM, who, short ID, title, from → to.
func (m AgentsModel) feedLine(f activity, now time.Time, width int) string {
	who := "you"
	style := MutedStyle
	if f.tr.Agent != "" {
		who, style = f.tr.Agent, claimStyle
	}
	stamp := MutedStyle.Render(f.tr.At.In(now.Location()).Format("02/01 15:04"))
	whoCell := style.Render(fmt.Sprintf("%-10s", truncate(who, 10)))
	move := lipgloss.NewStyle().Foreground(AccentFor(f.tr.From)).Render(statusWord(f.tr.From)) +
		MutedStyle.Render(" → ") +
		lipgloss.NewStyle().Foreground(AccentFor(f.tr.To)).Render(statusWord(f.tr.To))
	head := "  " + stamp + "  " + whoCell + "  " + MutedStyle.Render(shortTaskID(f.task.ID)) + "  "
	title := truncate(f.task.Title, max(width-lipgloss.Width(head)-lipgloss.Width(move)-2, 1))
	return lipgloss.NewStyle().MaxWidth(width).Render(head + title + "  " + move)
}

package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/stats"
	"gotodo/internal/task"
)

const (
	throughputDays = 14
	heatmapWeeks   = 12
	blockedTopN    = 5
)

// AnalyticsModel is the stats page. It has no keys of its own.
type AnalyticsModel struct {
	board *task.Board
	now   func() time.Time

	width  int
	height int
}

// NewAnalyticsModel wires a board into the analytics page.
func NewAnalyticsModel(b *task.Board) AnalyticsModel {
	return AnalyticsModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *AnalyticsModel) SetSize(w, h int) { m.width, m.height = w, h }

// View renders the whole analytics page.
func (m AnalyticsModel) View() string {
	now := m.now()
	tasks := m.board.Tasks

	sections := []string{
		m.renderTiles(m.board.Active()),
		m.renderThroughput(tasks, now),
		m.renderCycle(tasks, now),
		m.renderBlocked(m.board.Active(), now),
		m.renderStreak(tasks, now),
	}
	return strings.Join(sections, "\n\n")
}

func (m AnalyticsModel) renderTiles(tasks []task.Task) string {
	counts := stats.Counts(tasks)
	tiles := make([]string, 0, len(task.Statuses))
	for _, s := range task.Statuses {
		body := lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(AccentFor(s)).
				Render(fmt.Sprintf("%d", counts[s])),
			MutedStyle.Render(s.Label()),
		)
		tiles = append(tiles, StatTileStyle.Render(body))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tiles...)
}

func (m AnalyticsModel) renderThroughput(tasks []task.Task, now time.Time) string {
	series := stats.Throughput(tasks, throughputDays, now)
	values := make([]int, len(series))
	total := 0
	for i, d := range series {
		values[i] = d.N
		total += d.N
	}

	line := lipgloss.NewStyle().Foreground(AccentFor(task.StatusDone)).
		Render(Sparkline(values))

	span := ""
	if len(series) > 0 {
		span = fmt.Sprintf("%s → %s",
			FormatDate(series[0].Day), FormatDate(series[len(series)-1].Day))
	}

	return strings.Join([]string{
		TitleStyle.Render("THROUGHPUT"),
		line,
		MutedStyle.Render(fmt.Sprintf("%d completed over %d days · %s",
			total, throughputDays, span)),
	}, "\n")
}

func (m AnalyticsModel) renderCycle(tasks []task.Task, now time.Time) string {
	c := stats.CycleTimes(tasks, now)
	lines := []string{TitleStyle.Render("CYCLE TIME")}

	if c.N == 0 {
		return strings.Join(append(lines, MutedStyle.Render("no completed tasks yet")), "\n")
	}

	lines = append(lines, fmt.Sprintf("mean %s · median %s · over %d completed",
		FormatDuration(c.Mean), FormatDuration(c.Median), c.N))
	lines = append(lines, MutedStyle.Render("mean time spent per column:"))

	maxMinutes := 0
	for _, s := range task.Statuses {
		if v := int(c.PerColumn[s].Minutes()); v > maxMinutes {
			maxMinutes = v
		}
	}
	for _, s := range task.Statuses {
		d := c.PerColumn[s]
		bar := HBar(strings.ToLower(s.Label()), int(d.Minutes()), maxMinutes, 24)
		// Replace the raw minute count with a human duration.
		bar = strings.TrimSuffix(bar, fmt.Sprintf(" %d", int(d.Minutes())))
		lines = append(lines, lipgloss.NewStyle().Foreground(AccentFor(s)).Render(bar)+
			" "+MutedStyle.Render(FormatDuration(d)))
	}
	return strings.Join(lines, "\n")
}

func (m AnalyticsModel) renderBlocked(tasks []task.Task, now time.Time) string {
	items := stats.BlockedReport(tasks, now)
	lines := []string{TitleStyle.Render("BLOCKED")}

	if len(items) == 0 {
		return strings.Join(append(lines, MutedStyle.Render("nothing is blocked")), "\n")
	}
	lines = append(lines, fmt.Sprintf("%d currently blocked", len(items)))

	n := len(items)
	if n > blockedTopN {
		n = blockedTopN
	}
	for _, it := range items[:n] {
		lines = append(lines, fmt.Sprintf("  %-40s %s",
			truncate(it.Title, 40),
			lipgloss.NewStyle().Foreground(AccentFor(task.StatusBlocked)).
				Render(FormatDuration(it.For))))
	}
	if len(items) > n {
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("  … and %d more", len(items)-n)))
	}
	return strings.Join(lines, "\n")
}

func (m AnalyticsModel) renderStreak(tasks []task.Task, now time.Time) string {
	current, longest := stats.Streak(tasks, now)
	grid := stats.HeatmapGrid(tasks, heatmapWeeks, now)

	heat := lipgloss.NewStyle().Foreground(AccentFor(task.StatusDone)).Render(Heatmap(grid))
	labels := []string{"Mon", "   ", "Wed", "   ", "Fri", "   ", "Sun"}
	rows := strings.Split(heat, "\n")
	for i := range rows {
		if i < len(labels) {
			rows[i] = MutedStyle.Render(labels[i]) + " " + rows[i]
		}
	}

	return strings.Join([]string{
		TitleStyle.Render("STREAK"),
		fmt.Sprintf("current %d days · longest %d days", current, longest),
		strings.Join(rows, "\n"),
		MutedStyle.Render(fmt.Sprintf("last %d weeks, ending %s", heatmapWeeks, FormatDate(now))),
	}, "\n")
}

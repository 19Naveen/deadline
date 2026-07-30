// Package ui holds the Bubble Tea models and all rendering.
package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// Palette. AdaptiveColor keeps the board legible on light terminals too.
var (
	ColText   = lipgloss.AdaptiveColor{Light: "#1f2430", Dark: "#e6e9ef"}
	ColMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#7a8290"}
	ColBorder = lipgloss.AdaptiveColor{Light: "#c8ccd4", Dark: "#3a4150"}
	ColAccent = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}

	colTodo    = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}
	colDoing   = lipgloss.AdaptiveColor{Light: "#b06f00", Dark: "#e0af68"}
	colBlocked = lipgloss.AdaptiveColor{Light: "#b02a37", Dark: "#f7768e"}
	colDone    = lipgloss.AdaptiveColor{Light: "#2f7a4f", Dark: "#9ece6a"}
)

// AccentFor is the signature colour of a column.
func AccentFor(s task.Status) lipgloss.AdaptiveColor {
	switch s {
	case task.StatusDoing:
		return colDoing
	case task.StatusBlocked:
		return colBlocked
	case task.StatusDone:
		return colDone
	}
	return colTodo
}

var (
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColAccent)
	MutedStyle = lipgloss.NewStyle().Foreground(ColMuted)
	HelpStyle  = lipgloss.NewStyle().Foreground(ColMuted).PaddingTop(1)

	ColumnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColBorder).
			Padding(0, 1)

	ColumnFocusedStyle = ColumnStyle.Copy().BorderForeground(ColAccent)

	CardStyle = lipgloss.NewStyle().
			Foreground(ColText).
			PaddingLeft(1)

	CardSelectedStyle = CardStyle.Copy().
				Bold(true).
				Foreground(ColAccent).
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(ColAccent).
				PaddingLeft(1)

	CardGrabbedStyle = CardSelectedStyle.Copy().
				Foreground(colDoing).
				BorderForeground(colDoing)

	StatTileStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColBorder).
			Padding(0, 2).
			Align(lipgloss.Center)
)

// HeaderStyle is the per-column heading, tinted with that column's accent.
func HeaderStyle(s task.Status) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(AccentFor(s))
}

// FormatDate renders Singapore-style DD/MM/YYYY.
func FormatDate(t time.Time) string { return t.Format("02/01/2006") }

// FormatDuration renders a compact human duration: "3h", "1d 6h", "12m".
func FormatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}

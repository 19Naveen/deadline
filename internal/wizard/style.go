package wizard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette copied from internal/ui/theme.go so the wizard looks like the
// board without importing the ui package.
var (
	colText   = lipgloss.AdaptiveColor{Light: "#1f2430", Dark: "#e6e9ef"}
	colMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#7a8290"}
	colBorder = lipgloss.AdaptiveColor{Light: "#c8ccd4", Dark: "#3a4150"}
	colAccent = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}
	colAmber  = lipgloss.AdaptiveColor{Light: "#b06f00", Dark: "#e0af68"}
	colRed    = lipgloss.AdaptiveColor{Light: "#b02a37", Dark: "#f7768e"}
	colGreen  = lipgloss.AdaptiveColor{Light: "#2f7a4f", Dark: "#9ece6a"}
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	textStyle   = lipgloss.NewStyle().Foreground(colText)
	mutedStyle  = lipgloss.NewStyle().Foreground(colMuted)
	accentStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	amberStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAmber)
	errorStyle  = lipgloss.NewStyle().Bold(true).Foreground(colRed)
	okStyle     = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	headStyle   = lipgloss.NewStyle().Bold(true).Foreground(colMuted)
	// cellStyle marks the field under the cursor on the stages screen.
	cellStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Underline(true)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBorder).
			Padding(0, 1)
)

// cursorBar is the selected-row marker used on the board's cards.
func cursorBar(on bool) string {
	if on {
		return accentStyle.Render("▌ ")
	}
	return "  "
}

// keyHint renders one footer action: an accent key and a muted verb.
func keyHint(key, action string) string {
	return accentStyle.Render(key) + mutedStyle.Render(" "+action)
}

// hintBar joins footer items, wrapping between items at width.
func hintBar(width int, items ...string) string {
	var lines []string
	line := ""
	for _, it := range items {
		if line != "" && lipgloss.Width(line)+2+lipgloss.Width(it) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += it
	}
	return strings.Join(append(lines, line), "\n")
}

func errorLine(msg string) string { return errorStyle.Render("! " + msg) }

// pad fits s to exactly w cells, cutting with an ellipsis when too long.
func pad(s string, w int) string {
	s = trunc(s, w)
	if n := lipgloss.Width(s); n < w {
		s += strings.Repeat(" ", w-n)
	}
	return s
}

// trunc cuts plain text s to at most w cells.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

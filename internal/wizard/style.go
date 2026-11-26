package wizard

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Palette copied from internal/ui/theme.go so the wizard looks like the
// board without importing the ui package. Every state also has a glyph or a
// word, so nothing relies on colour (or a background) alone.
var (
	colText   = lipgloss.AdaptiveColor{Light: "#1f2430", Dark: "#e6e9ef"}
	colMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#7a8290"}
	colBorder = lipgloss.AdaptiveColor{Light: "#c8ccd4", Dark: "#3a4150"}
	colAccent = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}
	colAmber  = lipgloss.AdaptiveColor{Light: "#b06f00", Dark: "#e0af68"}
	colRed    = lipgloss.AdaptiveColor{Light: "#b02a37", Dark: "#f7768e"}
	colGreen  = lipgloss.AdaptiveColor{Light: "#2f7a4f", Dark: "#9ece6a"}
	// colHover is the focused row's background: a faint tint of the panel.
	colHover = lipgloss.AdaptiveColor{Light: "#e8edf5", Dark: "#232a3b"}
	// colOnPill is pill text on a coloured pill.
	colOnPill = lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#1a1b26"}
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	textStyle   = lipgloss.NewStyle().Foreground(colText)
	boldStyle   = lipgloss.NewStyle().Bold(true).Foreground(colText)
	mutedStyle  = lipgloss.NewStyle().Foreground(colMuted)
	accentStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	amberStyle  = lipgloss.NewStyle().Bold(true).Foreground(colAmber)
	errorStyle  = lipgloss.NewStyle().Bold(true).Foreground(colRed)
	okStyle     = lipgloss.NewStyle().Bold(true).Foreground(colGreen)
	headStyle   = lipgloss.NewStyle().Bold(true).Foreground(colMuted)
	ruleStyle   = lipgloss.NewStyle().Foreground(colBorder)
	// cellStyle marks the field under the cursor on the stages screen.
	cellStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Underline(true)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colBorder).
			Padding(0, 2)
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

// section is a small muted caps label above a group of rows.
func section(s string) string { return headStyle.Render(strings.ToUpper(s)) }

// pad fits s to exactly w cells, cutting with an ellipsis when too long.
func pad(s string, w int) string {
	s = trunc(s, w)
	if n := lipgloss.Width(s); n < w {
		s += strings.Repeat(" ", w-n)
	}
	return s
}

// padR pads an already styled string to w cells.
func padR(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
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

// theme holds what changes with NO_COLOR: pills turn into [brackets] and
// the focused row loses its background (the ▌ bar still marks it).
type theme struct{ plain bool }

// pill is a small coloured tag. Muted pills sit on the border colour.
func (t theme) pill(text string, c lipgloss.AdaptiveColor) string {
	if t.plain {
		return "[" + text + "]"
	}
	fg := colOnPill
	if c == colBorder {
		fg = colText
	}
	return lipgloss.NewStyle().Bold(true).Foreground(fg).Background(c).Padding(0, 1).Render(text)
}

// permColor is an access level's tag colour.
func permColor(perm string) lipgloss.AdaptiveColor {
	switch perm {
	case "readonly":
		return colGreen
	case "full":
		return colRed
	}
	return colAccent
}

// check is a checkbox; radio a radio button.
func check(on bool) string {
	if on {
		return accentStyle.Render("[✓]")
	}
	return mutedStyle.Render("[ ]")
}

func radio(on bool) string {
	if on {
		return accentStyle.Render("◉")
	}
	return mutedStyle.Render("○")
}

// row builds one line from styled segments, giving every segment the hover
// background when the row is focused so the tint runs unbroken.
type row struct {
	hover bool
	b     strings.Builder
}

func (t theme) newRow(focused bool) *row {
	r := &row{hover: focused && !t.plain}
	r.b.WriteString(cursorBar(focused))
	return r
}

func (r *row) style(st lipgloss.Style) lipgloss.Style {
	if r.hover {
		return st.Copy().Background(colHover)
	}
	return st
}

// add appends text in style.
func (r *row) add(st lipgloss.Style, s string) *row {
	r.b.WriteString(r.style(st).Render(s))
	return r
}

// cell appends text cut and padded to w cells; only the text is styled, so
// an underline never runs into the padding.
func (r *row) cell(st lipgloss.Style, s string, w int) *row {
	t := trunc(s, w-1)
	r.add(st, t)
	return r.space(w - lipgloss.Width(t))
}

// raw appends an already rendered segment (a pill) padded to w cells.
func (r *row) raw(s string, w int) *row {
	r.b.WriteString(s)
	return r.space(w - lipgloss.Width(s))
}

func (r *row) space(n int) *row {
	if n > 0 {
		r.add(lipgloss.NewStyle(), strings.Repeat(" ", n))
	}
	return r
}

// line ends the row, filling to width so the hover tint spans it.
func (r *row) line(width int) string {
	s := r.b.String()
	if r.hover {
		if n := width - lipgloss.Width(s); n > 0 {
			s += r.style(lipgloss.NewStyle()).Render(strings.Repeat(" ", n))
		}
	}
	return s
}

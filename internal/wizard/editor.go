package wizard

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// editor is the inline text field for a stage's model or skill: free text
// with fuzzy-filtered suggestions picked by tab or the arrows.
type editor struct {
	field   int
	label   string
	input   textinput.Model
	options []string
	matches []string
	sel     int               // -1: use the typed text
	tags    map[string]string // muted label after a suggestion, e.g. "project"
	hint    string            // shown when nothing matches the typed text
}

func newEditor(field int, label, value, placeholder string, options []string) *editor {
	in := textinput.New()
	in.Prompt = "> "
	in.Placeholder = placeholder
	in.CharLimit = 120
	in.Cursor.SetMode(cursor.CursorStatic) // no blink ticks
	in.SetValue(value)
	in.CursorEnd()
	in.Focus()
	// Open on the full list with the current value highlighted; typing
	// filters it.
	e := &editor{field: field, label: label, input: in, options: options, matches: options, sel: -1}
	for i, o := range options {
		if o == value {
			e.sel = i
		}
	}
	return e
}

func (e *editor) filter() {
	e.matches = fuzzy(e.input.Value(), e.options)
	e.sel = -1
}

// value is what enter commits.
func (e *editor) value() string {
	if e.sel >= 0 && e.sel < len(e.matches) {
		return e.matches[e.sel]
	}
	return strings.TrimSpace(e.input.Value())
}

// update handles one key; done reports enter (commit) or esc (cancel).
func (e *editor) update(msg tea.KeyMsg) (done, commit bool) {
	switch msg.String() {
	case "enter":
		return true, true
	case "esc":
		return true, false
	case "tab", "down", "ctrl+n":
		if len(e.matches) > 0 {
			e.sel = (e.sel + 1) % len(e.matches)
		}
		return false, false
	case "shift+tab", "up", "ctrl+p":
		if e.sel >= 0 {
			e.sel--
		}
		return false, false
	}
	e.input, _ = e.input.Update(msg)
	e.filter()
	return false, false
}

// view is two lines: the field, then a window of suggestions that keeps
// the highlighted one visible.
func (e *editor) view(width int) string {
	e.input.Width = width - lipgloss.Width(e.label) - 4
	first := accentStyle.Render(e.label) + " " + e.input.View()
	if len(e.matches) == 0 {
		hint := "no matches, enter uses what you typed"
		switch {
		case e.hint != "":
			hint = e.hint
		case len(e.options) == 0:
			hint = "type a value, or leave blank for the default"
		}
		return first + "\n" + mutedStyle.Render("  "+hint)
	}
	start := 0
	if e.sel > 0 {
		start = e.sel
	}
	// Pull earlier items back in while they fit, so the window is full.
	used := 2
	var shown []string
	for i := start; i < len(e.matches); i++ {
		w := lipgloss.Width(e.render(i)) + 3
		if used+w > width {
			break
		}
		used += w
		shown = append(shown, e.render(i))
	}
	for i := start - 1; i >= 0; i-- {
		w := lipgloss.Width(e.render(i)) + 3
		if used+w > width {
			break
		}
		used += w
		shown = append([]string{e.render(i)}, shown...)
	}
	return first + "\n  " + strings.Join(shown, mutedStyle.Render(" · "))
}

func (e *editor) render(i int) string {
	s := mutedStyle.Render(e.matches[i])
	if i == e.sel {
		s = cellStyle.Render(e.matches[i])
	}
	if t := e.tags[e.matches[i]]; t != "" {
		s += mutedStyle.Render(" (" + t + ")")
	}
	return s
}

// fuzzy keeps options matching query as a case-insensitive subsequence,
// ranked exact, prefix, substring, then scattered; shorter first on ties.
func fuzzy(query string, options []string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	type hit struct {
		s     string
		score int
	}
	var hits []hit
	for _, o := range options {
		l := strings.ToLower(o)
		score := -1
		switch {
		case q == "":
			score = 3
		case l == q:
			score = 0
		case strings.HasPrefix(l, q):
			score = 1
		case strings.Contains(l, q):
			score = 2
		case subsequence(q, l):
			score = 3
		}
		if score >= 0 {
			hits = append(hits, hit{o, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		if q != "" && len(hits[i].s) != len(hits[j].s) {
			return len(hits[i].s) < len(hits[j].s)
		}
		return false
	})
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.s
	}
	return out
}

func subsequence(q, s string) bool {
	qr := []rune(q)
	i := 0
	for _, r := range s {
		if i < len(qr) && qr[i] == r {
			i++
		}
	}
	return i == len(qr)
}

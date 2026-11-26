package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
)

// matchesQuery reports whether t matches every space-separated term of q:
// "#tag" matches a label, "@name" the claiming or last agent, and a plain
// word the title, description, a label or an ID prefix. Case-insensitive.
func matchesQuery(t task.Task, q string) bool {
	for _, term := range strings.Fields(strings.ToLower(q)) {
		if !matchesTerm(t, term) {
			return false
		}
	}
	return true
}

func matchesTerm(t task.Task, term string) bool {
	switch {
	case strings.HasPrefix(term, "#"):
		tag := strings.TrimPrefix(term, "#")
		for _, l := range t.Labels {
			if strings.HasPrefix(strings.ToLower(strings.TrimPrefix(l, "#")), tag) {
				return true
			}
		}
		return false
	case strings.HasPrefix(term, "@"):
		name := strings.TrimPrefix(term, "@")
		for _, a := range []string{t.ClaimedBy, t.LastAgent} {
			if a != "" && strings.HasPrefix(strings.ToLower(a), name) {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(strings.ToLower(t.ID), term) ||
		strings.Contains(strings.ToLower(t.Title), term) ||
		strings.Contains(strings.ToLower(t.Description), term) {
		return true
	}
	for _, l := range t.Labels {
		if strings.Contains(strings.ToLower(l), term) {
			return true
		}
	}
	return false
}

// filtering reports whether a family or search filter hides some cards.
func (m BoardModel) filtering() bool {
	return m.familyID != "" || strings.TrimSpace(m.query) != ""
}

// openSearch starts typing a filter in the footer, seeded with the current one.
func (m *BoardModel) openSearch() {
	m.mode = modeSearch
	m.search.SetValue(m.query)
	m.search.CursorEnd()
	m.search.Focus()
}

// setQuery applies a new filter, keeping the selected card when it still
// matches and clamping every cursor otherwise.
func (m *BoardModel) setQuery(q string) {
	selected := ""
	if t, ok := m.selectedTask(); ok {
		selected = t.ID
	}
	m.query = q
	if selected == "" || !m.selectTaskID(selected) {
		m.clampSelection()
	}
}

// updateSearch edits the filter live: enter keeps it, esc clears it.
func (m BoardModel) updateSearch(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.Type {
	case tea.KeyEnter:
		m.mode = modeNormal
		m.search.Blur()
		m.setQuery(strings.TrimSpace(m.search.Value()))
		return m, nil
	case tea.KeyEsc:
		m.mode = modeNormal
		m.search.Blur()
		m.search.SetValue("")
		m.setQuery("")
		return m, nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(k)
	m.setQuery(m.search.Value())
	return m, cmd
}

package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/project"
)

// Package-level hooks so tests never touch the real repository: the git
// runner new models start with, and the directory a board outside a
// project searches.
var (
	defaultGit project.Runner = project.Git
	workingDir                = os.Getwd
)

// pickRow is one option in a form dropdown.
type pickRow struct {
	label  string // main text
	tag    string // muted text after it (branch, short ID)
	value  string // what picking fills in or adds
	create bool   // the "+ create worktree" / "+ add path" row
}

// pickerRowCap bounds how many matches a picker keeps; only a handful show.
const pickerRowCap = 200

// worktreePicker is the Worktree field's dropdown state. The git lists are
// read when the field gains focus, the rows whenever the text changes.
type worktreePicker struct {
	repo      bool   // the project is inside a git repository
	top       string // git's working tree root for the project
	main      string // the main worktree, where new ones are placed beside
	worktrees []project.Worktree
	branches  []string
	rows      []pickRow // matches, then the create row when offered
	matches   int       // matches before the cap
	index     int       // highlighted row; -1 for none
	choosing  bool      // enter picks instead of saving
}

// docPicker is the Attachments / Working notes state: the chosen paths per
// field and the project's files, listed once per form open.
type docPicker struct {
	paths    [2][]string // attachments, working notes
	files    []string
	loaded   bool
	rows     []pickRow
	matches  int
	index    int
	choosing bool
}

// projectDir is where the pickers look: the board's project root, or the
// working directory for a board outside a project.
func (m BoardModel) projectDir() string {
	if m.root != "" {
		return m.root
	}
	if root := projectRoot(m.board.Path()); root != "" {
		return root
	}
	dir, err := workingDir()
	if err != nil {
		return "."
	}
	return dir
}

func (m BoardModel) project() project.Project {
	return project.Project{Root: m.projectDir(), Run: m.git}
}

// --- Worktree ---

// loadWorktrees reads the worktrees and branches; called when the field
// gains focus, never per keystroke. Outside git the field stays plain text.
func (m *BoardModel) loadWorktrees() {
	p := m.project()
	m.wt = worktreePicker{index: -1}
	top, ok := p.TopLevel()
	if !ok {
		return
	}
	m.wt.repo, m.wt.top, m.wt.main = true, top, top
	m.wt.worktrees, _ = p.Worktrees()
	m.wt.branches, _ = p.Branches()
	for _, wt := range m.wt.worktrees {
		if wt.Main {
			m.wt.main = wt.Path
		}
	}
	m.refreshWorktreeRows()
}

// worktreeLabel shows a worktree path relative to the repository's parent
// directory when that is shorter, so siblings read as their folder names.
func (m BoardModel) worktreeLabel(path string) string {
	if rel, err := filepath.Rel(filepath.Dir(m.wt.top), path); err == nil && len(rel) < len(path) {
		return rel
	}
	return path
}

// refreshWorktreeRows re-filters the cached lists against the typed text.
func (m *BoardModel) refreshWorktreeRows() {
	if !m.wt.repo {
		m.wt.rows = nil
		return
	}
	text := strings.TrimSpace(m.worktree.Value())
	var all []pickRow
	var keys []string
	exact := false
	for _, wt := range m.wt.worktrees {
		label := m.worktreeLabel(wt.Path)
		tag := wt.Branch
		if tag == "" {
			tag = "detached"
		}
		all = append(all, pickRow{label: label, tag: "⎇ " + tag, value: wt.Path})
		keys = append(keys, label+" "+wt.Branch)
		exact = exact || text == wt.Path || text == label || (text != "" && text == wt.Branch)
	}
	for _, b := range m.wt.branches {
		all = append(all, pickRow{label: b, tag: "branch", value: b})
		keys = append(keys, b)
		exact = exact || text == b
	}
	matches := project.Fuzzy(text, keys)
	m.wt.matches = len(matches)
	m.wt.rows = m.wt.rows[:0:0]
	for _, mt := range matches[:min(len(matches), pickerRowCap)] {
		m.wt.rows = append(m.wt.rows, all[mt.Index])
	}
	if text != "" && !exact {
		path := project.NewWorktreePath(m.wt.main, text)
		shown := path
		if rel, err := filepath.Rel(m.wt.top, path); err == nil {
			shown = rel
		}
		m.wt.rows = append(m.wt.rows, pickRow{
			label:  "+ create worktree " + shown + " on new branch " + text,
			value:  text,
			create: true,
		})
	}
	// The create row is never highlighted by default: creating needs an
	// explicit ↑↓ onto it and then enter.
	m.wt.index = -1
	if len(m.wt.rows) > 0 && !m.wt.rows[0].create {
		m.wt.index = 0
	}
}

// moveWorktreeHighlight steps through the rows, the create row included.
func (m *BoardModel) moveWorktreeHighlight(delta int) {
	n := len(m.wt.rows)
	if n == 0 {
		return
	}
	switch {
	case m.wt.index < 0 && delta > 0:
		m.wt.index = 0
	case m.wt.index < 0:
		m.wt.index = n - 1
	default:
		m.wt.index = (m.wt.index + delta + n) % n
	}
	m.wt.choosing = true
}

// pickWorktree acts on the highlighted row: an existing worktree or branch
// fills the field; the create row runs git worktree add. It reports whether
// enter was consumed.
func (m *BoardModel) pickWorktree() bool {
	if !m.wt.repo || !m.wt.choosing || m.wt.index < 0 || m.wt.index >= len(m.wt.rows) {
		return false
	}
	row := m.wt.rows[m.wt.index]
	if row.create {
		p := m.project()
		branch, err := p.CheckBranch(row.value)
		if err != nil {
			m.err = err.Error()
			m.sizeForm()
			return true
		}
		path := project.NewWorktreePath(m.wt.main, branch)
		if err := p.AddWorktree(branch, path); err != nil {
			m.err = "git worktree add: " + err.Error()
			m.sizeForm()
			return true
		}
		m.loadWorktrees()
		row.value = path
	}
	m.err = ""
	m.worktree.SetValue(row.value)
	m.worktree.CursorEnd()
	m.refreshWorktreeRows()
	m.wt.choosing = false
	m.sizeForm()
	return true
}

// --- Documents ---

// docSlot maps a document field to its index in docPicker.paths.
func docSlot(field int) int {
	if field == fieldWorkingNotes {
		return 1
	}
	return 0
}

func isDocField(field int) bool { return field == fieldAttachments || field == fieldWorkingNotes }

// docQuery is the search text of a document field.
func (m BoardModel) docQuery(field int) string {
	if field == fieldWorkingNotes {
		return m.workingNotes.Value()
	}
	return m.attachments.Value()
}

// loadFiles lists the project's files once per form open.
func (m *BoardModel) loadFiles() {
	if m.doc.loaded {
		return
	}
	m.doc.files = m.project().Files()
	m.doc.loaded = true
}

// literalPath reports whether a query names a path outside the project
// (absolute, home or parent-relative); those are offered as typed, since
// the picker only ever lists files inside the root.
func literalPath(q string) bool {
	return filepath.IsAbs(q) || q == "~" || strings.HasPrefix(q, "~/") || q == ".." || strings.HasPrefix(q, "../")
}

// refreshDocRows re-filters the files against the focused field's query.
func (m *BoardModel) refreshDocRows() {
	m.doc.rows, m.doc.matches, m.doc.index = nil, 0, 0
	if !isDocField(m.field) {
		return
	}
	q := strings.TrimSpace(m.docQuery(m.field))
	if q != "" && literalPath(q) {
		m.doc.rows = []pickRow{{label: "+ add " + q, value: q, create: true}}
		return
	}
	chosen := map[string]bool{}
	for _, p := range m.doc.paths[docSlot(m.field)] {
		chosen[p] = true
	}
	var files []string
	for _, f := range m.doc.files {
		if !chosen[f] {
			files = append(files, f)
		}
	}
	matches := project.Fuzzy(q, files)
	m.doc.matches = len(matches)
	for _, mt := range matches[:min(len(matches), pickerRowCap)] {
		m.doc.rows = append(m.doc.rows, pickRow{label: files[mt.Index], value: files[mt.Index]})
	}
	if q != "" && len(matches) == 0 && !chosen[q] {
		m.doc.rows = []pickRow{{label: "+ add " + q, value: q, create: true}}
	}
}

func (m *BoardModel) moveDocHighlight(delta int) {
	if n := len(m.doc.rows); n > 0 {
		m.doc.index = (m.doc.index + delta + n) % n
		m.doc.choosing = true
	}
}

// addDocument adds the highlighted row to the focused field's chips and
// clears the query; it reports whether enter was consumed.
func (m *BoardModel) addDocument() bool {
	if !isDocField(m.field) || !m.doc.choosing || m.doc.index >= len(m.doc.rows) {
		return false
	}
	slot := docSlot(m.field)
	m.doc.paths[slot] = append(m.doc.paths[slot], m.doc.rows[m.doc.index].value)
	m.setDocQuery("")
	return true
}

// removeLastDocument drops the newest chip of the focused field.
func (m *BoardModel) removeLastDocument() {
	slot := docSlot(m.field)
	if n := len(m.doc.paths[slot]); n > 0 {
		m.doc.paths[slot] = m.doc.paths[slot][:n-1]
		m.refreshDocRows()
		m.sizeForm()
	}
}

func (m *BoardModel) setDocQuery(q string) {
	if m.field == fieldWorkingNotes {
		m.workingNotes.SetValue(q)
	} else {
		m.attachments.SetValue(q)
	}
	m.doc.choosing = strings.TrimSpace(q) != ""
	m.refreshDocRows()
	m.sizeForm()
}

// chipStyle is a chosen document path; the × marks it as removable.
var chipStyle = lipgloss.NewStyle().Foreground(ColText)

func chip(path string, width int) string {
	return chipStyle.Render(truncate(path, width)) + MutedStyle.Render(" ×")
}

// chipLine fits chips into avail cells with a "+N" count for the rest:
// from the start when reading, from the end (newest) while editing.
func chipLine(paths []string, avail int, fromEnd bool) string {
	if len(paths) == 0 || avail < 4 {
		return ""
	}
	maxPath := max(avail-2, 1)
	if len(paths) > 1 {
		maxPath = max(avail-2-4, 1) // leave room for "+N  "
	}
	var shown []string
	width := 0
	for i := range paths {
		idx := i
		if fromEnd {
			idx = len(paths) - 1 - i
		}
		c := chip(paths[idx], min(maxPath, 30))
		need := lipgloss.Width(c)
		if len(shown) > 0 {
			need += 2
		}
		rest := len(paths) - len(shown) - 1
		reserve := 0
		if rest > 0 {
			reserve = len(strconv.Itoa(rest)) + 3
		}
		if width+need+reserve > avail && len(shown) > 0 {
			break
		}
		if width+need > avail {
			c = chip(paths[idx], max(avail-2-reserve, 1))
			need = lipgloss.Width(c)
		}
		shown = append(shown, c)
		width += need
	}
	more := ""
	if hidden := len(paths) - len(shown); hidden > 0 {
		more = MutedStyle.Render("+" + strconv.Itoa(hidden))
	}
	if fromEnd {
		for i, j := 0, len(shown)-1; i < j; i, j = i+1, j-1 {
			shown[i], shown[j] = shown[j], shown[i]
		}
		if more != "" {
			shown = append([]string{more}, shown...)
		}
	} else if more != "" {
		shown = append(shown, more)
	}
	return lipgloss.NewStyle().MaxWidth(avail).Render(strings.Join(shown, "  "))
}

// docInputMin is the narrowest the query input gets beside the chips.
const docInputMin = 10

// docChips is the chip text drawn before a focused field's query input.
func (m BoardModel) docChips(field, width int) string {
	return chipLine(m.doc.paths[docSlot(field)], width-2-docInputMin-2, true)
}

// docFieldView is a document field: chips then the query input while
// focused; chips from the start (or "none") otherwise.
func (m BoardModel) docFieldView(field, width int) string {
	in := m.attachments
	if field == fieldWorkingNotes {
		in = m.workingNotes
	}
	paths := m.doc.paths[docSlot(field)]
	if field != m.field {
		if len(paths) == 0 {
			if q := in.Value(); q != "" {
				return "› " + truncate(q, max(width-2, 1))
			}
			return "› " + MutedStyle.Render("none")
		}
		return "› " + chipLine(paths, width-2, false)
	}
	chips := m.docChips(field, width)
	if chips != "" {
		chips += " "
	}
	return "› " + chips + in.View()
}

// docInputWidth sizes a document input to the room the chips leave.
func (m BoardModel) docInputWidth(field, width int) int {
	used := 2 // "› "
	if chips := m.docChips(field, width); chips != "" {
		used += lipgloss.Width(chips) + 1
	}
	return max(width-used-1, 1) // the cursor cell
}

// docPlaceholder is the longest prompt that fits a document input.
func docPlaceholder(width int) string {
	for _, p := range []string{"file name or path", "add a file", "add"} {
		if lipgloss.Width(p) < width {
			return p
		}
	}
	return ""
}

// --- Dropdown ---

// pickerDropdownRows is the most options a dropdown shows at once.
const pickerDropdownRows = 4

// dropdownWant is the natural height of the focused field's dropdown
// (options plus a heading and two borders), 0 when it has none.
func (m BoardModel) dropdownWant() int {
	rows := 0
	switch {
	case m.field == fieldParent:
		rows = len(m.filteredParentOptions())
	case m.field == fieldWorktree && m.wt.repo:
		rows = len(m.wt.rows)
		if rows == 0 && strings.TrimSpace(m.worktree.Value()) == "" {
			return 0
		}
	case isDocField(m.field):
		rows = len(m.doc.rows)
		if rows == 0 && len(m.doc.files) == 0 {
			return 0
		}
	default:
		return 0
	}
	return min(max(rows, 1), pickerDropdownRows) + 3
}

// renderFieldDropdown draws the focused field's dropdown in height rows.
func (m BoardModel) renderFieldDropdown(width, height int) string {
	switch {
	case m.field == fieldParent:
		return m.renderParentDropdownSized(width, height)
	case m.field == fieldWorktree:
		heading := matchCount(m.wt.matches) + " in worktrees and branches"
		return renderDropdown(width, height, heading, m.wt.rows, m.wt.index, "No matching worktrees or branches")
	case isDocField(m.field):
		heading := matchCount(m.doc.matches) + " in the project"
		return renderDropdown(width, height, heading, m.doc.rows, m.doc.index, "No matching files")
	}
	return ""
}

// renderDropdown draws a bordered option list of at most height rows; under
// four rows it drops the border. index < 0 highlights nothing.
func renderDropdown(width, height int, heading string, rows []pickRow, index int, empty string) string {
	if height <= 0 {
		return ""
	}
	inner := max(width-4, 1) // border and padding, one cell each side
	lines := []string{MutedStyle.Copy().Bold(true).Render(truncate(heading, inner))}
	if len(rows) == 0 {
		lines = append(lines, MutedStyle.Render(truncate(empty, inner)))
	} else {
		visible := pickerDropdownRows
		if height >= 4 {
			visible = min(visible, height-3)
		} else {
			visible = max(1, height-1)
		}
		// The create row is always drawn last so it stays discoverable; the
		// matches scroll in the slots above it.
		matches, create := rows, -1
		if last := len(rows) - 1; rows[last].create && visible > 1 && last > 0 {
			matches, create = rows[:last], last
			visible--
		}
		start := 0
		if index >= visible && index < len(matches) {
			start = index - visible + 1
		}
		for i := start; i < min(start+visible, len(matches)); i++ {
			lines = append(lines, dropdownLine(matches[i], i == index, inner))
		}
		if create >= 0 {
			lines = append(lines, dropdownLine(rows[create], create == index, inner))
		}
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	if height < 4 {
		return strings.Join(lines, "\n")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(ColMuted).Padding(0, 1).Width(width - 2).Render(strings.Join(lines, "\n"))
}

// dropdownLine is one option: "› " when highlighted, the label cut to fit
// and the tag after two spaces.
func dropdownLine(r pickRow, highlighted bool, inner int) string {
	prefix, style := "  ", MutedStyle
	if r.create {
		style = lipgloss.NewStyle().Foreground(colDone)
	}
	if highlighted {
		prefix, style = "› ", lipgloss.NewStyle().Foreground(ColAccent).Bold(true)
	}
	tag := ""
	if r.tag != "" {
		tag = "  " + r.tag
	}
	label := truncate(r.label, max(inner-2-lipgloss.Width(tag), 1))
	return style.Render(truncate(prefix+label+tag, inner))
}

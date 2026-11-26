package wizard

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/agentsetup"
	"gotodo/internal/loops"
)

type screen int

const (
	scrWelcome screen = iota
	scrMCP
	scrLoops
	scrPacks
	scrExisting
	scrStages
	scrSummary
	scrApply
)

// crumbs are the steps in the header; existing-config counts as Stages.
var crumbs = []struct {
	label string
	scr   []screen
}{
	{"Welcome", []screen{scrWelcome}},
	{"MCP", []screen{scrMCP}},
	{"Loops", []screen{scrLoops}},
	{"Packs", []screen{scrPacks}},
	{"Stages", []screen{scrExisting, scrStages}},
	{"Summary", []screen{scrSummary}},
	{"Apply", []screen{scrApply}},
}

// Stage table columns.
const (
	colOn = iota
	colAgent
	colModel
	colSkill
	colPerm
	numCols
)

type packState int

const (
	packMissing packState = iota
	packInstalled
	packNeedsNpx
)

type packCell struct {
	Pack, Agent string
	State       packState
}

type existingConfig struct {
	Mod    time.Time
	Stages []string
	Err    error
}

// Model is the wizard's Bubble Tea model.
type Model struct {
	d Deps

	width, height int
	scr           screen
	back          []screen
	cur           map[screen]int // cursor per screen, kept on Back

	detected []string
	paths    map[string]string
	skills   map[string][]string
	npx      bool

	mcpOn map[string]bool

	loops       bool
	loopsPreset bool

	cells   []packCell
	checked map[string]bool

	existing *existingConfig
	replace  bool

	rows        []StageRow
	stageCol    int
	rowsTouched bool
	edit        *editor
	errMsg      string

	steps     []step
	finished  bool
	appliedAt time.Time
	quit      bool
}

// New builds the wizard, detecting agents, skills and an existing config.
func New(d Deps, o Options) Model {
	m := Model{
		d:       d,
		cur:     map[screen]int{},
		skills:  map[string][]string{},
		mcpOn:   map[string]bool{},
		checked: map[string]bool{},
	}
	m.detected, m.paths = detectAgents(d.LookPath)
	_, err := d.LookPath("npx")
	m.npx = err == nil
	for _, a := range m.detected {
		m.skills[a] = d.Skills(a)
		m.mcpOn[a] = true
		for _, p := range Packs {
			c := packCell{Pack: p, Agent: a}
			switch {
			case HasPack(p, m.skills[a]):
				c.State = packInstalled
			case isShared(p, a) && !m.npx:
				c.State = packNeedsNpx
			}
			m.cells = append(m.cells, c)
		}
	}
	if o.Loops != nil {
		m.loops, m.loopsPreset = *o.Loops, true
	}
	if m.loops {
		m.cur[scrLoops] = 1
	}
	if configExists(d.ConfigPath) {
		ex := &existingConfig{}
		if st, err := os.Stat(d.ConfigPath); err == nil {
			ex.Mod = st.ModTime()
		}
		c, err := loops.ReadFile(d.ConfigPath)
		ex.Err = err
		ex.Stages = c.StageNames()
		m.existing = ex
	}
	return m
}

// Init starts nothing; the wizard waits for keys.
func (m Model) Init() tea.Cmd { return nil }

// available is what an agent can load now plus what chosen packs bring.
func (m Model) available(agent string) []string {
	lists := [][]string{m.skills[agent]}
	for _, p := range Packs {
		if m.checked[cellKey(p, agent)] {
			lists = append(lists, packProvides[p])
		}
	}
	return union(lists...)
}

// writeConfig reports whether apply writes loops.json.
func (m Model) writeConfig() bool {
	return m.loops && len(m.detected) > 0 && (m.existing == nil || m.replace)
}

// Update handles keys, window size and finished apply steps.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case stepDoneMsg:
		return m.stepDone(msg)
	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		m.quit = true
		return m, tea.Quit
	}
	if m.edit != nil {
		done, commit := m.edit.update(msg)
		if done {
			if commit {
				r := &m.rows[m.cur[scrStages]]
				if m.edit.field == colModel {
					r.Model = m.edit.value()
				} else {
					r.Skill = m.edit.value()
				}
				m.rowsTouched = true
				m.errMsg = ""
			}
			m.edit = nil
		}
		return m, nil
	}
	if m.scr == scrApply {
		if m.finished && (k == "enter" || k == "q" || k == "esc") {
			m.quit = true
			return m, tea.Quit
		}
		return m, nil
	}
	switch k {
	case "q":
		m.quit = true
		return m, tea.Quit
	case "esc":
		if len(m.back) == 0 {
			m.quit = true
			return m, tea.Quit
		}
		m.scr = m.back[len(m.back)-1]
		m.back = m.back[:len(m.back)-1]
		m.errMsg = ""
		return m, nil
	case "enter":
		return m.next()
	}
	switch m.scr {
	case scrMCP:
		m.list(k, len(m.detected), func(i int) { m.mcpOn[m.detected[i]] = !m.mcpOn[m.detected[i]] })
	case scrLoops:
		m.list(k, 2, nil)
		switch k {
		case "y":
			m.cur[scrLoops] = 1
		case "n":
			m.cur[scrLoops] = 0
		}
	case scrPacks:
		offer := m.offered()
		m.list(k, len(offer), func(i int) { m.togglePack(offer[i]) })
	case scrExisting:
		m.list(k, 2, nil)
	case scrStages:
		m.stagesKey(k)
	}
	return m, nil
}

// list moves the current screen's cursor over n rows; space calls toggle.
func (m *Model) list(k string, n int, toggle func(int)) {
	i := m.cur[m.scr]
	switch k {
	case "up", "k", "left", "h", "shift+tab":
		if i > 0 {
			i--
		}
	case "down", "j", "right", "l", "tab":
		if i < n-1 {
			i++
		}
	case " ", "x":
		if toggle != nil && i < n {
			toggle(i)
		}
	}
	m.cur[m.scr] = i
}

// offered are the pack cells a person can tick: missing ones.
func (m Model) offered() []int {
	var out []int
	for i, c := range m.cells {
		if c.State == packMissing {
			out = append(out, i)
		}
	}
	return out
}

// togglePack flips one cell; shared mattpocock cells move together since
// one install covers both agents.
func (m *Model) togglePack(i int) {
	c := m.cells[i]
	on := !m.checked[cellKey(c.Pack, c.Agent)]
	for _, o := range m.cells {
		if o == c || (isShared(c.Pack, c.Agent) && o.Pack == c.Pack && isShared(o.Pack, o.Agent) && o.State == packMissing) {
			m.checked[cellKey(o.Pack, o.Agent)] = on
		}
	}
}

func (m *Model) stagesKey(k string) {
	if len(m.rows) == 0 {
		return
	}
	row, col := m.cur[scrStages], m.stageCol
	switch k {
	case "up", "k":
		if row > 0 {
			row--
		}
	case "down", "j":
		if row < len(m.rows)-1 {
			row++
		}
	case "left", "h", "shift+tab":
		if col > 0 {
			col--
		}
	case "right", "l", "tab":
		if col < numCols-1 {
			col++
		}
	case " ", "e", "x":
		m.change(row, col)
	}
	m.cur[scrStages], m.stageCol = row, col
}

// change acts on one stage cell: toggle, cycle, or open the editor.
func (m *Model) change(row, col int) {
	r := &m.rows[row]
	m.errMsg = ""
	switch col {
	case colOn:
		r.On = !r.On
	case colAgent:
		r.Agent = cycle(m.detected, r.Agent)
		r.Model = defaultModel(r.Agent, r.Name)
		if !contains(m.available(r.Agent), r.Skill) {
			r.Skill = pickSkill(r.Name, m.available(r.Agent))
		}
	case colModel:
		ph := ""
		switch r.Agent {
		case "codex":
			ph = "blank uses codex's default"
		case "opencode":
			ph = "provider/model, blank uses the default"
		}
		m.edit = newEditor(colModel, "Model for "+r.Name, r.Model, ph, ModelSuggestions[r.Agent])
		return
	case colSkill:
		m.edit = newEditor(colSkill, "Skill for "+r.Name, r.Skill, "type or pick a skill", m.available(r.Agent))
		return
	case colPerm:
		r.Perm = cycle(loops.Permissions, r.Perm)
	}
	m.rowsTouched = true
}

func cycle(list []string, cur string) string {
	for i, v := range list {
		if v == cur {
			return list[(i+1)%len(list)]
		}
	}
	if len(list) == 0 {
		return cur
	}
	return list[0]
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// next leaves the current screen forward.
func (m Model) next() (tea.Model, tea.Cmd) {
	var to screen
	switch m.scr {
	case scrWelcome:
		to = scrMCP
	case scrMCP:
		switch {
		case len(m.detected) == 0:
			to = scrSummary
		case m.loopsPreset && m.loops:
			to = scrPacks
		case m.loopsPreset:
			to = scrSummary
		default:
			to = scrLoops
		}
	case scrLoops:
		m.loops = m.cur[scrLoops] == 1
		to = scrSummary
		if m.loops {
			to = scrPacks
		}
	case scrPacks:
		to = scrStages
		if m.existing != nil {
			to = scrExisting
		}
	case scrExisting:
		m.replace = m.cur[scrExisting] == 1
		to = scrSummary
		if m.replace {
			to = scrStages
		}
	case scrStages:
		if msg := m.validate(); msg != "" {
			m.errMsg = msg
			return m, nil
		}
		to = scrSummary
	case scrSummary:
		m.back = append(m.back, m.scr)
		m.scr = scrApply
		return m.startApply()
	}
	if to == scrStages && !m.rowsTouched {
		m.rows = DefaultRows(m.detected, m.available)
	}
	m.back = append(m.back, m.scr)
	m.scr = to
	m.errMsg = ""
	return m, nil
}

func (m Model) validate() string {
	on := 0
	for _, r := range m.rows {
		if !r.On {
			continue
		}
		on++
		if strings.TrimSpace(r.Skill) == "" {
			return r.Name + " needs a skill: move to Skill and press space"
		}
	}
	if on == 0 {
		return "turn on at least one stage, or go back and choose No"
	}
	return ""
}

// Applied reports whether the wizard ran its steps.
func (m Model) Applied() bool { return m.finished }

// Report is the plain-text outcome printed after the screen closes.
func (m Model) Report() []string {
	if len(m.steps) == 0 && !m.finished {
		return []string{"Setup cancelled; nothing was changed."}
	}
	var out []string
	if !m.finished {
		out = append(out, "Aborted; steps not reported as ok may not have run or may be half done.")
	}
	for _, s := range m.steps {
		out = append(out, fmt.Sprintf("%-28s %s", s.Label+":", s.result()))
	}
	if len(out) == 0 {
		out = append(out, "Nothing to do.")
	}
	return append(out, restartNote)
}

const restartNote = "Restart open agent sessions to load the gotodo MCP server and new skills."

// ---- view ----

func (m Model) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	if w > 100 {
		w = 100
	}
	return w, h
}

// View renders header, panel and footer, with the footer pinned to the
// bottom and the panel body clipped so nothing overflows the terminal.
func (m Model) View() string {
	if m.quit {
		return ""
	}
	w, h := m.size()
	inner := w - 4
	header := m.header(w)
	footer := hintBar(w, m.hints()...)
	room := h - lipgloss.Height(header) - 1 - 2 - 1 - lipgloss.Height(footer)
	if room < 1 {
		room = 1
	}
	lines := strings.Split(m.body(inner), "\n")
	if len(lines) > room {
		lines = lines[:room]
	}
	for i, l := range lines {
		if lipgloss.Width(l) > inner {
			lines[i] = lipgloss.NewStyle().MaxWidth(inner).Render(l)
		}
	}
	panel := panelStyle.Width(w - 2).Render(strings.Join(lines, "\n"))
	gap := h - lipgloss.Height(header) - 1 - lipgloss.Height(panel) - lipgloss.Height(footer)
	if gap < 1 {
		gap = 1
	}
	return header + "\n\n" + panel + strings.Repeat("\n", gap) + footer
}

func (m Model) header(w int) string {
	parts := make([]string, 0, len(crumbs))
	for _, c := range crumbs {
		active := false
		for _, s := range c.scr {
			active = active || s == m.scr
		}
		if active {
			parts = append(parts, accentStyle.Render(c.label))
		} else {
			parts = append(parts, mutedStyle.Render(c.label))
		}
	}
	line := titleStyle.Render("gotodo setup") + "   " + strings.Join(parts, mutedStyle.Render(" · "))
	return lipgloss.NewStyle().MaxWidth(w).Render(line)
}

func (m Model) hints() []string {
	if m.edit != nil {
		return []string{keyHint("tab", "Suggestion"), keyHint("enter", "Use"), keyHint("esc", "Cancel")}
	}
	switch m.scr {
	case scrWelcome:
		return []string{keyHint("enter", "Next"), keyHint("q", "Quit")}
	case scrMCP, scrPacks:
		return []string{keyHint("↑↓", "Move"), keyHint("space", "Toggle"), keyHint("enter", "Next"), keyHint("esc", "Back")}
	case scrLoops, scrExisting:
		return []string{keyHint("↑↓", "Choose"), keyHint("enter", "Next"), keyHint("esc", "Back")}
	case scrStages:
		return []string{keyHint("↑↓", "Stage"), keyHint("←→", "Field"), keyHint("space", "Change"), keyHint("enter", "Next"), keyHint("esc", "Back")}
	case scrSummary:
		return []string{keyHint("enter", "Apply"), keyHint("esc", "Back"), keyHint("q", "Quit")}
	case scrApply:
		if m.finished {
			return []string{keyHint("enter", "Quit")}
		}
		return []string{keyHint("ctrl+c", "Abort")}
	}
	return nil
}

func (m Model) body(w int) string {
	switch m.scr {
	case scrWelcome:
		return m.welcomeView()
	case scrMCP:
		return m.mcpView()
	case scrLoops:
		return m.loopsView()
	case scrPacks:
		return m.packsView()
	case scrExisting:
		return m.existingView()
	case scrStages:
		return m.stagesView(w)
	case scrSummary:
		return m.summaryView(w)
	case scrApply:
		return m.applyView(w)
	}
	return ""
}

func check(on bool) string {
	if on {
		return accentStyle.Render("[x]")
	}
	return mutedStyle.Render("[ ]")
}

func radio(on bool) string {
	if on {
		return accentStyle.Render("●")
	}
	return mutedStyle.Render("○")
}

func (m Model) welcomeView() string {
	b := []string{
		textStyle.Render("Set up gotodo for the coding agents on this machine."),
		"",
		"  • Register the gotodo MCP server, so agents can read and move cards",
		"  • Optionally turn on stage loops: agents start when cards change column",
		"  • Optionally install the pstack and mattpocock skill packs",
		"",
		headStyle.Render("Agents found"),
	}
	for _, a := range loops.Agents {
		if p, ok := m.paths[a]; ok {
			b = append(b, "  "+okStyle.Render("✓")+" "+pad(a, 10)+mutedStyle.Render(tilde(p, m.d.Home)))
		} else {
			b = append(b, "  "+errorStyle.Render("✗")+" "+pad(a, 10)+mutedStyle.Render("not found"))
		}
	}
	return strings.Join(append(b, "", mutedStyle.Render("Nothing changes until you confirm on the summary screen.")), "\n")
}

var mcpWhere = map[string]string{
	"claude":   "Claude Code, user scope via claude mcp add",
	"codex":    "Codex, ~/.codex/config.toml",
	"opencode": "OpenCode, ~/.config/opencode/opencode.json",
}

func (m Model) mcpView() string {
	if len(m.detected) == 0 {
		return strings.Join([]string{
			errorLine("No coding agent CLI found on PATH (claude, codex, opencode)."),
			"",
			"Install one, then run gotodo setup again.",
		}, "\n")
	}
	b := []string{textStyle.Render("Register the gotodo MCP server with:"), ""}
	for i, a := range m.detected {
		b = append(b, cursorBar(i == m.cur[scrMCP])+check(m.mcpOn[a])+" "+pad(a, 10)+mutedStyle.Render(mcpWhere[a]))
	}
	return strings.Join(append(b, "",
		mutedStyle.Render("Agents load it on their next start. Unticked agents are left as they are.")), "\n")
}

func (m Model) loopsView() string {
	sel := m.cur[scrLoops]
	return strings.Join([]string{
		textStyle.Render("Start an agent automatically when a card moves?"),
		"",
		"When a card enters a column, gotodo starts an agent with a skill on it.",
		"Loops run unattended and spend tokens; change them later with gotodo loops.",
		"",
		cursorBar(sel == 0) + radio(sel == 0) + " No, just the MCP server",
		cursorBar(sel == 1) + radio(sel == 1) + " Yes, set up stage loops",
	}, "\n")
}

func (m Model) packsView() string {
	b := []string{
		textStyle.Render("Skill packs give loops their skills. Packs already present are not offered."),
		"",
		"  " + headStyle.Render(pad("", 12)+pad("pstack", 24)+"mattpocock"),
	}
	offer := m.offered()
	curCell := -1
	if i := m.cur[scrPacks]; i < len(offer) {
		curCell = offer[i]
	}
	needsNpx := false
	for _, a := range m.detected {
		line := pad(a, 12)
		rowCur := false
		for i, c := range m.cells {
			if c.Agent != a {
				continue
			}
			var cell string
			switch c.State {
			case packInstalled:
				cell = okStyle.Render("✓") + mutedStyle.Render(" installed")
			case packNeedsNpx:
				cell = amberStyle.Render("!") + mutedStyle.Render(" needs npx")
				needsNpx = true
			default:
				label := " install"
				if i == curCell {
					rowCur = true
					label = cellStyle.Render(label[1:])
					cell = check(m.checked[cellKey(c.Pack, a)]) + " " + label
				} else {
					cell = check(m.checked[cellKey(c.Pack, a)]) + mutedStyle.Render(label)
				}
			}
			if c.Pack == PackPstack {
				cell += strings.Repeat(" ", max(0, 24-lipgloss.Width(cell)))
			}
			line += cell
		}
		b = append(b, cursorBar(rowCur)+line)
	}
	if len(offer) == 0 {
		b = append(b, "", okStyle.Render("Every detected agent already has both packs."))
	}
	b = append(b, "",
		mutedStyle.Render(pad("pstack", 11)+packSource[PackPstack]),
		mutedStyle.Render(pad("mattpocock", 11)+packSource[PackMattpocock]),
		mutedStyle.Render("mattpocock for Codex and OpenCode is one shared install in ~/.agents/skills."))
	if needsNpx {
		b = append(b, amberStyle.Render("! npx not found: install Node.js, then rerun gotodo setup for mattpocock."))
	}
	return strings.Join(b, "\n")
}

func (m Model) existingView() string {
	ex := m.existing
	sel := m.cur[scrExisting]
	info := "modified " + ex.Mod.Format("02/01/2006 15:04")
	stages := "stages: " + strings.Join(ex.Stages, ", ")
	if len(ex.Stages) == 0 {
		stages = "no stages"
	}
	if ex.Err != nil {
		stages = errorStyle.Render("unreadable: " + trunc(ex.Err.Error(), 60))
	}
	return strings.Join([]string{
		textStyle.Render("Existing config found."),
		"",
		"  " + tilde(m.d.ConfigPath, m.d.Home) + mutedStyle.Render(" · "+info),
		"  " + stages,
		"",
		cursorBar(sel == 0) + radio(sel == 0) + " Keep it, leave the file untouched",
		cursorBar(sel == 1) + radio(sel == 1) + " Replace it with the stages set up next",
	}, "\n")
}

// stage table column widths: stage, on, agent, model, skill, access.
var colW = []int{15, 4, 10, 19, 17, 9}

func (m Model) stagesView(w int) string {
	row, col := m.cur[scrStages], m.stageCol
	b := []string{
		textStyle.Render("When a card enters a column, run:"),
		"  " + headStyle.Render(pad("Stage", colW[0])+pad("On", colW[1])+pad("Agent", colW[2])+
			pad("Model", colW[3])+pad("Skill", colW[4])+"Access"),
	}
	for i, r := range m.rows {
		cell := func(c int, text string, base lipgloss.Style) string {
			t := trunc(text, colW[c]-1)
			gap := strings.Repeat(" ", colW[c]-lipgloss.Width(t))
			if i == row && c == col+1 {
				return cellStyle.Render(t) + gap
			}
			if !r.On && c > 1 {
				base = mutedStyle
			}
			return base.Render(t) + gap
		}
		on, onStyle := "·", mutedStyle
		if r.On {
			on, onStyle = "✓", okStyle
		}
		model, modelStyle := r.Model, textStyle
		if model == "" {
			model, modelStyle = "default", mutedStyle
		}
		skill, skillStyle := r.Skill, textStyle
		if skill == "" {
			skill, skillStyle = "pick one", amberStyle
		}
		permStyle := textStyle
		if r.Perm == loops.PermFull {
			permStyle = errorStyle
		}
		b = append(b, cursorBar(i == row)+textStyle.Render(pad(r.Name, colW[0]))+
			cell(1, on, onStyle)+cell(2, r.Agent, textStyle)+cell(3, model, modelStyle)+
			cell(4, skill, skillStyle)+cell(5, r.Perm, permStyle))
	}
	b = append(b, mutedStyle.Render("Personal boards use doing/done: set those with gotodo loops set."), "")
	switch {
	case m.edit != nil:
		b = append(b, m.edit.view(w))
	case m.errMsg != "":
		b = append(b, errorLine(m.errMsg))
	default:
		b = append(b, mutedStyle.Render(m.fieldHelp(row, col)))
	}
	for _, r := range m.rows {
		if r.On && r.Perm == loops.PermFull {
			b = append(b, errorLine(r.Name+" has full access: it skips every permission prompt, unattended"))
			break
		}
	}
	return strings.Join(b, "\n")
}

func (m Model) fieldHelp(row, col int) string {
	if row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	switch col {
	case colOn:
		return "space turns " + r.Name + " on or off"
	case colAgent:
		return "space cycles the detected agents: " + strings.Join(m.detected, ", ")
	case colModel:
		switch r.Agent {
		case "claude":
			return "space edits: " + strings.Join(ModelSuggestions["claude"], ", ")
		case "codex":
			return "space edits; blank uses codex's default model"
		}
		return "space edits: provider/model, blank uses opencode's default"
	case colSkill:
		return fmt.Sprintf("space picks from %d skills %s can load, or type your own", len(m.available(r.Agent)), r.Agent)
	case colPerm:
		return "readonly: read + gotodo · edits: change files (default) · full: no prompts"
	}
	return ""
}

func (m Model) mcpChosen() []string {
	var out []string
	for _, a := range m.detected {
		if m.mcpOn[a] {
			out = append(out, a)
		}
	}
	return out
}

func (m Model) summaryView(w int) string {
	label := func(s string) string { return headStyle.Render(pad(s, 13)) }
	mcp := strings.Join(m.mcpChosen(), ", ")
	if mcp == "" {
		mcp = mutedStyle.Render("none")
	}
	b := []string{label("MCP server") + mcp}
	packs, shared := m.packLines()
	if len(packs) == 0 {
		packs = []string{mutedStyle.Render("none")}
	}
	for i, p := range packs {
		l := label("")
		if i == 0 {
			l = label("Skill packs")
		}
		b = append(b, l+p)
	}
	path := tilde(m.d.ConfigPath, m.d.Home)
	review := false
	switch {
	case !m.loops:
		b = append(b, label("Loops")+mutedStyle.Render("off, "+path+" not touched"))
	case !m.writeConfig():
		b = append(b, label("Loops")+"keep existing "+path)
	default:
		verb := "write "
		if m.existing != nil {
			verb = "replace "
		}
		b = append(b, label("Loops")+verb+path,
			label("")+mutedStyle.Render(fmt.Sprintf("max_parallel %d · max_runs_per_task %d",
				loops.DefaultMaxParallel, loops.DefaultMaxRunsPerTask)))
		// Warning before the table so a clipped view never hides it.
		for _, r := range m.rows {
			if r.On && r.Perm == loops.PermFull {
				b = append(b, errorLine("full access skips every permission prompt; the agent runs unattended"))
				break
			}
		}
		for _, r := range m.rows {
			if !r.On {
				continue
			}
			model := r.Model
			if model == "" {
				model = "default"
			}
			perm := r.Perm
			if perm == loops.PermFull {
				perm = errorStyle.Render(perm)
			}
			review = review || (r.Name == "testing-review" && r.Perm == loops.PermEdits)
			b = append(b, "  "+pad(r.Name, colW[0])+pad(r.Agent, colW[2])+pad(model, colW[3])+pad(r.Skill, colW[4])+perm)
		}
	}
	if shared || m.sharedPresent() {
		b = append(b, mutedStyle.Render("Update mattpocock later with: npx skills update -g"))
	}
	if review {
		b = append(b, mutedStyle.Render("testing-review uses edits so the reviewer can run gotodo to move the card."))
	}
	return strings.Join(b, "\n")
}

// packLines is one summary line per chosen pack, and whether the shared
// mattpocock install is among them.
func (m Model) packLines() (lines []string, shared bool) {
	byPack := map[string][]string{}
	for _, u := range units(m.checked, m.detected, m.d.Home) {
		a := strings.Join(u.Agents, ", ")
		if u.Needs != "" {
			shared = true
			a += mutedStyle.Render(" (one shared install)")
		}
		byPack[u.Pack] = append(byPack[u.Pack], a)
	}
	for _, p := range Packs {
		if len(byPack[p]) > 0 {
			lines = append(lines, p+" → "+strings.Join(byPack[p], ", "))
		}
	}
	return lines, shared
}

// sharedPresent reports whether mattpocock already sits in ~/.agents/skills.
func (m Model) sharedPresent() bool {
	for _, c := range m.cells {
		if isShared(c.Pack, c.Agent) && c.State == packInstalled {
			return true
		}
	}
	return false
}

// ---- apply ----

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepOK
	stepFailed
	stepSkipped
)

type step struct {
	Label  string
	State  stepState
	Detail string
	run    func() (stepState, string)
}

func (s step) result() string {
	word := map[stepState]string{stepPending: "not run", stepRunning: "running", stepOK: "ok", stepFailed: "failed", stepSkipped: "skipped"}[s.State]
	if s.Detail != "" {
		return word + " (" + s.Detail + ")"
	}
	return word
}

type stepDoneMsg struct {
	i      int
	state  stepState
	detail string
}

func (m Model) buildSteps() []step {
	var steps []step
	agentNames := map[string]string{"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode"}
	for _, a := range m.mcpChosen() {
		a := a
		env := m.d.MCP
		steps = append(steps, step{Label: "MCP · " + agentNames[a], run: func() (stepState, string) {
			res := agentsetup.InstallFor(env, []string{a})
			if len(res) == 0 {
				return stepSkipped, "unknown agent"
			}
			r := res[0]
			detail := r.Action
			if r.Detail != "" {
				detail += ": " + r.Detail
			}
			switch r.Action {
			case "failed":
				return stepFailed, r.Detail
			case "skipped":
				return stepSkipped, r.Detail
			}
			return stepOK, detail
		}})
	}
	for _, u := range units(m.checked, m.detected, m.d.Home) {
		u := u
		run, look := m.d.Run, m.d.LookPath
		steps = append(steps, step{Label: u.label(), run: func() (stepState, string) { return runUnit(u, run, look) }})
	}
	if m.loops && len(m.detected) > 0 {
		path := m.d.ConfigPath
		shown := tilde(path, m.d.Home)
		if m.writeConfig() {
			cfg := ConfigFromRows(m.rows)
			steps = append(steps, step{Label: "loops.json", run: func() (stepState, string) {
				if err := loops.WriteFile(path, cfg); err != nil {
					return stepFailed, err.Error()
				}
				return stepOK, "wrote " + shown
			}})
		} else {
			steps = append(steps, step{Label: "loops.json", State: stepSkipped, Detail: "kept existing " + shown})
		}
	}
	return steps
}

func (m Model) startApply() (tea.Model, tea.Cmd) {
	m.steps = m.buildSteps()
	return m.runFrom(0)
}

// runFrom starts the first runnable step at or after i, or finishes.
func (m Model) runFrom(i int) (tea.Model, tea.Cmd) {
	for ; i < len(m.steps); i++ {
		if m.steps[i].run == nil {
			continue
		}
		m.steps[i].State = stepRunning
		run := m.steps[i].run
		idx := i
		return m, func() tea.Msg {
			st, det := run()
			return stepDoneMsg{i: idx, state: st, detail: det}
		}
	}
	m.finished = true
	m.appliedAt = m.d.Now()
	return m, nil
}

func (m Model) stepDone(msg stepDoneMsg) (tea.Model, tea.Cmd) {
	if msg.i < 0 || msg.i >= len(m.steps) {
		return m, nil
	}
	m.steps = append([]step(nil), m.steps...)
	m.steps[msg.i].State, m.steps[msg.i].Detail = msg.state, msg.detail
	return m.runFrom(msg.i + 1)
}

func (m Model) applyView(w int) string {
	head := amberStyle.Render("Applying…")
	if m.finished {
		head = okStyle.Render("Done") + mutedStyle.Render(" · "+m.appliedAt.Format("02/01/2006 15:04"))
	}
	b := []string{head, ""}
	if len(m.steps) == 0 {
		b = append(b, mutedStyle.Render("Nothing to do: no agent or pack was chosen."))
	}
	for _, s := range m.steps {
		var mark, detail string
		switch s.State {
		case stepPending:
			mark, detail = mutedStyle.Render("·"), "waiting"
		case stepRunning:
			mark, detail = amberStyle.Render("…"), "running"
		case stepOK:
			mark, detail = okStyle.Render("✓"), s.Detail
		case stepFailed:
			mark, detail = errorStyle.Render("✗"), s.Detail
		case stepSkipped:
			mark, detail = amberStyle.Render("-"), s.Detail
		}
		b = append(b, mark+" "+pad(s.Label, 30)+mutedStyle.Render(trunc(detail, w-33)))
	}
	if m.finished && len(m.steps) > 0 {
		b = append(b, "", mutedStyle.Render(restartNote))
	}
	return strings.Join(b, "\n")
}

// Run shows the wizard full screen and returns the final model.
func Run(d Deps, o Options) (Model, error) {
	final, err := tea.NewProgram(New(d, o), tea.WithAltScreen()).Run()
	if err != nil {
		return Model{}, err
	}
	m, _ := final.(Model)
	return m, nil
}

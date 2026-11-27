package wizard

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

// crumbs are the steps in the header's stepper; existing-config counts as
// Stages and Apply comes after the last step.
var crumbs = []struct {
	label string
	scr   []screen
}{
	{"Welcome", []screen{scrWelcome}},
	{"MCP", []screen{scrMCP}},
	{"Loops", []screen{scrLoops}},
	{"Skills", []screen{scrPacks}},
	{"Stages", []screen{scrExisting, scrStages}},
	{"Summary", []screen{scrSummary}},
}

// Stage table columns.
const (
	colOn = iota
	colAgent
	colModel
	colSkill
	colPerm
	numCols
	fieldSource // the pack source input on the packs screen
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

// packItem is one cursor stop on the packs screen: the Recommended or Skip
// choice, an added skill source, or the add row.
type packItem struct {
	kind   int
	custom int // itemCustom: index into customs
}

const (
	itemRecommend = iota
	itemSkip
	itemContinue // shown instead of Recommend/Skip when nothing is missing
	itemCustom
	itemAdd
)

type existingConfig struct {
	Mod    time.Time
	Stages []string
	Err    error
}

// Model is the wizard's Bubble Tea model.
type Model struct {
	d Deps
	theme

	width, height int
	scr           screen
	back          []screen
	cur           map[screen]int // cursor per screen, kept on Back

	detected []string
	paths    map[string]string
	skills   map[string][]string        // user + project skills per agent
	proj     map[string]map[string]bool // project skills per agent
	npx      bool

	mcpOn map[string]bool

	loops       bool
	loopsPreset bool

	cells     []packCell
	checked   map[string]bool
	recommend bool // install the recommended packs wherever they are missing
	customs   []customPack
	src       *editor // pack source input, open on the packs screen
	srcErr    string

	existing *existingConfig
	replace  bool

	rows        []StageRow
	stageCol    int
	rowsTouched bool
	edit        *editor
	errMsg      string
	flash       string // one-shot warning, cleared by the next key

	steps     []step
	spin      spinner.Model
	frame     int // welcome animation step
	finished  bool
	appliedAt time.Time
	quit      bool
}

// New builds the wizard, detecting agents, skills and an existing config.
func New(d Deps, o Options) Model {
	if d.Cwd == "" {
		d.Cwd = d.Home
	}
	m := Model{
		d:       d,
		theme:   theme{plain: d.NoColor},
		cur:     map[screen]int{},
		skills:  map[string][]string{},
		proj:    map[string]map[string]bool{},
		mcpOn:   map[string]bool{},
		checked: map[string]bool{},
		spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(amberStyle)),
	}
	m.detected, m.paths = detectAgents(d.LookPath)
	_, err := d.LookPath("npx")
	m.npx = err == nil
	for _, a := range m.detected {
		var proj []string
		if d.ProjectSkills != nil {
			proj = d.ProjectSkills(a)
		}
		m.proj[a] = map[string]bool{}
		for _, s := range proj {
			m.proj[a][s] = true
		}
		m.skills[a] = union(d.Skills(a), proj)
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
	m.setRecommend(len(m.offered()) > 0)
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
// Init starts the welcome animation.
func (m Model) Init() tea.Cmd { return welcomeTick() }

// welcomeTickMsg advances the welcome board's demo card.
type welcomeTickMsg struct{}

// welcomeStep is how long the demo card stays in each column.
const welcomeStep = 1400 * time.Millisecond

func welcomeTick() tea.Cmd {
	return tea.Tick(welcomeStep, func(time.Time) tea.Msg { return welcomeTickMsg{} })
}

// available is what an agent can load now (user and project skills) plus
// what the chosen packs bring.
func (m Model) available(agent string) []string {
	lists := [][]string{m.skills[agent]}
	for _, p := range Packs {
		if m.checked[cellKey(p, agent)] {
			lists = append(lists, packProvides[p])
		}
	}
	for _, c := range m.customs {
		if c.Agents[agent] {
			lists = append(lists, c.Skills)
		}
	}
	return union(lists...)
}

// installed reports whether agent can load skill once the chosen packs are
// in; a plugin namespace ("pstack:poteto-mode") is ignored.
func (m Model) installed(agent, skill string) bool {
	if i := strings.LastIndex(skill, ":"); i >= 0 {
		skill = skill[i+1:]
	}
	return contains(m.available(agent), skill)
}

// writeConfig reports whether apply writes loops.json.
func (m Model) writeConfig() bool {
	return m.loops && len(m.detected) > 0 && (m.existing == nil || m.replace)
}

func (m Model) installUnits() []installUnit {
	return units(m.checked, m.detected, m.d.Home, m.customs)
}

// Update handles keys, window size, spinner ticks and finished apply steps.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case spinner.TickMsg:
		if m.scr != scrApply || m.finished {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case welcomeTickMsg:
		if m.scr != scrWelcome || m.quit {
			return m, nil // stop ticking once the user moves on
		}
		m.frame++
		return m, welcomeTick()
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
	m.flash = ""
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
	if m.src != nil {
		return m.sourceKey(msg)
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
		if m.scr == scrPacks {
			it, _ := m.packItem()
			switch it.kind {
			case itemAdd:
				m.openSource()
				return m, nil
			case itemRecommend, itemSkip:
				m.setRecommend(it.kind == itemRecommend)
			}
		}
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
		m.srcErr = ""
		items := m.packItems()
		m.list(k, len(items), func(i int) { m.togglePackItem(items[i]) })
		if (k == "d" || k == "delete" || k == "backspace") && m.cur[scrPacks] < len(items) {
			if it := items[m.cur[scrPacks]]; it.kind == itemCustom {
				m.customs = append(m.customs[:it.custom:it.custom], m.customs[it.custom+1:]...)
				m.cur[scrPacks] = min(m.cur[scrPacks], len(m.packItems())-1)
			}
		}
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

// offered are the pack cells the Recommended choice would install: packs
// an agent lacks and can install.
func (m Model) offered() []int {
	var out []int
	for i, c := range m.cells {
		if c.State == packMissing {
			out = append(out, i)
		}
	}
	return out
}

// setRecommend chooses Install recommended (every missing pack) or Skip.
func (m *Model) setRecommend(on bool) {
	m.recommend = on && len(m.offered()) > 0
	m.checked = map[string]bool{}
	if m.recommend {
		for _, i := range m.offered() {
			m.checked[cellKey(m.cells[i].Pack, m.cells[i].Agent)] = true
		}
	}
}

// packItems are the packs screen's cursor stops: Recommended and Skip (only
// when something is missing), each added source, then the add row.
func (m Model) packItems() []packItem {
	var items []packItem
	if len(m.offered()) > 0 {
		items = append(items, packItem{kind: itemRecommend}, packItem{kind: itemSkip})
	} else {
		// Something enter can always move on with; otherwise the add row
		// would be the only stop and enter would trap the user in it.
		items = append(items, packItem{kind: itemContinue})
	}
	for ci := range m.customs {
		items = append(items, packItem{kind: itemCustom, custom: ci})
	}
	return append(items, packItem{kind: itemAdd})
}

func (m Model) packItem() (packItem, bool) {
	items := m.packItems()
	if i := m.cur[scrPacks]; i < len(items) {
		return items[i], true
	}
	return packItem{}, false
}

func (m Model) onAddRow() bool {
	it, ok := m.packItem()
	return ok && it.kind == itemAdd
}

func (m *Model) togglePackItem(it packItem) {
	switch it.kind {
	case itemRecommend, itemSkip:
		m.setRecommend(it.kind == itemRecommend)
	case itemAdd:
		m.openSource()
	}
}

func (m *Model) openSource() {
	if !m.npx {
		m.srcErr = "adding a pack needs npx: install Node.js, then run gotodo setup again"
		return
	}
	m.srcErr = ""
	m.src = newEditor(fieldSource, "Source", "", "owner/repo, https://… or ./folder", nil)
	m.src.hint = "GitHub owner/repo, a git or https URL, or a folder with SKILL.md"
}

// sourceKey edits the pack source; enter validates and adds the pack for
// every detected agent, esc cancels.
func (m Model) sourceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	done, commit := m.src.update(msg)
	if !done {
		m.srcErr = ""
		return m, nil
	}
	if commit {
		c, err := parseSource(m.src.value(), m.d.Cwd, m.d.Home)
		if err == nil {
			for _, o := range m.customs {
				if o.Source == c.Source {
					err = fmt.Errorf("%s is already in the list", c.Label)
				}
			}
		}
		if err != nil {
			m.srcErr = err.Error()
			return m, nil
		}
		c.Agents = map[string]bool{}
		for _, a := range m.detected {
			c.Agents[a] = true
		}
		m.customs = append(append([]customPack(nil), m.customs...), c)
		m.cur[scrPacks] = len(m.packItems()) - 1 // stay on the add row
	}
	m.src, m.srcErr = nil, ""
	return m, nil
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
		m.edit.hint = "not installed for " + r.Agent + ": loops will skip until it is"
		m.edit.tags = map[string]string{}
		for s := range m.proj[r.Agent] {
			m.edit.tags[s] = "project"
		}
		return
	case colPerm:
		r.Perm = cycle(loops.Permissions, r.Perm)
		if r.Perm == loops.PermFull {
			m.flash = r.Name + " now has full access: it skips every permission prompt, unattended"
		}
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

func (m Model) mcpChosen() []string {
	var out []string
	for _, a := range m.detected {
		if m.mcpOn[a] {
			out = append(out, a)
		}
	}
	return out
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
	Group  string // "mcp", "pack" or "loops", for the done screen
	State  stepState
	Detail string
	Took   time.Duration
	start  time.Time
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

var agentNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode"}

func (m Model) buildSteps() []step {
	var steps []step
	for _, a := range m.mcpChosen() {
		a := a
		env := m.d.MCP
		steps = append(steps, step{Label: "MCP · " + agentNames[a], Group: "mcp", run: func() (stepState, string) {
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
	for _, u := range m.installUnits() {
		u := u
		run, look := m.d.Run, m.d.LookPath
		steps = append(steps, step{Label: u.label(), Group: "pack", run: func() (stepState, string) { return runUnit(u, run, look) }})
	}
	if m.loops && len(m.detected) > 0 {
		path := m.d.ConfigPath
		shown := tilde(path, m.d.Home)
		if m.writeConfig() {
			cfg := ConfigFromRows(m.rows)
			steps = append(steps, step{Label: "loops.json", Group: "loops", run: func() (stepState, string) {
				if err := loops.WriteFile(path, cfg); err != nil {
					return stepFailed, err.Error()
				}
				return stepOK, "wrote " + shown
			}})
		} else {
			steps = append(steps, step{Label: "loops.json", Group: "loops", State: stepSkipped, Detail: "kept existing " + shown})
		}
	}
	return steps
}

func (m Model) startApply() (tea.Model, tea.Cmd) {
	m.steps = m.buildSteps()
	next, cmd := m.runFrom(0)
	if m.d.Animate && cmd != nil {
		return next, tea.Batch(cmd, next.(Model).spin.Tick)
	}
	return next, cmd
}

// runFrom starts the first runnable step at or after i, or finishes.
func (m Model) runFrom(i int) (tea.Model, tea.Cmd) {
	for ; i < len(m.steps); i++ {
		if m.steps[i].run == nil {
			continue
		}
		m.steps[i].State = stepRunning
		m.steps[i].start = m.d.Now()
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
	s := &m.steps[msg.i]
	s.State, s.Detail = msg.state, msg.detail
	if !s.start.IsZero() {
		s.Took = m.d.Now().Sub(s.start)
	}
	return m.runFrom(msg.i + 1)
}

// Run shows the wizard full screen and returns the final model.
func Run(d Deps, o Options) (Model, error) {
	if d.NoColor {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	final, err := tea.NewProgram(New(d, o), tea.WithAltScreen()).Run()
	if err != nil {
		return Model{}, err
	}
	m, _ := final.(Model)
	return m, nil
}

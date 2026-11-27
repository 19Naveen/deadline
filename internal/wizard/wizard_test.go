package wizard

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/agentsetup"
	"gotodo/internal/loops"
)

type call struct {
	Dir  string
	Argv []string
}

// recorder stands in for every external command: pack installs (Run) and
// the claude CLI used for MCP registration (mcp).
type recorder struct {
	mu    sync.Mutex
	packs []call
	mcp   [][]string
	fail  map[string]string // argv joined → output; returned with an error
}

func (r *recorder) run(dir, name string, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	argv := append([]string{name}, args...)
	r.packs = append(r.packs, call{dir, argv})
	if out, ok := r.fail[strings.Join(argv, " ")]; ok {
		return out, errors.New("exit status 1")
	}
	return "ok", nil
}

func (r *recorder) mcpRun(name string, args ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcp = append(r.mcp, append([]string{filepath.Base(name)}, args...))
	if len(args) >= 2 && args[0] == "mcp" && args[1] == "get" {
		return "No MCP server found with name: gotodo", errors.New("exit status 1")
	}
	return "", nil
}

type fixture struct {
	home string
	rec  *recorder
	d    Deps
}

// newFixture fakes a machine with the given agent CLIs (plus npx when
// npx is true) and per-agent installed skills, inside a temp HOME.
func newFixture(t *testing.T, agents []string, skills map[string][]string, npx bool) *fixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	onPath := map[string]bool{}
	for _, a := range agents {
		onPath[a] = true
	}
	onPath["npx"] = npx
	look := func(name string) (string, error) {
		if onPath[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	rec := &recorder{fail: map[string]string{}}
	d := Deps{
		Cwd:      home,
		LookPath: look,
		Run:      rec.run,
		Skills:   func(a string) []string { return skills[a] },
		MCP: agentsetup.Env{Home: home, ConfigHome: filepath.Join(home, ".config"), Binary: "/opt/gotodo",
			LookPath: look, Run: rec.mcpRun},
		ConfigPath: filepath.Join(home, ".config", "gotodo", "loops.json"),
		Home:       home,
		Now:        func() time.Time { return time.Date(2026, 11, 26, 13, 5, 0, 0, time.UTC) },
	}
	return &fixture{home: home, rec: rec, d: d}
}

var keyTypes = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, " ": tea.KeySpace, "up": tea.KeyUp, "down": tea.KeyDown,
	"left": tea.KeyLeft, "right": tea.KeyRight, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
	"ctrl+c": tea.KeyCtrlC, "backspace": tea.KeyBackspace,
}

func keyMsg(k string) tea.KeyMsg {
	if t, ok := keyTypes[k]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

// press sends keys in order, running every command synchronously (apply
// steps) until none is left or the wizard quits.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, cmd := m.Update(keyMsg(k))
		m = next.(Model)
		m = drain(m, cmd)
	}
	return m
}

func drain(m Model, cmd tea.Cmd) Model {
	for cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			return m
		}
		next, c := m.Update(msg)
		m, cmd = next.(Model), c
	}
	return m
}

// typeText sends each rune as its own key.
func typeText(t *testing.T, m Model, s string) Model {
	for _, r := range s {
		m = press(t, m, string(r))
	}
	return m
}

func all3() []string { return []string{"claude", "codex", "opencode"} }

func TestNavigatesEveryScreen(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{}, true)
	m := New(f.d, Options{})
	if m.scr != scrWelcome {
		t.Fatalf("starts on %v", m.scr)
	}
	m = press(t, m, "enter")
	if m.scr != scrMCP {
		t.Fatalf("enter from welcome → %v, want MCP", m.scr)
	}
	m = press(t, m, "enter")
	if m.scr != scrLoops || m.cur[scrLoops] != 0 {
		t.Fatalf("MCP → %v cursor %d, want Loops defaulting to No", m.scr, m.cur[scrLoops])
	}
	m = press(t, m, "down", "enter")
	if m.scr != scrPacks || !m.loops {
		t.Fatalf("Loops yes → %v", m.scr)
	}
	if !m.recommend || m.cur[scrPacks] != 0 {
		t.Fatal("Install recommended should be the default when packs are missing")
	}
	m = press(t, m, "down", "enter") // Skip
	if m.scr != scrStages || m.recommend || len(m.checked) != 0 {
		t.Fatalf("Skip → %v recommend %v checked %v, want Stages with nothing chosen", m.scr, m.recommend, m.checked)
	}
	// Nothing installed: the enabled stages have no skill, so Next is refused.
	m = press(t, m, "enter")
	if m.scr != scrStages || !strings.Contains(m.errMsg, "indev needs a skill") {
		t.Fatalf("expected a missing-skill error, got screen %v err %q", m.scr, m.errMsg)
	}
	// indev is row 1; skill is the fourth column.
	m = press(t, m, "down", "right", "right", "right", " ")
	if m.edit == nil || m.edit.field != colSkill {
		t.Fatal("space on Skill should open the skill editor")
	}
	m = typeText(t, m, "my-skill")
	m = press(t, m, "enter")
	if m.rows[1].Skill != "my-skill" {
		t.Fatalf("free-text skill = %q", m.rows[1].Skill)
	}
	m = press(t, m, "down", " ")
	m = typeText(t, m, "reviewer")
	m = press(t, m, "enter", "enter")
	if m.scr != scrSummary {
		t.Fatalf("Stages → %v (err %q), want Summary", m.scr, m.errMsg)
	}
	m = press(t, m, "esc")
	if m.scr != scrStages || m.rows[1].Skill != "my-skill" {
		t.Fatalf("esc from Summary → %v, edits kept: %q", m.scr, m.rows[1].Skill)
	}
	m = press(t, m, "enter", "enter")
	if m.scr != scrApply || !m.finished {
		t.Fatalf("Summary enter → %v finished %v", m.scr, m.finished)
	}
	for _, s := range m.steps {
		if s.State != stepOK {
			t.Errorf("step %s: %s", s.Label, s.result())
		}
	}
	c, err := loops.ReadFile(f.d.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if c.Stages["indev"].Skill != "my-skill" || c.Stages["testing-review"].Skill != "reviewer" {
		t.Fatalf("written stages = %+v", c.Stages)
	}
	if len(f.rec.packs) != 0 {
		t.Fatalf("Skip was chosen, runner got %v", f.rec.packs)
	}
	if rep := strings.Join(m.Report(), "\n"); !strings.Contains(rep, "loops.json:") || !strings.Contains(rep, "Restart") {
		t.Fatalf("report = %s", rep)
	}
}

func TestBackAndQuit(t *testing.T) {
	f := newFixture(t, all3(), nil, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "esc")
	if m.scr != scrMCP {
		t.Fatalf("esc from Loops → %v", m.scr)
	}
	m = press(t, m, "esc", "esc")
	if !m.quit {
		t.Fatal("esc on Welcome should quit")
	}
	if got := m.Report(); len(got) != 1 || !strings.Contains(got[0], "nothing was changed") {
		t.Fatalf("cancel report = %v", got)
	}
}

func TestLoopsPreset(t *testing.T) {
	f := newFixture(t, all3(), nil, true)
	on, off := true, false
	if m := press(t, New(f.d, Options{Loops: &on}), "enter", "enter"); m.scr != scrPacks {
		t.Fatalf("-loops: MCP → %v, want Packs", m.scr)
	}
	if m := press(t, New(f.d, Options{Loops: &off}), "enter", "enter"); m.scr != scrSummary {
		t.Fatalf("-no-loops: MCP → %v, want Summary", m.scr)
	}
}

func TestNoAgentsSkipsLoops(t *testing.T) {
	f := newFixture(t, nil, nil, false)
	m := press(t, New(f.d, Options{}), "enter")
	if !strings.Contains(m.View(), "No coding agent CLI found") {
		t.Fatal("MCP screen should say no agents were found")
	}
	m = press(t, m, "enter")
	if m.scr != scrSummary {
		t.Fatalf("no agents: MCP → %v, want Summary", m.scr)
	}
	m = press(t, m, "enter")
	if !m.finished || len(m.steps) != 0 || configExists(f.d.ConfigPath) {
		t.Fatalf("nothing should run: steps %v", m.steps)
	}
}

func TestDefaultRows(t *testing.T) {
	avail := func(s map[string][]string) func(string) []string { return func(a string) []string { return s[a] } }
	rows := DefaultRows(all3(), avail(map[string][]string{"claude": {"code-review", "implement", "poteto-mode", "tdd"}}))
	got := map[string]StageRow{}
	for _, r := range rows {
		got[r.Name] = r
	}
	if r := got["indev"]; !r.On || r.Agent != "claude" || r.Model != "claude-sonnet-5-5" || r.Skill != "poteto-mode" || r.Perm != "edits" {
		t.Errorf("indev = %+v", r)
	}
	if r := got["testing-review"]; !r.On || r.Model != "claude-opus-5-5" || r.Skill != "code-review" || r.Perm != "edits" {
		t.Errorf("testing-review = %+v", r)
	}
	for _, n := range []string{"todo", "blocked", "shipped"} {
		if got[n].On {
			t.Errorf("%s should be off by default", n)
		}
	}
	if len(rows) != len(StageNames) {
		t.Errorf("rows = %d", len(rows))
	}

	rows = DefaultRows(all3(), avail(map[string][]string{"claude": {"implement"}}))
	if rows[1].Skill != "implement" || rows[2].Skill != "" {
		t.Errorf("implement only: indev %q review %q", rows[1].Skill, rows[2].Skill)
	}
	rows = DefaultRows(all3(), avail(map[string][]string{"claude": {"poteto-mode"}}))
	if rows[2].Skill != "poteto-mode" {
		t.Errorf("review falls back to poteto-mode, got %q", rows[2].Skill)
	}
	rows = DefaultRows([]string{"codex"}, avail(nil))
	if rows[1].Agent != "codex" || rows[1].Model != "" {
		t.Errorf("codex-only default = %+v", rows[1])
	}
	if DefaultRows(nil, avail(nil)) != nil {
		t.Error("no agents → no rows")
	}
}

func TestDefaultsCountChosenPacks(t *testing.T) {
	f := newFixture(t, []string{"claude"}, map[string][]string{}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	// Install recommended chooses pstack and mattpocock for claude.
	m = press(t, m, "enter")
	if m.scr != scrStages {
		t.Fatalf("→ %v", m.scr)
	}
	if m.rows[1].Skill != "poteto-mode" || m.rows[2].Skill != "code-review" {
		t.Fatalf("with recommended chosen: indev %q review %q", m.rows[1].Skill, m.rows[2].Skill)
	}
	// Back to Packs, Skip: nothing chosen, so no skill defaults.
	m = press(t, m, "esc", "down", "enter")
	if m.scr != scrStages || m.rows[1].Skill != "" || m.rows[2].Skill != "" {
		t.Fatalf("with Skip: %v indev %q review %q", m.scr, m.rows[1].Skill, m.rows[2].Skill)
	}
}

func TestHasPack(t *testing.T) {
	cases := []struct {
		pack   string
		skills []string
		want   bool
	}{
		{PackPstack, []string{"poteto-mode"}, true},
		{PackPstack, []string{"tdd"}, false},
		{PackMattpocock, []string{"tdd"}, true},
		{PackMattpocock, []string{"tdd", "poteto-mode"}, false}, // pstack's own tdd
		{PackMattpocock, []string{"poteto-mode", "tdd", "grilling"}, true},
		{PackMattpocock, []string{"implement"}, true},
		{PackMattpocock, nil, false},
	}
	for _, c := range cases {
		if got := HasPack(c.pack, c.skills); got != c.want {
			t.Errorf("HasPack(%s, %v) = %v", c.pack, c.skills, got)
		}
	}
}

func TestPacksOfferedOnlyWhenMissing(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{
		"claude":   {"poteto-mode", "tdd"},      // pstack yes; tdd is pstack's, so mattpocock missing
		"codex":    {"grilling", "code-review"}, // mattpocock via ~/.agents/skills
		"opencode": {"grilling", "poteto-mode"}, // both
	}, true)
	m := New(f.d, Options{})
	var offered []string
	for _, i := range m.offered() {
		offered = append(offered, cellKey(m.cells[i].Pack, m.cells[i].Agent))
	}
	if want := []string{"mattpocock/claude", "pstack/codex"}; !reflect.DeepEqual(offered, want) {
		t.Fatalf("offered = %v, want %v", offered, want)
	}
	m = press(t, m, "enter", "enter", "down", "enter")
	v := m.View()
	if !strings.Contains(v, "Install recommended") || !strings.Contains(v, "claude: mattpocock · codex: pstack") || !strings.Contains(v, "Skip") {
		t.Errorf("Install recommended should name what it adds:\n%s", v)
	}
	if n := strings.Count(v, "✓ pstack") + strings.Count(v, "✓ mattpocock"); n != 4 {
		t.Errorf("expected 4 installed packs, got %d:\n%s", n, v)
	}

	// Nothing missing: Recommend/Skip are hidden and Continue is focused,
	// so enter moves on instead of opening the add box (a past trap).
	f = newFixture(t, all3(), map[string][]string{
		"claude": {"poteto-mode", "grilling"}, "codex": {"poteto-mode", "grilling"}, "opencode": {"poteto-mode", "grilling"},
	}, true)
	m = press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	v = m.View()
	if m.recommend || len(m.packItems()) != 2 || m.onAddRow() || strings.Contains(v, "Install recommended") ||
		!strings.Contains(v, "Continue") || !strings.Contains(v, "Every agent has the recommended skills") {
		t.Errorf("nothing missing should offer Continue, not Recommend/Skip:\n%s", v)
	}
	m = press(t, m, "enter")
	if m.src != nil || m.scr != scrStages {
		t.Errorf("enter on Continue should go to Stages, got screen %v, add box open %v", m.scr, m.src != nil)
	}
}

func TestNpxMissingMarksSharedPack(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{}, false)
	m := New(f.d, Options{})
	for _, i := range m.offered() {
		if c := m.cells[i]; c.Pack == PackMattpocock && c.Agent != "claude" {
			t.Fatalf("%s offered without npx", cellKey(c.Pack, c.Agent))
		}
	}
	m = press(t, m, "enter", "enter", "down", "enter")
	if v := m.View(); !strings.Contains(v, "needs npx") || !strings.Contains(v, "! npx not found: install Node.js") {
		t.Fatalf("packs screen should flag npx:\n%s", v)
	}
	// Install recommended still runs, without the shared npx install.
	if !m.recommend || m.checked["mattpocock/codex"] || m.checked["mattpocock/opencode"] {
		t.Fatalf("recommended must leave the npx pack out: %v", m.checked)
	}
	m = press(t, m, "enter", "enter", "enter")
	if !m.finished {
		t.Fatalf("not finished on %v (err %q)", m.scr, m.errMsg)
	}
	for _, c := range f.rec.packs {
		if c.Argv[0] == "npx" {
			t.Errorf("npx is missing, yet ran %v", c.Argv)
		}
	}
	if len(f.rec.packs) == 0 {
		t.Error("the packs that need no npx should still install")
	}
}

func TestRunnerArgvPerPack(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	// Install recommended is the default when anything is missing; it
	// covers every missing pack, with one shared mattpocock install.
	if !m.recommend || !m.checked["mattpocock/opencode"] || !m.checked["pstack/claude"] {
		t.Fatalf("recommended should be chosen by default: %v %v", m.recommend, m.checked)
	}
	m = press(t, m, "enter", "enter", "enter")
	if !m.finished {
		t.Fatalf("not finished on %v (err %q)", m.scr, m.errMsg)
	}
	want := [][]string{
		{"claude", "plugin", "marketplace", "add", "michael-denyer/pstack-claude"},
		{"claude", "plugin", "install", "pstack@pstack-claude"},
		{"codex", "plugin", "marketplace", "add", "michael-denyer/pstack-claude"},
		{"codex", "plugin", "add", "pstack@pstack-claude"},
		{"opencode", "plugin", "add", "github:michael-denyer/pstack-claude#main"},
		{"claude", "plugin", "install", "mattpocock-skills@claude-plugins-official"},
		{"npx", "-y", "skills@latest", "add", "mattpocock/skills", "-g", "-a", "opencode", "-s", "*", "-y"},
	}
	var got [][]string
	for _, c := range f.rec.packs {
		got = append(got, c.Argv)
		if c.Dir != f.home {
			t.Errorf("%v ran in %q, want home", c.Argv, c.Dir)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runner argv:\n got %q\nwant %q", got, want)
	}
	if v := m.View(); !strings.Contains(v, "mattpocock → codex, opencode") {
		t.Errorf("apply view should show the shared install:\n%s", v)
	}
}

func TestAbortDuringApplyReportsSteps(t *testing.T) {
	f := newFixture(t, []string{"claude"}, map[string][]string{}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "enter")
	// Start apply but do not run the step command, then abort.
	next, cmd := m.Update(keyMsg("enter"))
	m = next.(Model)
	if cmd == nil || m.scr != scrApply {
		t.Fatal("apply should start a step")
	}
	m = press(t, m, "q")
	if m.quit {
		t.Fatal("q must not quit while steps run")
	}
	m = press(t, m, "ctrl+c")
	rep := strings.Join(m.Report(), "\n")
	if !m.quit || !strings.Contains(rep, "Aborted") || !strings.Contains(rep, "MCP · Claude Code") {
		t.Fatalf("abort report = %s", rep)
	}
}

func TestPackFailureDoesNotAbort(t *testing.T) {
	f := newFixture(t, []string{"claude"}, map[string][]string{}, true)
	f.rec.fail["claude plugin install pstack@pstack-claude"] = "Error: network down"
	f.rec.fail["claude plugin marketplace add michael-denyer/pstack-claude"] = "Marketplace already installed"
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter", "enter") // Install recommended
	m = press(t, m, "enter", "enter")
	if !m.finished {
		t.Fatal("wizard should finish despite a failed pack")
	}
	states := map[string]stepState{}
	for _, s := range m.steps {
		states[s.Label] = s.State
	}
	if states["pstack → claude"] != stepFailed || states["mattpocock → claude"] != stepOK || states["loops.json"] != stepOK {
		t.Fatalf("states = %v", states)
	}
	if !strings.Contains(strings.Join(m.Report(), "\n"), "network down") {
		t.Error("report should carry the failure output")
	}
}

func TestFullAccessWarnsOnlyWhenSwitchedTo(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode", "code-review"}}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter")
	if v := m.View(); strings.Contains(v, "spend tokens") || !strings.Contains(v, "Change them any time with gotodo loops or gotodo setup.") {
		t.Fatalf("loops screen should be neutral:\n%s", v)
	}
	m = press(t, m, "down", "enter", "enter")
	if strings.Contains(m.View(), "full access") {
		t.Fatal("no warning before choosing full")
	}
	// indev → Access column; edits → full.
	m = press(t, m, "down", "right", "right", "right", "right", " ")
	if m.rows[1].Perm != loops.PermFull {
		t.Fatalf("perm = %q", m.rows[1].Perm)
	}
	if v := m.View(); strings.Count(v, "full access") != 1 || !strings.Contains(v, "indev now has full access") {
		t.Fatalf("one warning right after switching to full:\n%s", v)
	}
	m = press(t, m, "up")
	if strings.Contains(m.View(), "full access") {
		t.Fatal("the next key clears the warning")
	}
	m = press(t, m, "enter")
	if v := m.View(); m.scr != scrSummary || strings.Contains(v, "full access") || strings.Contains(v, "unattended") {
		t.Fatalf("summary must not warn (the access tag says full):\n%s", v)
	}
	if v := m.View(); !strings.Contains(v, " full ") {
		t.Fatalf("summary should tag the stage full:\n%s", v)
	}
	// Cycling full → readonly → edits → full warns again, once each time.
	m = press(t, m, "esc", "down", " ")
	if m.rows[1].Perm != loops.PermReadOnly || strings.Contains(m.View(), "full access") {
		t.Fatal("cycling past full shows no warning")
	}
	m = press(t, m, " ", " ")
	if !strings.Contains(m.View(), "full access") {
		t.Fatal("switching to full again warns again")
	}
}

func TestAgentCycleResetsModel(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode"}, "codex": {"implement"}}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter", "down", "enter") // Skip
	m = press(t, m, "down", "right", " ")
	r := m.rows[1]
	if r.Agent != "codex" || r.Model != "" || r.Skill != "implement" {
		t.Fatalf("after cycling agent: %+v", r)
	}
	// Model editor offers claude's models once back on claude.
	m = press(t, m, " ", " ", "right", " ")
	if m.edit == nil || len(m.edit.matches) != 3 {
		t.Fatalf("claude model suggestions missing")
	}
	if m.edit.value() != "claude-sonnet-5-5" {
		t.Fatalf("editor should open on the current model, got %q", m.edit.value())
	}
	m = press(t, m, "tab", "enter")
	if m.rows[1].Model != "claude-opus-5-5" {
		t.Fatalf("picked model = %q", m.rows[1].Model)
	}
}

func writeExisting(t *testing.T, path string) []byte {
	t.Helper()
	c := loops.Config{Enabled: true, Stages: map[string]loops.Stage{"shipped": {Agent: "codex", Skill: "old"}}}
	if err := loops.WriteFile(path, c); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	return data
}

func TestExistingConfigKeep(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode"}}, true)
	before := writeExisting(t, f.d.ConfigPath)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter", "enter")
	if m.scr != scrExisting {
		t.Fatalf("→ %v, want existing-config choice", m.scr)
	}
	if v := m.View(); !strings.Contains(v, "stages: shipped") || !strings.Contains(v, "Keep it") {
		t.Fatalf("existing view:\n%s", v)
	}
	m = press(t, m, "enter")
	if m.scr != scrSummary || !strings.Contains(m.View(), "keep existing") {
		t.Fatalf("keep → %v", m.scr)
	}
	m = press(t, m, "enter")
	after, _ := os.ReadFile(f.d.ConfigPath)
	if !bytes.Equal(before, after) {
		t.Fatal("kept config was rewritten")
	}
}

func TestExistingConfigReplace(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode"}}, true)
	writeExisting(t, f.d.ConfigPath)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter", "enter", "down", "enter")
	if m.scr != scrStages {
		t.Fatalf("replace → %v", m.scr)
	}
	m = press(t, m, "enter")
	if !strings.Contains(m.View(), "replace ") {
		t.Fatal("summary should say replace")
	}
	m = press(t, m, "enter")
	c, err := loops.ReadFile(f.d.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, old := c.Stages["shipped"]; old || c.Stages["indev"].Skill != "poteto-mode" {
		t.Fatalf("replaced config = %+v", c.Stages)
	}
}

func TestConfigAndMCPWritten(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"code-review", "poteto-mode"}}, true)
	m := press(t, New(f.d, Options{}), "enter")
	m = press(t, m, "down", " ") // untick codex
	m = press(t, m, "enter", "down", "enter", "enter", "enter", "enter")
	if !m.finished {
		t.Fatalf("not finished: %v %q", m.scr, m.errMsg)
	}
	c, err := loops.ReadFile(f.d.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]loops.Stage{
		"indev":          {Agent: "claude", Model: "claude-sonnet-5-5", Skill: "poteto-mode", Permissions: "edits"},
		"testing-review": {Agent: "claude", Model: "claude-opus-5-5", Skill: "code-review", Permissions: "edits"},
	}
	if !reflect.DeepEqual(c.Stages, want) || !c.Enabled || c.MaxParallel != 2 || c.MaxRunsPerTask != 3 {
		t.Fatalf("config = %+v", c)
	}
	added := false
	for _, argv := range f.rec.mcp {
		if strings.Join(argv, " ") == "claude mcp add -s user gotodo -e GOTODO_AGENT=claude -- /opt/gotodo mcp" {
			added = true
		}
	}
	if !added {
		t.Errorf("claude mcp add not run: %v", f.rec.mcp)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".codex", "config.toml")); err == nil {
		t.Error("codex was unticked but its config was written")
	}
	if data, err := os.ReadFile(filepath.Join(f.home, ".config", "opencode", "opencode.json")); err != nil || !strings.Contains(string(data), "/opt/gotodo") {
		t.Errorf("opencode not registered: %v", err)
	}
	if v := m.View(); !strings.Contains(v, "26/11/2026 13:05") {
		t.Errorf("done screen should show DD/MM/YYYY time:\n%s", v)
	}
}

func TestNonInteractiveMCPOnly(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode"}}, true)
	var out bytes.Buffer
	if err := NonInteractive(f.d, Options{}, &out); err != nil {
		t.Fatal(err)
	}
	if configExists(f.d.ConfigPath) {
		t.Fatal("-yes without -loops must not write loops.json")
	}
	if len(f.rec.packs) != 0 {
		t.Fatalf("-yes must not install packs: %v", f.rec.packs)
	}
	if _, err := os.Stat(filepath.Join(f.home, ".codex", "config.toml")); err != nil {
		t.Error("codex MCP not registered")
	}
	if s := out.String(); !strings.Contains(s, "Claude Code: registered") || !strings.Contains(s, "loops:      off") {
		t.Fatalf("output:\n%s", s)
	}
}

func TestNonInteractiveLoops(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"implement"}}, true)
	on := true
	var out bytes.Buffer
	if err := NonInteractive(f.d, Options{Loops: &on}, &out); err != nil {
		t.Fatal(err)
	}
	c, err := loops.ReadFile(f.d.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	// testing-review has no suitable skill, so only indev is written.
	if len(c.Stages) != 1 || c.Stages["indev"].Skill != "implement" {
		t.Fatalf("stages = %+v\n%s", c.Stages, out.String())
	}
	if !strings.Contains(out.String(), "testing-review skipped") {
		t.Errorf("output should name the skipped stage:\n%s", out.String())
	}
	// A second run keeps the existing file.
	out.Reset()
	before, _ := os.ReadFile(f.d.ConfigPath)
	if err := NonInteractive(f.d, Options{Loops: &on}, &out); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(f.d.ConfigPath)
	if !bytes.Equal(before, after) || !strings.Contains(out.String(), "kept existing") {
		t.Fatalf("existing config not kept:\n%s", out.String())
	}
}

func TestFuzzy(t *testing.T) {
	opts := []string{"code-review", "poteto-mode", "review", "tdd", "thermo-nuclear-review"}
	got := fuzzy("rev", opts)
	if strings.Join(got, ",") != "review,code-review,thermo-nuclear-review" {
		t.Errorf("fuzzy(rev) = %v", got)
	}
	if got := fuzzy("ptm", opts); len(got) != 1 || got[0] != "poteto-mode" {
		t.Errorf("fuzzy(ptm) = %v", got)
	}
	if got := fuzzy("", opts); len(got) != len(opts) {
		t.Errorf("empty query keeps all: %v", got)
	}
}

// screensAt walks to every screen with the busiest content (five stages
// on, full access, an open editor) and returns each view.
func screensAt(t *testing.T, w, h int) map[string]string {
	f := newFixture(t, all3(), map[string][]string{"claude": {"code-review", "poteto-mode", "frontend-design", "impeccable", "ponytail-review"}}, true)
	writeExisting(t, f.d.ConfigPath)
	m := New(f.d, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(Model)
	views := map[string]string{"welcome": m.View()}
	m = press(t, m, "enter")
	views["mcp"] = m.View()
	m = press(t, m, "enter")
	views["loops"] = m.View()
	m = press(t, m, "down", "enter")
	m = toAddRow(t, m)
	m = press(t, m, "enter")
	m = typeText(t, m, "acme/agent-skills")
	views["source"] = m.View()
	m = press(t, m, "enter")
	views["packs"] = m.View()
	m = press(t, m, "up", "enter") // off the add row, then Next
	views["existing"] = m.View()
	m = press(t, m, "down", "enter")
	for i := 0; i < 5; i++ { // every stage on, with a skill and full access
		if !m.rows[i].On {
			m = press(t, m, " ")
		}
		m.rows[i].Skill = "poteto-mode"
		m.rows[i].Perm = loops.PermFull
		m = press(t, m, "down")
	}
	views["stages"] = m.View()
	m = press(t, m, "right", "right", "right", " ")
	views["editor"] = m.View()
	m = press(t, m, "esc", "enter")
	views["summary"] = m.View()
	m = press(t, m, "enter")
	views["apply"] = m.View()
	if m.scr != scrApply {
		t.Fatalf("did not reach apply: %v %q", m.scr, m.errMsg)
	}
	return views
}

func TestViewsFitTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 18}, {80, 24}, {120, 40}, {100, 30}} {
		w, h := size[0], size[1]
		for name, v := range screensAt(t, w, h) {
			lines := strings.Split(v, "\n")
			if len(lines) > h {
				t.Errorf("%dx%d %s: %d lines", w, h, name, len(lines))
			}
			for i, l := range lines {
				if lw := lipgloss.Width(l); lw > w {
					t.Errorf("%dx%d %s line %d is %d wide: %q", w, h, name, i, lw, l)
				}
			}
			last := lines[len(lines)-1]
			if !strings.Contains(last, "esc") && !strings.Contains(last, "Quit") && !strings.Contains(last, "Abort") {
				t.Errorf("%dx%d %s: footer not on the last line: %q", w, h, name, last)
			}
		}
	}
}

func TestStagesFitWithoutClipping(t *testing.T) {
	// At 80x18 the busiest stages screen keeps every row, the headers and
	// the access tags.
	v := screensAt(t, 80, 18)["stages"]
	for _, want := range append(append([]string{}, StageNames...), "STAGE", "AGENT", "MODEL", "SKILL", "ACCESS", " full ", "gotodo loops set") {
		if !strings.Contains(v, want) {
			t.Errorf("80x18 stages view lost %q:\n%s", want, v)
		}
	}
}

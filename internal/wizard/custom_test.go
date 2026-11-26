package wizard

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// toAddRow moves the packs cursor to the "+ add a skill pack…" row.
func toAddRow(t *testing.T, m Model) Model {
	t.Helper()
	for i := 0; i < len(m.packItems()); i++ {
		m = press(t, m, "down")
	}
	if !m.onAddRow() {
		t.Fatal("cursor did not reach the add row")
	}
	return m
}

func writeSkill(t *testing.T, parts ...string) {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// addPack types a source on the add row and presses enter.
func addPack(t *testing.T, m Model, source string) Model {
	t.Helper()
	m = press(t, toAddRow(t, m), "enter")
	if m.src == nil {
		t.Fatalf("enter on the add row should open the source input (err %q)", m.srcErr)
	}
	return press(t, typeText(t, m, source), "enter")
}

func TestParseSource(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "work")
	writeSkill(t, cwd, "my-pack", "skills", "alpha")
	writeSkill(t, cwd, "my-pack", "beta")
	os.MkdirAll(filepath.Join(cwd, "empty", "a", "b"), 0o755)
	os.WriteFile(filepath.Join(cwd, "file.txt"), []byte("x"), 0o644)
	writeSkill(t, cwd, "deep", "a", "b", "c")

	ok := []struct{ in, source, label string }{
		{"acme/agent-skills", "acme/agent-skills", "acme/agent-skills"},
		{" acme/agent-skills@v1.2 ", "acme/agent-skills@v1.2", "acme/agent-skills@v1.2"},
		{"https://github.com/acme/skills", "https://github.com/acme/skills", "https://github.com/acme/skills"},
		{"git@github.com:acme/skills.git", "git@github.com:acme/skills.git", "git@github.com:acme/skills.git"},
		{"./my-pack", filepath.Join(cwd, "my-pack"), "~/work/my-pack"},
		{"~/work/my-pack", filepath.Join(cwd, "my-pack"), "~/work/my-pack"},
		{filepath.Join(cwd, "my-pack"), filepath.Join(cwd, "my-pack"), "~/work/my-pack"},
		{"my-pack", filepath.Join(cwd, "my-pack"), "~/work/my-pack"}, // an existing folder, no slash
	}
	for _, c := range ok {
		got, err := parseSource(c.in, cwd, home)
		if err != nil || got.Source != c.source || got.Label != c.label {
			t.Errorf("parseSource(%q) = %+v, %v; want source %q label %q", c.in, got, err, c.source, c.label)
		}
	}
	local, _ := parseSource("./my-pack", cwd, home)
	if !local.Local || !reflect.DeepEqual(local.Skills, []string{"alpha", "beta"}) {
		t.Errorf("local pack = %+v", local)
	}
	bad := map[string]string{
		"":           "type a GitHub owner/repo",
		"./empty":    "no SKILL.md in ~/work/empty",
		"./deep":     "no SKILL.md",
		"./missing":  "no folder at ~/work/missing",
		"./file.txt": "is a file",
		"not a repo": "is not a GitHub owner/repo, a git URL or a folder",
	}
	for in, want := range bad {
		if _, err := parseSource(in, cwd, home); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseSource(%q) error = %v, want %q", in, err, want)
		}
	}
}

func TestCustomArgv(t *testing.T) {
	head := "npx -y skills@latest add acme/skills -g -a "
	cases := map[string][]string{
		"claude-code":          {"claude"},
		"codex":                {"codex"},
		"opencode":             {"opencode"},
		"opencode ":            {"codex", "opencode"}, // one shared install
		"claude-code opencode": {"claude", "codex", "opencode"},
		"claude-code codex":    {"claude", "codex"},
	}
	for ids, agents := range cases {
		want := head + strings.TrimSpace(ids) + " -s * -y"
		if got := strings.Join(customArgv("acme/skills", agents), " "); got != want {
			t.Errorf("customArgv(%v) = %q, want %q", agents, got, want)
		}
	}
}

func TestCustomPacksInstall(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{}, true)
	writeSkill(t, f.home, "packs", "mine", "skills", "house-style")
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	m = addPack(t, m, "acme/agent-skills")
	m = addPack(t, m, "~/packs/mine")
	if len(m.customs) != 2 || !m.onAddRow() {
		t.Fatalf("customs = %+v, cursor on add row %v", m.customs, m.onAddRow())
	}
	v := m.View()
	for _, want := range []string{"◆ acme/agent-skills", "◆ ~/packs/mine", "will install"} {
		if !strings.Contains(v, want) {
			t.Errorf("packs view lacks %q:\n%s", want, v)
		}
	}
	// A duplicate is refused and keeps the input open.
	m = addPack(t, m, "acme/agent-skills")
	if m.src == nil || !strings.Contains(m.View(), "already in the list") {
		t.Fatalf("duplicate should be refused:\n%s", m.View())
	}
	m = press(t, m, "esc")
	// Added skills install for every detected agent; Skip the recommended.
	m.cur[scrPacks] = 1
	f.rec.fail["npx -y skills@latest add acme/agent-skills -g -a claude-code opencode -s * -y"] = "Error: repo not found"
	m = press(t, m, "enter", "enter")
	// The local pack's skill counts as available once chosen.
	if !contains(m.available("codex"), "house-style") {
		t.Errorf("local pack skills should be suggested: %v", m.available("codex"))
	}
	m.rows[1].Skill, m.rows[2].Skill = "house-style", "house-style"
	m = press(t, m, "enter")
	if v := m.View(); !strings.Contains(v, "acme/agent-skills → claude, codex, opencode") || !strings.Contains(v, "~/packs/mine → claude, codex, opencode") {
		t.Errorf("summary should list custom packs:\n%s", v)
	}
	m = press(t, m, "enter")
	if !m.finished {
		t.Fatalf("not finished: %v %q", m.scr, m.errMsg)
	}
	var got []string
	for _, c := range f.rec.packs {
		got = append(got, strings.Join(c.Argv, " "))
		if c.Dir != f.home {
			t.Errorf("%v ran in %q, want home", c.Argv, c.Dir)
		}
	}
	want := []string{
		"npx -y skills@latest add acme/agent-skills -g -a claude-code opencode -s * -y",
		"npx -y skills@latest add " + filepath.Join(f.home, "packs", "mine") + " -g -a claude-code opencode -s * -y",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv:\n got %q\nwant %q", got, want)
	}
	states := map[string]stepState{}
	for _, s := range m.steps {
		states[s.Label] = s.State
	}
	if states["acme/agent-skills → claude, codex, opencode"] != stepFailed || states["~/packs/mine → claude, codex, opencode"] != stepOK || states["loops.json"] != stepOK {
		t.Fatalf("a failed pack must not stop the rest: %v", states)
	}
	if rep := strings.Join(m.Report(), "\n"); !strings.Contains(rep, "repo not found") {
		t.Errorf("report should carry the failure:\n%s", rep)
	}
}

func TestCustomPackRemoveAndErrors(t *testing.T) {
	f := newFixture(t, []string{"claude"}, map[string][]string{}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	m = addPack(t, m, "./nowhere")
	if m.src == nil || !strings.Contains(m.View(), "no folder at") {
		t.Fatalf("a missing folder is refused in place:\n%s", m.View())
	}
	m = typeText(t, m, "x")
	if m.srcErr != "" {
		t.Error("typing clears the error")
	}
	m = press(t, m, "esc")
	if m.src != nil || len(m.customs) != 0 {
		t.Fatal("esc cancels the input")
	}
	m = addPack(t, m, "acme/skills")
	m = press(t, m, "up", "d")
	if len(m.customs) != 0 {
		t.Fatalf("d removes the custom pack: %+v", m.customs)
	}
}

func TestAddPackNeedsNpx(t *testing.T) {
	f := newFixture(t, []string{"claude"}, map[string][]string{}, false)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	m = press(t, toAddRow(t, m), "enter")
	if m.src != nil || m.scr != scrPacks || !strings.Contains(m.View(), "needs npx") {
		t.Fatalf("without npx the add row explains instead of opening:\n%s", m.View())
	}
}

func TestOtherSkillsShown(t *testing.T) {
	f := newFixture(t, []string{"claude", "codex"}, map[string][]string{
		"claude": {"poteto-mode", "tdd", "principle-x", "frontend-design", "my-own"},
		"codex":  {"grilling"},
	}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	v := m.View()
	if !strings.Contains(v, "ALREADY ON THIS MACHINE") {
		t.Errorf("packs screen should list what each agent has:\n%s", v)
	}
	if !regexp.MustCompile(`claude +✓ pstack +· mattpocock +\+ 2 own: frontend-design, my-`).MatchString(v) {
		t.Errorf("claude should show 2 other skills:\n%s", v)
	}
	codex := regexp.MustCompile(`codex +· pstack +✓ mattpocock(.*)`).FindStringSubmatch(v)
	if codex == nil || strings.Contains(codex[1], "own") {
		t.Errorf("codex has no other skills:\n%s", v)
	}
	if got := otherSkills([]string{"poteto-mode", "principle-a", "implement", "mine"}); !reflect.DeepEqual(got, []string{"mine"}) {
		t.Errorf("otherSkills = %v", got)
	}
}

func TestProjectSkillsSuggested(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode"}}, true)
	f.d.ProjectSkills = func(a string) []string {
		if a == "claude" {
			return []string{"house-review"}
		}
		return nil
	}
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter", "enter")
	if !contains(m.available("claude"), "house-review") || contains(m.available("codex"), "house-review") {
		t.Fatalf("available: claude %v codex %v", m.available("claude"), m.available("codex"))
	}
	if v := m.View(); !strings.Contains(v, "1 own: house-review") && !strings.Contains(m.packsViewText(), "house-review") {
		t.Log(v)
	}
	m = press(t, m, "down", "right", "right", "right", " ")
	if m.edit == nil || !strings.Contains(m.View(), "house-review (project)") {
		t.Fatalf("skill editor should mark project skills:\n%s", m.View())
	}
	if v := m.View(); !strings.Contains(v, "poteto-mode") || strings.Contains(v, "poteto-mode (project)") {
		t.Errorf("user skills carry no project tag:\n%s", v)
	}
}

// packsViewText renders the packs body regardless of the current screen.
func (m Model) packsViewText() string {
	_, _, b := m.packsView(74)
	return strings.Join(b, "\n")
}

func TestStageSkillNotInstalledNote(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode", "code-review"}}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter", "enter")
	m = press(t, m, "right", "right", "right", " ") // todo has no skill yet
	m = typeText(t, m, "zzz-custom")
	if !strings.Contains(m.View(), "not installed for claude: loops will skip until it is") {
		t.Fatalf("editor should note a missing skill:\n%s", m.View())
	}
	m = press(t, m, "enter")
	v := m.View()
	if !strings.Contains(v, "zzz-custom is not installed for claude: loops will skip until it is") || strings.Contains(v, "! ") {
		t.Fatalf("a muted note, not an error:\n%s", v)
	}
	m = press(t, m, "enter")
	if m.scr != scrSummary {
		t.Fatalf("a missing skill must not block: %v %q", m.scr, m.errMsg)
	}
}

func TestFindProject(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, "code", "app")
	os.MkdirAll(filepath.Join(proj, ".git"), 0o755)
	os.MkdirAll(filepath.Join(proj, "src", "pkg"), 0o755)
	if got := FindProject(filepath.Join(proj, "src", "pkg"), home); got != proj {
		t.Errorf("FindProject = %q, want %q", got, proj)
	}
	os.MkdirAll(filepath.Join(home, ".deadline"), 0o755)
	if got := FindProject(filepath.Join(home, "code"), home); got != "" {
		t.Errorf("home is never a project, got %q", got)
	}
	board := filepath.Join(home, "notes")
	os.MkdirAll(filepath.Join(board, ".deadline"), 0o755)
	if got := FindProject(board, home); got != board {
		t.Errorf(".deadline marks a project: %q", got)
	}
}

func TestStepperStates(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{}, true)
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	h := m.header(80)
	for _, want := range []string{"✓ Welcome", "✓ MCP", "✓ Loops", "● Skills", "○ Stages", "○ Summary", "◆ gotodo"} {
		if !strings.Contains(h, want) {
			t.Errorf("stepper lacks %q: %s", want, h)
		}
	}
	// Loops off skips Packs and Stages.
	m = press(t, New(f.d, Options{}), "enter", "enter", "enter")
	if h := m.header(80); !strings.Contains(h, "– Skills") || !strings.Contains(h, "– Stages") || !strings.Contains(h, "● Summary") {
		t.Errorf("skipped steps should show –: %s", h)
	}
	if h := m.header(50); lipglossWidthMax(h) > 50 {
		t.Errorf("narrow header overflows: %q", h)
	}
}

func TestNoColorUsesBracketTags(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode", "code-review"}}, true)
	f.d.NoColor = true
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter")
	m = addPack(t, m, "acme/agent-skills")
	if v := m.View(); !strings.Contains(v, "◆ acme/agent-skills") || !strings.Contains(v, "[will install]") || strings.Contains(v, "\x1b[") {
		t.Errorf("NO_COLOR packs should use bracket tags:\n%s", v)
	}
	m = press(t, m, "up", "enter") // the added skill's row, then Next
	if v := m.View(); !strings.Contains(v, "[edits]") || strings.Contains(v, "\x1b[") {
		t.Errorf("NO_COLOR stages:\n%s", v)
	}
}

func TestApplyAndDoneScreens(t *testing.T) {
	f := newFixture(t, all3(), map[string][]string{"claude": {"poteto-mode", "code-review"}}, true)
	// Install recommended (the default): claude mattpocock, codex and
	// opencode pstack, one shared codex+opencode mattpocock.
	m := press(t, New(f.d, Options{}), "enter", "enter", "down", "enter", "enter", "enter")
	next, cmd := m.Update(keyMsg("enter")) // start applying, run nothing yet
	m = next.(Model)
	v := m.View()
	if m.finished || !strings.Contains(v, "Applying…") || !strings.Contains(v, "0/8") || !strings.Contains(v, "running") {
		t.Fatalf("apply screen:\n%s", v)
	}
	m = drain(m, cmd)
	v = m.View()
	for _, want := range []string{"✓ gotodo is set up", "WHAT CHANGED", "MCP server", "✓ Claude Code", "✓ mattpocock → claude", "✓ mattpocock → codex, opencode",
		"wrote ~/.config/gotodo/loops.json", "TRY NEXT", "gotodo loops", "gotodo help", "26/11/2026 13:05"} {
		if !strings.Contains(v, want) {
			t.Errorf("done screen lacks %q:\n%s", want, v)
		}
	}
}

func lipglossWidthMax(s string) int {
	w := 0
	for _, l := range strings.Split(s, "\n") {
		w = max(w, len([]rune(l)))
	}
	return w
}

func TestViewsFitInColour(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	for _, size := range [][2]int{{80, 18}, {80, 24}} {
		for name, v := range screensAt(t, size[0], size[1]) {
			if !strings.Contains(v, "\x1b[") {
				t.Fatalf("%s rendered without colour", name)
			}
			lines := strings.Split(v, "\n")
			if len(lines) > size[1] {
				t.Errorf("%dx%d %s: %d lines", size[0], size[1], name, len(lines))
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w > size[0] {
					t.Errorf("%dx%d %s line %d is %d wide", size[0], size[1], name, i, w)
				}
			}
		}
	}
}

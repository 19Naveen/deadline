package agentsetup

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const testSnippet = "<!-- deadline:begin - managed -->\n## GoTodo\nuse gotodo\n<!-- deadline:end -->\n"

func wiredEnv(t *testing.T, installed ...string) Env {
	t.Helper()
	e, _ := testEnv(t, installed...)
	e.Assets = Assets{Skill: []byte("---\nname: gotodo-task-board\n---\n"), Snippet: []byte(testSnippet)}
	return e
}

// files lists every regular file under root, relative and sorted.
func files(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func TestWireIsIdempotentAndUnwireCleans(t *testing.T) {
	e := wiredEnv(t, "claude", "codex", "opencode")
	// A legacy skill from an old installer is replaced, not kept beside.
	os.MkdirAll(filepath.Join(e.Home, ".claude", "skills", "deadline"), 0o755)
	os.WriteFile(filepath.Join(e.Home, ".claude", "skills", "deadline", "SKILL.md"), []byte("old"), 0o644)

	first := Wire(e)
	for _, r := range first {
		if r.Action == "failed" || r.Action == "skipped" {
			t.Errorf("first wire: %s", r)
		}
	}
	want := []string{
		".agents/skills/gotodo-task-board/SKILL.md",
		".claude/CLAUDE.md",
		".claude/skills/gotodo-task-board/SKILL.md",
		".codex/AGENTS.md",
		".codex/skills/gotodo-task-board/SKILL.md",
		".config/opencode/skills/gotodo-task-board/SKILL.md",
	}
	if got := files(t, e.Home); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("files after wire:\n%s", strings.Join(got, "\n"))
	}
	for _, r := range Wire(e) {
		if r.Action != "already installed" {
			t.Errorf("second wire should change nothing: %s", r)
		}
	}
	md, _ := os.ReadFile(filepath.Join(e.Home, ".claude", "CLAUDE.md"))
	if string(md) != testSnippet {
		t.Errorf("CLAUDE.md = %q", md)
	}

	for _, r := range Unwire(e) {
		if r.Action != "removed" {
			t.Errorf("unwire: %s", r)
		}
	}
	if got := files(t, e.Home); strings.Join(got, "\n") != ".claude/CLAUDE.md\n.codex/AGENTS.md" {
		t.Fatalf("files after unwire: %v", got)
	}
	if md, _ := os.ReadFile(filepath.Join(e.Home, ".claude", "CLAUDE.md")); len(md) != 0 {
		t.Errorf("CLAUDE.md after unwire = %q", md)
	}
	for _, r := range Unwire(e) {
		if r.Action != "not installed" {
			t.Errorf("second unwire: %s", r)
		}
	}
}

// Only agents on the machine get files: no ~/.codex for a Claude-only user.
func TestWireOnlyTouchesPresentAgents(t *testing.T) {
	e := wiredEnv(t, "claude")
	rs := Wire(e)
	if got := files(t, e.Home); strings.Join(got, "\n") != ".claude/CLAUDE.md\n.claude/skills/gotodo-task-board/SKILL.md" {
		t.Fatalf("claude only wired: %v", got)
	}
	for _, p := range []string{".codex", ".agents", ".config/opencode"} {
		if exists(filepath.Join(e.Home, p)) {
			t.Errorf("%s created for an absent agent", p)
		}
	}
	if s := strings.Join(resultLines(rs), "\n"); !strings.Contains(s, "Codex skill:") || !strings.Contains(s, "codex not found") {
		t.Errorf("absent agents should be reported skipped:\n%s", s)
	}

	// A home dir alone counts as present (the CLI may live off PATH).
	os.MkdirAll(filepath.Join(e.Home, ".codex"), 0o755)
	if r := InstallSkill(e, "codex"); r.Action != "installed" {
		t.Errorf("codex with ~/.codex: %s", r)
	}
	// Nothing on the machine at all: nothing written.
	e = wiredEnv(t)
	Wire(e)
	if got := files(t, e.Home); len(got) != 0 {
		t.Errorf("no agents, yet wrote %v", got)
	}
}

func resultLines(rs []Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.String())
	}
	return out
}

func TestInstructionBlockKeepsSurroundings(t *testing.T) {
	e := wiredEnv(t, "codex")
	path := filepath.Join(e.Home, ".codex", "AGENTS.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	old := "<!-- deadline:begin - old -->\nstale rules\n<!-- deadline:end -->\n"
	before := "# Mine\n\nkeep this\n\n"
	after := "\n## Later\nand this" // no trailing newline
	os.WriteFile(path, []byte(before+old+after), 0o600)

	if r := InstallInstructions(e, "codex"); r.Action != "updated" {
		t.Fatalf("install: %s", r)
	}
	data, _ := os.ReadFile(path)
	if string(data) != before+after+"\n"+testSnippet {
		t.Fatalf("AGENTS.md = %q", data)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode changed to %v", st.Mode().Perm())
	}
	if r := InstallInstructions(e, "codex"); r.Action != "already installed" {
		t.Errorf("reinstall: %s", r)
	}
	RemoveInstructions(e, "codex")
	if data, _ := os.ReadFile(path); string(data) != before+after+"\n" {
		t.Errorf("after remove = %q", data)
	}

	// An unterminated block is left alone rather than eating the file.
	broken := "mine\n<!-- deadline:begin -->\nrest of my notes\n"
	os.WriteFile(path, []byte(broken), 0o600)
	if r := InstallInstructions(e, "codex"); r.Action != "failed" {
		t.Errorf("unterminated block: %s", r)
	}
	if data, _ := os.ReadFile(path); string(data) != broken {
		t.Errorf("unterminated block rewrote the file: %q", data)
	}
}

func TestWireRespectsCodexHomeAndDryRun(t *testing.T) {
	e := wiredEnv(t, "codex")
	alt := filepath.Join(e.Home, "alt-codex")
	t.Setenv("CODEX_HOME", alt)
	e.DryRun = true
	for _, r := range Wire(e) {
		if strings.HasPrefix(r.Agent, "Codex") && !strings.HasPrefix(r.Action, "would be") {
			t.Errorf("dry run: %s", r)
		}
	}
	if got := files(t, e.Home); len(got) != 0 {
		t.Fatalf("dry run wrote %v", got)
	}
	e.DryRun = false
	Wire(e)
	for _, p := range []string{"alt-codex/AGENTS.md", "alt-codex/skills/gotodo-task-board/SKILL.md"} {
		if !exists(filepath.Join(e.Home, p)) {
			t.Errorf("CODEX_HOME not used: %s missing", p)
		}
	}
	if exists(filepath.Join(e.Home, ".codex")) {
		t.Error("~/.codex created despite CODEX_HOME")
	}
	if got := Connected(e); strings.Join(got, ",") != "Codex" {
		t.Errorf("connected = %v", got)
	}
}

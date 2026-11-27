package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupRejectsConflictingLoopFlags(t *testing.T) {
	err := runSetup([]string{"-loops", "-no-loops"})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("err = %v", err)
	}
	if err := runSetup([]string{"extra"}); err == nil {
		t.Fatal("stray argument should fail")
	}
	for _, args := range [][]string{{"-no-agents", "-remove"}, {"-no-agents", "-loops"}} {
		if err := runSetup(args); err == nil || !strings.Contains(err.Error(), "conflict") {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

// fakeHome points HOME, XDG and CODEX_HOME at a temp dir with Claude Code
// and Codex dirs present, and empties PATH so no real agent CLI can run.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("PATH", t.TempDir())
	for _, d := range []string{".claude", ".codex", ".config/opencode"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func homeFiles(home string) []string {
	var out []string
	filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(home, p)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

func TestSetupNoAgentsWritesNothing(t *testing.T) {
	home := fakeHome(t)
	for _, args := range [][]string{{"-no-agents"}, {"-no-agents", "-yes"}, {"-no-agents", "-yes", "-no-loops"}} {
		if err := runSetup(args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if got := homeFiles(home); len(got) != 0 {
		t.Fatalf("-no-agents wrote %v", got)
	}
}

// -yes connects the agents that are present; -remove takes everything
// back out and leaves the user's own text.
func TestSetupYesThenRemove(t *testing.T) {
	home := fakeHome(t)
	md := filepath.Join(home, ".claude", "CLAUDE.md")
	os.WriteFile(md, []byte("# my rules\n"), 0o644)
	if err := runSetup([]string{"-yes", "-no-loops"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".claude/skills/gotodo-task-board/SKILL.md", ".codex/AGENTS.md", ".codex/config.toml",
		".agents/skills/gotodo-task-board/SKILL.md", ".config/opencode/opencode.json"} {
		if _, err := os.Stat(filepath.Join(home, p)); err != nil {
			t.Errorf("-yes did not write %s", p)
		}
	}
	data, _ := os.ReadFile(md)
	if !strings.HasPrefix(string(data), "# my rules\n<!-- deadline:begin") {
		t.Errorf("CLAUDE.md = %q", data)
	}
	skill, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "gotodo-task-board", "SKILL.md"))
	if string(skill) != string(skillMD) || len(skillMD) == 0 {
		t.Error("installed skill is not the embedded one")
	}

	if err := runSetup([]string{"-remove"}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(md); string(data) != "# my rules\n" {
		t.Errorf("CLAUDE.md after remove = %q", data)
	}
	for _, p := range homeFiles(home) {
		data, _ := os.ReadFile(filepath.Join(home, p))
		if strings.Contains(p, "gotodo-task-board") || strings.Contains(string(data), "gotodo") {
			t.Errorf("-remove left %s: %q", p, data)
		}
	}
}

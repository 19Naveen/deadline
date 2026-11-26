package loops

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInstalledSkillsFindsDirsAndPlugins(t *testing.T) {
	home := t.TempDir()
	mk := func(parts ...string) {
		dir := filepath.Join(append([]string{home}, parts...)...)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\n"), 0o644)
	}
	mk(".claude", "skills", "verify")
	mk(".claude", "plugins", "cache", "pstack", "pstack", "1.2.1", "skills", "poteto-mode")
	mk(".claude", "plugins", "cache", "mp", "mattpocock-skills", "1.3.1", "skills", "engineering", "tdd")
	t.Setenv("CODEX_HOME", "")
	got := installedSkillsIn("claude", home, filepath.Join(home, ".config"))
	if want := []string{"poteto-mode", "tdd", "verify"}; !reflect.DeepEqual(got, want) {
		t.Errorf("claude skills = %v, want %v", got, want)
	}
	if got := installedSkillsIn("codex", home, filepath.Join(home, ".config")); len(got) != 0 {
		t.Errorf("codex skills = %v, want none", got)
	}
}

func TestInstalledSkillsFindsOpenCodeGitPlugins(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".cache", "opencode", "npm", "git-pstack-x", "123", "node_modules", "pstack", "plugins", "pstack", "skills", "poteto-mode")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644)
	if got := installedSkillsIn("opencode", home, filepath.Join(home, ".config")); len(got) != 1 || got[0] != "poteto-mode" {
		t.Errorf("opencode skills = %v", got)
	}
}

func mkSkill(t *testing.T, parts ...string) {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectSkillsPerAgent(t *testing.T) {
	root := t.TempDir()
	mkSkill(t, root, ".claude", "skills", "x")
	mkSkill(t, root, ".agents", "skills", "shared")
	mkSkill(t, root, ".codex", "skills", "cx")
	mkSkill(t, root, ".opencode", "skills", "group", "oc")
	cases := map[string][]string{
		"claude":   {"x"},
		"codex":    {"cx", "shared"},
		"opencode": {"oc", "shared", "x"},
	}
	for agent, want := range cases {
		if got := ProjectSkills(agent, root); !reflect.DeepEqual(got, want) {
			t.Errorf("ProjectSkills(%s) = %v, want %v", agent, got, want)
		}
	}
	if got := ProjectSkills("claude", ""); got != nil {
		t.Errorf("no project root → %v", got)
	}
}

func TestInstalledSkillsForAndHasSkillIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", "")
	mkSkill(t, home, ".claude", "skills", "user-skill")
	root := t.TempDir()
	mkSkill(t, root, ".claude", "skills", "x")
	if got, want := InstalledSkillsFor("claude", root), []string{"user-skill", "x"}; !reflect.DeepEqual(got, want) {
		t.Errorf("InstalledSkillsFor = %v, want %v", got, want)
	}
	if got := InstalledSkillsFor("claude", ""); !reflect.DeepEqual(got, []string{"user-skill"}) {
		t.Errorf("without a project = %v", got)
	}
	if !HasSkillIn("claude", "x", root) || !HasSkillIn("claude", "plug:x", root) || !HasSkillIn("claude", "user-skill", root) {
		t.Error("HasSkillIn should find project and user skills")
	}
	if HasSkill("claude", "x") {
		t.Error("HasSkill must not look in a project")
	}
	if HasSkillIn("codex", "x", root) || HasSkillIn("claude", "nope", root) {
		t.Error("HasSkillIn found a skill the agent cannot load")
	}
}

func TestSkillsIn(t *testing.T) {
	pack := t.TempDir()
	mkSkill(t, pack, "skills", "a")
	mkSkill(t, pack, "b")
	mkSkill(t, pack, "x", "y", "too-deep")
	if got := SkillsIn(pack); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("SkillsIn = %v", got)
	}
	single := filepath.Join(t.TempDir(), "solo")
	mkSkill(t, single)
	if got := SkillsIn(single); !reflect.DeepEqual(got, []string{"solo"}) {
		t.Errorf("a folder that is a skill = %v", got)
	}
}

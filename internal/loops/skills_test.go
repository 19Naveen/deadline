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

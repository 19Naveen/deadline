package loops

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRoundTripAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loops.json")
	c := Config{Enabled: true, Stages: map[string]Stage{
		"indev": {Agent: "claude", Model: "claude-sonnet-5-5", Skill: "implement"},
	}}
	if err := WriteFile(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(path)
	if err != nil || got.Stages["indev"].Skill != "implement" || got.Stages["indev"].Perm() != PermEdits {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	bad := Config{Stages: map[string]Stage{"indev": {Agent: "gpt", Skill: "x"}}}
	if err := WriteFile(path, bad); err == nil {
		t.Error("unknown agent accepted")
	}
	if err := (Config{Stages: map[string]Stage{"indev": {Agent: "codex"}}}).Validate(); err == nil {
		t.Error("stage without skill or prompt accepted")
	}
}

func TestProjectOverridesGlobal(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("HOME", cfg)
	global, _ := GlobalPath()
	WriteFile(global, Config{Enabled: true, Stages: map[string]Stage{"indev": {Agent: "codex", Skill: "global"}}})
	board := filepath.Join(t.TempDir(), ".deadline", "board.json")
	c, from, err := Load(board)
	if err != nil || from != global || c.Stages["indev"].Skill != "global" {
		t.Fatalf("global: %+v %s %v", c, from, err)
	}
	WriteFile(ProjectPath(board), Config{Enabled: true, Stages: map[string]Stage{"testing-review": {Agent: "claude", Skill: "verify"}}})
	c, from, _ = Load(board)
	if from != ProjectPath(board) || len(c.Stages) != 1 || c.Stages["testing-review"].Skill != "verify" {
		t.Fatalf("project override: %+v %s", c, from)
	}
}

func TestPromptTemplate(t *testing.T) {
	p := Stage{Skill: "verify"}.PromptFor("3fa1c9e2", "Fix parser", "testing-review", "/b.json")
	for _, want := range []string{"verify", "3fa1c9e2", "Fix parser", "testing-review", "gotodo context 3fa1c9e2"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q: %s", want, p)
		}
	}
}

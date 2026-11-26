package agentsetup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallForOnlyNamedAgents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	var ran [][]string
	e := Env{
		Home: home, ConfigHome: filepath.Join(home, ".config"), Binary: "/opt/gotodo",
		LookPath: func(n string) (string, error) { return "/usr/bin/" + n, nil },
		Run: func(n string, a ...string) (string, error) {
			ran = append(ran, append([]string{n}, a...))
			return "No MCP server found", nil
		},
	}
	res := InstallFor(e, []string{"opencode", "bogus"})
	if len(res) != 1 || res[0].Agent != "OpenCode" || res[0].Action != "registered" {
		t.Fatalf("results = %v", res)
	}
	if len(ran) != 0 {
		t.Fatalf("claude CLI should not run: %v", ran)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); err == nil {
		t.Fatal("codex was not asked for but got configured")
	}
	if got := InstallFor(e, []string{"codex", "claude"}); len(got) != 2 || got[0].Agent != "Claude Code" || got[1].Agent != "Codex" {
		t.Fatalf("order = %v", got)
	}
}

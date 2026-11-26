package agentsetup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeClaude records registrations like the real CLI's user scope.
type fakeClaude struct{ registered string }

func (f *fakeClaude) run(name string, args ...string) (string, error) {
	switch strings.Join(args[:2], " ") {
	case "mcp get":
		if f.registered == "" {
			return `No MCP server named "gotodo".`, errors.New("exit 1")
		}
		return "gotodo:\n  Command: " + f.registered, nil
	case "mcp add":
		f.registered = args[len(args)-2] + " " + args[len(args)-1]
		return "", nil
	case "mcp remove":
		f.registered = ""
		return "", nil
	}
	return "", errors.New("unexpected " + strings.Join(args, " "))
}

func testEnv(t *testing.T, installed ...string) (Env, *fakeClaude) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CODEX_HOME", "")
	f := &fakeClaude{}
	have := map[string]bool{}
	for _, n := range installed {
		have[n] = true
	}
	return Env{
		Home: home, ConfigHome: filepath.Join(home, ".config"), Binary: "/opt/bin/gotodo",
		LookPath: func(n string) (string, error) {
			if have[n] {
				return "/usr/bin/" + n, nil
			}
			return "", errors.New("not found")
		},
		Run: f.run,
	}, f
}

func actions(rs []Result) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Agent+"="+r.Action)
	}
	return strings.Join(out, ", ")
}

func TestInstallIsIdempotentAndRemoveCleans(t *testing.T) {
	e, f := testEnv(t, "claude", "codex", "opencode")
	toml := filepath.Join(e.Home, ".codex", "config.toml")
	os.MkdirAll(filepath.Dir(toml), 0o755)
	os.WriteFile(toml, []byte("[tui]\nscreen_reader_detection_done = true\n"), 0o600)
	oc := filepath.Join(e.ConfigHome, "opencode", "opencode.json")
	os.MkdirAll(filepath.Dir(oc), 0o755)
	os.WriteFile(oc, []byte(`{"theme":"dark","mcp":{"other":{"type":"local","command":["x"]}}}`), 0o644)

	if got := actions(Install(e)); got != "Claude Code=registered, Codex=registered, OpenCode=registered" {
		t.Fatalf("first install: %s", got)
	}
	if f.registered != "/opt/bin/gotodo mcp" {
		t.Errorf("claude registered %q", f.registered)
	}
	if got := actions(Install(e)); got != "Claude Code=already registered, Codex=already registered, OpenCode=already registered" {
		t.Fatalf("second install: %s", got)
	}

	data, _ := os.ReadFile(toml)
	if !strings.Contains(string(data), "screen_reader_detection_done = true") || strings.Count(string(data), "[mcp_servers.gotodo]") != 1 {
		t.Errorf("codex config:\n%s", data)
	}
	if st, _ := os.Stat(toml); st.Mode().Perm() != 0o600 {
		t.Errorf("codex config mode changed to %v", st.Mode().Perm())
	}
	var cfg map[string]any
	data, _ = os.ReadFile(oc)
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	servers := cfg["mcp"].(map[string]any)
	if cfg["theme"] != "dark" || servers["other"] == nil || servers["gotodo"] == nil {
		t.Errorf("opencode config lost settings: %s", data)
	}

	if got := actions(Remove(e)); got != "Claude Code=removed, Codex=removed, OpenCode=removed" {
		t.Fatalf("remove: %s", got)
	}
	data, _ = os.ReadFile(toml)
	if string(data) != "[tui]\nscreen_reader_detection_done = true\n" {
		t.Errorf("codex config after remove:\n%q", data)
	}
	data, _ = os.ReadFile(oc)
	if strings.Contains(string(data), "gotodo") || !strings.Contains(string(data), "other") {
		t.Errorf("opencode config after remove: %s", data)
	}
}

func TestSkipsMissingAgentsAndUnparseableConfig(t *testing.T) {
	e, _ := testEnv(t, "opencode")
	oc := filepath.Join(e.ConfigHome, "opencode", "opencode.jsonc")
	os.MkdirAll(filepath.Dir(oc), 0o755)
	jsonc := "{\n  // my settings\n  \"theme\": \"dark\"\n}\n"
	os.WriteFile(oc, []byte(jsonc), 0o644)
	got := actions(Install(e))
	if got != "Claude Code=skipped, Codex=skipped, OpenCode=skipped" {
		t.Fatalf("install: %s", got)
	}
	if data, _ := os.ReadFile(oc); string(data) != jsonc {
		t.Errorf("unparseable config was rewritten:\n%s", data)
	}
}

func TestCreatesOpenCodeConfigAndDryRunWritesNothing(t *testing.T) {
	e, f := testEnv(t, "claude", "opencode")
	e.DryRun = true
	if got := actions(Install(e)); !strings.Contains(got, "Claude Code=would be registered") || !strings.Contains(got, "OpenCode=would be registered") {
		t.Fatalf("dry run: %s", got)
	}
	if f.registered != "" || exists(filepath.Join(e.ConfigHome, "opencode", "opencode.json")) {
		t.Fatal("dry run wrote something")
	}
	e.DryRun = false
	Install(e)
	data, err := os.ReadFile(filepath.Join(e.ConfigHome, "opencode", "opencode.json"))
	if err != nil || !strings.Contains(string(data), `"$schema"`) || !strings.Contains(string(data), `"GOTODO_AGENT": "opencode"`) {
		t.Errorf("created config: %s %v", data, err)
	}
}

// A registration pointing at an old binary is re-pointed, not duplicated.
func TestStaleRegistrationIsUpdated(t *testing.T) {
	e, f := testEnv(t, "claude")
	f.registered = "/old/gotodo mcp"
	if got := actions(Install(e)); !strings.Contains(got, "Claude Code=updated") {
		t.Fatalf("install: %s", got)
	}
	if f.registered != "/opt/bin/gotodo mcp" {
		t.Errorf("registered %q", f.registered)
	}
}

func TestStripBlockKeepsSurroundings(t *testing.T) {
	text := "[a]\nx = 1\n\n" + codexBlock("/b/gotodo") + "\n[z]\ny = 2\n"
	got, ok := stripBlock(text, codexBegin, codexEnd)
	if !ok || got != "[a]\nx = 1\n\n[z]\ny = 2\n" {
		t.Errorf("strip = %q", got)
	}
}

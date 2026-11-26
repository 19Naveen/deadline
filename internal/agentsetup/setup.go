// Package agentsetup registers gotodo's MCP server with the coding agents
// installed on this machine (Claude Code, Codex, OpenCode), so users never
// edit agent configs by hand. Every step is idempotent, leaves the rest of
// each config alone, and skips agents that are not installed.
package agentsetup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Name is the MCP server name registered with every agent.
const Name = "gotodo"

// Env holds what the steps touch, injectable for tests.
type Env struct {
	Home       string // user home directory
	ConfigHome string // XDG config dir (~/.config)
	Binary     string // absolute path of the gotodo binary to register
	DryRun     bool
	// LookPath finds an agent CLI; Run executes one and returns its output.
	LookPath func(string) (string, error)
	Run      func(name string, args ...string) (string, error)
}

// DefaultEnv is the real machine: os home, XDG config and the running binary.
func DefaultEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, err
	}
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	bin, err := os.Executable()
	if err != nil {
		return Env{}, err
	}
	if resolved, err := filepath.EvalSymlinks(bin); err == nil {
		bin = resolved
	}
	return Env{Home: home, ConfigHome: cfg, Binary: bin, LookPath: exec.LookPath, Run: runCommand}, nil
}

func runCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// Result is one agent's outcome, printed one line each.
type Result struct {
	Agent  string
	Action string // registered, updated, already registered, removed, skipped…
	Detail string
}

func (r Result) String() string {
	if r.Detail == "" {
		return fmt.Sprintf("%-11s %s", r.Agent+":", r.Action)
	}
	return fmt.Sprintf("%-11s %s (%s)", r.Agent+":", r.Action, r.Detail)
}

// Install registers the MCP server with every installed agent.
func Install(e Env) []Result {
	return []Result{claude(e, false), codex(e, false), opencode(e, false)}
}

// Remove unregisters it everywhere it was registered.
func Remove(e Env) []Result {
	return []Result{claude(e, true), codex(e, true), opencode(e, true)}
}

func would(e Env, action string) string {
	if e.DryRun {
		return "would be " + action
	}
	return action
}

// --- Claude Code: through its own CLI, user scope (every project). ---

func claude(e Env, remove bool) Result {
	r := Result{Agent: "Claude Code"}
	cli, err := e.LookPath("claude")
	if err != nil {
		r.Action = "skipped"
		r.Detail = "claude CLI not found"
		return r
	}
	current, getErr := e.Run(cli, "mcp", "get", Name)
	registered := getErr == nil && !strings.Contains(current, "No MCP server")
	if remove {
		if !registered {
			r.Action = "not registered"
			return r
		}
		r.Action = would(e, "removed")
		if !e.DryRun {
			if out, err := e.Run(cli, "mcp", "remove", "-s", "user", Name); err != nil {
				r.Action, r.Detail = "failed", strings.TrimSpace(out)
			}
		}
		return r
	}
	if registered && strings.Contains(current, e.Binary) {
		r.Action = "already registered"
		return r
	}
	r.Action = would(e, "registered")
	if registered {
		r.Action = would(e, "updated")
	}
	if e.DryRun {
		return r
	}
	if registered {
		// Re-point a stale registration (e.g. an old binary path).
		_, _ = e.Run(cli, "mcp", "remove", "-s", "user", Name)
	}
	if out, err := e.Run(cli, "mcp", "add", "-s", "user", Name, "-e", "GOTODO_AGENT=claude", "--", e.Binary, "mcp"); err != nil {
		r.Action, r.Detail = "failed", strings.TrimSpace(out)
	}
	return r
}

// --- Codex: a marked block appended to config.toml. ---

const (
	codexBegin = "# gotodo:begin - managed by gotodo setup, do not edit between the markers"
	codexEnd   = "# gotodo:end"
)

func codexDir(e Env) string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	return filepath.Join(e.Home, ".codex")
}

func codex(e Env, remove bool) Result {
	r := Result{Agent: "Codex"}
	dir := codexDir(e)
	if _, err := e.LookPath("codex"); err != nil && !exists(dir) {
		r.Action, r.Detail = "skipped", "codex not found"
		return r
	}
	path := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	text := string(data)
	stripped, hadBlock := stripBlock(text, codexBegin, codexEnd)
	if remove {
		if !hadBlock {
			r.Action = "not registered"
			return r
		}
		r.Action = would(e, "removed")
		if !e.DryRun {
			if err := writeAtomic(path, []byte(stripped)); err != nil {
				r.Action, r.Detail = "failed", err.Error()
			}
		}
		return r
	}
	block := codexBlock(e.Binary)
	if hadBlock && strings.Contains(text, block) {
		r.Action = "already registered"
		return r
	}
	if !hadBlock && strings.Contains(text, "[mcp_servers."+Name+"]") {
		r.Action, r.Detail = "skipped", "a hand-written [mcp_servers.gotodo] already exists"
		return r
	}
	r.Action = would(e, "registered")
	if hadBlock {
		r.Action = would(e, "updated")
	}
	if e.DryRun {
		return r
	}
	out := strings.TrimRight(stripped, "\n")
	if out != "" {
		out += "\n\n"
	}
	out += block
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	if err := writeAtomic(path, []byte(out)); err != nil {
		r.Action, r.Detail = "failed", err.Error()
	}
	return r
}

func codexBlock(bin string) string {
	return fmt.Sprintf("%s\n[mcp_servers.%s]\ncommand = %s\nargs = [\"mcp\"]\nenv = { GOTODO_AGENT = \"codex\" }\n%s\n",
		codexBegin, Name, tomlString(bin), codexEnd)
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	b, _ := json.Marshal(s) // JSON string escapes are valid TOML basic-string escapes
	return string(b)
}

// stripBlock removes the marked block (markers inclusive) and the blank
// line before it, reporting whether one was there.
func stripBlock(text, begin, end string) (string, bool) {
	i := strings.Index(text, begin)
	if i < 0 {
		return text, false
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		return text, false
	}
	after := strings.TrimLeft(text[i+j+len(end):], "\n")
	before := strings.TrimRight(text[:i], "\n")
	switch {
	case before == "":
		return after, true
	case after == "":
		return before + "\n", true
	default:
		return before + "\n\n" + after, true
	}
}

// --- OpenCode: merge mcp.gotodo into opencode.json. ---

func opencode(e Env, remove bool) Result {
	r := Result{Agent: "OpenCode"}
	dir := filepath.Join(e.ConfigHome, "opencode")
	if _, err := e.LookPath("opencode"); err != nil && !exists(dir) {
		r.Action, r.Detail = "skipped", "opencode not found"
		return r
	}
	path := filepath.Join(dir, "opencode.json")
	if !exists(path) && exists(filepath.Join(dir, "opencode.jsonc")) {
		path = filepath.Join(dir, "opencode.jsonc")
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	cfg := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			// JSONC comments or a broken file: never rewrite what we can't parse.
			r.Action = "skipped"
			r.Detail = fmt.Sprintf("cannot parse %s; add the gotodo server by hand (see gotodo help setup)", path)
			return r
		}
	}
	servers := map[string]json.RawMessage{}
	if raw, ok := cfg["mcp"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			r.Action, r.Detail = "skipped", "the mcp key is not an object"
			return r
		}
	}
	want, _ := json.Marshal(map[string]any{
		"type":        "local",
		"command":     []string{e.Binary, "mcp"},
		"enabled":     true,
		"environment": map[string]string{"GOTODO_AGENT": "opencode"},
	})
	current, has := servers[Name]
	if remove {
		if !has {
			r.Action = "not registered"
			return r
		}
		delete(servers, Name)
		r.Action = would(e, "removed")
	} else {
		if has && jsonEqual(current, want) {
			r.Action = "already registered"
			return r
		}
		r.Action = would(e, "registered")
		if has {
			r.Action = would(e, "updated")
		}
		servers[Name] = want
	}
	if e.DryRun {
		return r
	}
	if len(servers) == 0 {
		delete(cfg, "mcp")
	} else {
		cfg["mcp"], _ = json.Marshal(servers)
	}
	if _, ok := cfg["$schema"]; !ok && !remove {
		cfg["$schema"] = json.RawMessage(`"https://opencode.ai/config.json"`)
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	if err := writeAtomic(path, append(out, '\n')); err != nil {
		r.Action, r.Detail = "failed", err.Error()
	}
	return r
}

func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// writeAtomic replaces path via a temp file and rename, keeping its mode.
func writeAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".gotodo-setup-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

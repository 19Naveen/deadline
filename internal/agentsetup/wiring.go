package agentsetup

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Assets are the agent files gotodo installs, embedded in the binary.
type Assets struct {
	Skill   []byte // SKILL.md of the gotodo-task-board skill
	Snippet []byte // instruction block, between deadline:begin and deadline:end
}

const (
	// SkillName is the skill folder installed into each agent's skills dir.
	SkillName = "gotodo-task-board"
	// legacySkill is the old skill name, removed so agents never see two.
	legacySkill = "deadline"

	blockBegin = "deadline:begin"
	blockEnd   = "deadline:end"
)

// AgentNames are the display names per agent CLI.
var AgentNames = map[string]string{"claude": "Claude Code", "codex": "Codex", "opencode": "OpenCode"}

// Present reports whether an agent is on this machine: its CLI is on PATH
// or its home dir exists. Wiring never touches an absent agent's files.
func Present(e Env, agent string) bool {
	if _, err := e.LookPath(agent); err == nil {
		return true
	}
	switch agent {
	case "claude":
		return exists(filepath.Join(e.Home, ".claude"))
	case "codex":
		return exists(codexDir(e))
	case "opencode":
		return exists(filepath.Join(e.ConfigHome, "opencode"))
	}
	return false
}

// skillDirs are the skills dirs an agent loads from. Codex and OpenCode
// also read the shared Agent Skills dir ~/.agents/skills.
func skillDirs(e Env, agent string) []string {
	shared := filepath.Join(e.Home, ".agents", "skills")
	switch agent {
	case "claude":
		return []string{filepath.Join(e.Home, ".claude", "skills")}
	case "codex":
		return []string{filepath.Join(codexDir(e), "skills"), shared}
	case "opencode":
		return []string{filepath.Join(e.ConfigHome, "opencode", "skills"), shared}
	}
	return nil
}

// InstructionsFile is the global instruction file an agent reads every
// session, or "" when gotodo writes none for it (OpenCode loads the skill).
func InstructionsFile(e Env, agent string) string {
	switch agent {
	case "claude":
		return filepath.Join(e.Home, ".claude", "CLAUDE.md")
	case "codex":
		return filepath.Join(codexDir(e), "AGENTS.md")
	}
	return ""
}

func (e Env) short(path string) string {
	if e.Home != "" && strings.HasPrefix(path, e.Home+string(filepath.Separator)) {
		return "~" + path[len(e.Home):]
	}
	return path
}

// InstallSkill writes SKILL.md into every skills dir the agent loads from
// and removes the legacy deadline skill there.
func InstallSkill(e Env, agent string) Result {
	r := Result{Agent: AgentNames[agent], Item: "skill"}
	switch {
	case len(e.Assets.Skill) == 0:
		r.Action, r.Detail = "skipped", "no skill content in this build"
		return r
	case !Present(e, agent):
		r.Action, r.Detail = "skipped", agent+" not found"
		return r
	}
	changed, fresh := false, true
	var shown []string
	for _, dir := range skillDirs(e, agent) {
		legacy := filepath.Join(dir, legacySkill)
		path := filepath.Join(dir, SkillName, "SKILL.md")
		shown = append(shown, e.short(filepath.Dir(path)))
		if exists(legacy) {
			changed, fresh = true, false
			if !e.DryRun {
				if err := os.RemoveAll(legacy); err != nil {
					r.Action, r.Detail = "failed", err.Error()
					return r
				}
			}
		}
		cur, err := os.ReadFile(path)
		if err == nil && bytes.Equal(cur, e.Assets.Skill) {
			continue
		}
		changed = true
		fresh = fresh && err != nil
		if e.DryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			r.Action, r.Detail = "failed", err.Error()
			return r
		}
		if err := writeAtomic(path, e.Assets.Skill); err != nil {
			r.Action, r.Detail = "failed", err.Error()
			return r
		}
	}
	r.Detail = strings.Join(shown, ", ")
	switch {
	case !changed:
		r.Action = "already installed"
	case fresh:
		r.Action = would(e, "installed")
	default:
		r.Action = would(e, "updated")
	}
	return r
}

// RemoveSkill deletes the skill (and the legacy one) wherever the agent
// loads skills from. It needs no agent: an uninstall cleans up even after
// the agent itself is gone.
func RemoveSkill(e Env, agent string) Result {
	r := Result{Agent: AgentNames[agent], Item: "skill"}
	var gone []string
	for _, dir := range skillDirs(e, agent) {
		for _, name := range []string{SkillName, legacySkill} {
			path := filepath.Join(dir, name)
			if !exists(path) {
				continue
			}
			gone = append(gone, e.short(path))
			if e.DryRun {
				continue
			}
			if err := os.RemoveAll(path); err != nil {
				r.Action, r.Detail = "failed", err.Error()
				return r
			}
		}
	}
	if len(gone) == 0 {
		r.Action = "not installed"
		return r
	}
	r.Action, r.Detail = would(e, "removed"), strings.Join(gone, ", ")
	return r
}

// InstallInstructions puts the marked workflow block at the end of the
// agent's global instruction file, replacing an older block. Agents read
// that file every session, while a skill loads only when picked.
func InstallInstructions(e Env, agent string) Result {
	r := Result{Agent: AgentNames[agent], Item: "instructions"}
	path := InstructionsFile(e, agent)
	switch {
	case path == "":
		r.Action, r.Detail = "skipped", "uses the skill"
		return r
	case len(e.Assets.Snippet) == 0:
		r.Action, r.Detail = "skipped", "no instructions in this build"
		return r
	case !Present(e, agent):
		r.Action, r.Detail = "skipped", agent+" not found"
		return r
	}
	r.Detail = e.short(path)
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	text := string(data)
	stripped, had, err := stripMarked(text)
	if err != nil {
		r.Action, r.Detail = "failed", err.Error()+" in "+e.short(path)
		return r
	}
	if stripped != "" && !strings.HasSuffix(stripped, "\n") {
		stripped += "\n"
	}
	want := stripped + string(e.Assets.Snippet)
	if want == text {
		r.Action = "already installed"
		return r
	}
	r.Action = would(e, "added")
	if had {
		r.Action = would(e, "updated")
	}
	if e.DryRun {
		return r
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	if err := writeAtomic(path, []byte(want)); err != nil {
		r.Action, r.Detail = "failed", err.Error()
	}
	return r
}

// RemoveInstructions strips the marked block and leaves the rest of the
// file as it was.
func RemoveInstructions(e Env, agent string) Result {
	r := Result{Agent: AgentNames[agent], Item: "instructions"}
	path := InstructionsFile(e, agent)
	data, err := os.ReadFile(path)
	if path == "" || errors.Is(err, fs.ErrNotExist) {
		r.Action = "not installed"
		return r
	}
	if err != nil {
		r.Action, r.Detail = "failed", err.Error()
		return r
	}
	stripped, had, err := stripMarked(string(data))
	switch {
	case err != nil:
		r.Action, r.Detail = "failed", err.Error()+" in "+e.short(path)
		return r
	case !had:
		r.Action = "not installed"
		return r
	}
	r.Action, r.Detail = would(e, "removed"), e.short(path)
	if !e.DryRun {
		if err := writeAtomic(path, []byte(stripped)); err != nil {
			r.Action, r.Detail = "failed", err.Error()
		}
	}
	return r
}

// stripMarked removes every line from one holding deadline:begin through
// the next holding deadline:end, markers inclusive, as install.sh did. A
// begin without an end is an error: guessing would delete the user's text.
func stripMarked(text string) (string, bool, error) {
	var b strings.Builder
	had, skip := false, false
	for _, line := range strings.SplitAfter(text, "\n") {
		switch {
		case skip:
			if strings.Contains(line, blockEnd) {
				skip = false
			}
		case strings.Contains(line, blockBegin):
			had, skip = true, !strings.Contains(line, blockEnd)
		default:
			b.WriteString(line)
		}
	}
	if skip {
		return text, false, fmt.Errorf("%s without %s", blockBegin, blockEnd)
	}
	return b.String(), had, nil
}

// Wire installs the skill and instructions for every agent present.
func Wire(e Env) []Result {
	var out []Result
	for _, a := range AgentCLIs {
		if !Present(e, a) {
			out = append(out, Result{Agent: AgentNames[a], Item: "skill", Action: "skipped", Detail: a + " not found"})
			continue
		}
		out = append(out, InstallSkill(e, a))
		if InstructionsFile(e, a) != "" {
			out = append(out, InstallInstructions(e, a))
		}
	}
	return out
}

// Unwire removes the skill and instructions everywhere they were put.
func Unwire(e Env) []Result {
	var out []Result
	for _, a := range AgentCLIs {
		out = append(out, RemoveSkill(e, a))
		if InstructionsFile(e, a) != "" {
			out = append(out, RemoveInstructions(e, a))
		}
	}
	return out
}

// Connected lists the agents (display names) that already have gotodo's
// skill, instruction block or MCP entry on disk. It reads files only, so
// it never starts an agent CLI.
func Connected(e Env) []string {
	var out []string
	for _, a := range AgentCLIs {
		if connected(e, a) {
			out = append(out, AgentNames[a])
		}
	}
	return out
}

func connected(e Env, agent string) bool {
	if exists(filepath.Join(skillDirs(e, agent)[0], SkillName)) {
		return true
	}
	if p := InstructionsFile(e, agent); p != "" {
		if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), blockBegin) {
			return true
		}
	}
	switch agent {
	case "codex":
		data, _ := os.ReadFile(filepath.Join(codexDir(e), "config.toml"))
		return strings.Contains(string(data), codexBegin)
	case "opencode":
		data, _ := os.ReadFile(filepath.Join(e.ConfigHome, "opencode", "opencode.json"))
		return strings.Contains(string(data), `"`+Name+`"`)
	}
	return false
}

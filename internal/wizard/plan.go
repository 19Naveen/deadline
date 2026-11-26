// Package wizard is the interactive `gotodo setup`: it registers the MCP
// server with the chosen agents, optionally installs skill packs, and
// writes the stage-loops config. NonInteractive is the scripted path.
package wizard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gotodo/internal/agentsetup"
	"gotodo/internal/loops"
)

// Runner executes one command in dir and returns its combined output.
type Runner func(dir, name string, args ...string) (string, error)

// Deps is everything the wizard touches, injectable for tests.
type Deps struct {
	LookPath   func(string) (string, error)
	Run        Runner                      // pack installs
	Skills     func(agent string) []string // skills an agent can load
	MCP        agentsetup.Env              // MCP registration
	ConfigPath string                      // loops.json to write
	Home       string                      // cwd for installs, ~ in paths
	Now        func() time.Time

	// Project is the project the wizard was started in ("" for none);
	// ProjectSkills lists the skills an agent loads from it.
	Project       string
	ProjectSkills func(agent string) []string
	Cwd           string // resolves relative pack folders
	NoColor       bool   // NO_COLOR: plain tags, no colours
	Animate       bool   // spinner ticks while applying
}

// Options preset answers from flags.
type Options struct {
	// Loops presets the stage-loops question; nil asks (default No).
	Loops *bool
}

// DefaultDeps is the real machine.
func DefaultDeps() (Deps, error) {
	env, err := agentsetup.DefaultEnv()
	if err != nil {
		return Deps{}, err
	}
	path, err := loops.GlobalPath()
	if err != nil {
		return Deps{}, err
	}
	cwd, _ := os.Getwd()
	project := FindProject(cwd, env.Home)
	return Deps{
		LookPath:      exec.LookPath,
		Run:           execRunner,
		Skills:        loops.InstalledSkills,
		MCP:           env,
		ConfigPath:    path,
		Home:          env.Home,
		Now:           time.Now,
		Project:       project,
		ProjectSkills: func(agent string) []string { return loops.ProjectSkills(agent, project) },
		Cwd:           cwd,
		NoColor:       os.Getenv("NO_COLOR") != "",
		Animate:       true,
	}, nil
}

// FindProject walks up from dir to the nearest folder holding .deadline or
// .git. Home itself is never a project: its skill folders are the user's.
func FindProject(dir, home string) string {
	if dir == "" {
		return ""
	}
	for {
		if dir == home {
			return ""
		}
		for _, marker := range []string{".deadline", ".git"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// execRunner runs a pack install with no stdin, so a prompt fails fast
// instead of hanging behind the wizard's screen.
func execRunner(dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// StageNames are the development-board columns the wizard configures.
// Personal boards use doing/done; `gotodo loops set` handles those.
var StageNames = []string{"todo", "indev", "testing-review", "blocked", "shipped"}

// ModelSuggestions are offered in the model picker per agent.
var ModelSuggestions = map[string][]string{
	"claude": {"claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-5-5"},
}

// StageRow is one stage as edited on the stages screen.
type StageRow struct {
	Name  string
	On    bool
	Agent string
	Model string
	Skill string
	Perm  string
}

// skillPrefs lists default skills per stage, best first.
var skillPrefs = map[string][]string{
	"indev":          {"poteto-mode", "implement"},
	"testing-review": {"code-review", "poteto-mode"},
}

func defaultModel(agent, stage string) string {
	if agent != "claude" {
		return "" // the CLI's own default
	}
	if stage == "testing-review" {
		return "claude-opus-5-5"
	}
	return "claude-sonnet-5-5"
}

func pickSkill(stage string, available []string) string {
	for _, want := range skillPrefs[stage] {
		for _, s := range available {
			if s == want {
				return s
			}
		}
	}
	return ""
}

// primaryAgent is claude when detected, else the first detected agent.
func primaryAgent(detected []string) string {
	for _, a := range detected {
		if a == "claude" {
			return a
		}
	}
	if len(detected) == 0 {
		return ""
	}
	return detected[0]
}

// DefaultRows are the stage defaults: indev and testing-review on with the
// primary agent, everything else off. testing-review gets edits, not
// readonly, so the reviewing agent can run gotodo to move the card.
func DefaultRows(detected []string, available func(agent string) []string) []StageRow {
	agent := primaryAgent(detected)
	if agent == "" {
		return nil
	}
	rows := make([]StageRow, 0, len(StageNames))
	for _, name := range StageNames {
		on := name == "indev" || name == "testing-review"
		rows = append(rows, StageRow{
			Name:  name,
			On:    on,
			Agent: agent,
			Model: defaultModel(agent, name),
			Skill: pickSkill(name, available(agent)),
			Perm:  loops.PermEdits,
		})
	}
	return rows
}

// ConfigFromRows builds the loops config from the enabled rows.
func ConfigFromRows(rows []StageRow) loops.Config {
	c := loops.Config{
		Enabled:        true,
		MaxParallel:    loops.DefaultMaxParallel,
		MaxRunsPerTask: loops.DefaultMaxRunsPerTask,
		Stages:         map[string]loops.Stage{},
	}
	for _, r := range rows {
		if !r.On {
			continue
		}
		c.Stages[r.Name] = loops.Stage{Agent: r.Agent, Model: r.Model, Skill: r.Skill, Permissions: r.Perm}
	}
	return c
}

// detectAgents returns the agent CLIs found on PATH.
func detectAgents(lookPath func(string) (string, error)) (found []string, paths map[string]string) {
	paths = map[string]string{}
	for _, a := range loops.Agents {
		if p, err := lookPath(a); err == nil {
			found = append(found, a)
			paths[a] = p
		}
	}
	return found, paths
}

// tilde shortens a path under home to ~/….
func tilde(path, home string) string {
	if home != "" && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

func configExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

// NonInteractive is `gotodo setup -yes` and the no-terminal path: register
// the MCP server with every installed agent (as before the wizard), and
// write default loops only when asked with -loops and no config exists.
// Packs are never installed without a person choosing them.
func NonInteractive(d Deps, o Options, w io.Writer) error {
	failed := 0
	registered := false
	for _, r := range agentsetup.Install(d.MCP) {
		fmt.Fprintln(w, r)
		switch r.Action {
		case "failed":
			failed++
		case "registered", "updated":
			registered = true
		}
	}
	if o.Loops != nil && *o.Loops {
		if err := defaultLoops(d, w); err != nil {
			fmt.Fprintf(w, "loops:      failed (%v)\n", err)
			failed++
		}
	} else {
		fmt.Fprintln(w, "loops:      off (run gotodo setup in a terminal to set them up)")
	}
	if registered {
		fmt.Fprintln(w, "Restart open agent sessions to load the gotodo MCP server.")
	}
	if failed > 0 {
		return fmt.Errorf("%d setup step(s) failed", failed)
	}
	return nil
}

func defaultLoops(d Deps, w io.Writer) error {
	path := tilde(d.ConfigPath, d.Home)
	if configExists(d.ConfigPath) {
		fmt.Fprintf(w, "loops:      kept existing %s\n", path)
		return nil
	}
	detected, _ := detectAgents(d.LookPath)
	rows := DefaultRows(detected, d.Skills)
	if rows == nil {
		fmt.Fprintln(w, "loops:      skipped (no claude, codex or opencode CLI found)")
		return nil
	}
	var kept []StageRow
	var names []string
	for _, r := range rows {
		if !r.On {
			continue
		}
		if r.Skill == "" {
			fmt.Fprintf(w, "loops:      %s skipped (no suitable skill installed; run gotodo setup to pick one)\n", r.Name)
			continue
		}
		kept = append(kept, r)
		names = append(names, r.Name)
	}
	if len(kept) == 0 {
		return nil
	}
	if err := loops.WriteFile(d.ConfigPath, ConfigFromRows(kept)); err != nil {
		return err
	}
	fmt.Fprintf(w, "loops:      wrote %s (%s)\n", path, strings.Join(names, ", "))
	return nil
}

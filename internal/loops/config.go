// Package loops runs a coding agent with a chosen skill when a card enters a
// column: e.g. indev starts an implementation skill, testing-review starts a
// verification skill. The config says, per stage, which agent CLI, model,
// skill and prompt to use.
package loops

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Agents are the CLIs a stage can run.
var Agents = []string{"claude", "codex", "opencode"}

// Permission levels a stage may grant the agent it starts.
const (
	PermReadOnly = "readonly" // may read and run gotodo, no file edits
	PermEdits    = "edits"    // may edit files in the worktree (default)
	PermFull     = "full"     // skips every permission prompt; opt-in only
)

// Permissions lists the valid permission levels, safest first.
var Permissions = []string{PermReadOnly, PermEdits, PermFull}

// DefaultPrompt is used when a stage sets none. {skill}, {id}, {title},
// {stage}, {next} and {board} are substituted.
//
// Loop runs are expensive, so the prompt asks for one complete pass: follow
// the skill's own workflow to the end, prove every acceptance criterion, and
// only then hand the card on. A half-done move starts the next paid run on
// unfinished work.
const DefaultPrompt = "Use the {skill} skill on gotodo task {id} ({title}), which just entered {stage}. " +
	"Start by running `gotodo context {id}` and read it fully, including the working notes.\n\n" +
	"Finish the whole job in this run. Follow every step of the {skill} skill's workflow to its end, " +
	"including its own review and verification steps; do not skip steps to save time or tokens.\n\n" +
	"Before you run `gotodo move {id} {next}`, all of these must be true: every acceptance criterion in the " +
	"description is met and ticked ([x]); the tests and checks for this change pass, and you ran them in this " +
	"run; and the description has a **Verification** section saying what you ran and what it showed. " +
	"Record plans and notes you write with `gotodo edit {id} -working-note <path>`.\n\n" +
	"If you cannot finish, do not move the card to {next}. Move it to blocked only when you need a human, " +
	"and add a **Blocked** section to the description naming the exact question or missing access. " +
	"Otherwise leave it in {stage} with a **Progress** section listing what is done and what remains."

// Stage is what runs when a card enters one column.
type Stage struct {
	Agent       string `json:"agent"`                 // claude, codex or opencode
	Model       string `json:"model,omitempty"`       // passed to the CLI; blank uses its default
	Skill       string `json:"skill"`                 // skill name the prompt invokes
	Prompt      string `json:"prompt,omitempty"`      // template; blank uses DefaultPrompt
	Permissions string `json:"permissions,omitempty"` // readonly, edits (default) or full
	Enabled     *bool  `json:"enabled,omitempty"`     // nil means enabled
}

// On reports whether the stage runs.
func (s Stage) On() bool { return s.Enabled == nil || *s.Enabled }

// Perm is the stage's permission level, defaulting to edits.
func (s Stage) Perm() string {
	if s.Permissions == "" {
		return PermEdits
	}
	return s.Permissions
}

// PromptFor fills the stage's prompt template for one task. next is the
// column the card moves to when this stage's work is complete.
func (s Stage) PromptFor(id, title, stage, next, board string) string {
	p := s.Prompt
	if strings.TrimSpace(p) == "" {
		p = DefaultPrompt
	}
	if next == "" {
		next = "the next column"
	}
	return strings.NewReplacer("{skill}", s.Skill, "{id}", id, "{title}", title,
		"{stage}", stage, "{next}", next, "{board}", board).Replace(p)
}

// NextStage is the column after stage in the pipeline, skipping blocked
// (a detour, not a step forward). "" for the last column.
func NextStage(columns []string, stage string) string {
	for i, c := range columns {
		if c != stage {
			continue
		}
		for _, n := range columns[i+1:] {
			if n != "blocked" {
				return n
			}
		}
	}
	return ""
}

// Config is the whole loops setup.
type Config struct {
	Enabled bool `json:"enabled"`
	// MaxParallel caps loop runs alive at once across the board.
	MaxParallel int `json:"max_parallel,omitempty"`
	// MaxRunsPerTask caps runs per task per stage, so a card bouncing
	// between columns cannot loop forever.
	MaxRunsPerTask int `json:"max_runs_per_task,omitempty"`
	// TimeoutMinutes stops a run that outlives it; zero means the default.
	TimeoutMinutes int `json:"timeout_minutes,omitempty"`
	// Stages maps a column name (e.g. "indev") to what runs on entry.
	Stages map[string]Stage `json:"stages,omitempty"`
}

// Defaults for the limits when the config leaves them zero.
const (
	DefaultMaxParallel    = 2
	DefaultMaxRunsPerTask = 3
	DefaultTimeout        = 2 * time.Hour
)

// Timeout is how long one run may take before it is stopped.
func (c Config) Timeout() time.Duration {
	if c.TimeoutMinutes <= 0 {
		return DefaultTimeout
	}
	return time.Duration(c.TimeoutMinutes) * time.Minute
}

// Limits returns the effective limits.
func (c Config) Limits() (parallel, perTask int) {
	parallel, perTask = c.MaxParallel, c.MaxRunsPerTask
	if parallel <= 0 {
		parallel = DefaultMaxParallel
	}
	if perTask <= 0 {
		perTask = DefaultMaxRunsPerTask
	}
	return parallel, perTask
}

// StageNames returns the configured stages in a stable order.
func (c Config) StageNames() []string {
	names := make([]string, 0, len(c.Stages))
	for n := range c.Stages {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Validate checks agents, permissions and that each stage names a skill.
func (c Config) Validate() error {
	if c.MaxParallel < 0 || c.MaxRunsPerTask < 0 || c.TimeoutMinutes < 0 {
		return errors.New("max_parallel, max_runs_per_task and timeout_minutes must not be negative")
	}
	for name, s := range c.Stages {
		if !contains(Agents, s.Agent) {
			return fmt.Errorf("stage %s: agent %q (want %s)", name, s.Agent, strings.Join(Agents, ", "))
		}
		if strings.TrimSpace(s.Skill) == "" && strings.TrimSpace(s.Prompt) == "" {
			return fmt.Errorf("stage %s: set a skill or a prompt", name)
		}
		if !contains(Permissions, s.Perm()) {
			return fmt.Errorf("stage %s: permissions %q (want %s)", name, s.Permissions, strings.Join(Permissions, ", "))
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// GlobalPath is the per-user default config, ~/.config/gotodo/loops.json.
func GlobalPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gotodo", "loops.json"), nil
}

// ProjectPath is the per-project override beside a project board
// (<root>/.deadline/loops.json), or "" for a board outside .deadline.
func ProjectPath(boardPath string) string {
	abs, err := filepath.Abs(boardPath)
	if err != nil || filepath.Base(filepath.Dir(abs)) != ".deadline" {
		return ""
	}
	return filepath.Join(filepath.Dir(abs), "loops.json")
}

// Load returns the effective config for a board: the project file if it
// exists, else the global one, else a disabled empty config. The project
// file replaces the global one entirely (no field merging), so what you
// see in it is what runs.
func Load(boardPath string) (Config, string, error) {
	paths := []string{ProjectPath(boardPath)}
	if g, err := GlobalPath(); err == nil {
		paths = append(paths, g)
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		c, err := ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return c, p, err
	}
	return Config{}, "", nil
}

// ReadFile parses and validates one config file.
func ReadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// WriteFile validates c and writes it atomically.
func WriteFile(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".loops-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

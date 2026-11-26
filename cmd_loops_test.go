package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotodo/internal/loops"
	"gotodo/internal/task"
)

// loopsEnv isolates HOME/XDG, fakes every process hook and returns a dev
// project board path with one task.
func loopsEnv(t *testing.T) (board, id string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv(task.AgentEnv, "")
	t.Setenv(loops.LoopEnv, "")
	old := loops.Hooks
	t.Cleanup(func() { loops.Hooks = old })
	loops.Hooks = loops.HookSet{
		StartRunner: func(string, string, string, string) (int, error) { return 0, fmt.Errorf("no runners in tests") },
		ExecAgent: func(_ context.Context, c loops.Command, out io.Writer, started func(int)) (int, error) {
			started(1)
			fmt.Fprintln(out, "fake agent ran")
			return 0, nil
		},
		Alive:     func(int) bool { return true },
		KillGroup: func(int) error { return nil },
		HasSkill:  func(agent, skill string) bool { return skill == "tdd" },
		LookPath:  func(n string) (string, error) { return n, nil },
	}
	board = filepath.Join(t.TempDir(), "proj", ".deadline", "board.json")
	b, err := task.Load(board)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	id = b.Add("Fix parser", "", nil, testRef).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	return board, id
}

func runOut(t *testing.T, argv ...string) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = run(argv) })
	return out, err
}

func TestLoopsStatusWithoutConfig(t *testing.T) {
	board, _ := loopsEnv(t)
	out, err := runOut(t, "-file", board, "loops")
	if err != nil || !strings.Contains(out, "loops: off (no config)") {
		t.Fatalf("status = %q %v", out, err)
	}
}

func TestLoopsSetOnOffUnsetWriteTheRightFile(t *testing.T) {
	board, _ := loopsEnv(t)
	global, _ := loops.GlobalPath()
	project := loops.ProjectPath(board)

	if _, err := runOut(t, "-file", board, "loops", "set", "indev", "-agent", "claude", "-skill", "tdd", "-model", "claude-sonnet-5-5"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Fatalf("set without -project wrote the project file: %v", err)
	}
	g, err := loops.ReadFile(global)
	if err != nil || g.Stages["indev"].Model != "claude-sonnet-5-5" || g.Enabled {
		t.Fatalf("global = %+v %v", g, err)
	}

	// -project creates the project file seeded from the global config.
	if _, err := runOut(t, "-file", board, "loops", "set", "testing-review", "-agent", "codex", "-skill", "review", "-permissions", "readonly", "-project"); err != nil {
		t.Fatal(err)
	}
	p, err := loops.ReadFile(project)
	if err != nil || len(p.Stages) != 2 || p.Stages["testing-review"].Perm() != loops.PermReadOnly {
		t.Fatalf("project = %+v %v", p, err)
	}
	// Once it exists, plain on/off/unset write the project file.
	if out, err := runOut(t, "-file", board, "loops", "on"); err != nil || !strings.Contains(out, project) {
		t.Fatalf("on = %q %v", out, err)
	}
	if p, _ = loops.ReadFile(project); !p.Enabled {
		t.Error("on did not enable the project file")
	}
	if g, _ = loops.ReadFile(global); g.Enabled {
		t.Error("on touched the global file")
	}
	out, err := runOut(t, "-file", board, "loops")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"loops: on · config " + project, "indev", "claude-sonnet-5-5", "testing-review", "readonly", "✓", "✗", "running: none"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
	if _, err := runOut(t, "-file", board, "loops", "unset", "indev"); err != nil {
		t.Fatal(err)
	}
	if p, _ = loops.ReadFile(project); len(p.Stages) != 1 {
		t.Errorf("unset left %+v", p.Stages)
	}
	if _, err := runOut(t, "-file", board, "loops", "off"); err != nil {
		t.Fatal(err)
	}
	if p, _ = loops.ReadFile(project); p.Enabled {
		t.Error("off did not disable")
	}
}

func TestLoopsSetValidates(t *testing.T) {
	board, _ := loopsEnv(t)
	if _, err := runOut(t, "-file", board, "loops", "set", "review", "-agent", "claude", "-skill", "x"); err == nil ||
		!strings.Contains(err.Error(), "unknown status") {
		t.Errorf("unknown stage = %v", err)
	}
	if _, err := runOut(t, "-file", board, "loops", "set", "indev", "-skill", "x"); err == nil ||
		!strings.Contains(err.Error(), "-agent is required") {
		t.Errorf("missing agent = %v", err)
	}
	if _, err := runOut(t, "-file", board, "loops", "set", "indev", "-agent", "gpt", "-skill", "x"); err == nil {
		t.Error("unknown agent accepted")
	}
}

func TestLoopsRefusesAgentsAndLoops(t *testing.T) {
	board, _ := loopsEnv(t)
	t.Setenv(task.AgentEnv, "claude")
	for _, argv := range [][]string{{"on"}, {"off"}, {"set", "indev", "-agent", "claude", "-skill", "tdd"}, {"unset", "indev"}} {
		_, err := runOut(t, append([]string{"-file", board, "loops"}, argv...)...)
		if err == nil || !strings.Contains(err.Error(), "agents cannot reconfigure loops") {
			t.Errorf("%v as agent = %v", argv, err)
		}
	}
	t.Setenv(task.AgentEnv, "")
	t.Setenv(loops.LoopEnv, "1")
	if _, err := runOut(t, "-file", board, "loops", "on"); err == nil || !strings.Contains(err.Error(), "a loop cannot") {
		t.Errorf("on in a loop = %v", err)
	}
	if _, err := runOut(t, "-file", board, "loops", "stop", "abc"); err == nil || !strings.Contains(err.Error(), "a loop cannot") {
		t.Errorf("stop in a loop = %v", err)
	}
	global, _ := loops.GlobalPath()
	if _, err := os.Stat(global); !os.IsNotExist(err) {
		t.Errorf("refused commands wrote config: %v", err)
	}
}

func TestLoopsRunThenLogs(t *testing.T) {
	board, id := loopsEnv(t)
	if _, err := runOut(t, "-file", board, "loops", "set", "indev", "-agent", "claude", "-skill", "tdd", "-project"); err != nil {
		t.Fatal(err)
	}
	if _, err := runOut(t, "-file", board, "loops", "run", id, "indev"); err != nil {
		t.Fatal(err)
	}
	out, err := runOut(t, "-file", board, "loops", "logs", id[:8])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"gotodo loop · indev · run 1", "command:  claude -p", "fake agent ran", "· done · exit 0"} {
		if !strings.Contains(out, want) {
			t.Errorf("logs missing %q:\n%s", want, out)
		}
	}
	if out, err := runOut(t, "-file", board, "loops", "stop", id[:8]); err != nil || !strings.Contains(out, "no live loop runs") {
		t.Errorf("stop with nothing running = %q %v", out, err)
	}
}

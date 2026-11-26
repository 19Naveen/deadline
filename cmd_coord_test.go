package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotodo/internal/task"
)

// coordBoard inits a dev board in a temp project and adds titled tasks,
// returning the board path and the full IDs in order.
func coordBoard(t *testing.T, titles ...string) (string, []string) {
	t.Helper()
	t.Setenv(task.AgentEnv, "")
	chdir(t, t.TempDir())
	captureStdout(t, func() {
		if err := run([]string{"init"}); err != nil {
			t.Fatal(err)
		}
		for _, title := range titles {
			if err := run([]string{"add", title}); err != nil {
				t.Fatal(err)
			}
		}
	})
	path, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(b.Tasks))
	for _, tk := range b.Tasks {
		ids = append(ids, tk.ID)
	}
	return path, ids
}

func loadTask(t *testing.T, path, id string) task.Task {
	t.Helper()
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tk, ok := b.TaskByID(id)
	if !ok {
		t.Fatalf("task %s missing", id)
	}
	return tk
}

func TestNextClaimStartAndClaimConflicts(t *testing.T) {
	path, ids := coordBoard(t, "First", "Second")

	out := captureStdout(t, func() {
		if err := run([]string{"next"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "First") || strings.Contains(out, "claimed") {
		t.Fatalf("peek output = %q", out)
	}
	if loadTask(t, path, ids[0]).ClaimedBy != "" {
		t.Fatal("plain next wrote a claim")
	}
	if err := run([]string{"next", "-claim"}); err == nil ||
		!strings.Contains(err.Error(), task.AgentEnv) {
		t.Fatalf("next -claim without agent = %v, want a %s error", err, task.AgentEnv)
	}

	t.Setenv(task.AgentEnv, "alice")
	out = captureStdout(t, func() {
		if err := run([]string{"next", "-start", "-ttl", "30m"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "claimed by alice") || !strings.Contains(out, "moved to indev") ||
		!strings.Contains(out, "First") {
		t.Fatalf("next -start output = %q", out)
	}
	first := loadTask(t, path, ids[0])
	if first.ClaimedBy != "alice" || first.Status != task.StatusInDev || first.LastAgent != "alice" {
		t.Fatalf("after next -start: %+v", first)
	}

	t.Setenv(task.AgentEnv, "bob")
	out = captureStdout(t, func() {
		if err := run([]string{"next", "-claim"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "Second") {
		t.Fatalf("bob's next = %q, want Second", out)
	}
	if err := run([]string{"claim", ids[0][:8]}); err == nil ||
		!strings.Contains(err.Error(), "claimed by alice") {
		t.Fatalf("bob claiming alice's task = %v, want conflict", err)
	}
	if err := run([]string{"release", ids[0][:8]}); err == nil {
		t.Fatal("bob released alice's claim")
	}
	// Claims never block moves.
	if err := captureErr(t, func() error { return run([]string{"move", ids[0][:8], "testing-review"}) }); err != nil {
		t.Fatalf("move of claimed task = %v", err)
	}
	t.Setenv(task.AgentEnv, "carol")
	out = captureStdout(t, func() {
		if err := run([]string{"next"}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.TrimSpace(out) != "no available tasks" {
		t.Fatalf("next with everything claimed = %q", out)
	}

	t.Setenv(task.AgentEnv, "alice")
	out = captureStdout(t, func() {
		if err := run([]string{"release", ids[0][:8]}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "released") || loadTask(t, path, ids[0]).ClaimedBy != "" {
		t.Fatalf("release output = %q", out)
	}
	t.Setenv(task.AgentEnv, "bob")
	if err := captureErr(t, func() error { return run([]string{"claim", "-ttl", "1h", ids[0][:8]}) }); err != nil {
		t.Fatalf("claim after release = %v", err)
	}
	if loadTask(t, path, ids[0]).ClaimedBy != "bob" {
		t.Fatal("bob's claim not saved")
	}
}

func captureErr(t *testing.T, fn func() error) error {
	t.Helper()
	var err error
	captureStdout(t, func() { err = fn() })
	return err
}

func TestHumanNextStartMovesWithoutClaim(t *testing.T) {
	path, ids := coordBoard(t, "Only")
	if err := captureErr(t, func() error { return run([]string{"next", "-start"}) }); err != nil {
		t.Fatal(err)
	}
	tk := loadTask(t, path, ids[0])
	if tk.Status != task.StatusInDev || tk.ClaimedBy != "" {
		t.Fatalf("human next -start: %+v", tk)
	}
	if err := run([]string{"claim", ids[0]}); err == nil {
		t.Fatal("claim without GOTODO_AGENT = nil")
	}
}

func TestHumanReleasesAnyClaimAndShippedClaimsAreSwept(t *testing.T) {
	path, ids := coordBoard(t, "A", "B")
	t.Setenv(task.AgentEnv, "alice")
	for _, id := range ids {
		if err := captureErr(t, func() error { return run([]string{"claim", id}) }); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(task.AgentEnv, "")
	if err := captureErr(t, func() error { return run([]string{"release", ids[0]}) }); err != nil {
		t.Fatalf("human release = %v", err)
	}
	if err := captureErr(t, func() error { return run([]string{"move", ids[1], "shipped"}) }); err != nil {
		t.Fatal(err)
	}
	// Any coordination transaction drops claims on shipped cards.
	if err := captureErr(t, func() error { return run([]string{"next", "-start"}) }); err != nil {
		t.Fatal(err)
	}
	if c := loadTask(t, path, ids[1]).ClaimedBy; c != "" {
		t.Fatalf("shipped task still claimed by %q", c)
	}
}

func TestContextCommandInlinesNotesFromProjectRoot(t *testing.T) {
	_, ids := coordBoard(t, "Resume me")
	root, _ := os.Getwd()
	if err := os.MkdirAll(filepath.Join(root, "docs", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "plan.md"), []byte("PLAN-CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "ref.txt"), []byte("ATTACH-CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := captureErr(t, func() error {
		return run([]string{"edit", ids[0], "-working-note", "docs/plan.md", "-attachment", "docs/ref.txt"})
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(task.AgentEnv, "alice")
	if err := captureErr(t, func() error { return run([]string{"next", "-start"}) }); err != nil {
		t.Fatal(err)
	}
	// Run from a subdirectory: notes still resolve against the project root.
	chdir(t, filepath.Join(root, "docs", "sub"))
	out := captureStdout(t, func() {
		if err := run([]string{"context", ids[0][:8]}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"# Resume me", "- status: indev", "- claimed by: alice",
		"### docs/plan.md", "PLAN-CONTENT", "- docs/ref.txt", "todo → indev by alice"} {
		if !strings.Contains(out, want) {
			t.Errorf("context missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ATTACH-CONTENT") {
		t.Error("context printed attachment contents")
	}
	if err := run([]string{"context"}); err == nil {
		t.Error("context without id = nil")
	}
}

func TestApprovalRefusesAgents(t *testing.T) {
	path, _ := coordBoard(t)
	t.Setenv(task.AgentEnv, "claude")
	for _, mode := range []string{"on", "off"} {
		if err := run([]string{"approval", mode}); err == nil || !strings.Contains(err.Error(), "agents cannot") {
			t.Errorf("agent approval %s = %v, want refusal", mode, err)
		}
	}
	out := captureStdout(t, func() {
		if err := run([]string{"approval", "status"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "approval: off") {
		t.Errorf("status = %q", out)
	}

	t.Setenv(task.AgentEnv, "")
	out = captureStdout(t, func() {
		if err := run([]string{"approval", "on"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "approval: on") {
		t.Errorf("approval on output = %q", out)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !b.RequireApproval {
		t.Fatal("approval on not saved")
	}
	t.Setenv(task.AgentEnv, "claude")
	if err := run([]string{"approval", "off"}); err == nil {
		t.Fatal("agent turned the gate off")
	}
	if b, _ := task.Load(path); !b.RequireApproval {
		t.Fatal("agent refusal still changed the board")
	}
	if err := run([]string{"approval", "maybe"}); err == nil {
		t.Error("approval maybe = nil")
	}
}

func TestCoordCommandsHelpAndUsage(t *testing.T) {
	coordBoard(t)
	captureStdout(t, func() {
		for _, argv := range [][]string{{"next", "-h"}, {"claim", "-h"}, {"release", "-h"},
			{"context", "-h"}, {"approval", "-h"}} {
			if err := run(argv); err != nil {
				t.Errorf("run(%q) = %v, want nil", argv, err)
			}
		}
	})
	for _, argv := range [][]string{{"claim"}, {"release"}, {"approval"}, {"next", "extra"},
		{"release", "a", "b"}} {
		if err := run(argv); err == nil {
			t.Errorf("run(%q) = nil, want usage error", argv)
		}
	}
}

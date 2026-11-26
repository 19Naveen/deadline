package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

// devBoardPath makes an empty dev board file and returns its path.
func devBoardPath(t *testing.T) string {
	t.Helper()
	t.Setenv(task.AgentEnv, "")
	path := filepath.Join(t.TempDir(), "board.json")
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	return path
}

// mustRun runs a headless command and returns its stdout.
func mustRun(t *testing.T, argv ...string) string {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = run(argv) })
	if err != nil {
		t.Fatalf("run(%q) = %v", argv, err)
	}
	return out
}

// taskByTitle loads the board and returns the task with title.
func taskByTitle(t *testing.T, path, title string) task.Task {
	t.Helper()
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range b.Tasks {
		if x.Title == title {
			return x
		}
	}
	t.Fatalf("no task titled %q", title)
	return task.Task{}
}

func TestPriorityAndLabelsRoundTrip(t *testing.T) {
	path := devBoardPath(t)
	mustRun(t, "-file", path, "add", "Low", "-priority", "low")
	mustRun(t, "-file", path, "add", "Plain", "-label", "#Docs")
	mustRun(t, "-file", path, "add", "High", "-priority", "p1", "-label", "Bug", "-label", " backend ", "-label", "bug")

	hi := taskByTitle(t, path, "High")
	if hi.Priority != task.PriorityHigh || !slices.Equal(hi.Labels, []string{"bug", "backend"}) {
		t.Fatalf("High = %v %q", hi.Priority, hi.Labels)
	}

	out := mustRun(t, "-file", path, "list")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "High") || !strings.Contains(lines[1], "Low") || !strings.Contains(lines[2], "Plain") {
		t.Fatalf("list not sorted by priority desc:\n%s", out)
	}
	if !strings.Contains(lines[0], " · P:high · #bug #backend") {
		t.Errorf("list line = %q, want priority and labels", lines[0])
	}
	if strings.Contains(lines[2], "P:") {
		t.Errorf("unset priority printed: %q", lines[2])
	}

	if out := mustRun(t, "-file", path, "list", "-label", "BUG"); !strings.Contains(out, "High") || strings.Contains(out, "Plain") {
		t.Errorf("list -label bug = %q", out)
	}
	if out := mustRun(t, "-file", path, "list", "-priority", "none"); !strings.Contains(out, "Plain") || strings.Contains(out, "High") {
		t.Errorf("list -priority none = %q", out)
	}
	if out := mustRun(t, "-file", path, "list", "-priority", "medium"); strings.TrimSpace(out) != "no tasks" {
		t.Errorf("list -priority medium = %q, want no tasks", out)
	}
	if err := run([]string{"-file", path, "list", "-priority", "urgent"}); err == nil {
		t.Error("list -priority urgent = nil, want an error")
	}

	id := hi.ID[:8]
	out = mustRun(t, "-file", path, "show", id)
	if !strings.Contains(out, "priority: high\n") || !strings.Contains(out, "labels: bug, backend\n") {
		t.Errorf("show = %q", out)
	}

	mustRun(t, "-file", path, "edit", id, "-label", "ui", "-priority", "none")
	hi = taskByTitle(t, path, "High")
	if hi.Priority != task.PriorityNone || !slices.Equal(hi.Labels, []string{"ui"}) {
		t.Fatalf("after edit = %v %q, want none [ui]", hi.Priority, hi.Labels)
	}
	mustRun(t, "-file", path, "edit", id, "-title", "High")
	if hi = taskByTitle(t, path, "High"); !slices.Equal(hi.Labels, []string{"ui"}) {
		t.Errorf("edit without -label changed labels to %q", hi.Labels)
	}
	mustRun(t, "-file", path, "edit", id, "-label", "")
	if hi = taskByTitle(t, path, "High"); hi.Labels != nil {
		t.Errorf("edit -label \"\" left %q", hi.Labels)
	}
}

func TestBadPriorityWritesNothing(t *testing.T) {
	path := devBoardPath(t)
	if err := run([]string{"-file", path, "add", "X", "-priority", "urgent"}); err == nil {
		t.Fatal("add -priority urgent = nil, want an error")
	}
	b, _ := task.Load(path)
	if len(b.Tasks) != 0 {
		t.Fatalf("bad add wrote %d tasks", len(b.Tasks))
	}
}

func TestBlockedByAutoBlockAndUnblock(t *testing.T) {
	path := devBoardPath(t)
	mustRun(t, "-file", path, "add", "Schema")
	dep := taskByTitle(t, path, "Schema").ID

	out := mustRun(t, "-file", path, "add", "API", "-status", "indev", "-blocked-by", dep[:6])
	if !strings.Contains(out, "auto-blocked") {
		t.Errorf("add output = %q, want an auto-blocked receipt", out)
	}
	api := taskByTitle(t, path, "API")
	if api.Status != task.StatusBlocked || !slices.Equal(api.BlockedBy, []string{dep}) {
		t.Fatalf("API = %s %v, want blocked by the full schema id", api.Status, api.BlockedBy)
	}
	if out := mustRun(t, "-file", path, "list"); !strings.Contains(out, "blocked by "+dep[:8]) {
		t.Errorf("list = %q, want blocked by %s", out, dep[:8])
	}
	out = mustRun(t, "-file", path, "show", api.ID[:8])
	if !strings.Contains(out, "blocked by:\n  "+dep[:8]+" [todo] Schema\n") {
		t.Errorf("show = %q", out)
	}

	// Shipping the blocker unblocks its dependant in the same move.
	out = mustRun(t, "-file", path, "move", dep[:8], "shipped")
	if !strings.Contains(out, "unblocked "+api.ID[:8]) {
		t.Errorf("move output = %q, want an unblocked receipt", out)
	}
	if api = taskByTitle(t, path, "API"); api.Status != task.StatusInDev {
		t.Errorf("API = %s, want back in indev", api.Status)
	}
	if out := mustRun(t, "-file", path, "list"); strings.Contains(out, "blocked by") {
		t.Errorf("list still shows a done blocker: %q", out)
	}
}

func TestManualBlockSurvivesHeadless(t *testing.T) {
	path := devBoardPath(t)
	mustRun(t, "-file", path, "add", "Dep")
	dep := taskByTitle(t, path, "Dep").ID
	mustRun(t, "-file", path, "add", "Waiting", "-status", "blocked", "-blocked-by", dep[:8])
	mustRun(t, "-file", path, "move", dep[:8], "shipped")
	mustRun(t, "-file", path, "add", "Trigger")
	if w := taskByTitle(t, path, "Waiting"); w.Status != task.StatusBlocked {
		t.Errorf("manually blocked card moved to %s", w.Status)
	}
}

func TestEditBlockedByCycleAndClear(t *testing.T) {
	path := devBoardPath(t)
	mustRun(t, "-file", path, "add", "A")
	a := taskByTitle(t, path, "A").ID
	mustRun(t, "-file", path, "add", "B", "-blocked-by", a[:8])
	bID := taskByTitle(t, path, "B").ID

	if err := run([]string{"-file", path, "edit", a[:8], "-title", "Renamed", "-blocked-by", bID[:8]}); err == nil {
		t.Fatal("cyclic edit = nil, want an error")
	} else if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cyclic edit error = %v", err)
	}
	if got := taskByTitle(t, path, "A"); got.BlockedBy != nil {
		t.Errorf("rejected edit wrote BlockedBy %v", got.BlockedBy)
	}
	if err := run([]string{"-file", path, "edit", a[:8], "-blocked-by", a[:8]}); err == nil {
		t.Error("self block = nil, want an error")
	}
	if err := run([]string{"-file", path, "add", "C", "-blocked-by", "zzzz"}); err == nil {
		t.Error("unknown blocker = nil, want an error")
	}

	mustRun(t, "-file", path, "edit", bID[:8], "-blocked-by", "")
	b := taskByTitle(t, path, "B")
	if b.BlockedBy != nil || b.Status != task.StatusTodo {
		t.Errorf("after clearing B = %s %v, want todo with no blockers", b.Status, b.BlockedBy)
	}
}

func TestWIPCommandAndListWarning(t *testing.T) {
	path := devBoardPath(t)
	for _, title := range []string{"one", "two"} {
		mustRun(t, "-file", path, "add", title, "-status", "indev")
	}
	out := mustRun(t, "-file", path, "wip")
	if !strings.Contains(out, "indev: 2 (no limit)") {
		t.Errorf("wip = %q", out)
	}
	mustRun(t, "-file", path, "wip", "indev", "1")
	out = mustRun(t, "-file", path, "wip")
	if !strings.Contains(out, "indev: 2/1 over limit") || !strings.Contains(out, "todo: 0 (no limit)") {
		t.Errorf("wip = %q", out)
	}
	if out := mustRun(t, "-file", path, "list"); !strings.HasPrefix(out, "! indev over WIP limit (2/1)\n") {
		t.Errorf("list = %q, want the WIP warning first", out)
	}
	if out := mustRun(t, "-file", path, "list", "-status", "todo"); strings.Contains(out, "WIP") {
		t.Errorf("list -status todo warned about indev: %q", out)
	}
	mustRun(t, "-file", path, "wip", "indev", "0")
	if out := mustRun(t, "-file", path, "list"); strings.Contains(out, "WIP") {
		t.Errorf("cleared limit still warns: %q", out)
	}
	for _, argv := range [][]string{{"wip", "indev"}, {"wip", "nope", "2"}, {"wip", "indev", "-1"}, {"wip", "indev", "x"}} {
		if err := run(append([]string{"-file", path}, argv...)); err == nil {
			t.Errorf("run(%q) = nil, want an error", argv)
		}
	}
}

func TestShowAndListAgentFields(t *testing.T) {
	path := devBoardPath(t)
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Date(2026, 11, 27, 9, 30, 0, 0, time.Local)
	b.Tasks = append(b.Tasks, task.Task{
		ID: "feedbeefcafe0001", Title: "Claimed", Status: task.StatusInDev,
		ClaimedBy: "claude", ClaimUntil: &until, PR: "#42", LastAgent: "codex",
	})
	b.SetColumns(task.DevColumns) // dirty so Save writes the injected task
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	out := mustRun(t, "-file", path, "show", "feedbeef")
	for _, want := range []string{"claimed by: claude until 27/11/2026 09:30\n", "pr: #42\n", "last agent: codex\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("show missing %q: %q", want, out)
		}
	}
	out = mustRun(t, "-file", path, "list")
	if !strings.Contains(out, " · claimed by claude · PR #42 · last codex") {
		t.Errorf("list = %q", out)
	}
}

// Shipping a blocker through `move` unblocks its dependants and drops the
// shipped card's claim, without waiting for a later add or edit.
func TestMoveReconcilesDependenciesAndClaims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b, _ := task.Load(path)
	b.SetColumns(task.DevColumns)
	blocker := b.Add("blocker", "", nil, testRef).ID
	dep := b.Add("dep", "", nil, testRef).ID
	if err := b.SetBlockedBy(dep, []string{blocker}, testRef); err != nil {
		t.Fatal(err)
	}
	b.SyncBlocked(testRef)
	if err := b.Claim(blocker, "claude", time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	captureStdout(t, func() {
		if err := runMove(path, []string{blocker, "shipped"}); err != nil {
			t.Fatal(err)
		}
	})
	b, _ = task.Load(path)
	got, _ := b.TaskByID(dep)
	if got.Status != task.StatusTodo {
		t.Errorf("dependant status = %s, want todo after blocker shipped", got.Status)
	}
	if shipped, _ := b.TaskByID(blocker); shipped.ClaimedBy != "" {
		t.Errorf("claim survived shipping: %q", shipped.ClaimedBy)
	}
}

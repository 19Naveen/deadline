package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

var ref = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

// fakeRunner answers `gh pr view <pr>` from a map and records calls.
type fakeRunner struct {
	views map[string]string // PR ref -> JSON
	calls [][]string
	err   error
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.err != nil {
		return nil, f.err
	}
	if len(args) >= 3 && args[0] == "pr" && args[1] == "view" {
		if out, ok := f.views[args[2]]; ok {
			return []byte(out), nil
		}
		return nil, errors.New("no pull requests found")
	}
	return nil, nil
}

const (
	mergedJSON  = `{"state":"MERGED","mergedAt":"2026-11-26T08:00:00Z","url":"https://github.com/o/r/pull/1","statusCheckRollup":[]}`
	failingJSON = `{"state":"OPEN","mergedAt":null,"url":"https://github.com/o/r/pull/2","statusCheckRollup":[{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE"}]}`
	statusErr   = `{"state":"OPEN","mergedAt":"","url":"https://github.com/o/r/pull/4","statusCheckRollup":[{"__typename":"StatusContext","context":"ci","state":"ERROR"}]}`
	greenJSON   = `{"state":"OPEN","mergedAt":null,"url":"https://github.com/o/r/pull/3","statusCheckRollup":[{"conclusion":"SUCCESS"},{"status":"IN_PROGRESS","conclusion":""}]}`
)

func devBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	b.SetColumns(task.DevColumns)
	return b
}

func addWithPR(t *testing.T, b *task.Board, title, pr string, status task.Status) string {
	t.Helper()
	id := b.Add(title, "", nil, ref).ID
	if err := b.SetPR(id, pr, ref); err != nil {
		t.Fatal(err)
	}
	if status != b.Statuses()[0] {
		if err := b.Move(id, status, ref); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestPRStatusParsing(t *testing.T) {
	r := &fakeRunner{views: map[string]string{"1": mergedJSON, "2": failingJSON, "3": greenJSON, "4": statusErr}}
	cases := map[string][2]bool{"1": {true, false}, "2": {false, true}, "3": {false, false}, "4": {false, true}}
	for pr, want := range cases {
		st, err := View(context.Background(), r, pr)
		if err != nil {
			t.Fatal(err)
		}
		if st.Merged() != want[0] || st.Failing() != want[1] {
			t.Errorf("PR %s: merged %v failing %v, want %v", pr, st.Merged(), st.Failing(), want)
		}
	}
	want := "pr view 1 --json state,mergedAt,statusCheckRollup,url"
	found := false
	for _, c := range r.calls {
		found = found || strings.Join(c, " ") == want
	}
	if !found {
		t.Errorf("gh calls = %v, want one %q", r.calls, want)
	}
}

func TestSyncMergedShips(t *testing.T) {
	b := devBoard(t)
	b.Actor = "claude"
	id := addWithPR(t, b, "Parser", "1", task.StatusTestingReview)
	idle := b.Add("No PR", "", nil, ref).ID
	actions := Sync(context.Background(), b, &fakeRunner{views: map[string]string{"1": mergedJSON}}, ref)
	if len(actions) != 1 || actions[0].Kind != ActionShipped {
		t.Fatalf("actions = %+v, want one shipped", actions)
	}
	got, _ := b.TaskByID(id)
	if got.Status != task.StatusShipped || got.LastAgent != "claude" {
		t.Fatalf("task = %s by %q, want shipped by claude", got.Status, got.LastAgent)
	}
	if other, _ := b.TaskByID(idle); other.Status != task.StatusTodo {
		t.Fatal("task without a PR was touched")
	}
	if line := actions[0].String(false); !strings.Contains(line, "→ shipped") || !strings.Contains(line, "pull/1") {
		t.Errorf("line = %q", line)
	}
}

func TestSyncApprovalGateRefusal(t *testing.T) {
	b := devBoard(t)
	b.SetRequireApproval(true)
	b.Actor = "claude"
	id := addWithPR(t, b, "Gate", "1", task.StatusTestingReview)
	actions := Sync(context.Background(), b, &fakeRunner{views: map[string]string{"1": mergedJSON}}, ref)
	if len(actions) != 1 || actions[0].Kind != ActionAwaiting {
		t.Fatalf("actions = %+v, want awaiting approval", actions)
	}
	if !strings.Contains(actions[0].String(false), "awaits human approval") {
		t.Errorf("line = %q", actions[0].String(false))
	}
	if got, _ := b.TaskByID(id); got.Status != task.StatusTestingReview {
		t.Fatalf("gate bypassed: %s", got.Status)
	}
	// A human may ship through the gate.
	b.Actor = ""
	actions = Sync(context.Background(), b, &fakeRunner{views: map[string]string{"1": mergedJSON}}, ref)
	if len(actions) != 1 || actions[0].Kind != ActionShipped {
		t.Fatalf("human sync = %+v, want shipped", actions)
	}
}

func TestSyncUnfinishedSubtasksIsNotApproval(t *testing.T) {
	b := devBoard(t)
	b.SetRequireApproval(true)
	b.Actor = "claude"
	id := addWithPR(t, b, "Parent", "1", task.StatusInDev)
	if _, err := b.AddChild(id, "child", "", nil, ref); err != nil {
		t.Fatal(err)
	}
	actions := Sync(context.Background(), b, &fakeRunner{views: map[string]string{"1": mergedJSON}}, ref)
	if len(actions) != 1 || actions[0].Kind != ActionRefused || !strings.Contains(actions[0].Detail, "subtasks") {
		t.Fatalf("actions = %+v, want refused for subtasks", actions)
	}
}

func TestSyncFailingCIBlocksAndNotesOnce(t *testing.T) {
	b := devBoard(t)
	id := addWithPR(t, b, "Flaky", "2", task.StatusInDev)
	if err := b.Edit(id, "Flaky", "Existing notes", nil, ref); err != nil {
		t.Fatal(err)
	}
	deadline := ref.Add(48 * time.Hour)
	if err := b.Edit(id, "Flaky", "Existing notes", &deadline, ref); err != nil {
		t.Fatal(err)
	}
	r := &fakeRunner{views: map[string]string{"2": failingJSON}}
	actions := Sync(context.Background(), b, r, ref)
	if len(actions) != 1 || actions[0].Kind != ActionBlocked {
		t.Fatalf("actions = %+v, want blocked", actions)
	}
	got, _ := b.TaskByID(id)
	wantDesc := "Existing notes\nCI failing on PR https://github.com/o/r/pull/2 (26/11/2026)"
	if got.Status != task.StatusBlocked || got.Description != wantDesc {
		t.Fatalf("task = %s %q, want blocked %q", got.Status, got.Description, wantDesc)
	}
	if got.Deadline == nil || !got.Deadline.Equal(deadline) || got.Title != "Flaky" {
		t.Fatal("other fields not preserved")
	}
	// A later run on another day: already blocked and noted, nothing to do.
	if again := Sync(context.Background(), b, r, ref.Add(48*time.Hour)); len(again) != 0 {
		t.Fatalf("second sync = %+v, want no actions", again)
	}
	if got, _ := b.TaskByID(id); strings.Count(got.Description, "CI failing") != 1 {
		t.Fatalf("note duplicated: %q", got.Description)
	}
}

func TestSyncFailingWhenAlreadyBlockedOnlyNotes(t *testing.T) {
	b := devBoard(t)
	id := addWithPR(t, b, "Already blocked", "4", task.StatusBlocked)
	actions := Sync(context.Background(), b, &fakeRunner{views: map[string]string{"4": statusErr}}, ref)
	if len(actions) != 1 || actions[0].Kind != ActionNoted {
		t.Fatalf("actions = %+v, want noted", actions)
	}
	if got, _ := b.TaskByID(id); !strings.HasPrefix(got.Description, "CI failing on PR https://github.com/o/r/pull/4") {
		t.Fatalf("description = %q", got.Description)
	}
}

func TestSyncSkipsGreenDoneAndReportsErrors(t *testing.T) {
	b := devBoard(t)
	addWithPR(t, b, "Green", "3", task.StatusInDev)
	addWithPR(t, b, "Shipped", "1", task.StatusShipped)
	addWithPR(t, b, "Missing", "99", task.StatusInDev)
	r := &fakeRunner{views: map[string]string{"1": mergedJSON, "3": greenJSON}}
	actions := Sync(context.Background(), b, r, ref)
	if len(actions) != 1 || actions[0].Kind != ActionError || actions[0].Title != "Missing" {
		t.Fatalf("actions = %+v, want one error for Missing", actions)
	}
	for _, c := range r.calls {
		if len(c) > 2 && c[2] == "1" {
			t.Fatal("shipped task's PR was queried")
		}
	}
}

func TestDryRunLinesSayWould(t *testing.T) {
	a := Action{Kind: ActionBlocked, TaskID: "0123456789abcdef", Title: "T", PR: "u", Detail: "blocked"}
	if got := a.String(true); !strings.HasPrefix(got, "would move 01234567 T → blocked") {
		t.Errorf("dry-run line = %q", got)
	}
}

func TestCheckAuth(t *testing.T) {
	if err := CheckAuth(context.Background(), &fakeRunner{}); err != nil {
		t.Fatalf("authenticated = %v", err)
	}
	if err := CheckAuth(context.Background(), &fakeRunner{err: errors.New("exit 1")}); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("unauthenticated = %v", err)
	}
	if err := CheckAuth(context.Background(), &fakeRunner{err: ErrNotInstalled}); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("missing gh = %v", err)
	}
}

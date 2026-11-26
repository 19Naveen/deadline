package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotodo/internal/gh"
	"gotodo/internal/task"
)

// devBoardFile writes a dev board with the given tasks built by fn.
func devBoardFile(t *testing.T, fn func(b *task.Board)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".deadline", "board.json")
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	fn(b)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStandupCommand(t *testing.T) {
	t.Setenv(task.AgentEnv, "")
	now := time.Now()
	path := devBoardFile(t, func(b *task.Board) {
		id := b.Add("Fresh feature", "", nil, now.Add(-2*time.Hour)).ID
		b.Actor = "claude"
		_ = b.Move(id, task.StatusShipped, now.Add(-time.Hour))
	})
	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "standup"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "Done\n- Fresh feature") || !strings.Contains(out, "by claude") {
		t.Errorf("standup text = %q", out)
	}
	md := captureStdout(t, func() {
		if err := run([]string{"-file", path, "standup", "-format", "markdown", "-since", "7d"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(md, "**Done**") {
		t.Errorf("markdown = %q", md)
	}
	if err := run([]string{"-file", path, "standup", "-format", "html"}); err == nil {
		t.Error("bad format accepted")
	}
	if err := run([]string{"-file", path, "standup", "-since", "soon"}); err == nil {
		t.Error("bad -since accepted")
	}
}

func TestPRLinkShowClear(t *testing.T) {
	t.Setenv(task.AgentEnv, "")
	var id string
	path := devBoardFile(t, func(b *task.Board) { id = b.Add("Linked", "", nil, testRef).ID })
	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "pr", id[:6], "#12"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "→ PR 12") {
		t.Errorf("link output = %q", out)
	}
	out = captureStdout(t, func() { _ = run([]string{"-file", path, "pr", id[:6]}) })
	if !strings.Contains(out, "PR 12") {
		t.Errorf("show output = %q", out)
	}
	if err := run([]string{"-file", path, "pr", id[:6], "not a pr"}); err == nil {
		t.Error("bad PR accepted")
	}
	out = captureStdout(t, func() { _ = run([]string{"-file", path, "pr", id[:6], ""}) })
	if !strings.Contains(out, "cleared PR") {
		t.Errorf("clear output = %q", out)
	}
	b, _ := task.Load(path)
	if got, _ := b.TaskByID(id); got.PR != "" {
		t.Errorf("PR = %q after clear", got.PR)
	}
}

type cmdFakeGH struct {
	views   map[string]string
	authErr error
	calls   int
}

func (f *cmdFakeGH) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls++
	if args[0] == "auth" {
		return nil, f.authErr
	}
	if out, ok := f.views[args[2]]; ok {
		return []byte(out), nil
	}
	return nil, errors.New("not found")
}

func withFakeGH(t *testing.T, f *cmdFakeGH) {
	t.Helper()
	old := newGHRunner
	newGHRunner = func(string) gh.Runner { return f }
	t.Cleanup(func() { newGHRunner = old })
}

func TestPRSyncCommand(t *testing.T) {
	var merged, failing string
	path := devBoardFile(t, func(b *task.Board) {
		merged = b.Add("Merged", "", nil, testRef).ID
		failing = b.Add("Failing", "", nil, testRef).ID
		_ = b.SetPR(merged, "1", testRef)
		_ = b.SetPR(failing, "2", testRef)
		_ = b.Move(failing, task.StatusInDev, testRef)
	})
	fake := &cmdFakeGH{views: map[string]string{
		"1": `{"state":"MERGED","mergedAt":"2026-11-26T08:00:00Z","url":"https://github.com/o/r/pull/1","statusCheckRollup":[]}`,
		"2": `{"state":"OPEN","mergedAt":null,"url":"https://github.com/o/r/pull/2","statusCheckRollup":[{"conclusion":"CANCELLED"}]}`,
	}}
	withFakeGH(t, fake)

	// Dry run: lines say "would", nothing saved.
	t.Setenv(task.AgentEnv, "claude")
	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "pr", "sync", "-dry-run"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "would move "+merged[:8]+" Merged → shipped") || !strings.Contains(out, "would move "+failing[:8]+" Failing → blocked") {
		t.Errorf("dry run = %q", out)
	}
	b, _ := task.Load(path)
	if got, _ := b.TaskByID(merged); got.Status != task.StatusTodo {
		t.Fatal("dry run saved changes")
	}

	// Real run as an agent: moves recorded under the agent's name.
	out = captureStdout(t, func() {
		if err := run([]string{"-file", path, "pr", "sync"}); err != nil {
			t.Fatal(err)
		}
	})
	b, _ = task.Load(path)
	m, _ := b.TaskByID(merged)
	f, _ := b.TaskByID(failing)
	if m.Status != task.StatusShipped || m.LastAgent != "claude" {
		t.Errorf("merged = %s by %q", m.Status, m.LastAgent)
	}
	if f.Status != task.StatusBlocked || !strings.HasPrefix(f.Description, "CI failing on PR https://github.com/o/r/pull/2 (") {
		t.Errorf("failing = %s %q", f.Status, f.Description)
	}
	if strings.Contains(out, "would") {
		t.Errorf("real run says would: %q", out)
	}
}

func TestPRSyncApprovalGateAndAuth(t *testing.T) {
	var id string
	path := devBoardFile(t, func(b *task.Board) {
		b.SetRequireApproval(true)
		id = b.Add("Gated", "", nil, testRef).ID
		_ = b.SetPR(id, "1", testRef)
	})
	fake := &cmdFakeGH{views: map[string]string{
		"1": `{"state":"MERGED","mergedAt":"2026-11-26T08:00:00Z","url":"https://github.com/o/r/pull/1"}`,
	}}
	withFakeGH(t, fake)
	t.Setenv(task.AgentEnv, "claude")
	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "pr", "sync"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "awaits human approval") {
		t.Errorf("gate output = %q", out)
	}

	fake.authErr = errors.New("exit status 1")
	if err := run([]string{"-file", path, "pr", "sync"}); !errors.Is(err, gh.ErrNotAuthenticated) {
		t.Errorf("unauthenticated = %v", err)
	}
	fake.authErr = gh.ErrNotInstalled
	if err := run([]string{"-file", path, "pr", "sync"}); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("missing gh = %v", err)
	}
}

func TestPRSyncNoLinkedTasksSkipsGH(t *testing.T) {
	path := devBoardFile(t, func(b *task.Board) { b.Add("No PR", "", nil, testRef) })
	fake := &cmdFakeGH{}
	withFakeGH(t, fake)
	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "pr", "sync"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "no open tasks with a linked PR") || fake.calls != 0 {
		t.Errorf("out %q, gh calls %d", out, fake.calls)
	}
}

type cmdFakeSender struct{ sent []string }

func (f *cmdFakeSender) Send(title, body string) error {
	f.sent = append(f.sent, title+": "+body)
	return nil
}
func (f *cmdFakeSender) Method() string { return "fake" }

func TestNotifyCommandOnePassDedupes(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv(task.AgentEnv, "")
	path := devBoardFile(t, func(b *task.Board) {
		today := time.Now()
		b.Add("Due now", "", &today, time.Now())
	})
	fake := &cmdFakeSender{}
	old := newNotifySender
	newNotifySender = func() notifySender { return fake }
	t.Cleanup(func() { newNotifySender = old })

	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "notify"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "sent 1 notification(s) via fake") || len(fake.sent) != 1 {
		t.Fatalf("out %q sent %v", out, fake.sent)
	}
	wantTitle := "gotodo · " + filepath.Base(filepath.Dir(filepath.Dir(path))) + ": due today"
	if !strings.HasPrefix(fake.sent[0], wantTitle) {
		t.Errorf("sent = %q, want title %q", fake.sent[0], wantTitle)
	}
	out = captureStdout(t, func() { _ = run([]string{"-file", path, "notify"}) })
	if !strings.Contains(out, "no new events") || len(fake.sent) != 1 {
		t.Errorf("second pass out %q sent %v", out, fake.sent)
	}
	if err := run([]string{"-file", path, "notify", "-interval", "1s"}); err == nil {
		t.Error("tiny interval accepted")
	}
}

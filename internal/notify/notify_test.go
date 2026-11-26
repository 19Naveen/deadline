package notify

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

var now = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

type fakeSender struct {
	sent []string
	fail bool
}

func (f *fakeSender) Send(title, body string) error {
	if f.fail {
		return errors.New("no desktop")
	}
	f.sent = append(f.sent, title+" | "+body)
	return nil
}

func saveBoard(t *testing.T, path string, build func(b *task.Board)) {
	t.Helper()
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	build(b)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
}

func TestEventsDetection(t *testing.T) {
	b := &task.Board{}
	b.SetColumns(task.DevColumns)
	today, past := now, now.Add(-48*time.Hour)
	blocked := b.Add("Blocked", "", nil, now.Add(-time.Hour)).ID
	b.Actor = "claude"
	_ = b.Move(blocked, task.StatusBlocked, now.Add(-time.Hour))
	b.Actor = ""
	review := b.Add("Review", "", nil, now).ID
	_ = b.Move(review, task.StatusTestingReview, now.Add(-10*time.Minute))
	oldBlocked := b.Add("Stale", "", nil, now.Add(-90*24*time.Hour)).ID
	_ = b.Move(oldBlocked, task.StatusBlocked, now.Add(-60*24*time.Hour))
	b.Add("Due", "", &today, now)
	b.Add("Late", "", &past, now)
	shippedLate := b.Add("Shipped late", "", &past, now).ID
	_ = b.Move(shippedLate, task.StatusShipped, now)
	b.Add("Idle", "", nil, now)

	evs := Events(b, now, now.Add(-24*time.Hour), "proj")
	got := map[Kind][]string{}
	for _, e := range evs {
		got[e.Kind] = append(got[e.Kind], e.Body)
	}
	if len(evs) != 4 {
		t.Fatalf("events = %+v, want 4", evs)
	}
	if !strings.HasPrefix(got[KindBlocked][0], "Blocked (") || !strings.HasSuffix(got[KindBlocked][0], "by claude") {
		t.Errorf("blocked = %v", got[KindBlocked])
	}
	if len(got[KindReview]) != 1 || len(got[KindDueToday]) != 1 {
		t.Errorf("review/due = %v / %v", got[KindReview], got[KindDueToday])
	}
	if len(got[KindOverdue]) != 1 || !strings.Contains(got[KindOverdue][0], "was due 24/11/2026") {
		t.Errorf("overdue = %v", got[KindOverdue])
	}
	if evs[0].Title != "gotodo · proj: blocked" {
		t.Errorf("title = %q", evs[0].Title)
	}
}

func TestPersonalReviewColumnIsDoing(t *testing.T) {
	b := &task.Board{}
	if ReviewStatus(b) != task.StatusDoing {
		t.Fatal("personal review column should be doing")
	}
	id := b.Add("x", "", nil, now).ID
	_ = b.Move(id, task.StatusDoing, now)
	if evs := Events(b, now, now.Add(-time.Hour), ""); len(evs) != 1 || evs[0].Kind != KindReview || evs[0].Title != "gotodo: ready for review" {
		t.Fatalf("events = %+v", evs)
	}
}

func TestPassDedupesAndRefiresOnNewTransition(t *testing.T) {
	dir := t.TempDir()
	boardPath := filepath.Join(dir, "board.json")
	var id string
	saveBoard(t, boardPath, func(b *task.Board) {
		b.SetColumns(task.DevColumns)
		id = b.Add("Flaky", "", nil, now).ID
		_ = b.Move(id, task.StatusBlocked, now.Add(-time.Minute))
	})
	before, _ := os.ReadFile(boardPath)
	clock := now
	fs := &fakeSender{}
	n := &Notifier{BoardPath: boardPath, StateFile: StatePath(filepath.Join(dir, "cache"), boardPath), Sender: fs, Now: func() time.Time { return clock }}

	sent, err := n.Pass()
	if err != nil || len(sent) != 1 {
		t.Fatalf("first pass = %v, %v", sent, err)
	}
	if sent, _ := n.Pass(); len(sent) != 0 {
		t.Fatalf("second pass resent: %+v", sent)
	}
	if after, _ := os.ReadFile(boardPath); !bytes.Equal(before, after) {
		t.Fatal("notify wrote the board")
	}
	// Unblock then block again: a new event.
	clock = now.Add(time.Hour)
	saveBoard(t, boardPath, func(b *task.Board) {
		_ = b.Move(id, task.StatusInDev, clock)
		_ = b.Move(id, task.StatusBlocked, clock)
	})
	if sent, _ := n.Pass(); len(sent) != 1 || sent[0].Kind != KindBlocked {
		t.Fatalf("re-block pass = %+v", sent)
	}
	if len(fs.sent) != 2 {
		t.Fatalf("sender calls = %v", fs.sent)
	}
	// State lives in the cache dir, not beside the board.
	if _, err := os.Stat(n.StateFile); err != nil || !strings.HasPrefix(n.StateFile, filepath.Join(dir, "cache", "gotodo", "notify-")) {
		t.Fatalf("state file %q: %v", n.StateFile, err)
	}
}

func TestPassRetriesFailedSendsAndPrunesDeleted(t *testing.T) {
	dir := t.TempDir()
	boardPath := filepath.Join(dir, "board.json")
	var id string
	saveBoard(t, boardPath, func(b *task.Board) {
		d := now
		id = b.Add("Due", "", &d, now).ID
	})
	fs := &fakeSender{fail: true}
	n := &Notifier{BoardPath: boardPath, StateFile: filepath.Join(dir, "state.json"), Sender: fs, Now: func() time.Time { return now }}
	if sent, err := n.Pass(); err == nil || len(sent) != 0 {
		t.Fatalf("failing sender = %v, %v", sent, err)
	}
	fs.fail = false
	if sent, err := n.Pass(); err != nil || len(sent) != 1 {
		t.Fatalf("retry = %v, %v", sent, err)
	}
	saveBoard(t, boardPath, func(b *task.Board) { _ = b.Delete(id) })
	if _, err := n.Pass(); err != nil {
		t.Fatal(err)
	}
	st, _ := LoadState(n.StateFile, boardPath, now)
	if len(st.Sent) != 0 {
		t.Fatalf("deleted task's records kept: %v", st.Sent)
	}
}

func TestFirstRunSkipsOldTransitions(t *testing.T) {
	dir := t.TempDir()
	boardPath := filepath.Join(dir, "board.json")
	saveBoard(t, boardPath, func(b *task.Board) {
		id := b.Add("Old block", "", nil, now.Add(-10*24*time.Hour)).ID
		_ = b.Move(id, task.StatusBlocked, now.Add(-5*24*time.Hour))
	})
	n := &Notifier{BoardPath: boardPath, StateFile: filepath.Join(dir, "s.json"), Sender: &fakeSender{}, Now: func() time.Time { return now }}
	if sent, _ := n.Pass(); len(sent) != 0 {
		t.Fatalf("old transition notified: %+v", sent)
	}
}

func TestStatePathStablePerBoard(t *testing.T) {
	a := StatePath("/c", "/p/one/.deadline/board.json")
	if a != StatePath("/c", "/p/one/.deadline/board.json") || a == StatePath("/c", "/p/two/.deadline/board.json") {
		t.Fatal("state path should be stable per board and differ across boards")
	}
}

func TestWatchLoopsUntilCancelled(t *testing.T) {
	dir := t.TempDir()
	n := &Notifier{BoardPath: filepath.Join(dir, "b.json"), StateFile: filepath.Join(dir, "s.json"), Sender: &fakeSender{}, Now: func() time.Time { return now }}
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	passes := 0
	done := make(chan struct{})
	go func() {
		n.Watch(ctx, tick, func([]Event, error) {
			passes++
			if passes == 3 {
				cancel()
			}
		})
		close(done)
	}()
	tick <- now
	tick <- now
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not stop")
	}
	if passes != 3 {
		t.Fatalf("passes = %d, want 3", passes)
	}
}

func TestDesktopMethods(t *testing.T) {
	var calls []string
	mk := func(goos string, env map[string]string, procVersion string, have ...string) *Desktop {
		return &Desktop{
			GOOS:     goos,
			Getenv:   func(k string) string { return env[k] },
			ReadFile: func(string) ([]byte, error) { return []byte(procVersion), nil },
			LookPath: func(name string) (string, error) {
				for _, h := range have {
					if h == name {
						return "/usr/bin/" + name, nil
					}
				}
				return "", errors.New("not found")
			},
			Exec: func(name string, args ...string) error {
				calls = append(calls, name+" "+strings.Join(args, " "))
				return nil
			},
			Out: &bytes.Buffer{},
		}
	}
	linux := mk("linux", nil, "Linux 6.1 generic", "notify-send")
	if linux.Method() != "notify-send" {
		t.Fatalf("linux = %s", linux.Method())
	}
	_ = linux.Send(`t"; rm -rf /`, "-b")
	if calls[0] != `notify-send --app-name=gotodo -- t"; rm -rf / -b` {
		t.Errorf("notify-send args = %q", calls[0])
	}
	wsl := mk("linux", nil, "Linux 6.18 microsoft-standard-WSL2", "notify-send")
	if wsl.Method() != "stdout" {
		t.Fatalf("wsl = %s", wsl.Method())
	}
	_ = wsl.Send("gotodo: blocked", "X (1234)")
	if out := wsl.Out.(*bytes.Buffer).String(); out != "gotodo: blocked: X (1234)\n" {
		t.Errorf("wsl stdout = %q", out)
	}
	if m := mk("linux", map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, "", "notify-send").Method(); m != "stdout" {
		t.Errorf("WSL env = %s", m)
	}
	if m := mk("linux", nil, "", "").Method(); m != "stdout" {
		t.Errorf("no notify-send = %s", m)
	}
	mac := mk("darwin", nil, "", "osascript")
	_ = mac.Send("T", "B")
	if last := calls[len(calls)-1]; !strings.HasPrefix(last, "osascript -e on run argv") || !strings.HasSuffix(last, "end run T B") {
		t.Errorf("osascript args = %q", last)
	}
	if m := mk("windows", nil, "", "").Method(); m != "stdout" {
		t.Errorf("windows = %s", m)
	}
}

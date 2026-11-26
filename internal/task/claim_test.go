package task

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClaimConflictsAndExpiry(t *testing.T) {
	b := &Board{}
	id := b.Add("x", "", nil, ref).ID
	if err := b.Claim(id, "", time.Hour, ref); err == nil {
		t.Fatal("claim without agent = nil, want an error")
	}
	if err := b.Claim(id, "alice", time.Hour, ref); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := b.Claim(id, "bob", time.Hour, ref.Add(30*time.Minute)); err == nil ||
		!strings.Contains(err.Error(), "claimed by alice") {
		t.Fatalf("conflicting claim = %v, want claimed-by-alice error", err)
	}
	// Re-claiming your own extends it.
	if err := b.Claim(id, "alice", 2*time.Hour, ref.Add(30*time.Minute)); err != nil {
		t.Fatalf("own re-claim: %v", err)
	}
	got, _ := b.TaskByID(id)
	if want := ref.Add(150 * time.Minute); !got.ClaimUntil.Equal(want) {
		t.Fatalf("ClaimUntil = %v, want %v", got.ClaimUntil, want)
	}
	// After expiry anyone may take it.
	later := ref.Add(3 * time.Hour)
	if got.ClaimHeld(later) {
		t.Fatal("claim still held after expiry")
	}
	if err := b.Claim(id, "bob", time.Hour, later); err != nil {
		t.Fatalf("claim after expiry: %v", err)
	}
	got, _ = b.TaskByID(id)
	if got.ClaimedBy != "bob" {
		t.Fatalf("ClaimedBy = %q, want bob", got.ClaimedBy)
	}
	// Claims never block moves.
	b.Actor = "alice"
	if err := b.Move(id, StatusDoing, later); err != nil {
		t.Fatalf("move of a task claimed by another = %v, want nil", err)
	}
}

func TestReleaseRules(t *testing.T) {
	b := &Board{}
	id := b.Add("x", "", nil, ref).ID
	if ok, err := b.Release(id, "alice", ref); ok || err != nil {
		t.Fatalf("release unclaimed = %v, %v; want false, nil", ok, err)
	}
	_ = b.Claim(id, "alice", time.Hour, ref)
	if _, err := b.Release(id, "bob", ref); err == nil {
		t.Fatal("bob released alice's live claim")
	}
	if ok, err := b.Release(id, "", ref); !ok || err != nil {
		t.Fatalf("human release = %v, %v; want true, nil", ok, err)
	}
	_ = b.Claim(id, "alice", time.Hour, ref)
	if ok, err := b.Release(id, "bob", ref.Add(2*time.Hour)); !ok || err != nil {
		t.Fatalf("release of expired claim = %v, %v; want true, nil", ok, err)
	}
	_ = b.Claim(id, "alice", time.Hour, ref)
	if ok, err := b.Release(id, "alice", ref); !ok || err != nil {
		t.Fatalf("own release = %v, %v", ok, err)
	}
	got, _ := b.TaskByID(id)
	if got.ClaimedBy != "" || got.ClaimUntil != nil {
		t.Fatalf("claim not cleared: %+v", got)
	}
}

func TestClaimRejectsDoneAndReleaseFinished(t *testing.T) {
	b := &Board{}
	id := b.Add("x", "", nil, ref).ID
	_ = b.Claim(id, "alice", time.Hour, ref)
	_ = b.Move(id, StatusDone, ref)
	if err := b.Claim(id, "bob", time.Hour, ref.Add(2*time.Hour)); err == nil {
		t.Fatal("claimed a done task")
	}
	if n := b.ReleaseFinishedClaims(ref); n != 1 {
		t.Fatalf("ReleaseFinishedClaims = %d, want 1", n)
	}
	if got, _ := b.TaskByID(id); got.ClaimedBy != "" {
		t.Fatalf("done task still claimed: %+v", got)
	}
}

func TestNextTaskOrdering(t *testing.T) {
	b := &Board{}
	soon := ref.Add(24 * time.Hour)
	later := ref.Add(48 * time.Hour)
	old := b.Add("old low", "", nil, ref.Add(-time.Hour)).ID
	young := b.Add("young none", "", nil, ref).ID
	b.Tasks[0].Priority = PriorityLow
	if got, _ := b.NextTask("a", ref); got.ID != old {
		t.Fatalf("priority beats none: got %q", got.Title)
	}
	b.Tasks[1].Priority = PriorityLow
	b.Tasks[1].Deadline = &later
	if got, _ := b.NextTask("a", ref); got.ID != young {
		t.Fatalf("deadline beats no deadline: got %q", got.Title)
	}
	b.Tasks[0].Deadline = &soon
	if got, _ := b.NextTask("a", ref); got.ID != old {
		t.Fatalf("earlier deadline wins: got %q", got.Title)
	}
	b.Tasks[0].Deadline = &later
	if got, _ := b.NextTask("a", ref); got.ID != old {
		t.Fatalf("older CreatedAt breaks ties: got %q", got.Title)
	}
	hi := b.Add("high", "", nil, ref.Add(time.Hour)).ID
	b.Tasks[2].Priority = PriorityHigh
	if got, _ := b.NextTask("a", ref); got.ID != hi {
		t.Fatalf("high priority first: got %q", got.Title)
	}
	// Only the first column counts.
	_ = b.Move(hi, StatusDoing, ref)
	if got, _ := b.NextTask("a", ref); got.ID == hi {
		t.Fatal("picked a task outside the first column")
	}
}

func TestNextTaskSkipsClaimedBlockedParentsAndArchived(t *testing.T) {
	b := &Board{}
	blocker := b.Add("blocker", "", nil, ref).ID
	blocked := b.Add("blocked", "", nil, ref).ID
	parent := b.Add("parent", "", nil, ref).ID
	child, _ := b.AddChild(parent, "child", "", nil, ref)
	childID := child.ID
	gone := b.Add("archived", "", nil, ref).ID
	for i := range b.Tasks {
		switch b.Tasks[i].ID {
		case blocked:
			b.Tasks[i].BlockedBy = []string{blocker}
			b.Tasks[i].Priority = PriorityHigh
		case parent:
			b.Tasks[i].Priority = PriorityHigh
		case gone:
			b.Tasks[i].Archived = true
			b.Tasks[i].Priority = PriorityHigh
		case blocker:
			b.Tasks[i].Priority = PriorityMedium
		}
	}
	if got, _ := b.NextTask("a", ref); got.ID != blocker {
		t.Fatalf("got %q, want blocker (blocked, parent and archived skipped)", got.Title)
	}
	_ = b.Claim(blocker, "other", time.Hour, ref)
	if got, _ := b.NextTask("a", ref); got.ID != childID {
		t.Fatalf("got %q, want the leaf child once blocker is claimed", got.Title)
	}
	if got, _ := b.NextTask("other", ref); got.ID != blocker {
		t.Fatalf("own claim should stay eligible, got %q", got.Title)
	}
	if got, _ := b.NextTask("", ref); got.ID != childID {
		t.Fatalf("human must skip agent claims, got %q", got.Title)
	}
	// Shipping the blocker frees the blocked task.
	_ = b.Move(blocker, StatusDone, ref)
	if got, _ := b.NextTask("a", ref); got.ID != blocked {
		t.Fatalf("got %q, want unblocked high-priority task", got.Title)
	}
	// A subtask inherits its parent's blockers.
	_ = b.Move(blocked, StatusDoing, ref)
	for i := range b.Tasks {
		if b.Tasks[i].ID == parent {
			b.Tasks[i].BlockedBy = []string{blocked}
		}
	}
	if _, ok := b.NextTask("a", ref); ok {
		t.Fatal("picked a subtask whose parent is blocked")
	}
	// Deleted blockers stop blocking.
	_ = b.Delete(blocked)
	if got, _ := b.NextTask("a", ref); got.ID != childID {
		t.Fatalf("got %q, want child after its parent's blocker was deleted", got.Title)
	}
}

func TestUpdateLockedSerialisesClaims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	b, _ := Load(path)
	id := b.Add("x", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	agents := []string{"a", "b", "c", "d"}
	errs := make([]error, len(agents))
	var wg sync.WaitGroup
	for i, name := range agents {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			errs[i] = UpdateLocked(path, name, func(b *Board) error {
				return b.Claim(id, name, time.Hour, time.Now())
			})
		}(i, name)
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("claim winners = %d, want exactly 1 (errs %v)", wins, errs)
	}
	final, _ := Load(path)
	if final.Tasks[0].ClaimedBy == "" {
		t.Fatal("winning claim not persisted")
	}
}

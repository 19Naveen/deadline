package task

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func devBoard(n int) (*Board, []string) {
	b := &Board{}
	b.SetColumns(DevColumns)
	ids := make([]string, n)
	for i := range ids {
		ids[i] = b.Add("t", "", nil, ref).ID
	}
	return b, ids
}

func TestSetBlockedByValidates(t *testing.T) {
	b, ids := devBoard(3)
	a, c, d := ids[0], ids[1], ids[2]
	if err := b.SetBlockedBy(a, []string{c, "", c, d}, ref); err != nil {
		t.Fatal(err)
	}
	got, _ := b.TaskByID(a)
	if !slices.Equal(got.BlockedBy, []string{c, d}) {
		t.Fatalf("BlockedBy = %v, want deduped [%s %s]", got.BlockedBy, c, d)
	}
	if err := b.SetBlockedBy(a, []string{a}, ref); err == nil {
		t.Error("self-block accepted")
	}
	if err := b.SetBlockedBy(a, []string{"nope"}, ref); err == nil {
		t.Error("unknown blocker accepted")
	}
	if err := b.SetBlockedBy(a, nil, ref); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.TaskByID(a); got.BlockedBy != nil {
		t.Errorf("BlockedBy after clear = %v", got.BlockedBy)
	}
}

func TestSetBlockedByRejectsCycles(t *testing.T) {
	b, ids := devBoard(4)
	a, c, d, e := ids[0], ids[1], ids[2], ids[3]
	// a ← c ← d (a blocked by c, c blocked by d)
	if err := b.SetBlockedBy(a, []string{c}, ref); err != nil {
		t.Fatal(err)
	}
	if err := b.SetBlockedBy(c, []string{d}, ref); err != nil {
		t.Fatal(err)
	}
	// Direct cycle: c blocked by a.
	if err := b.SetBlockedBy(c, []string{a}, ref); err == nil {
		t.Error("direct cycle accepted")
	}
	// Transitive cycle: d blocked by a closes d → a → c → d.
	err := b.SetBlockedBy(d, []string{e, a}, ref)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("transitive cycle error = %v", err)
	}
	if got, _ := b.TaskByID(d); got.BlockedBy != nil {
		t.Errorf("rejected edit changed the board: %v", got.BlockedBy)
	}
	// A diamond is not a cycle.
	if err := b.SetBlockedBy(a, []string{c, d}, ref); err != nil {
		t.Errorf("diamond rejected: %v", err)
	}
}

func TestOpenBlockersIgnoresDoneAndDeleted(t *testing.T) {
	b, ids := devBoard(3)
	a, c, d := ids[0], ids[1], ids[2]
	b.SetBlockedBy(a, []string{c, d}, ref)
	b.Move(c, StatusShipped, ref)
	if open := b.OpenBlockers(a); len(open) != 1 || open[0].ID != d {
		t.Fatalf("OpenBlockers = %v, want only %s", open, d)
	}
	b.Delete(d)
	if open := b.OpenBlockers(a); len(open) != 0 {
		t.Errorf("deleted blocker still open: %v", open)
	}
	if all := b.Blockers(a); len(all) != 1 || all[0].ID != c {
		t.Errorf("Blockers = %v, want only the surviving %s", all, c)
	}
}

func TestSyncBlockedAutoBlocksAndUnblocks(t *testing.T) {
	b, ids := devBoard(3)
	dep, waiting, other := ids[0], ids[1], ids[2]
	b.Actor = "claude"
	b.Move(waiting, StatusInDev, ref)
	b.SetBlockedBy(waiting, []string{dep}, ref)

	later := ref.Add(time.Hour)
	moved := b.SyncBlocked(later)
	if !slices.Equal(moved, []string{waiting}) {
		t.Fatalf("SyncBlocked moved %v, want [%s]", moved, waiting)
	}
	got, _ := b.TaskByID(waiting)
	last := got.History[len(got.History)-1]
	if got.Status != StatusBlocked || last.Agent != DepsAgent || last.From != StatusInDev {
		t.Fatalf("auto-block = %+v / %+v", got.Status, last)
	}
	if got.LastAgent != "claude" {
		t.Errorf("LastAgent = %q, want the real actor kept", got.LastAgent)
	}
	if b.Actor != "claude" {
		t.Errorf("Actor = %q, want it restored", b.Actor)
	}
	if again := b.SyncBlocked(later); len(again) != 0 {
		t.Errorf("second sync moved %v, want nothing", again)
	}

	b.Move(dep, StatusShipped, later)
	moved = b.SyncBlocked(later)
	if !slices.Equal(moved, []string{waiting}) {
		t.Fatalf("unblock moved %v", moved)
	}
	if got, _ := b.TaskByID(waiting); got.Status != StatusInDev {
		t.Errorf("unblocked to %s, want back to indev", got.Status)
	}
	if got, _ := b.TaskByID(other); got.Status != StatusTodo {
		t.Errorf("unrelated task moved to %s", got.Status)
	}
}

func TestSyncBlockedKeepsManualBlocks(t *testing.T) {
	b, ids := devBoard(2)
	dep, manual := ids[0], ids[1]
	b.SetBlockedBy(manual, []string{dep}, ref)
	b.Actor = "claude"
	b.Move(manual, StatusBlocked, ref) // blocked by hand, not by the rule
	b.Move(dep, StatusShipped, ref)
	if moved := b.SyncBlocked(ref); len(moved) != 0 {
		t.Fatalf("SyncBlocked moved %v, want the manual block kept", moved)
	}
	// A card auto-blocked, then re-blocked by hand after a detour, stays.
	b2, ids2 := devBoard(2)
	dep2, w := ids2[0], ids2[1]
	b2.SetBlockedBy(w, []string{dep2}, ref)
	b2.SyncBlocked(ref)
	b2.Move(w, StatusTestingReview, ref) // human pulls it out (not auto-blockable)
	b2.Move(w, StatusBlocked, ref)       // and blocks it themselves
	b2.Move(dep2, StatusShipped, ref)
	if moved := b2.SyncBlocked(ref); len(moved) != 0 {
		t.Errorf("SyncBlocked moved manually re-blocked card %v", moved)
	}
}

func TestSyncBlockedClearedBlockersUnblock(t *testing.T) {
	b, ids := devBoard(2)
	dep, w := ids[0], ids[1]
	b.SetBlockedBy(w, []string{dep}, ref)
	b.SyncBlocked(ref)
	if got, _ := b.TaskByID(w); got.Status != StatusBlocked {
		t.Fatalf("todo card with open blocker = %s, want blocked", got.Status)
	}
	b.SetBlockedBy(w, nil, ref)
	b.SyncBlocked(ref)
	if got, _ := b.TaskByID(w); got.Status != StatusTodo {
		t.Errorf("cleared blockers left card in %s, want todo", got.Status)
	}
}

func TestSyncBlockedSkipsLaterColumnsAndArchived(t *testing.T) {
	b, ids := devBoard(3)
	dep, review, archived := ids[0], ids[1], ids[2]
	b.SetBlockedBy(review, []string{dep}, ref)
	b.SetBlockedBy(archived, []string{dep}, ref)
	b.Move(review, StatusTestingReview, ref)
	b.Tasks[2].Archived = true
	if moved := b.SyncBlocked(ref); len(moved) != 0 {
		t.Errorf("SyncBlocked moved %v, want testing-review and archived cards left alone", moved)
	}
}

func TestSyncBlockedNeedsBlockedColumn(t *testing.T) {
	b := &Board{Columns: []Status{StatusTodo, StatusDone}}
	dep := b.Add("d", "", nil, ref).ID
	w := b.Add("w", "", nil, ref).ID
	b.SetBlockedBy(w, []string{dep}, ref)
	if moved := b.SyncBlocked(ref); moved != nil {
		t.Errorf("SyncBlocked on a board without blocked moved %v", moved)
	}
}

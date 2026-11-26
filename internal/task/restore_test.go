package task

import (
	"path/filepath"
	"testing"
)

// Restoring a snapshot replaces the live task in place, keeping its slot.
func TestRestoreTaskReplacesByID(t *testing.T) {
	var b Board
	first := b.Add("[First]", "", nil, ref).ID
	b.Add("[Second]", "", nil, ref)
	before, _ := b.TaskByID(first)
	if err := b.Move(first, StatusDoing, ref); err != nil {
		t.Fatal(err)
	}

	b.RestoreTask(before)
	if len(b.Tasks) != 2 || b.Tasks[0].ID != first {
		t.Fatalf("restore did not keep the slot: %+v", b.Tasks)
	}
	if b.Tasks[0].Status != StatusTodo || len(b.Tasks[0].History) != 0 {
		t.Errorf("restored task = %+v, want the pre-move snapshot", b.Tasks[0])
	}
	if !b.Dirty() {
		t.Error("restore did not mark the board dirty")
	}
}

// A deleted task comes back and survives Save: its tombstone is cleared, so
// the disk merge does not drop it again.
func TestRestoreTaskReinsertsDeletedAndSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b := &Board{}
	b.SetPath(path)
	id := b.Add("[Doomed]", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	snap, _ := b.TaskByID(id)
	if err := b.Delete(id); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}

	b.RestoreTask(snap)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.TaskByID(id); !ok {
		t.Errorf("restored task missing after save: %+v", reloaded.Tasks)
	}
}

// Restoring an unsaved delete in the same session also clears the tombstone.
func TestRestoreTaskBeforeSaveClearsTombstone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b := &Board{}
	b.SetPath(path)
	id := b.Add("[Doomed]", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	snap, _ := b.TaskByID(id)
	_ = b.Delete(id)
	b.RestoreTask(snap)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := Load(path)
	if len(reloaded.Tasks) != 1 {
		t.Errorf("tasks after restore+save = %d, want 1", len(reloaded.Tasks))
	}
}

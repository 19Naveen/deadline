package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyBoard(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v, want nil", err)
	}
	if len(b.Tasks) != 0 {
		t.Errorf("Tasks = %+v, want empty", b.Tasks)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")

	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	id := b.Add("[Task title]", ref).ID
	if err := b.Move(id, StatusBlocked, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}

	got, err := Load(p)
	if err != nil {
		t.Fatalf("reload returned %v", err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(got.Tasks))
	}
	if got.Tasks[0].Status != StatusBlocked {
		t.Errorf("Status = %q, want %q", got.Tasks[0].Status, StatusBlocked)
	}
	if len(got.Tasks[0].History) != 1 {
		t.Errorf("len(History) = %d, want 1", len(got.Tasks[0].History))
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	b.Add("[Task title]", ref)
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "tasks.json" {
		t.Errorf("dir contents = %v, want only tasks.json", entries)
	}
}

func TestLoadCorruptFileErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned %v", err)
	}
	if _, err := Load(p); err == nil {
		t.Error("Load returned nil error for corrupt JSON, want an error")
	}
}

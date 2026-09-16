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
	if b.Dirty() {
		t.Error("Dirty() = true for a freshly loaded board, want false")
	}
}

func TestSaveClearsDirty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	b.Add("[Task title]", "", nil, ref)
	if !b.Dirty() {
		t.Fatal("Dirty() = false after Add, want true")
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}
	if b.Dirty() {
		t.Error("Dirty() = true after a successful Save, want false")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")

	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	id := b.Add("[Task title]", "", nil, ref).ID
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
	b.Add("[Task title]", "", nil, ref)
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

func TestLoadCoercesUnknownStatusToTodo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")
	body := `{"tasks":[{"id":"abc123","title":"[Ghost task]","status":"archived",` +
		`"created_at":"2026-07-30T12:00:00Z","updated_at":"2026-07-30T12:00:00Z"}]}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile returned %v", err)
	}

	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v, want nil", err)
	}
	if len(b.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(b.Tasks))
	}
	if b.Tasks[0].Status != StatusTodo {
		t.Errorf("Status = %q, want %q", b.Tasks[0].Status, StatusTodo)
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

// Two sessions saving in turn must keep both sides' cards: the agent
// logging while the TUI is open is the reason Save merges.
func TestSaveMergesConcurrentAdds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	tui, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	tui.Add("[TUI card]", "", nil, ref)
	if err := tui.Save(); err != nil {
		t.Fatal(err)
	}
	agent.Add("[Agent card]", "", nil, ref)
	if err := agent.Save(); err != nil {
		t.Fatal(err)
	}
	re, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (no lost update)", len(re.Tasks))
	}
}

// On the same task the later save wins whole: merging is per task, not
// per field, so concurrent edits of one card resolve last-writer-wins.
func TestSaveSessionEditsWinOverDisk(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	first, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	id := first.Add("[Title]", "", nil, ref).ID
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	mover, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	renamer, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := mover.Move(id, StatusDoing, ref); err != nil {
		t.Fatal(err)
	}
	if err := mover.Save(); err != nil {
		t.Fatal(err)
	}
	if err := renamer.Edit(id, "[Renamed]", "", nil, ref); err != nil {
		t.Fatal(err)
	}
	if err := renamer.Save(); err != nil {
		t.Fatal(err)
	}
	re, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if re.Tasks[0].Title != "[Renamed]" {
		t.Errorf("Title = %q, want [Renamed] (later save wins)", re.Tasks[0].Title)
	}
}

// A clean board with nothing new on disk writes nothing: read-only
// sessions stay read-only even with the merge step in Save.
func TestSaveCleanBoardWritesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.Add("[Title]", "", nil, ref)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := quiet.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("clean Save rewrote the file, want it untouched")
	}
}

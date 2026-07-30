package task

import (
	"errors"
	"testing"
	"time"
)

func TestBoardAddAppendsToTodo(t *testing.T) {
	var b Board
	got := b.Add("[Task title]", ref)
	if got == nil {
		t.Fatal("Add returned nil")
	}
	if len(b.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(b.Tasks))
	}
	if b.Tasks[0].Status != StatusTodo {
		t.Errorf("Status = %q, want %q", b.Tasks[0].Status, StatusTodo)
	}
}

func TestBoardMoveRecordsTransition(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", ref).ID
	later := ref.Add(time.Hour)

	if err := b.Move(id, StatusDoing, later); err != nil {
		t.Fatalf("Move returned %v, want nil", err)
	}

	got := b.Tasks[0]
	if got.Status != StatusDoing {
		t.Errorf("Status = %q, want %q", got.Status, StatusDoing)
	}
	if len(got.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(got.History))
	}
	tr := got.History[0]
	if tr.From != StatusTodo || tr.To != StatusDoing || !tr.At.Equal(later) {
		t.Errorf("History[0] = %+v, want {todo doing %v}", tr, later)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
	}
}

func TestBoardMoveToSameStatusIsNoOp(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", ref).ID
	if err := b.Move(id, StatusTodo, ref.Add(time.Hour)); err != nil {
		t.Fatalf("Move returned %v, want nil", err)
	}
	if len(b.Tasks[0].History) != 0 {
		t.Errorf("History = %v, want empty for a same-column move", b.Tasks[0].History)
	}
}

func TestBoardMoveUnknownID(t *testing.T) {
	var b Board
	if err := b.Move("missing", StatusDone, ref); !errors.Is(err, ErrNotFound) {
		t.Errorf("Move error = %v, want ErrNotFound", err)
	}
}

func TestBoardEdit(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", ref).ID
	later := ref.Add(time.Hour)
	if err := b.Edit(id, "[New title]", later); err != nil {
		t.Fatalf("Edit returned %v, want nil", err)
	}
	if b.Tasks[0].Title != "[New title]" {
		t.Errorf("Title = %q, want %q", b.Tasks[0].Title, "[New title]")
	}
	if !b.Tasks[0].UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", b.Tasks[0].UpdatedAt, later)
	}
}

func TestBoardDelete(t *testing.T) {
	var b Board
	keep := b.Add("[Keep]", ref).ID
	drop := b.Add("[Drop]", ref).ID

	if err := b.Delete(drop); err != nil {
		t.Fatalf("Delete returned %v, want nil", err)
	}
	if len(b.Tasks) != 1 || b.Tasks[0].ID != keep {
		t.Fatalf("Tasks = %+v, want only the kept task", b.Tasks)
	}
	if err := b.Delete(drop); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete error = %v, want ErrNotFound", err)
	}
}

func TestBoardByStatusPreservesOrder(t *testing.T) {
	var b Board
	first := b.Add("[First]", ref).ID
	second := b.Add("[Second]", ref).ID
	third := b.Add("[Third]", ref).ID
	if err := b.Move(second, StatusDoing, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	todo := b.ByStatus(StatusTodo)
	if len(todo) != 2 || todo[0].ID != first || todo[1].ID != third {
		t.Errorf("ByStatus(todo) = %+v, want [First Third]", todo)
	}
	if doing := b.ByStatus(StatusDoing); len(doing) != 1 || doing[0].ID != second {
		t.Errorf("ByStatus(doing) = %+v, want [Second]", doing)
	}
}

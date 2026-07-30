package task

import (
	"testing"
	"time"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

func TestNewTaskDefaults(t *testing.T) {
	got := NewTask("[Task title]", ref)
	if got.Title != "[Task title]" {
		t.Errorf("Title = %q, want %q", got.Title, "[Task title]")
	}
	if got.Status != StatusTodo {
		t.Errorf("Status = %q, want %q", got.Status, StatusTodo)
	}
	if !got.CreatedAt.Equal(ref) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ref)
	}
	if got.ID == "" {
		t.Error("ID is empty")
	}
	if len(got.History) != 0 {
		t.Errorf("History = %v, want empty", got.History)
	}
}

func TestNewTaskIDsAreUnique(t *testing.T) {
	a := NewTask("[Task title]", ref)
	b := NewTask("[Task title]", ref)
	if a.ID == b.ID {
		t.Fatalf("IDs collided: %q", a.ID)
	}
}

func TestStatusIndex(t *testing.T) {
	cases := map[Status]int{StatusTodo: 0, StatusDoing: 1, StatusBlocked: 2, StatusDone: 3}
	for s, want := range cases {
		if got := s.Index(); got != want {
			t.Errorf("%q.Index() = %d, want %d", s, got, want)
		}
	}
	if got := Status("nope").Index(); got != 0 {
		t.Errorf("unknown status index = %d, want 0", got)
	}
}

func TestCompletedAtReturnsLastDoneTransition(t *testing.T) {
	first := ref.Add(-48 * time.Hour)
	last := ref.Add(-2 * time.Hour)
	tk := Task{
		Status: StatusDone,
		History: []Transition{
			{From: StatusTodo, To: StatusDone, At: first},
			{From: StatusDone, To: StatusDoing, At: ref.Add(-24 * time.Hour)},
			{From: StatusDoing, To: StatusDone, At: last},
		},
	}
	got, ok := CompletedAt(tk)
	if !ok {
		t.Fatal("CompletedAt returned ok=false, want true")
	}
	if !got.Equal(last) {
		t.Errorf("CompletedAt = %v, want %v", got, last)
	}
}

func TestCompletedAtNotDone(t *testing.T) {
	tk := Task{Status: StatusDoing, History: []Transition{{From: StatusTodo, To: StatusDoing, At: ref}}}
	if _, ok := CompletedAt(tk); ok {
		t.Error("CompletedAt returned ok=true for a non-done task")
	}
}

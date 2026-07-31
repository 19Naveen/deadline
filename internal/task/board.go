package task

import (
	"errors"
	"strings"
	"time"
)

// ErrNotFound is returned when no task matches the given ID.
var ErrNotFound = errors.New("task not found")

// Board is the whole set of tasks. Columns are derived from Task.Status
// rather than stored separately, so a move is a single field write.
type Board struct {
	Tasks []Task `json:"tasks"`

	path  string // where Save writes; unexported so it stays out of the JSON
	dirty bool   // true when there are unsaved mutations; unexported, not serialised
}

// Dirty reports whether the board has mutations not yet written by Save.
func (b *Board) Dirty() bool { return b.dirty }

// Add appends a new todo task and returns a pointer into b.Tasks.
func (b *Board) Add(title string, now time.Time) *Task {
	b.Tasks = append(b.Tasks, NewTask(strings.TrimSpace(title), "", nil, now))
	b.dirty = true
	return &b.Tasks[len(b.Tasks)-1]
}

func (b *Board) find(id string) (*Task, error) {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			return &b.Tasks[i], nil
		}
	}
	return nil, ErrNotFound
}

// Move changes a task's column and records the transition. Moving to the
// column it already occupies is a no-op, so the history stays meaningful.
func (b *Board) Move(id string, to Status, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	if t.Status == to {
		return nil
	}
	t.History = append(t.History, Transition{From: t.Status, To: to, At: now})
	t.Status = to
	t.UpdatedAt = now
	b.dirty = true
	return nil
}

// Edit replaces the title. Blank titles are rejected so the board cannot
// grow unreadable empty cards.
func (b *Board) Edit(id, title string, now time.Time) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must not be blank")
	}
	t, err := b.find(id)
	if err != nil {
		return err
	}
	t.Title = title
	t.UpdatedAt = now
	b.dirty = true
	return nil
}

// Delete removes a task permanently.
func (b *Board) Delete(id string) error {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			b.Tasks = append(b.Tasks[:i], b.Tasks[i+1:]...)
			b.dirty = true
			return nil
		}
	}
	return ErrNotFound
}

// ByStatus returns the tasks in one column, in insertion order.
func (b *Board) ByStatus(s Status) []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if t.Status == s {
			out = append(out, t)
		}
	}
	return out
}

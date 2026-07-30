// Package task holds the todo domain model and its JSON persistence.
package task

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Status is the kanban column a task sits in.
type Status string

const (
	StatusTodo    Status = "todo"
	StatusDoing   Status = "doing"
	StatusBlocked Status = "blocked"
	StatusDone    Status = "done"
)

// Statuses lists the columns in left-to-right board order.
var Statuses = []Status{StatusTodo, StatusDoing, StatusBlocked, StatusDone}

// Label is the human-readable column heading.
func (s Status) Label() string {
	switch s {
	case StatusTodo:
		return "TODO"
	case StatusDoing:
		return "DOING"
	case StatusBlocked:
		return "BLOCKED"
	case StatusDone:
		return "DONE"
	}
	return string(s)
}

// Index is the column position, 0-based. Unknown statuses report 0.
func (s Status) Index() int {
	for i, c := range Statuses {
		if c == s {
			return i
		}
	}
	return 0
}

// Transition records one status change, so analytics can reconstruct history.
type Transition struct {
	From Status    `json:"from"`
	To   Status    `json:"to"`
	At   time.Time `json:"at"`
}

// Task is one card on the board.
type Task struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Status    Status       `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	History   []Transition `json:"history,omitempty"`
}

// NewTask builds a task in the todo column with a fresh random ID.
func NewTask(title string, now time.Time) Task {
	return Task{
		ID:        newID(),
		Title:     title,
		Status:    StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means the process is broken; a time-based
		// fallback keeps the app usable rather than panicking on Add.
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b)
}

// CompletedAt reports when the task last entered done. ok is false unless the
// task is currently done, so reopened tasks are not counted as completions.
func CompletedAt(t Task) (time.Time, bool) {
	if t.Status != StatusDone {
		return time.Time{}, false
	}
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].To == StatusDone {
			return t.History[i].At, true
		}
	}
	return time.Time{}, false
}

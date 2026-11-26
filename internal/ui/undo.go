package ui

import (
	"slices"

	"gotodo/internal/task"
)

// undoLimit caps the in-memory undo stack.
const undoLimit = 20

// undoEntry reverses one TUI change: restore puts snapshots back, removeID
// deletes a task the change added.
type undoEntry struct {
	what     string // "move Fix login", shown after "undone: "
	restore  []task.Task
	removeID string
	selectID string // card to select afterwards
}

// cloneTask deep-copies a snapshot so later in-place edits never reach it.
func cloneTask(t task.Task) task.Task {
	c := t
	c.Attachments = slices.Clone(t.Attachments)
	c.WorkingNotes = slices.Clone(t.WorkingNotes)
	c.Labels = slices.Clone(t.Labels)
	c.BlockedBy = slices.Clone(t.BlockedBy)
	c.History = slices.Clone(t.History)
	if t.Deadline != nil {
		d := *t.Deadline
		c.Deadline = &d
	}
	if t.ClaimUntil != nil {
		d := *t.ClaimUntil
		c.ClaimUntil = &d
	}
	if t.ArchivedAt != nil {
		d := *t.ArchivedAt
		c.ArchivedAt = &d
	}
	return c
}

// snapshotTasks copies the tasks with the given IDs as they are now.
func (m BoardModel) snapshotTasks(ids ...string) []task.Task {
	var out []task.Task
	for _, id := range ids {
		if t, ok := m.board.TaskByID(id); ok {
			out = append(out, cloneTask(t))
		}
	}
	return out
}

// pushUndo records a change, dropping the oldest past undoLimit.
func (m *BoardModel) pushUndo(e undoEntry) {
	m.undo = append(m.undo, e)
	if len(m.undo) > undoLimit {
		m.undo = slices.Clone(m.undo[len(m.undo)-undoLimit:])
	}
}

// undoLast reverses the newest change and reports whether there was one.
func (m *BoardModel) undoLast() bool {
	if len(m.undo) == 0 {
		m.info = "nothing to undo"
		return false
	}
	e := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	if e.removeID != "" {
		if err := m.board.Delete(e.removeID); err != nil && err != task.ErrNotFound {
			m.err = err.Error()
			return false
		}
		if m.familyID == e.removeID {
			m.familyID = ""
		}
	}
	for _, t := range e.restore {
		m.board.RestoreTask(cloneTask(t))
	}
	m.info = "undone: " + e.what
	m.reconcileFamily(e.selectID)
	return true
}

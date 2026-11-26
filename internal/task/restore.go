package task

// RestoreTask puts a snapshot of a task back on the board: it replaces the
// task with the same ID, or re-appends one that was deleted, clearing its
// tombstone so the next Save writes it rather than dropping it. Undo uses it.
func (b *Board) RestoreTask(t Task) {
	delete(b.deleted, t.ID)
	for i := range b.Tasks {
		if b.Tasks[i].ID == t.ID {
			b.Tasks[i] = t
			b.markChanged(t.ID)
			return
		}
	}
	b.Tasks = append(b.Tasks, t)
	b.markChanged(t.ID)
}

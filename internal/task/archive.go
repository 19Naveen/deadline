package task

import "time"

// ArchiveAfter is how long a task stays in the done column before it is
// swept off the board into the archive.
const ArchiveAfter = 14 * 24 * time.Hour

// SweepArchive archives every done task that has sat in done for at least
// ArchiveAfter, and reports how many it moved. It is idempotent: an already
// archived task is skipped. Tasks reopened out of done are never archived,
// because CompletedAt only reports a time for tasks currently in done.
func (b *Board) SweepArchive(now time.Time) int {
	moved := 0
	for i := range b.Tasks {
		t := &b.Tasks[i]
		if t.Archived || t.Status != StatusDone {
			continue
		}
		at, ok := CompletedAt(*t)
		if !ok || now.Sub(at) < ArchiveAfter {
			continue
		}
		stamp := now
		t.Archived = true
		t.ArchivedAt = &stamp
		t.UpdatedAt = now
		moved++
	}
	if moved > 0 {
		b.dirty = true
	}
	return moved
}

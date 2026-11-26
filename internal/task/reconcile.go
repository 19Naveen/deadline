package task

import "time"

// Reconcile applies the rules that follow any status change: dependency
// auto-block/unblock, then dropping claims on finished or archived cards.
// Callers run it after a move and before Save. It returns the IDs that
// SyncBlocked moved.
func (b *Board) Reconcile(now time.Time) []string {
	moved := b.SyncBlocked(now)
	b.ReleaseFinishedClaims(now)
	return moved
}

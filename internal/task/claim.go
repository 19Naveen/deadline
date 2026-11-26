package task

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// DefaultClaimTTL is how long a claim lasts when the caller gives no TTL:
// long enough for one working session, short enough that a crashed agent's
// card comes free the same day.
const DefaultClaimTTL = 2 * time.Hour

// ClaimHeld reports whether someone holds an unexpired claim on t. A claim
// without an expiry (hand-edited files) counts as held until released.
func (t Task) ClaimHeld(now time.Time) bool {
	if t.ClaimedBy == "" {
		return false
	}
	return t.ClaimUntil == nil || now.Before(*t.ClaimUntil)
}

// Claim marks a task as being worked on by agent until now+ttl. Re-claiming
// your own task extends it; an expired claim by anyone can be taken over.
// Claims only gate other agents' claims: humans can still move the card.
func (b *Board) Claim(id, agent string, ttl time.Duration, now time.Time) error {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return fmt.Errorf("claiming needs an agent name; set %s", AgentEnv)
	}
	if ttl <= 0 {
		return errors.New("claim ttl must be positive")
	}
	t, err := b.find(id)
	if err != nil {
		return err
	}
	if t.Archived {
		return errors.New("task is archived")
	}
	if t.Status == b.DoneStatus() {
		return fmt.Errorf("task is already %s", t.Status)
	}
	if t.ClaimHeld(now) && t.ClaimedBy != agent {
		return fmt.Errorf("task is claimed by %s%s", t.ClaimedBy, claimUntilSuffix(t.ClaimUntil))
	}
	until := now.Add(ttl)
	t.ClaimedBy = agent
	t.ClaimUntil = &until
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

// Release drops a task's claim. Agents may only release their own claim or
// an expired one; a human (agent == "") may release any claim. Releasing an
// unclaimed task is a no-op that reports false.
func (b *Board) Release(id, agent string, now time.Time) (bool, error) {
	t, err := b.find(id)
	if err != nil {
		return false, err
	}
	if t.ClaimedBy == "" {
		return false, nil
	}
	agent = strings.TrimSpace(agent)
	if agent != "" && t.ClaimedBy != agent && t.ClaimHeld(now) {
		return false, fmt.Errorf("task is claimed by %s%s; only they or a human can release it",
			t.ClaimedBy, claimUntilSuffix(t.ClaimUntil))
	}
	t.ClaimedBy, t.ClaimUntil = "", nil
	t.UpdatedAt = now
	b.markChanged(id)
	return true, nil
}

// ReleaseFinishedClaims drops claims on cards that reached the terminal
// column, so a shipped card never shows a stale owner. Coordination commands
// run it on every transaction because move itself does not touch claims.
func (b *Board) ReleaseFinishedClaims(now time.Time) int {
	n := 0
	done := b.DoneStatus()
	for i := range b.Tasks {
		t := &b.Tasks[i]
		if t.ClaimedBy == "" || (t.Status != done && !t.Archived) {
			continue
		}
		t.ClaimedBy, t.ClaimUntil = "", nil
		t.UpdatedAt = now
		b.markChanged(t.ID)
		n++
	}
	return n
}

// BlockingTasks returns the BlockedBy tasks (and the parent's, since a
// subtask cannot finish before its parent may start) that have not reached
// the terminal column. Deleted blockers no longer block.
func (b *Board) BlockingTasks(t Task) []Task {
	ids := append([]string(nil), t.BlockedBy...)
	if t.ParentID != "" {
		if parent, ok := b.TaskByID(t.ParentID); ok {
			ids = append(ids, parent.BlockedBy...)
		}
	}
	var out []Task
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if blocker, ok := b.TaskByID(id); ok && blocker.Status != b.DoneStatus() {
			out = append(out, blocker)
		}
	}
	return out
}

// NextTask picks the task an agent should start next: from the first column,
// not archived, not claimed by anyone else, not blocked by an unfinished
// task, and not a parent with open subtasks (work the leaves instead). Ties
// break by priority (high first), earliest deadline, then oldest created.
// agent "" (a human) treats every live claim as someone else's.
func (b *Board) NextTask(agent string, now time.Time) (Task, bool) {
	first := b.Statuses()[0]
	var candidates []Task
	for _, t := range b.Tasks {
		if t.Archived || t.Status != first {
			continue
		}
		if t.ClaimHeld(now) && (agent == "" || t.ClaimedBy != agent) {
			continue
		}
		if done, total := b.ChildProgress(t.ID); done != total {
			continue
		}
		if len(b.BlockingTasks(t)) > 0 {
			continue
		}
		candidates = append(candidates, t)
	}
	if len(candidates) == 0 {
		return Task{}, false
	}
	slices.SortStableFunc(candidates, func(x, y Task) int {
		if c := cmp.Compare(y.Priority, x.Priority); c != 0 {
			return c
		}
		switch {
		case x.Deadline != nil && y.Deadline == nil:
			return -1
		case x.Deadline == nil && y.Deadline != nil:
			return 1
		case x.Deadline != nil && y.Deadline != nil:
			if c := x.Deadline.Compare(*y.Deadline); c != 0 {
				return c
			}
		}
		if c := x.CreatedAt.Compare(y.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(x.ID, y.ID)
	})
	return candidates[0], true
}

// UpdateLocked loads path fresh while holding the board lock, runs fn acting
// as actor, and writes the result before unlocking. Save's merge lets the
// last writer win a race; claims need check-then-set, so they go through
// here and two agents can never both claim one card.
func UpdateLocked(path, actor string, fn func(*Board) error) error {
	lock, err := LockBoard(path)
	if err != nil {
		return err
	}
	defer lock.Close()
	b, err := Load(path)
	if err != nil {
		return err
	}
	b.Actor = actor
	if err := fn(b); err != nil {
		return err
	}
	if !b.dirty {
		return nil
	}
	return b.writeLocked()
}

// writeLocked writes the board atomically (temp file + rename). The caller
// must hold the board lock and have loaded the board under it.
func (b *Board) writeLocked() error {
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encode board: %w", err)
	}
	f, err := os.CreateTemp(dir, "tasks-*.json")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := os.Rename(tmp, b.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp: %w", err)
	}
	b.dirty = false
	b.settingsChanged = false
	b.snapshot()
	return nil
}

func claimUntilSuffix(until *time.Time) string { return claimUntilSuffixIn(until, time.Local) }

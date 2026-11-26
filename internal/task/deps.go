package task

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// DepsAgent is the actor SyncBlocked records on the transitions it makes.
// Only cards it moved into blocked are ever moved back out automatically.
const DepsAgent = "gotodo:deps"

// SetBlockedBy replaces the tasks that must reach the terminal column before
// id can proceed. blockers are exact task IDs (archived tasks allowed);
// blanks and duplicates are dropped and an empty list clears the field. It
// rejects unknown IDs, the task itself, and any change that would create a
// dependency cycle, leaving the board untouched on error.
func (b *Board) SetBlockedBy(id string, blockers []string, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	var clean []string
	for _, bid := range blockers {
		bid = strings.TrimSpace(bid)
		if bid == "" || slices.Contains(clean, bid) {
			continue
		}
		if bid == id {
			return errors.New("a task cannot be blocked by itself")
		}
		if _, err := b.find(bid); err != nil {
			return fmt.Errorf("blocker %s not found", bid)
		}
		clean = append(clean, bid)
	}
	if slices.Equal(t.BlockedBy, clean) {
		return nil
	}
	if cyc := b.dependencyCycle(id, clean); cyc != "" {
		return fmt.Errorf("dependency cycle: %s", cyc)
	}
	t.BlockedBy = clean
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

// dependencyCycle returns a readable path when giving id the blockers would
// let a blocker transitively depend on id, else "".
func (b *Board) dependencyCycle(id string, blockers []string) string {
	deps := make(map[string][]string, len(b.Tasks))
	for _, t := range b.Tasks {
		deps[t.ID] = t.BlockedBy
	}
	deps[id] = blockers
	// Depth-first search from id along blocked-by edges; reaching id again
	// closes a cycle. parent records the walk for the error message.
	parent := map[string]string{}
	visited := map[string]bool{}
	var found string
	var walk func(cur string) bool
	walk = func(cur string) bool {
		for _, next := range deps[cur] {
			if next == id {
				path := []string{shortRef(id)}
				for n := cur; n != id; n = parent[n] {
					path = append(path, shortRef(n))
				}
				path = append(path, shortRef(id))
				slices.Reverse(path)
				found = strings.Join(path, " → ")
				return true
			}
			if visited[next] {
				continue
			}
			visited[next] = true
			parent[next] = cur
			if walk(next) {
				return true
			}
		}
		return false
	}
	walk(id)
	return found
}

// shortRef is the 8-character ID prefix used in messages.
func shortRef(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Blockers returns copies of every task id is blocked by, in BlockedBy
// order. IDs of tasks that no longer exist are skipped.
func (b *Board) Blockers(id string) []Task {
	t, err := b.find(id)
	if err != nil {
		return nil
	}
	var out []Task
	for _, bid := range t.BlockedBy {
		if bt, ok := b.TaskByID(bid); ok {
			out = append(out, bt)
		}
	}
	return out
}

// OpenBlockers returns the blockers not yet in the terminal column.
func (b *Board) OpenBlockers(id string) []Task {
	var open []Task
	for _, bt := range b.Blockers(id) {
		if bt.Status != b.DoneStatus() {
			open = append(open, bt)
		}
	}
	return open
}

// autoBlockable reports whether SyncBlocked may move a card in s into
// blocked: the first column and the active one right after it.
func (b *Board) autoBlockable(s Status) bool {
	i := b.ColumnIndex(s)
	return (i == 0 || i == 1) && s != StatusBlocked && s != b.DoneStatus()
}

// autoBlocked reports whether t's latest transition into its current
// (blocked) column was made by SyncBlocked, and where it came from.
func autoBlocked(t Task) (from Status, ok bool) {
	if len(t.History) == 0 {
		return "", false
	}
	last := t.History[len(t.History)-1]
	if last.To != t.Status || last.Agent != DepsAgent {
		return "", false
	}
	return last.From, true
}

// SyncBlocked applies dependency rules and returns the IDs it moved:
//   - an active card in the first or active column with open blockers moves
//     to blocked (on boards that have a blocked column);
//   - a card SyncBlocked itself moved into blocked returns to the column it
//     came from once all its blockers are done (or its blockers are cleared).
//
// Cards a person or agent moved into blocked are never moved out. Its
// transitions are recorded with Agent DepsAgent, and Task.LastAgent keeps
// naming the last real actor. Save does not call it: headless commands and
// the TUI call it after mutating, before saving.
func (b *Board) SyncBlocked(now time.Time) []string {
	if !b.HasStatus(StatusBlocked) {
		return nil
	}
	var moved []string
	actor := b.Actor
	b.Actor = DepsAgent
	defer func() { b.Actor = actor }()
	for i := range b.Tasks {
		t := b.Tasks[i]
		if t.Archived {
			continue
		}
		var to Status
		switch {
		case b.autoBlockable(t.Status) && len(b.OpenBlockers(t.ID)) > 0:
			to = StatusBlocked
		case t.Status == StatusBlocked && len(b.OpenBlockers(t.ID)) == 0:
			from, ok := autoBlocked(t)
			if !ok {
				continue
			}
			to = from
			if !b.HasStatus(to) || to == StatusBlocked || to == b.DoneStatus() {
				to = b.Statuses()[0]
			}
		default:
			continue
		}
		if err := b.Move(t.ID, to, now); err != nil {
			continue
		}
		b.Tasks[i].LastAgent = t.LastAgent
		moved = append(moved, t.ID)
	}
	return moved
}

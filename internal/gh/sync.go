package gh

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gotodo/internal/task"
)

// dateLayout is the Singapore DD/MM/YYYY format used in board text.
const dateLayout = "02/01/2006"

// ActionKind classifies one sync outcome.
type ActionKind string

const (
	ActionShipped  ActionKind = "shipped"  // merged PR moved the task to the terminal column
	ActionAwaiting ActionKind = "awaiting" // merged, but the approval gate needs a human
	ActionRefused  ActionKind = "refused"  // merged, but the move failed for another reason
	ActionBlocked  ActionKind = "blocked"  // failing CI moved the task to blocked
	ActionNoted    ActionKind = "noted"    // failing CI noted in the description only
	ActionError    ActionKind = "error"    // gh could not report on the PR
)

// Action is one line of sync output.
type Action struct {
	Kind   ActionKind
	TaskID string
	Title  string
	PR     string // PR URL when gh reported one, else the stored reference
	Detail string
}

// String renders the action as one output line; dry-run lines say "would".
func (a Action) String(dryRun bool) string {
	id := a.TaskID
	if len(id) > 8 {
		id = id[:8]
	}
	would := ""
	if dryRun {
		would = "would "
	}
	switch a.Kind {
	case ActionShipped:
		return fmt.Sprintf("%smove %s %s → %s (PR %s merged)", would, id, a.Title, a.Detail, a.PR)
	case ActionAwaiting:
		return fmt.Sprintf("%s %s: PR %s merged, awaits human approval to ship (move it in the TUI)", id, a.Title, a.PR)
	case ActionRefused:
		return fmt.Sprintf("%s %s: PR %s merged, but cannot ship: %s", id, a.Title, a.PR, a.Detail)
	case ActionBlocked:
		return fmt.Sprintf("%smove %s %s → %s (CI failing on PR %s)", would, id, a.Title, a.Detail, a.PR)
	case ActionNoted:
		return fmt.Sprintf("%snote %s %s: CI failing on PR %s", would, id, a.Title, a.PR)
	default:
		return fmt.Sprintf("%s %s: cannot check PR %s: %s", id, a.Title, a.PR, a.Detail)
	}
}

// CINote is the description line recorded when a PR's checks fail.
func CINote(prURL string, now time.Time) string {
	return fmt.Sprintf("%s (%s)", ciNotePrefix(prURL), now.Format(dateLayout))
}

func ciNotePrefix(prURL string) string { return "CI failing on PR " + prURL }

// Sync checks every open task with a linked PR and applies the result to b
// in memory: merged PRs ship the task, failing CI blocks it and notes the
// failure once. Moves are attributed to b.Actor. The caller decides whether
// to Save (a dry run simply doesn't). Errors from gh are reported per task.
func Sync(ctx context.Context, b *task.Board, r Runner, now time.Time) []Action {
	done := b.DoneStatus()
	var actions []Action
	for _, t := range b.Active() {
		if t.PR == "" || t.Status == done {
			continue
		}
		st, err := View(ctx, r, t.PR)
		if err != nil {
			actions = append(actions, Action{Kind: ActionError, TaskID: t.ID, Title: t.Title, PR: t.PR, Detail: err.Error()})
			continue
		}
		url := st.URL
		if url == "" {
			url = t.PR
		}
		a := Action{TaskID: t.ID, Title: t.Title, PR: url}
		switch {
		case st.Merged():
			if err := b.Move(t.ID, done, now); err != nil {
				a.Kind, a.Detail = ActionRefused, err.Error()
				if finished, total := b.ChildProgress(t.ID); finished == total && b.RequireApproval && b.Actor != "" {
					a.Kind = ActionAwaiting
				}
			} else {
				a.Kind, a.Detail = ActionShipped, string(done)
			}
			actions = append(actions, a)
		case st.Failing():
			moved := false
			if b.HasStatus(task.StatusBlocked) && t.Status != task.StatusBlocked {
				if err := b.Move(t.ID, task.StatusBlocked, now); err != nil {
					actions = append(actions, Action{Kind: ActionError, TaskID: t.ID, Title: t.Title, PR: url, Detail: err.Error()})
					continue
				}
				moved = true
			}
			noted := false
			if !strings.Contains(t.Description, ciNotePrefix(url)) {
				desc := strings.TrimSpace(t.Description)
				if desc != "" {
					desc += "\n"
				}
				desc += CINote(url, now)
				if err := b.EditWithParent(t.ID, t.ParentID, t.Title, desc, t.Deadline, now); err != nil {
					actions = append(actions, Action{Kind: ActionError, TaskID: t.ID, Title: t.Title, PR: url, Detail: err.Error()})
					continue
				}
				noted = true
			}
			switch {
			case moved:
				a.Kind, a.Detail = ActionBlocked, string(task.StatusBlocked)
				actions = append(actions, a)
			case noted:
				a.Kind = ActionNoted
				actions = append(actions, a)
			}
		}
	}
	// Shipped or blocked cards change dependants and claims.
	b.Reconcile(now)
	return actions
}

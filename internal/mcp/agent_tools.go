package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gotodo/internal/task"
)

// agentTools are the coordination tools: pick the next task, claim and
// release cards, and read a task's resume pack. Claims go through
// task.UpdateLocked so two agents can never both hold one card.
func agentTools() []Tool {
	ttl := map[string]any{
		"type":        "integer",
		"minimum":     1,
		"maximum":     1440,
		"description": "Claim duration in minutes (default 120).",
	}
	return []Tool{
		{
			Name:  "next_task",
			Title: "Next task",
			Description: "Pick the best task to work on next: unclaimed, unblocked, in the first column, " +
				"highest priority, then earliest deadline. With claim=true it is claimed for you so other " +
				"agents skip it; with start=true it is also moved to the active column. Use this to pick up work.",
			InputSchema: objectSchema(map[string]any{
				"claim":       map[string]any{"type": "boolean", "description": "Claim the task for yourself."},
				"start":       map[string]any{"type": "boolean", "description": "Also move it to the active column (implies claim)."},
				"ttl_minutes": ttl,
			}),
			SavesItself: true,
			Handler:     toolNextTask,
		},
		{
			Name:  "claim_task",
			Title: "Claim task",
			Description: "Claim a specific task so other agents skip it. Fails if another agent holds a live " +
				"claim. Claiming again extends your own claim.",
			InputSchema: objectSchema(map[string]any{
				"id":          stringProp("Task ID or unique ID prefix."),
				"ttl_minutes": ttl,
			}, "id"),
			Idempotent:  true,
			SavesItself: true,
			Handler:     toolClaimTask,
		},
		{
			Name:        "release_task",
			Title:       "Release task",
			Description: "Release your claim on a task when you stop working on it without finishing.",
			InputSchema: objectSchema(map[string]any{
				"id": stringProp("Task ID or unique ID prefix."),
			}, "id"),
			Idempotent:  true,
			SavesItself: true,
			Handler:     toolReleaseTask,
		},
		{
			Name:  "task_context",
			Title: "Task context",
			Description: "Everything needed to resume a task: fields, parent, subtasks, blockers, attachment " +
				"paths, working-note contents (small text files), description and recent history. " +
				"Read this before continuing work another agent started.",
			InputSchema: objectSchema(map[string]any{
				"id": stringProp("Task ID or unique ID prefix."),
			}, "id"),
			ReadOnly:   true,
			Idempotent: true,
			Handler:    toolTaskContext,
		},
	}
}

func priorityProp(desc string) map[string]any {
	return map[string]any{"type": "string", "enum": []string{"none", "low", "medium", "high"}, "description": desc}
}

func claimTTL(minutes int) time.Duration {
	if minutes <= 0 {
		return task.DefaultClaimTTL
	}
	return time.Duration(minutes) * time.Minute
}

type nextArgs struct {
	Claim      bool `json:"claim"`
	Start      bool `json:"start"`
	TTLMinutes int  `json:"ttl_minutes"`
}

func toolNextTask(c *Call) (string, error) {
	var a nextArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	agent := c.Board.Actor
	var picked task.Task
	var found bool
	err := task.UpdateLocked(c.Path, agent, func(b *task.Board) error {
		b.SweepArchive(c.Now)
		b.ReleaseFinishedClaims(c.Now)
		picked, found = b.NextTask(agent, c.Now)
		if !found || (!a.Claim && !a.Start) {
			return nil
		}
		if agent != "" {
			if err := b.Claim(picked.ID, agent, claimTTL(a.TTLMinutes), c.Now); err != nil {
				return err
			}
		}
		if a.Start {
			cols := b.Statuses()
			if len(cols) < 2 {
				return errors.New("board has no active column to start in")
			}
			if err := b.Move(picked.ID, cols[1], c.Now); err != nil {
				return fmt.Errorf("start %s: %w", shortID(picked.ID), err)
			}
			b.Reconcile(c.Now)
		}
		picked, _ = b.TaskByID(picked.ID)
		return nil
	})
	if err != nil {
		return "", err
	}
	if !found {
		return "no available tasks", nil
	}
	out := fmt.Sprintf("%s [%s] %s", shortID(picked.ID), picked.Status, picked.Title)
	if picked.ClaimedBy != "" && picked.ClaimUntil != nil {
		out += fmt.Sprintf("\nclaimed by %s until %s", picked.ClaimedBy, picked.ClaimUntil.In(c.Now.Location()).Format("02/01/2006 15:04"))
	}
	if picked.Description != "" {
		out += "\n\n" + picked.Description
	}
	return out + "\n\nCall task_context with this ID for the full resume pack.", nil
}

type claimArgs struct {
	ID         string `json:"id"`
	TTLMinutes int    `json:"ttl_minutes"`
}

func toolClaimTask(c *Call) (string, error) {
	var a claimArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	agent := c.Board.Actor
	if agent == "" {
		return "", errors.New("claims need an agent name (set GOTODO_AGENT)")
	}
	var claimed task.Task
	err := task.UpdateLocked(c.Path, agent, func(b *task.Board) error {
		b.ReleaseFinishedClaims(c.Now)
		t, err := resolveID(b, a.ID)
		if err != nil {
			return err
		}
		if err := b.Claim(t.ID, agent, claimTTL(a.TTLMinutes), c.Now); err != nil {
			return fmt.Errorf("claim %s: %w", shortID(t.ID), err)
		}
		claimed, _ = b.TaskByID(t.ID)
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("claimed %s [%s] %s for %s until %s", shortID(claimed.ID), claimed.Status, claimed.Title,
		agent, claimed.ClaimUntil.In(c.Now.Location()).Format("02/01/2006 15:04")), nil
}

func toolReleaseTask(c *Call) (string, error) {
	var a idArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	agent := c.Board.Actor
	var t task.Task
	var released bool
	err := task.UpdateLocked(c.Path, agent, func(b *task.Board) error {
		var err error
		if t, err = resolveID(b, a.ID); err != nil {
			return err
		}
		released, err = b.Release(t.ID, agent, c.Now)
		return err
	})
	if err != nil {
		return "", err
	}
	if !released {
		return fmt.Sprintf("%s %s was not claimed", shortID(t.ID), t.Title), nil
	}
	return fmt.Sprintf("released %s %s", shortID(t.ID), t.Title), nil
}

func toolTaskContext(c *Call) (string, error) {
	var a idArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	t, err := c.Resolve(a.ID)
	if err != nil {
		return "", err
	}
	var roots []string
	if root := projectRoot(c.Path); root != "" {
		roots = append(roots, root)
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	return c.Board.Context(t.ID, task.ContextOptions{Now: c.Now, NoteRoots: roots})
}

// projectRoot is <root> for a <root>/.deadline/board.json path, else "".
func projectRoot(boardPath string) string {
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) != ".deadline" {
		return ""
	}
	return filepath.Dir(dir)
}

// applyMeta sets priority, labels, blockers and PR. Nil pointers leave a
// field alone; blocker prefixes resolve to full IDs.
func (c *Call) applyMeta(id string, priority *string, labels, blockedBy *[]string, pr *string) error {
	b := c.Board
	if priority != nil {
		p, err := task.ParsePriority(*priority)
		if err != nil {
			return err
		}
		if err := b.SetPriority(id, p, c.Now); err != nil {
			return err
		}
	}
	if labels != nil {
		if err := b.SetLabels(id, *labels, c.Now); err != nil {
			return err
		}
	}
	if blockedBy != nil {
		ids := make([]string, 0, len(*blockedBy))
		for _, prefix := range *blockedBy {
			if strings.TrimSpace(prefix) == "" {
				continue
			}
			t, err := c.Resolve(prefix)
			if err != nil {
				return fmt.Errorf("blocked_by: %w", err)
			}
			ids = append(ids, t.ID)
		}
		if err := b.SetBlockedBy(id, ids, c.Now); err != nil {
			return err
		}
	}
	if pr != nil {
		if err := b.SetPR(id, *pr, c.Now); err != nil {
			return err
		}
	}
	return nil
}

// syncedLines reports cards the dependency rule moved, one per line.
func (c *Call) syncedLines(moved []string) string {
	var out strings.Builder
	for _, id := range moved {
		t, ok := c.Board.TaskByID(id)
		if !ok {
			continue
		}
		if t.Status == task.StatusBlocked {
			fmt.Fprintf(&out, "\nauto-blocked %s %s", shortID(t.ID), t.Title)
		} else {
			fmt.Fprintf(&out, "\nunblocked %s %s → %s", shortID(t.ID), t.Title, t.Status)
		}
	}
	return out.String()
}

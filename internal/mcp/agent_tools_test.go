package mcp

import (
	"strings"
	"testing"

	"gotodo/internal/task"
)

// The coordination tools and new fields work end to end: blockers
// auto-block, next_task skips blocked work and claims, a second agent can't
// claim the same card, shipping the blocker unblocks the dependant.
func TestAgentToolsEndToEnd(t *testing.T) {
	path := devBoard(t)
	c := newClient(t, Config{Path: path})
	c.initialize("claude")

	schema := addedID(t, c.mustCall("add_task", map[string]any{"title": "Schema", "priority": "low"}))
	api := c.mustCall("add_task", map[string]any{
		"title": "API", "priority": "high", "labels": []string{"Backend"},
		"blocked_by": []string{schema}, "status": "indev", "pr": "42",
	})
	apiID := addedID(t, api)
	if !strings.Contains(api, "auto-blocked "+apiID) {
		t.Errorf("add_task = %q, want an auto-blocked line", api)
	}
	got := loadTask(t, path, apiID)
	if got.Status != task.StatusBlocked || got.Priority != task.PriorityHigh || !got.HasLabel("backend") || got.PR == "" {
		t.Fatalf("API task = %+v", got)
	}

	next := c.mustCall("next_task", map[string]any{"start": true})
	if !strings.HasPrefix(next, schema+" [indev] Schema") || !strings.Contains(next, "claimed by claude") {
		t.Fatalf("next_task = %q, want Schema started and claimed", next)
	}

	other := newClient(t, Config{Path: path})
	other.initialize("codex")
	if msg := other.mustFail("claim_task", map[string]any{"id": schema}); !strings.Contains(msg, "claim") {
		t.Errorf("second claim error = %q", msg)
	}

	if ctx := c.mustCall("task_context", map[string]any{"id": apiID}); !strings.Contains(ctx, "Schema") {
		t.Errorf("task_context = %q, want the blocker listed", ctx)
	}

	moved := c.mustCall("move_task", map[string]any{"id": schema, "status": "shipped"})
	if !strings.Contains(moved, "unblocked "+apiID) {
		t.Errorf("move_task = %q, want an unblocked line", moved)
	}
	if done := loadTask(t, path, schema); done.ClaimedBy != "" {
		t.Errorf("claim survived shipping: %q", done.ClaimedBy)
	}
	if out := c.mustCall("edit_task", map[string]any{"id": apiID, "labels": []string{}, "priority": "none"}); !strings.Contains(out, "edited") {
		t.Errorf("edit_task = %q", out)
	}
	if got := loadTask(t, path, apiID); len(got.Labels) != 0 || got.Priority != task.PriorityNone {
		t.Errorf("edit did not clear: %+v", got)
	}
	if out := c.mustCall("release_task", map[string]any{"id": apiID}); !strings.Contains(out, "was not claimed") {
		t.Errorf("release_task = %q", out)
	}
}

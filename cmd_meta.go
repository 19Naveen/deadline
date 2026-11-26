package main

import (
	"fmt"
	"strings"
	"time"

	"gotodo/internal/task"
)

// resolveBlockers turns -blocked-by ID prefixes into full IDs. Blank values
// are dropped, so `-blocked-by ""` yields an empty list (edit uses it to
// clear).
func resolveBlockers(b *task.Board, prefixes []string) ([]string, error) {
	var ids []string
	for _, p := range prefixes {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		t, err := resolveID(b, p)
		if err != nil {
			return nil, fmt.Errorf("blocked-by: %w", err)
		}
		ids = append(ids, t.ID)
	}
	return ids, nil
}

// printSynced prints one receipt line per card Board.SyncBlocked moved.
func printSynced(b *task.Board, moved []string) {
	for _, id := range moved {
		t, ok := b.TaskByID(id)
		if !ok {
			continue
		}
		if t.Status == task.StatusBlocked {
			fmt.Printf("auto-blocked %s %s (waiting on %s)\n", shortID(t.ID), t.Title, shortIDs(b.OpenBlockers(t.ID)))
		} else {
			fmt.Printf("unblocked %s %s → %s\n", shortID(t.ID), t.Title, t.Status)
		}
	}
}

func shortIDs(tasks []task.Task) string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, shortID(t.ID))
	}
	return strings.Join(ids, ", ")
}

// metaSuffix is the list-line tail for priority, labels, open blockers,
// claim, PR and last agent; empty fields are omitted.
func metaSuffix(b *task.Board, t task.Task) string {
	var s strings.Builder
	if t.Priority != task.PriorityNone {
		s.WriteString(" · P:" + t.Priority.String())
	}
	if len(t.Labels) > 0 {
		s.WriteString(" · #" + strings.Join(t.Labels, " #"))
	}
	if open := b.OpenBlockers(t.ID); len(open) > 0 {
		s.WriteString(" · blocked by " + shortIDs(open))
	}
	if t.ClaimedBy != "" {
		s.WriteString(" · claimed by " + t.ClaimedBy)
	}
	if t.PR != "" {
		s.WriteString(" · PR " + t.PR)
	}
	if t.LastAgent != "" {
		s.WriteString(" · last " + t.LastAgent)
	}
	return s.String()
}

// printShowMeta prints show's priority, labels, blockers, claim, PR and last
// agent lines; empty fields are omitted.
func printShowMeta(b *task.Board, t task.Task) {
	if t.Priority != task.PriorityNone {
		fmt.Printf("priority: %s\n", t.Priority)
	}
	if len(t.Labels) > 0 {
		fmt.Printf("labels: %s\n", strings.Join(t.Labels, ", "))
	}
	if blockers := b.Blockers(t.ID); len(blockers) > 0 {
		fmt.Println("blocked by:")
		for _, bt := range blockers {
			fmt.Printf("  %s [%s] %s\n", shortID(bt.ID), bt.Status, bt.Title)
		}
	}
	if t.ClaimedBy != "" {
		line := "claimed by: " + t.ClaimedBy
		if t.ClaimUntil != nil {
			line += " until " + t.ClaimUntil.In(time.Local).Format(dateLayout+" 15:04")
		}
		fmt.Println(line)
	}
	if t.PR != "" {
		fmt.Printf("pr: %s\n", t.PR)
	}
	if t.LastAgent != "" {
		fmt.Printf("last agent: %s\n", t.LastAgent)
	}
}

// printWIPWarnings prints `! <col> over WIP limit (n/limit)` for each listed
// column over its cap.
func printWIPWarnings(b *task.Board, only task.Status) {
	for _, s := range b.Statuses() {
		if only != "" && s != only {
			continue
		}
		if count, limit, over := b.OverWIP(s); over {
			fmt.Printf("! %s over WIP limit (%d/%d)\n", s, count, limit)
		}
	}
}

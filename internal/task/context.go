package task

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ContextNoteLimit caps how big a working note may be and still be inlined
// into a context dump; larger notes are listed by path only.
const ContextNoteLimit = 64 * 1024

// contextHistoryLimit is how many recent transitions a context dump shows.
const contextHistoryLimit = 10

// ContextOptions tunes a context dump.
type ContextOptions struct {
	Now time.Time
	// NoteRoots are directories tried, in order, for relative working-note
	// paths (typically the project root, then the working directory).
	NoteRoots []string
}

// Context renders one task as markdown an agent can read to resume work:
// metadata, claim, family, blockers, document paths, inlined working notes,
// description and recent history. Attachment contents are never read, only
// listed, since they may be large or binary references for humans.
func (b *Board) Context(id string, opts ContextOptions) (string, error) {
	t, err := b.find(id)
	if err != nil {
		return "", err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	loc := now.Location()
	stamp := func(at time.Time) string { return at.In(loc).Format("02/01/2006 15:04") }

	var w strings.Builder
	fmt.Fprintf(&w, "# %s\n\n", t.Title)
	field := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&w, "- %s: %s\n", name, value)
		}
	}
	field("id", t.ID)
	status := string(t.Status)
	if t.Archived {
		status += " (archived)"
	}
	field("status", status)
	field("priority", contextPriority(t.Priority))
	field("labels", strings.Join(t.Labels, ", "))
	if t.Deadline != nil {
		field("deadline", t.Deadline.In(loc).Format("02/01/2006"))
	}
	field("worktree", t.Worktree)
	field("pr", t.PR)
	if t.ClaimedBy != "" {
		claim := t.ClaimedBy + claimUntilSuffixIn(t.ClaimUntil, loc)
		if !t.ClaimHeld(now) {
			claim += " (expired)"
		}
		field("claimed by", claim)
	}
	field("last agent", t.LastAgent)
	field("created", stamp(t.CreatedAt))
	field("updated", stamp(t.UpdatedAt))

	if t.ParentID != "" {
		w.WriteString("\n## Parent\n\n")
		if parent, ok := b.TaskByID(t.ParentID); ok {
			fmt.Fprintf(&w, "- %s [%s] %s\n", contextShortID(parent.ID), parent.Status, parent.Title)
		} else {
			fmt.Fprintf(&w, "- %s (missing)\n", contextShortID(t.ParentID))
		}
	}

	if children := b.Children(t.ID); len(children) > 0 {
		done, total := b.ChildProgress(t.ID)
		fmt.Fprintf(&w, "\n## Subtasks (%d/%d complete)\n\n", done, total)
		for _, c := range children {
			fmt.Fprintf(&w, "- %s [%s] %s\n", contextShortID(c.ID), c.Status, c.Title)
		}
	}

	if len(t.BlockedBy) > 0 {
		w.WriteString("\n## Blocked by\n\n")
		for _, bid := range t.BlockedBy {
			blocker, ok := b.TaskByID(bid)
			if !ok {
				fmt.Fprintf(&w, "- %s (deleted, no longer blocks)\n", contextShortID(bid))
				continue
			}
			state := "open"
			if blocker.Status == b.DoneStatus() {
				state = "done"
			}
			fmt.Fprintf(&w, "- %s [%s] %s (%s)\n", contextShortID(blocker.ID), blocker.Status, blocker.Title, state)
		}
	}

	if len(t.Attachments) > 0 {
		w.WriteString("\n## Attachments\n\n")
		for _, p := range t.Attachments {
			fmt.Fprintf(&w, "- %s\n", p)
		}
	}

	if len(t.WorkingNotes) > 0 {
		w.WriteString("\n## Working notes\n")
		for _, p := range t.WorkingNotes {
			fmt.Fprintf(&w, "\n### %s\n\n", p)
			body, note := readContextNote(p, opts.NoteRoots)
			if note != "" {
				fmt.Fprintf(&w, "(%s)\n", note)
				continue
			}
			fence := contextFence(body)
			fmt.Fprintf(&w, "%s\n%s", fence, body)
			if !strings.HasSuffix(body, "\n") {
				w.WriteString("\n")
			}
			fmt.Fprintf(&w, "%s\n", fence)
		}
	}

	if t.Description != "" {
		fmt.Fprintf(&w, "\n## Description\n\n%s\n", t.Description)
	}

	if len(t.History) > 0 {
		hist := t.History
		if len(hist) > contextHistoryLimit {
			hist = hist[len(hist)-contextHistoryLimit:]
		}
		fmt.Fprintf(&w, "\n## History (last %d of %d)\n\n", len(hist), len(t.History))
		for _, tr := range hist {
			who := tr.Agent
			if who == "" {
				who = "human"
			}
			fmt.Fprintf(&w, "- %s %s → %s by %s\n", stamp(tr.At), tr.From, tr.To, who)
		}
	}
	return w.String(), nil
}

// readContextNote returns a note's text, or a short reason it was skipped.
// Absolute paths are read as-is; relative ones are tried under each root.
func readContextNote(p string, roots []string) (body, reason string) {
	candidates := []string{p}
	if !filepath.IsAbs(p) {
		candidates = candidates[:0]
		for _, r := range roots {
			if r != "" {
				candidates = append(candidates, filepath.Join(r, p))
			}
		}
	}
	for _, c := range candidates {
		st, err := os.Stat(c)
		if err != nil {
			continue
		}
		if st.IsDir() {
			return "", "directory, not inlined"
		}
		if st.Size() >= ContextNoteLimit {
			return "", fmt.Sprintf("%d KB, too large to inline; read the file directly", st.Size()/1024)
		}
		data, err := os.ReadFile(c)
		if err != nil {
			return "", "unreadable: " + err.Error()
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return "", "binary, not inlined"
		}
		return string(data), ""
	}
	return "", "not found"
}

// contextFence returns a backtick fence longer than any run inside body, so
// a note containing its own code blocks cannot close the fence early.
func contextFence(body string) string {
	longest, run := 0, 0
	for _, r := range body {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}

func contextPriority(p Priority) string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityMedium:
		return "medium"
	case PriorityHigh:
		return "high"
	}
	return ""
}

func contextShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func claimUntilSuffixIn(until *time.Time, loc *time.Location) string {
	if until == nil {
		return ""
	}
	return " until " + until.In(loc).Format("02/01/2006 15:04")
}

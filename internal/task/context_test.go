package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContextIncludesNotesButNotAttachments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("NOTE-BODY\n```go\nx\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spec.pdf"), []byte("ATTACHMENT-BODY"), 0o644); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("y", ContextNoteLimit)
	if err := os.WriteFile(filepath.Join(root, "big.md"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}

	b := &Board{}
	b.SetColumns(DevColumns)
	loc := time.FixedZone("SGT", 8*3600)
	at := time.Date(2026, 11, 26, 9, 5, 0, 0, loc)
	dl := time.Date(2026, 12, 4, 0, 0, 0, 0, loc)
	parent := b.Add("Parent work", "", nil, at).ID
	child, _ := b.AddChild(parent, "Child work", "Do the thing", &dl, at)
	id := child.ID
	sib, _ := b.AddChild(parent, "Sibling", "", nil, at)
	_ = sib
	dep := b.Add("Dependency", "", nil, at).ID
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			b.Tasks[i].Priority = PriorityHigh
			b.Tasks[i].Labels = []string{"backend", "api"}
			b.Tasks[i].BlockedBy = []string{dep, "deadbeefdeadbeef"}
			b.Tasks[i].PR = "#42"
			b.Tasks[i].Worktree = "../wt"
			b.Tasks[i].Attachments = []string{"spec.pdf"}
			b.Tasks[i].WorkingNotes = []string{"notes.md", "big.md", "missing.md"}
		}
	}
	_ = b.Claim(id, "claude", time.Hour, at)
	b.Actor = "claude"
	for i := 0; i < 6; i++ {
		_ = b.Move(id, StatusInDev, at.Add(time.Duration(2*i)*time.Minute))
		_ = b.Move(id, StatusTodo, at.Add(time.Duration(2*i+1)*time.Minute))
	}

	out, err := b.Context(id, ContextOptions{Now: at, NoteRoots: []string{"", root}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Child work", "- priority: high", "- labels: backend, api", "- deadline: 04/12/2026",
		"- worktree: ../wt", "- pr: #42", "- claimed by: claude until 26/11/2026 10:05",
		"## Parent", "[todo] Parent work", "## Blocked by",
		"[todo] Dependency (open)", "(deleted, no longer blocks)",
		"## Attachments", "- spec.pdf",
		"### notes.md", "NOTE-BODY", "````\nNOTE-BODY", // fence outgrows the note's own ```
		"too large to inline", "### missing.md\n\n(not found)",
		"## Description\n\nDo the thing", "## History (last 10 of 12)",
		"26/11/2026 09:16 indev → todo by claude",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("context missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ATTACHMENT-BODY") {
		t.Error("context inlined an attachment's contents")
	}
	if strings.Contains(out, big[:100]) {
		t.Error("context inlined an over-limit note")
	}
	if strings.Contains(out, "09:00 todo") || strings.Contains(out, "09:05 todo → indev") {
		t.Errorf("history not trimmed to the last 10:\n%s", out)
	}
	if strings.Contains(out, "## Subtasks") {
		t.Error("leaf context has a subtasks section")
	}

	parentOut, _ := b.Context(parent, ContextOptions{Now: at})
	if !strings.Contains(parentOut, "## Subtasks (0/2 complete)") || !strings.Contains(parentOut, "Sibling") {
		t.Errorf("parent context missing subtasks:\n%s", parentOut)
	}
	if _, err := b.Context("nope", ContextOptions{}); err == nil {
		t.Error("Context(missing) = nil error")
	}
}

func TestContextMarksExpiredClaim(t *testing.T) {
	b := &Board{}
	id := b.Add("x", "", nil, ref).ID
	_ = b.Claim(id, "claude", time.Hour, ref)
	out, _ := b.Context(id, ContextOptions{Now: ref.Add(2 * time.Hour)})
	if !strings.Contains(out, "(expired)") {
		t.Errorf("expired claim not marked:\n%s", out)
	}
}

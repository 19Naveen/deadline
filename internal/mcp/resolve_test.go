package mcp

import (
	"strings"
	"testing"
)

// The board is resolved per tool call, read-only for initialize and reads,
// with write=true only for tools that change the board.
func TestResolveIsPerCallAndWriteOnlyForWriteTools(t *testing.T) {
	path := devBoard(t)
	var writes, reads int
	c := newClient(t, Config{Resolve: func(write bool) (BoardRef, error) {
		ref := BoardRef{Path: path, Scope: "project"}
		if write {
			writes++
			ref.Created = writes == 1
		} else {
			reads++
			ref.Note = "note: test hint"
		}
		return ref, nil
	}})
	c.initialize("test")
	c.request("tools/list", nil)
	c.mustCall("list_tasks", nil)
	if writes != 0 {
		t.Fatalf("write resolutions after initialize/list = %d, want 0", writes)
	}
	board := c.mustCall("board", nil)
	if !strings.Contains(board, "path: "+path+" (project)") || !strings.Contains(board, "note: test hint") {
		t.Errorf("board = %q, want path, scope and note", board)
	}
	added := c.mustCall("add_task", map[string]any{"title": "One"})
	if writes != 1 {
		t.Errorf("write resolutions after add_task = %d, want 1", writes)
	}
	if !strings.HasPrefix(added, "initialised project board "+path) || !strings.Contains(added, "added ") {
		t.Errorf("first add reply = %q, want the init notice then the add receipt", added)
	}
	if again := c.mustCall("add_task", map[string]any{"title": "Two"}); strings.Contains(again, "initialised") {
		t.Errorf("second add reply = %q, want no init notice", again)
	}
	if reads < 3 {
		t.Errorf("read resolutions = %d, want one per read call", reads)
	}
}

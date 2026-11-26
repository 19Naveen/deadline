package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPCommandServesStdio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	t.Setenv("GOTODO_AGENT", "")

	in, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	msgs := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"Codex"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"add_task","arguments":{"title":"Via MCP","status":"doing"}}}
`
	if _, err := in.WriteString(msgs); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	oldIn := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = oldIn }()

	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "mcp"}); err != nil {
			t.Fatalf("mcp = %v", err)
		}
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q, want exactly two JSON-RPC replies", out)
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Errorf("stdout line is not JSON: %q", line)
		}
	}
	if !strings.Contains(lines[0], `"protocolVersion":"2025-03-26"`) {
		t.Errorf("initialize reply = %s", lines[0])
	}
	if !strings.Contains(lines[1], "[doing] Via MCP") {
		t.Errorf("add reply = %s", lines[1])
	}

	b, err := loadBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 1 || b.Tasks[0].History[0].Agent != "codex" {
		t.Errorf("board = %+v, want one task moved by codex", b.Tasks)
	}
}

func TestMCPCommandRejectsArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := run([]string{"-file", path, "mcp", "extra"}); err == nil {
		t.Error("mcp extra = nil, want an error")
	}
}

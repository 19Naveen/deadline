package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

// testNow pins the server clock.
var testNow = time.Date(2026, 11, 26, 10, 30, 0, 0, time.UTC)

// client drives a Server over in-memory pipes, one message at a time.
type client struct {
	t    *testing.T
	in   *io.PipeWriter
	out  *bufio.Reader
	done chan error
	next int
}

func newClient(t *testing.T, cfg Config) *client {
	t.Helper()
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return testNow }
	}
	srv := NewServer(cfg)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{t: t, in: inW, out: bufio.NewReader(outR), done: make(chan error, 1)}
	go func() {
		err := srv.Serve(inR, outW)
		outW.Close()
		c.done <- err
	}()
	t.Cleanup(func() {
		inW.Close()
		if err := <-c.done; err != nil {
			t.Errorf("Serve = %v, want nil", err)
		}
	})
	return c
}

// rpcReply is a decoded response line.
type rpcReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// send writes one raw line.
func (c *client) send(line string) {
	c.t.Helper()
	if _, err := io.WriteString(c.in, line+"\n"); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// readLine reads one reply line and checks it is a JSON-RPC message.
func (c *client) readLine() []byte {
	c.t.Helper()
	line, err := c.out.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("read reply: %v", err)
	}
	if !json.Valid(line) {
		c.t.Fatalf("server wrote non-JSON to stdout: %q", line)
	}
	return line
}

// request sends a request with a fresh numeric id and returns its reply.
func (c *client) request(method string, params any) rpcReply {
	c.t.Helper()
	c.next++
	msg := map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method}
	if params != nil {
		msg["params"] = params
	}
	data, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatal(err)
	}
	c.send(string(data))
	var r rpcReply
	if err := json.Unmarshal(c.readLine(), &r); err != nil {
		c.t.Fatal(err)
	}
	if r.JSONRPC != "2.0" {
		c.t.Errorf("jsonrpc = %q, want 2.0", r.JSONRPC)
	}
	if string(r.ID) != jsonInt(c.next) {
		c.t.Errorf("reply id = %s, want %d", r.ID, c.next)
	}
	return r
}

func jsonInt(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}

// initialize performs the handshake as clientName.
func (c *client) initialize(clientName string) map[string]any {
	c.t.Helper()
	r := c.request("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": "1.0"},
	})
	if r.Error != nil {
		c.t.Fatalf("initialize error: %v", r.Error.Message)
	}
	c.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	var res map[string]any
	if err := json.Unmarshal(r.Result, &res); err != nil {
		c.t.Fatal(err)
	}
	return res
}

// toolResult is a decoded tools/call result.
type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// call runs a tool and returns its text and isError flag.
func (c *client) call(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	r := c.request("tools/call", map[string]any{"name": name, "arguments": args})
	if r.Error != nil {
		c.t.Fatalf("tools/call %s: JSON-RPC error %d %s", name, r.Error.Code, r.Error.Message)
	}
	var res toolResult
	if err := json.Unmarshal(r.Result, &res); err != nil {
		c.t.Fatal(err)
	}
	if len(res.Content) != 1 || res.Content[0].Type != "text" {
		c.t.Fatalf("content = %+v, want one text item", res.Content)
	}
	return res.Content[0].Text, res.IsError
}

// mustCall runs a tool that must succeed.
func (c *client) mustCall(name string, args map[string]any) string {
	c.t.Helper()
	text, isErr := c.call(name, args)
	if isErr {
		c.t.Fatalf("%s(%v) isError: %s", name, args, text)
	}
	return text
}

// mustFail runs a tool that must return isError, and returns its message.
func (c *client) mustFail(name string, args map[string]any) string {
	c.t.Helper()
	text, isErr := c.call(name, args)
	if !isErr {
		c.t.Fatalf("%s(%v) succeeded with %q, want isError", name, args, text)
	}
	return text
}

// devBoard writes an empty development board to a temp dir.
func devBoard(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".deadline", "board.json")
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	return path
}

func loadTask(t *testing.T, path, prefix string) task.Task {
	t.Helper()
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveID(b, prefix)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// addedID pulls the short ID out of an "added <id> [...] title" reply.
func addedID(t *testing.T, text string) string {
	t.Helper()
	fields := strings.Fields(text)
	if len(fields) < 3 || fields[0] != "added" {
		t.Fatalf("add reply = %q, want \"added <id> [status] title\"", text)
	}
	return fields[1]
}

func TestInitializeHandshake(t *testing.T) {
	c := newClient(t, Config{Path: devBoard(t), Version: "abc123"})
	res := c.initialize("Claude-Code")
	if res["protocolVersion"] != "2025-06-18" {
		t.Errorf("protocolVersion = %v, want 2025-06-18", res["protocolVersion"])
	}
	info, _ := res["serverInfo"].(map[string]any)
	if info["name"] != "gotodo" || info["version"] != "abc123" {
		t.Errorf("serverInfo = %v, want gotodo abc123", info)
	}
	caps, _ := res["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Errorf("capabilities = %v, want tools", caps)
	}
	if s, _ := res["instructions"].(string); !strings.Contains(s, "add_task") {
		t.Errorf("instructions = %q, want tool guidance", s)
	}
	// notifications/initialized got no reply: the next line answers ping.
	if r := c.request("ping", nil); r.Error != nil || string(r.Result) != "{}" {
		t.Errorf("ping = %s %v, want {}", r.Result, r.Error)
	}
}

func TestInitializeNegotiatesVersion(t *testing.T) {
	for _, tc := range []struct{ asked, want string }{
		{"2025-06-18", "2025-06-18"},
		{"2025-03-26", "2025-03-26"},
		{"2024-11-05", "2024-11-05"},
		{"1999-01-01", "2025-06-18"},
		{"", "2025-06-18"},
	} {
		c := newClient(t, Config{Path: devBoard(t)})
		r := c.request("initialize", map[string]any{"protocolVersion": tc.asked})
		var res struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(r.Result, &res); err != nil {
			t.Fatal(err)
		}
		if res.ProtocolVersion != tc.want {
			t.Errorf("asked %q: protocolVersion = %q, want %q", tc.asked, res.ProtocolVersion, tc.want)
		}
	}
}

func TestToolsListHasSchemas(t *testing.T) {
	c := newClient(t, Config{Path: devBoard(t)})
	c.initialize("test")
	r := c.request("tools/list", map[string]any{})
	if r.Error != nil {
		t.Fatal(r.Error.Message)
	}
	var res struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type       string                    `json:"type"`
				Properties map[string]map[string]any `json:"properties"`
				Required   []string                  `json:"required"`
			} `json:"inputSchema"`
			Annotations struct {
				ReadOnlyHint    bool `json:"readOnlyHint"`
				DestructiveHint bool `json:"destructiveHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(r.Result, &res); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"board":        nil,
		"list_tasks":   nil,
		"show_task":    {"id"},
		"add_task":     {"title"},
		"move_task":    {"id", "status"},
		"edit_task":    {"id"},
		"delete_task":  {"id"},
		"next_task":    nil,
		"claim_task":   {"id"},
		"release_task": {"id"},
		"task_context": {"id"},
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
		if tool.Description == "" {
			t.Errorf("%s: empty description", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("%s: schema type = %q, want object", tool.Name, tool.InputSchema.Type)
		}
		if strings.Join(tool.InputSchema.Required, ",") != strings.Join(want[tool.Name], ",") {
			t.Errorf("%s: required = %v, want %v", tool.Name, tool.InputSchema.Required, want[tool.Name])
		}
		for _, req := range tool.InputSchema.Required {
			if _, ok := tool.InputSchema.Properties[req]; !ok {
				t.Errorf("%s: required %q has no property", tool.Name, req)
			}
		}
		for name, prop := range tool.InputSchema.Properties {
			if prop["type"] == nil || prop["description"] == nil {
				t.Errorf("%s.%s: property needs type and description, got %v", tool.Name, name, prop)
			}
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("tools/list missing %s", name)
		}
	}
	if len(res.Tools) != len(want) {
		t.Errorf("tools/list has %d tools, want %d", len(res.Tools), len(want))
	}
	addProps := res.Tools[3].InputSchema.Properties
	for _, field := range []string{"title", "description", "deadline", "status", "parent", "worktree", "attachments", "working_notes"} {
		if _, ok := addProps[field]; !ok {
			t.Errorf("add_task schema missing %q", field)
		}
	}
	for _, tool := range res.Tools {
		if tool.Name == "list_tasks" && !tool.Annotations.ReadOnlyHint {
			t.Error("list_tasks should be annotated read-only")
		}
		if tool.Name == "delete_task" && !tool.Annotations.DestructiveHint {
			t.Error("delete_task should be annotated destructive")
		}
	}
}

func TestAddListMoveShowRoundTrip(t *testing.T) {
	path := devBoard(t)
	c := newClient(t, Config{Path: path})
	c.initialize("Claude-Code")

	board := c.mustCall("board", nil)
	if !strings.Contains(board, "board: development") ||
		!strings.Contains(board, "columns: todo, indev, testing-review, blocked, shipped") ||
		!strings.Contains(board, "acting as: claude-code") {
		t.Errorf("board = %q", board)
	}

	if got := c.mustCall("list_tasks", nil); got != "no tasks" {
		t.Errorf("empty list = %q, want no tasks", got)
	}

	added := c.mustCall("add_task", map[string]any{
		"title":         "Rewrite the parser",
		"description":   "Handle empty input",
		"deadline":      "04/12/2026",
		"worktree":      "../deadline-parser",
		"attachments":   []string{"docs/spec.md"},
		"working_notes": []string{"notes/plan.md"},
	})
	id := addedID(t, added)
	if !strings.Contains(added, "[todo] Rewrite the parser") {
		t.Errorf("add = %q", added)
	}
	child := addedID(t, c.mustCall("add_task", map[string]any{
		"title": "Write parser tests", "parent": id, "status": "indev",
	}))

	list := c.mustCall("list_tasks", nil)
	for _, want := range []string{
		id + " [todo] Rewrite the parser · due 04/12/2026 · subtasks 0/1 · worktree ../deadline-parser",
		child + " [indev] Write parser tests · parent " + id + " Rewrite the parser",
	} {
		if !strings.Contains(list, want) {
			t.Errorf("list = %q, want line %q", list, want)
		}
	}
	if got := c.mustCall("list_tasks", map[string]any{"status": "indev"}); strings.Contains(got, "[todo]") || !strings.HasPrefix(got, child) {
		t.Errorf("list status=indev = %q, want only the subtask", got)
	}
	if got := c.mustCall("list_tasks", map[string]any{"parent": id}); !strings.Contains(got, child) || strings.Count(got, "\n") != 0 {
		t.Errorf("list parent = %q, want only the subtask", got)
	}

	moved := c.mustCall("move_task", map[string]any{"id": id, "status": "indev"})
	if moved != "moved "+id+" Rewrite the parser: todo → indev" {
		t.Errorf("move = %q", moved)
	}
	if again := c.mustCall("move_task", map[string]any{"id": id, "status": "indev"}); !strings.Contains(again, "already in indev") {
		t.Errorf("repeat move = %q", again)
	}

	show := c.mustCall("show_task", map[string]any{"id": id[:4]})
	for _, want := range []string{
		id + " [indev] Rewrite the parser",
		"deadline: 04/12/2026",
		"worktree: ../deadline-parser",
		"attachment: docs/spec.md",
		"working note: notes/plan.md",
		"last agent: claude-code",
		"description:\nHandle empty input",
		"subtasks: 0/1 complete",
		"  " + child + " [indev] Write parser tests",
		"26/11/2026 10:30 todo → indev (claude-code)",
	} {
		if !strings.Contains(show, want) {
			t.Errorf("show = %q, want %q", show, want)
		}
	}

	// The move was persisted and attributed to the client.
	got := loadTask(t, path, id)
	if got.Status != task.StatusInDev || len(got.History) != 1 || got.History[0].Agent != "claude-code" || got.LastAgent != "claude-code" {
		t.Errorf("persisted task = status %s history %+v last %q, want indev by claude-code", got.Status, got.History, got.LastAgent)
	}
	if sub := loadTask(t, path, child); sub.History[0].Agent != "claude-code" {
		t.Errorf("subtask add-with-status history = %+v, want agent claude-code", sub.History)
	}
}

func TestEnvAgentWinsOverClientName(t *testing.T) {
	path := devBoard(t)
	c := newClient(t, Config{Path: path, Agent: "codex-worker"})
	c.initialize("OpenCode")
	id := addedID(t, c.mustCall("add_task", map[string]any{"title": "Task"}))
	c.mustCall("move_task", map[string]any{"id": id, "status": "indev"})
	if got := loadTask(t, path, id).History[0].Agent; got != "codex-worker" {
		t.Errorf("History.Agent = %q, want codex-worker", got)
	}
}

func TestActorFallsBackWithoutClientName(t *testing.T) {
	path := devBoard(t)
	c := newClient(t, Config{Path: path})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18"})
	id := addedID(t, c.mustCall("add_task", map[string]any{"title": "Task", "status": "indev"}))
	if got := loadTask(t, path, id).History[0].Agent; got != fallbackActor {
		t.Errorf("History.Agent = %q, want %q (never a human)", got, fallbackActor)
	}
}

func TestEditAndDelete(t *testing.T) {
	path := devBoard(t)
	c := newClient(t, Config{Path: path})
	c.initialize("test")
	parent := addedID(t, c.mustCall("add_task", map[string]any{"title": "Parent"}))
	id := addedID(t, c.mustCall("add_task", map[string]any{
		"title": "Child", "description": "old", "deadline": "01/12/2026", "parent": parent,
		"worktree": "wt", "attachments": []string{"a.md", "b.md"}, "working_notes": []string{"n.md"},
	}))

	edited := c.mustCall("edit_task", map[string]any{
		"id": id, "title": "Renamed", "deadline": "", "parent": "", "attachments": []string{"c.md"},
	})
	if edited != "edited "+id+" [todo] Renamed" {
		t.Errorf("edit = %q", edited)
	}
	got := loadTask(t, path, id)
	if got.Title != "Renamed" || got.Description != "old" || got.Deadline != nil || got.ParentID != "" ||
		got.Worktree != "wt" || strings.Join(got.Attachments, ",") != "c.md" || strings.Join(got.WorkingNotes, ",") != "n.md" {
		t.Errorf("after edit = %+v, want renamed, desc kept, deadline/parent cleared, attachments replaced, notes kept", got)
	}
	c.mustCall("edit_task", map[string]any{"id": id, "working_notes": []string{}, "worktree": "", "description": ""})
	got = loadTask(t, path, id)
	if len(got.WorkingNotes) != 0 || got.Worktree != "" || got.Description != "" {
		t.Errorf("after clearing = %+v", got)
	}

	c.mustCall("edit_task", map[string]any{"id": id, "parent": parent})
	deleted := c.mustCall("delete_task", map[string]any{"id": parent})
	if deleted != "deleted "+parent+" [todo] Parent · detached 1 subtasks" {
		t.Errorf("delete = %q", deleted)
	}
	if got := loadTask(t, path, id); got.ParentID != "" {
		t.Errorf("child ParentID = %q, want detached", got.ParentID)
	}
	if msg := c.mustFail("show_task", map[string]any{"id": parent}); !strings.Contains(msg, "no task id starts with") {
		t.Errorf("show deleted = %q", msg)
	}
}

func TestToolErrorsAreResults(t *testing.T) {
	path := devBoard(t)
	c := newClient(t, Config{Path: path})
	c.initialize("test")
	id := addedID(t, c.mustCall("add_task", map[string]any{"title": "Task"}))

	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"add_task", map[string]any{"title": "  "}, "title must not be blank"},
		{"add_task", map[string]any{"title": "x", "deadline": "2026-12-01"}, "want DD/MM/YYYY"},
		{"add_task", map[string]any{"title": "x", "status": "doing"}, `unknown status "doing"`},
		{"add_task", map[string]any{"title": "x", "desc": "typo"}, `unknown field "desc"`},
		{"add_task", map[string]any{"title": 7}, "invalid arguments"},
		{"move_task", map[string]any{"id": "zzzz", "status": "indev"}, "no task id starts with"},
		{"move_task", map[string]any{"id": id}, "status must not be blank"},
		{"show_task", map[string]any{}, "task id must not be blank"},
		{"edit_task", map[string]any{"id": id}, "nothing to edit"},
		{"edit_task", map[string]any{"id": id, "title": ""}, "title must not be blank"},
		{"list_tasks", map[string]any{"parent": "nope"}, "parent: no task id"},
	} {
		if msg := c.mustFail(tc.tool, tc.args); !strings.Contains(msg, tc.want) {
			t.Errorf("%s(%v) error = %q, want %q", tc.tool, tc.args, msg, tc.want)
		}
	}
	// A failed add_task saves nothing.
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 1 {
		t.Errorf("board has %d tasks after failed adds, want 1", len(b.Tasks))
	}
}

func TestApprovalGateBlocksAgentShipping(t *testing.T) {
	path := devBoard(t)
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetRequireApproval(true)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	c := newClient(t, Config{Path: path})
	c.initialize("test")
	if got := c.mustCall("board", nil); !strings.Contains(got, "only a human may move tasks into shipped") {
		t.Errorf("board = %q, want approval note", got)
	}
	id := addedID(t, c.mustCall("add_task", map[string]any{"title": "Task"}))
	if msg := c.mustFail("move_task", map[string]any{"id": id, "status": "shipped"}); !strings.Contains(msg, "requires a human") {
		t.Errorf("ship error = %q", msg)
	}
	if got := loadTask(t, path, id).Status; got != task.StatusTodo {
		t.Errorf("status = %s, want todo", got)
	}
}

func TestReloadsBoardEveryCall(t *testing.T) {
	path := devBoard(t)
	c := newClient(t, Config{Path: path})
	c.initialize("test")
	c.mustCall("add_task", map[string]any{"title": "From MCP"})

	// Another writer (the TUI or CLI) adds a task behind the server's back.
	other, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	other.Add("From CLI", "", nil, testNow)
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}

	list := c.mustCall("list_tasks", nil)
	if !strings.Contains(list, "From MCP") || !strings.Contains(list, "From CLI") {
		t.Errorf("list = %q, want both tasks", list)
	}
	c.mustCall("add_task", map[string]any{"title": "Second MCP"})
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 3 {
		t.Errorf("board has %d tasks, want 3 (no lost writes)", len(b.Tasks))
	}
}

func TestProtocolErrors(t *testing.T) {
	c := newClient(t, Config{Path: devBoard(t)})
	c.initialize("test")

	if r := c.request("resources/list", nil); r.Error == nil || r.Error.Code != codeMethodNotFound {
		t.Errorf("unknown method = %+v, want -32601", r.Error)
	}
	if r := c.request("tools/call", map[string]any{"name": "nope"}); r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Errorf("unknown tool = %+v, want -32602", r.Error)
	}

	// Unknown notifications get no reply, so the next line answers the ping.
	c.send(`{"jsonrpc":"2.0","method":"notifications/whatever"}`)
	c.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`)

	c.send(`{not json`)
	var r rpcReply
	if err := json.Unmarshal(c.readLine(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Error == nil || r.Error.Code != codeParseError || string(r.ID) != "null" {
		t.Errorf("malformed = %+v id %s, want -32700 with null id", r.Error, r.ID)
	}

	c.send(`{"jsonrpc":"1.0","id":"x","method":"ping"}`)
	if err := json.Unmarshal(c.readLine(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Error == nil || r.Error.Code != codeInvalidRequest || string(r.ID) != `"x"` {
		t.Errorf("bad version = %+v id %s, want -32600 echoing id", r.Error, r.ID)
	}

	if r := c.request("ping", nil); r.Error != nil {
		t.Errorf("ping after errors = %v", r.Error)
	}
}

func TestBatch(t *testing.T) {
	c := newClient(t, Config{Path: devBoard(t)})
	c.send(`[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"},{"jsonrpc":"2.0","id":2,"method":"nope"}]`)
	var replies []rpcReply
	if err := json.Unmarshal(c.readLine(), &replies); err != nil {
		t.Fatal(err)
	}
	if len(replies) != 2 || replies[0].Error != nil || replies[1].Error == nil || replies[1].Error.Code != codeMethodNotFound {
		t.Errorf("batch replies = %+v, want ping ok and method-not-found", replies)
	}
}

func TestServeStopsAtEOF(t *testing.T) {
	var out strings.Builder
	srv := NewServer(Config{Path: devBoard(t)})
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" +
		`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n\n" +
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`) // no trailing newline
	if err := srv.Serve(in, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output = %q, want two replies", out.String())
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Errorf("non-JSON output line %q", line)
		}
	}
}

func TestPanicBecomesToolError(t *testing.T) {
	tools := append(DefaultTools(), Tool{
		Name:        "boom",
		Description: "panics",
		InputSchema: objectSchema(nil),
		ReadOnly:    true,
		Handler:     func(*Call) (string, error) { panic("kaboom") },
	})
	c := newClient(t, Config{Path: devBoard(t), Tools: tools})
	if msg := c.mustFail("boom", nil); !strings.Contains(msg, "kaboom") {
		t.Errorf("panic message = %q", msg)
	}
	if r := c.request("ping", nil); r.Error != nil {
		t.Errorf("ping after panic = %v", r.Error)
	}
}

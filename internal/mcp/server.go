// Package mcp serves the gotodo board to coding agents over the Model
// Context Protocol: newline-delimited JSON-RPC 2.0 on stdin/stdout (the MCP
// stdio transport). It is hand-written on encoding/json to keep gotodo free
// of extra dependencies.
//
// Only protocol messages are ever written to the output stream; diagnostics
// go to the log writer (stderr in production).
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gotodo/internal/loops"
)

// LatestProtocolVersion is offered when the client asks for a version this
// server does not know.
const LatestProtocolVersion = "2025-06-18"

// SupportedProtocolVersions are echoed back when a client requests one.
var SupportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// fallbackActor names the agent when neither GOTODO_AGENT nor the client's
// clientInfo.name is available. An MCP client is always an agent, never the
// human at the TUI, so moves are never attributed to "".
const fallbackActor = "mcp"

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// serverInstructions is returned from initialize; clients may surface it to
// the model as a system hint.
const serverInstructions = `gotodo is the kanban board for this project. Log every code change on it:
call board first to learn the columns (todo → indev → testing-review → shipped on development boards,
todo → doing → done on personal ones; both may use blocked), list_tasks to avoid duplicates,
add_task before starting work, then move_task through the columns as the work progresses.
Work in a git repository goes on its project board (board shows the path; your first change creates
the board if missing). Link working notes as project-relative files inside the project, never temp dirs.
Task IDs accept any unique prefix (the 8-character short IDs shown in output). Dates are DD/MM/YYYY.`

// BoardRef is the board one tool call runs against.
type BoardRef struct {
	// Path is the board file the call loads and saves.
	Path string
	// Scope names the board in the board tool: "project", "personal" or
	// "file". Blank omits it.
	Scope string
	// Note is a hint the board tool appends (e.g. the repository has no
	// project board yet).
	Note string
	// Created reports that resolving this call initialised the board; the
	// tool's reply says so.
	Created bool
}

// Config configures a Server.
type Config struct {
	// Path is the board file every tool call loads and saves when Resolve
	// is nil.
	Path string
	// Resolve picks the board for each tool call, so the server follows
	// the same rules as the CLI. write is true for tools that change the
	// board; only those may create one. Nil means always Path.
	Resolve func(write bool) (BoardRef, error)
	// Version is reported as serverInfo.version.
	Version string
	// Agent is the GOTODO_AGENT name; when blank the client's
	// clientInfo.name (lowercased) is used instead.
	Agent string
	// Log receives diagnostics; nil discards them.
	Log io.Writer
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Tools overrides the tool table; nil means DefaultTools().
	Tools []Tool
}

// Server is one MCP session over a pair of streams.
type Server struct {
	resolve func(write bool) (BoardRef, error)
	version string
	agent   string // from GOTODO_AGENT; wins over clientInfo
	client  string // lowercased clientInfo.name from initialize
	log     io.Writer
	now     func() time.Time
	tools   []Tool
	byName  map[string]*Tool
}

// NewServer builds a server from cfg.
func NewServer(cfg Config) *Server {
	s := &Server{
		resolve: cfg.Resolve,
		version: cfg.Version,
		agent:   strings.TrimSpace(cfg.Agent),
		log:     cfg.Log,
		now:     cfg.Now,
		tools:   cfg.Tools,
	}
	if s.version == "" {
		s.version = "dev"
	}
	if s.log == nil {
		s.log = io.Discard
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.tools == nil {
		s.tools = DefaultTools()
	}
	if s.resolve == nil {
		path := cfg.Path
		s.resolve = func(bool) (BoardRef, error) { return BoardRef{Path: path}, nil }
	}
	s.byName = make(map[string]*Tool, len(s.tools))
	for i := range s.tools {
		s.byName[s.tools[i].Name] = &s.tools[i]
	}
	return s
}

// Actor is who board moves are attributed to: GOTODO_AGENT, else the
// client's name, else "mcp".
func (s *Server) Actor() string {
	switch {
	case s.agent != "":
		return s.agent
	case s.client != "":
		return s.client
	}
	return fallbackActor
}

// request is an incoming JSON-RPC message. A missing id marks a notification.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r *request) isNotification() bool { return len(r.ID) == 0 }

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

var nullID = json.RawMessage("null")

// Serve reads newline-delimited JSON-RPC messages from in and writes the
// replies to out until in reaches EOF. Requests are handled in order.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	w := bufio.NewWriter(out)
	for {
		line, err := r.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if reply := s.handleLine(line); reply != nil {
				if _, werr := w.Write(append(reply, '\n')); werr != nil {
					return fmt.Errorf("write: %w", werr)
				}
				if werr := w.Flush(); werr != nil {
					return fmt.Errorf("write: %w", werr)
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
	}
}

// handleLine processes one message or batch and returns the encoded reply,
// or nil when nothing should be written (notifications).
func (s *Server) handleLine(line []byte) []byte {
	line = bytes.TrimSpace(line)
	if line[0] == '[' {
		var batch []json.RawMessage
		if err := json.Unmarshal(line, &batch); err != nil {
			return s.encode(errorResponse(nullID, codeParseError, "parse error: "+err.Error()))
		}
		if len(batch) == 0 {
			return s.encode(errorResponse(nullID, codeInvalidRequest, "empty batch"))
		}
		var replies []response
		for _, msg := range batch {
			if resp := s.handleMessage(msg); resp != nil {
				replies = append(replies, *resp)
			}
		}
		if len(replies) == 0 {
			return nil
		}
		return s.encode(replies)
	}
	if resp := s.handleMessage(line); resp != nil {
		return s.encode(resp)
	}
	return nil
}

func (s *Server) encode(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(s.log, "gotodo mcp: encode reply: %v\n", err)
		data, _ = json.Marshal(errorResponse(nullID, codeInternalError, "encode reply failed"))
	}
	return data
}

func errorResponse(id json.RawMessage, code int, msg string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

// handleMessage dispatches one JSON-RPC message; nil means no reply.
func (s *Server) handleMessage(raw json.RawMessage) *response {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return errorResponse(nullID, codeParseError, "parse error: "+err.Error())
		}
		return errorResponse(nullID, codeInvalidRequest, "invalid request: "+err.Error())
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		if req.isNotification() {
			return nil
		}
		return errorResponse(req.ID, codeInvalidRequest, `invalid request: want jsonrpc "2.0" and a method`)
	}
	if req.isNotification() {
		// notifications/initialized, notifications/cancelled and any
		// others need no reply; requests are handled synchronously, so
		// there is nothing in flight to cancel.
		return nil
	}

	result, err := s.dispatch(&req)
	if err != nil {
		var rerr *rpcError
		if !errors.As(err, &rerr) {
			rerr = &rpcError{Code: codeInternalError, Message: err.Error()}
		}
		return errorResponse(req.ID, rerr.Code, rerr.Message)
	}
	return &response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) dispatch(req *request) (any, error) {
	switch req.Method {
	case "initialize":
		return s.initialize(req.Params)
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return s.listTools(), nil
	case "tools/call":
		return s.callTool(req.Params)
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
}

func (s *Server) initialize(params json.RawMessage) (any, error) {
	var p initializeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid initialize params: " + err.Error()}
		}
	}
	version := LatestProtocolVersion
	for _, v := range SupportedProtocolVersions {
		if p.ProtocolVersion == v {
			version = v
			break
		}
	}
	s.client = strings.ToLower(strings.TrimSpace(p.ClientInfo.Name))
	// Read-only resolution: opening a session never creates a board.
	where := "an unresolved board"
	if ref, err := s.resolve(false); err == nil {
		where = ref.Path
	}
	fmt.Fprintf(s.log, "gotodo mcp: client %q (protocol %s), acting as %q on %s\n",
		p.ClientInfo.Name, version, s.Actor(), where)
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{"listChanged": false},
		},
		"serverInfo": map[string]any{
			"name":    "gotodo",
			"version": s.version,
		},
		"instructions": serverInstructions,
	}, nil
}

// toolInfo is one tools/list entry.
type toolInfo struct {
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Description string           `json:"description"`
	InputSchema json.RawMessage  `json:"inputSchema"`
	Annotations *toolAnnotations `json:"annotations,omitempty"`
}

type toolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

func (s *Server) listTools() any {
	infos := make([]toolInfo, 0, len(s.tools))
	for _, t := range s.tools {
		infos = append(infos, toolInfo{
			Name:        t.Name,
			Title:       t.Title,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Annotations: &toolAnnotations{
				Title:           t.Title,
				ReadOnlyHint:    t.ReadOnly,
				DestructiveHint: t.Destructive,
				IdempotentHint:  t.Idempotent,
				OpenWorldHint:   false,
			},
		})
	}
	return map[string]any{"tools": infos}
}

type callParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

func textResult(text string, isError bool) callResult {
	return callResult{Content: []textContent{{Type: "text", Text: text}}, IsError: isError}
}

// callTool runs one tool. Unknown tools are a protocol error; anything the
// tool itself rejects (bad arguments, unknown IDs, board rules) is returned
// as an isError result so the model can read it and correct itself.
func (s *Server) callTool(params json.RawMessage) (any, error) {
	var p callParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid tools/call params: " + err.Error()}
	}
	tool, ok := s.byName[p.Name]
	if !ok {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool: " + p.Name}
	}
	text, err := s.runTool(tool, p.Arguments)
	if err != nil {
		fmt.Fprintf(s.log, "gotodo mcp: %s: %v\n", tool.Name, err)
		return textResult(err.Error(), true), nil
	}
	return textResult(text, false), nil
}

// runTool reloads the board from disk (other sessions write it), runs the
// handler, and saves after mutating tools. A panic becomes a tool error so
// one bad call never takes down the session.
func (s *Server) runTool(tool *Tool, args json.RawMessage) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error in %s: %v", tool.Name, r)
		}
	}()
	if len(bytes.TrimSpace(args)) == 0 || string(bytes.TrimSpace(args)) == "null" {
		args = json.RawMessage("{}")
	}
	call, err := s.newCall(args, !tool.ReadOnly)
	if err != nil {
		return "", err
	}
	text, err = tool.Handler(call)
	if err != nil {
		return "", err
	}
	if !tool.ReadOnly && !tool.SavesItself {
		if err := call.Board.Save(); err != nil {
			return "", fmt.Errorf("save board: %w", err)
		}
	}
	if !tool.ReadOnly {
		// A move may enter a stage with a loop; never fail the tool for it.
		if _, err := loops.Dispatch(call.Path, call.Now); err != nil {
			fmt.Fprintln(os.Stderr, "gotodo mcp: loops:", err)
		}
	}
	if call.Ref.Created {
		text = fmt.Sprintf("initialised project board %s (agents always use a project board inside a git repo)\n%s",
			call.Path, text)
	}
	return text, nil
}

package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gotodo/internal/task"
)

// Tool is one entry in the tool table. Adding a tool is one entry in
// DefaultTools: a name, a description the model reads, a JSON Schema for
// its arguments, and a handler.
type Tool struct {
	Name        string
	Title       string
	Description string
	// InputSchema is the JSON Schema for the arguments object.
	InputSchema json.RawMessage
	// ReadOnly tools never save; every other tool saves the board after a
	// successful handler (Save merges concurrent writers and writes nothing
	// when nothing changed).
	ReadOnly    bool
	Destructive bool
	Idempotent  bool
	// Handler does the work and returns human-readable text. A returned
	// error is shown to the model as an isError result and nothing is saved.
	Handler func(c *Call) (string, error)
}

// Call is one tool invocation against a freshly loaded board.
type Call struct {
	// Board is reloaded from disk for every call, with Actor set and the
	// archive swept.
	Board *task.Board
	// Now is the call's timestamp.
	Now time.Time
	// Args is the raw arguments object ({} when the client sent none).
	Args json.RawMessage
}

func (s *Server) newCall(args json.RawMessage) (*Call, error) {
	b, err := task.Load(s.path)
	if err != nil {
		return nil, err
	}
	b.Actor = s.Actor()
	now := s.now()
	b.SweepArchive(now)
	return &Call{Board: b, Now: now, Args: args}, nil
}

// Decode strictly unmarshals the arguments into v: unknown fields and wrong
// types are reported so the model can correct its call.
func (c *Call) Decode(v any) error {
	dec := json.NewDecoder(bytes.NewReader(c.Args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid arguments: %s", strings.TrimPrefix(err.Error(), "json: "))
	}
	return nil
}

// Resolve finds the one active task whose ID starts with prefix.
func (c *Call) Resolve(prefix string) (task.Task, error) {
	return resolveID(c.Board, prefix)
}

// Status parses a column name against this board's columns.
func (c *Call) Status(name string) (task.Status, error) {
	return boardStatus(c.Board, name)
}

// DefaultTools is the tool table served by tools/list, in display order.
func DefaultTools() []Tool {
	return []Tool{
		{
			Name:  "board",
			Title: "Board columns",
			Description: "Report the board type (development or personal), its columns in pipeline order, " +
				"WIP limits, whether shipping needs human approval, and who your moves are recorded as. " +
				"Call this first: status arguments must be one of these columns.",
			InputSchema: objectSchema(nil),
			ReadOnly:    true,
			Idempotent:  true,
			Handler:     toolBoard,
		},
		{
			Name:  "list_tasks",
			Title: "List tasks",
			Description: "List active (non-archived) tasks, one line each: short ID, [status], title, then " +
				"deadline, parent or subtask progress and worktree when set. Optionally filter by column " +
				"or by parent task. Check this before add_task to avoid duplicate cards.",
			InputSchema: objectSchema(map[string]any{
				"status": stringProp("Only list tasks in this column (a name from the board tool)."),
				"parent": stringProp("Only list subtasks of this task (ID or unique ID prefix)."),
			}),
			ReadOnly:   true,
			Idempotent: true,
			Handler:    toolListTasks,
		},
		{
			Name:  "show_task",
			Title: "Show task",
			Description: "Show one task in full: status, parent, deadline, worktree, attachments, working " +
				"notes, description, agent fields, subtasks with progress, and its status history.",
			InputSchema: objectSchema(map[string]any{
				"id": stringProp("Task ID or unique ID prefix (the 8-character short ID is enough)."),
			}, "id"),
			ReadOnly:   true,
			Idempotent: true,
			Handler:    toolShowTask,
		},
		{
			Name:  "add_task",
			Title: "Add task",
			Description: "Create a task card. Log one before starting any code change. It starts in the " +
				"first column unless status is given. Use parent to make it a subtask of a top-level task " +
				"(hierarchy is one level deep). Returns the new task's short ID.",
			InputSchema: objectSchema(map[string]any{
				"title":         stringProp("Short imperative title, e.g. \"Fix parser crash on empty input\"."),
				"description":   stringProp("Optional detail: scope, acceptance criteria, links."),
				"deadline":      dateProp("Optional due date as DD/MM/YYYY."),
				"status":        stringProp("Optional starting column (a name from the board tool); defaults to the first column."),
				"parent":        stringProp("Optional parent task ID or unique prefix; the new task becomes its subtask."),
				"worktree":      stringProp("Optional git worktree path or branch where the work lives."),
				"attachments":   stringArrayProp("Optional reference file paths (specs, designs, logs). Paths are stored, files are not copied."),
				"working_notes": stringArrayProp("Optional paths of agent working documents (plans, scratch notes) for this task."),
			}, "title"),
			Handler: toolAddTask,
		},
		{
			Name:  "move_task",
			Title: "Move task",
			Description: "Move a task to another column and record the transition under your agent name. " +
				"Move it to the active column when starting, then through review to the terminal column " +
				"as work is verified. A parent cannot finish before all its subtasks, and boards that " +
				"require approval only let a human move cards into the terminal column.",
			InputSchema: objectSchema(map[string]any{
				"id":     stringProp("Task ID or unique ID prefix."),
				"status": stringProp("Destination column (a name from the board tool)."),
			}, "id", "status"),
			Idempotent: true,
			Handler:    toolMoveTask,
		},
		{
			Name:  "edit_task",
			Title: "Edit task",
			Description: "Change a task's fields. Only the fields you pass change; an empty string clears " +
				"description, deadline, worktree or parent (detaching a subtask), and attachments or " +
				"working_notes replace the whole list (an empty array clears it). Use move_task for status.",
			InputSchema: objectSchema(map[string]any{
				"id":            stringProp("Task ID or unique ID prefix."),
				"title":         stringProp("Replacement title (must not be blank)."),
				"description":   stringProp("Replacement description; empty clears it."),
				"deadline":      dateOrEmptyProp("Replacement due date as DD/MM/YYYY; empty clears it."),
				"parent":        stringProp("Replacement parent task ID or prefix; empty makes it a top-level task."),
				"worktree":      stringProp("Replacement git worktree path or branch; empty clears it."),
				"attachments":   stringArrayProp("Replacement list of reference file paths; [] clears them."),
				"working_notes": stringArrayProp("Replacement list of working document paths; [] clears them."),
			}, "id"),
			Idempotent: true,
			Handler:    toolEditTask,
		},
		{
			Name:  "delete_task",
			Title: "Delete task",
			Description: "Permanently delete a task logged by mistake (there is no undo). Its subtasks are " +
				"detached, not deleted. Finished work should be moved to the terminal column instead.",
			InputSchema: objectSchema(map[string]any{
				"id": stringProp("Task ID or unique ID prefix."),
			}, "id"),
			Destructive: true,
			Handler:     toolDeleteTask,
		},
	}
}

// Schema helpers. Properties are plain maps so json.Marshal sorts them.

func objectSchema(props map[string]any, required ...string) json.RawMessage {
	if props == nil {
		props = map[string]any{}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	data, err := json.Marshal(schema)
	if err != nil {
		panic(err) // static schemas; cannot fail
	}
	return data
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

const datePattern = `^\d{2}/\d{2}/\d{4}$`

func dateProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "pattern": datePattern}
}

func dateOrEmptyProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "pattern": `^(\d{2}/\d{2}/\d{4})?$`}
}

func stringArrayProp(desc string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
}

// Handlers.

func toolBoard(c *Call) (string, error) {
	if err := c.Decode(&struct{}{}); err != nil {
		return "", err
	}
	b := c.Board
	kind := "personal"
	if b.DoneStatus() == task.StatusShipped {
		kind = "development"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "board: %s\ncolumns: %s\n", kind, statusNames(b.Statuses()))
	var limits []string
	for _, s := range b.Statuses() {
		if n := b.WIPLimits[s]; n > 0 {
			limits = append(limits, fmt.Sprintf("%s %d", s, n))
		}
	}
	if len(limits) > 0 {
		fmt.Fprintf(&out, "wip limits: %s\n", strings.Join(limits, ", "))
	}
	if b.RequireApproval {
		fmt.Fprintf(&out, "approval: only a human may move tasks into %s\n", b.DoneStatus())
	}
	fmt.Fprintf(&out, "acting as: %s\n", b.Actor)
	return out.String(), nil
}

type listArgs struct {
	Status string `json:"status"`
	Parent string `json:"parent"`
}

func toolListTasks(c *Call) (string, error) {
	var a listArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	b := c.Board
	filter := task.Status("")
	if a.Status != "" {
		s, err := c.Status(a.Status)
		if err != nil {
			return "", err
		}
		filter = s
	}
	parentID := ""
	if a.Parent != "" {
		parent, err := c.Resolve(a.Parent)
		if err != nil {
			return "", fmt.Errorf("parent: %w", err)
		}
		parentID = parent.ID
	}
	var lines []string
	for _, s := range b.Statuses() {
		if filter != "" && s != filter {
			continue
		}
		for _, t := range b.ByStatus(s) {
			if parentID != "" && t.ParentID != parentID {
				continue
			}
			lines = append(lines, listLine(b, t, c.Now))
		}
	}
	if len(lines) == 0 {
		return "no tasks", nil
	}
	return strings.Join(lines, "\n"), nil
}

func listLine(b *task.Board, t task.Task, now time.Time) string {
	line := fmt.Sprintf("%s [%s] %s", shortID(t.ID), t.Status, t.Title)
	if p := priorityName(t.Priority); p != "" {
		line += " · priority " + p
	}
	if t.Deadline != nil {
		line += " · due " + t.Deadline.In(now.Location()).Format(dateLayout)
	}
	if t.ParentID != "" {
		if parent, ok := b.TaskByID(t.ParentID); ok {
			line += " · parent " + shortID(parent.ID) + " " + parent.Title
		}
	} else if done, total := b.ChildProgress(t.ID); total > 0 {
		line += fmt.Sprintf(" · subtasks %d/%d", done, total)
	}
	if len(t.Labels) > 0 {
		line += " · labels " + strings.Join(t.Labels, ", ")
	}
	if t.ClaimedBy != "" && (t.ClaimUntil == nil || t.ClaimUntil.After(now)) {
		line += " · claimed by " + t.ClaimedBy
	}
	if t.Worktree != "" {
		line += " · worktree " + t.Worktree
	}
	return line
}

type idArgs struct {
	ID string `json:"id"`
}

func toolShowTask(c *Call) (string, error) {
	var a idArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	b := c.Board
	t, err := c.Resolve(a.ID)
	if err != nil {
		return "", err
	}
	loc := c.Now.Location()
	var out strings.Builder
	fmt.Fprintf(&out, "%s [%s] %s\n", shortID(t.ID), t.Status, t.Title)
	fmt.Fprintf(&out, "id: %s\n", t.ID)
	if t.ParentID != "" {
		if parent, ok := b.TaskByID(t.ParentID); ok {
			fmt.Fprintf(&out, "parent: %s [%s] %s\n", shortID(parent.ID), parent.Status, parent.Title)
		}
	}
	if p := priorityName(t.Priority); p != "" {
		fmt.Fprintf(&out, "priority: %s\n", p)
	}
	if len(t.Labels) > 0 {
		fmt.Fprintf(&out, "labels: %s\n", strings.Join(t.Labels, ", "))
	}
	if t.Deadline != nil {
		fmt.Fprintf(&out, "deadline: %s\n", t.Deadline.In(loc).Format(dateLayout))
	}
	for _, id := range t.BlockedBy {
		if dep, ok := b.TaskByID(id); ok {
			fmt.Fprintf(&out, "blocked by: %s [%s] %s\n", shortID(dep.ID), dep.Status, dep.Title)
		} else {
			fmt.Fprintf(&out, "blocked by: %s\n", shortID(id))
		}
	}
	if t.ClaimedBy != "" {
		line := "claimed by: " + t.ClaimedBy
		if t.ClaimUntil != nil {
			line += " until " + t.ClaimUntil.In(loc).Format(stampLayout)
			if !t.ClaimUntil.After(c.Now) {
				line += " (expired)"
			}
		}
		fmt.Fprintln(&out, line)
	}
	if t.PR != "" {
		fmt.Fprintf(&out, "pr: %s\n", t.PR)
	}
	if t.Worktree != "" {
		fmt.Fprintf(&out, "worktree: %s\n", t.Worktree)
	}
	for _, path := range t.Attachments {
		fmt.Fprintf(&out, "attachment: %s\n", path)
	}
	for _, path := range t.WorkingNotes {
		fmt.Fprintf(&out, "working note: %s\n", path)
	}
	if t.LastAgent != "" {
		fmt.Fprintf(&out, "last agent: %s\n", t.LastAgent)
	}
	if t.Description != "" {
		fmt.Fprintf(&out, "description:\n%s\n", t.Description)
	}
	if children := b.Children(t.ID); len(children) > 0 {
		done, total := b.ChildProgress(t.ID)
		fmt.Fprintf(&out, "subtasks: %d/%d complete\n", done, total)
		for _, child := range children {
			archived := ""
			if child.Archived {
				archived = " archived"
			}
			fmt.Fprintf(&out, "  %s [%s%s] %s\n", shortID(child.ID), child.Status, archived, child.Title)
		}
	}
	if len(t.History) > 0 {
		fmt.Fprintln(&out, "history:")
		for _, h := range t.History {
			who := h.Agent
			if who == "" {
				who = "human"
			}
			fmt.Fprintf(&out, "  %s %s → %s (%s)\n", h.At.In(loc).Format(stampLayout), h.From, h.To, who)
		}
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

type addArgs struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Deadline     string   `json:"deadline"`
	Status       string   `json:"status"`
	Parent       string   `json:"parent"`
	Worktree     string   `json:"worktree"`
	Attachments  []string `json:"attachments"`
	WorkingNotes []string `json:"working_notes"`
}

func toolAddTask(c *Call) (string, error) {
	var a addArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	b := c.Board
	title := strings.TrimSpace(a.Title)
	if title == "" {
		return "", errors.New("title must not be blank")
	}
	deadline, err := parseDeadline(a.Deadline, c.Now.Location())
	if err != nil {
		return "", err
	}
	status := b.Statuses()[0]
	if a.Status != "" {
		if status, err = c.Status(a.Status); err != nil {
			return "", err
		}
	}
	var t *task.Task
	if a.Parent == "" {
		t = b.Add(title, a.Description, deadline, c.Now)
	} else {
		parent, err := c.Resolve(a.Parent)
		if err != nil {
			return "", fmt.Errorf("parent: %w", err)
		}
		if t, err = b.AddChild(parent.ID, title, a.Description, deadline, c.Now); err != nil {
			return "", err
		}
	}
	id, addedTitle := t.ID, t.Title // t is invalidated by later appends
	if err := b.SetWorktree(id, a.Worktree, c.Now); err != nil {
		return "", err
	}
	if err := b.SetDocuments(id, a.Attachments, a.WorkingNotes, c.Now); err != nil {
		return "", err
	}
	if status != b.Statuses()[0] {
		if err := b.Move(id, status, c.Now); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("added %s [%s] %s", shortID(id), status, addedTitle), nil
}

type moveArgs struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func toolMoveTask(c *Call) (string, error) {
	var a moveArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	t, err := c.Resolve(a.ID)
	if err != nil {
		return "", err
	}
	if a.Status == "" {
		return "", errors.New("status must not be blank")
	}
	to, err := c.Status(a.Status)
	if err != nil {
		return "", err
	}
	if t.Status == to {
		return fmt.Sprintf("%s %s is already in %s", shortID(t.ID), t.Title, to), nil
	}
	if err := c.Board.Move(t.ID, to, c.Now); err != nil {
		return "", err
	}
	return fmt.Sprintf("moved %s %s: %s → %s", shortID(t.ID), t.Title, t.Status, to), nil
}

// editArgs uses pointers so an omitted field keeps its value while an
// explicit empty value clears it.
type editArgs struct {
	ID           string    `json:"id"`
	Title        *string   `json:"title"`
	Description  *string   `json:"description"`
	Deadline     *string   `json:"deadline"`
	Parent       *string   `json:"parent"`
	Worktree     *string   `json:"worktree"`
	Attachments  *[]string `json:"attachments"`
	WorkingNotes *[]string `json:"working_notes"`
}

func toolEditTask(c *Call) (string, error) {
	var a editArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	if a.Title == nil && a.Description == nil && a.Deadline == nil && a.Parent == nil &&
		a.Worktree == nil && a.Attachments == nil && a.WorkingNotes == nil {
		return "", errors.New("nothing to edit: pass at least one of title, description, deadline, parent, worktree, attachments, working_notes")
	}
	b := c.Board
	t, err := c.Resolve(a.ID)
	if err != nil {
		return "", err
	}
	title, desc, deadline, parentID := t.Title, t.Description, t.Deadline, t.ParentID
	if a.Title != nil {
		title = *a.Title
	}
	if a.Description != nil {
		desc = *a.Description
	}
	if a.Deadline != nil {
		if deadline, err = parseDeadline(*a.Deadline, c.Now.Location()); err != nil {
			return "", err
		}
	}
	if a.Parent != nil {
		parentID = ""
		if *a.Parent != "" {
			parent, err := c.Resolve(*a.Parent)
			if err != nil {
				return "", fmt.Errorf("parent: %w", err)
			}
			parentID = parent.ID
		}
	}
	if err := b.EditWithParent(t.ID, parentID, title, desc, deadline, c.Now); err != nil {
		return "", err
	}
	if a.Worktree != nil {
		if err := b.SetWorktree(t.ID, *a.Worktree, c.Now); err != nil {
			return "", err
		}
	}
	if a.Attachments != nil || a.WorkingNotes != nil {
		attachments, notes := t.Attachments, t.WorkingNotes
		if a.Attachments != nil {
			attachments = *a.Attachments
		}
		if a.WorkingNotes != nil {
			notes = *a.WorkingNotes
		}
		if err := b.SetDocuments(t.ID, attachments, notes, c.Now); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("edited %s [%s] %s", shortID(t.ID), t.Status, strings.TrimSpace(title)), nil
}

func toolDeleteTask(c *Call) (string, error) {
	var a idArgs
	if err := c.Decode(&a); err != nil {
		return "", err
	}
	b := c.Board
	t, err := c.Resolve(a.ID)
	if err != nil {
		return "", err
	}
	detached := len(b.Children(t.ID))
	if err := b.Delete(t.ID); err != nil {
		return "", err
	}
	text := fmt.Sprintf("deleted %s [%s] %s", shortID(t.ID), t.Status, t.Title)
	if detached > 0 {
		text += fmt.Sprintf(" · detached %d subtasks", detached)
	}
	return text, nil
}

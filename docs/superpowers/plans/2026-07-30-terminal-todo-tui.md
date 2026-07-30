# Keyboard-Driven Terminal Todo (Kanban + Analytics) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fully keyboard-driven terminal kanban todo app in Go with four columns (todo / doing / blocked / done) and a second Analytics page, switched with Tab.

**Architecture:** Bubble Tea (Elm architecture) root model owns a `page` enum and routes `tea.Msg` to one of two sub-models (Board, Analytics). All domain logic lives in `internal/task` (pure data + JSON store) and `internal/stats` (pure functions over `[]task.Task`, every one taking an explicit `now time.Time` so tests are deterministic). The UI layer is render-only: it never computes analytics inline. Persistence is a single JSON file written atomically after every mutation.

**Tech Stack:** Go 1.22, `bubbletea` (event loop), `lipgloss` (styling/layout), `bubbles/textinput` (add/edit prompt). Storage: `encoding/json` stdlib, single file. No database, no ORM, no external chart library — charts are Unicode block runes.

## Global Constraints

- Go 1.22 (`go 1.22` in go.mod). Module name: `gotodo`.
- Dependencies limited to exactly three: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`. Do not add any other module.
- Naming: Go idiomatic (`CamelCase` exported, `camelCase` unexported). File names use underscore style: `task_test.go`, `store_test.go`, `chart_test.go`.
- All dates rendered to the user use Singapore format `DD/MM/YYYY` (Go layout `"02/01/2006"`). Never `MM/DD/YYYY`.
- No PII in test fixtures, sample data, or comments — use placeholders like `"[Task title]"`, never real names or emails.
- Every stats function takes `now time.Time` as its last parameter. No stats function may call `time.Now()` internally.
- Every test uses a fixed reference time, never `time.Now()`. Reference time for all tests: `time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)`.
- Tests run with `go test ./...` from the repo root. Every task ends green.
- Commit after every task using conventional commits (`feat:`, `test:`, `chore:`).

## Keybinding Contract (frozen — implement exactly)

Global:
| Key | Action |
|---|---|
| `tab` | switch page: Board ↔ Analytics |
| `ctrl+t` | toggle focus mode: Column ↔ Item (Board page only) |
| `?` | toggle help overlay |
| `q`, `ctrl+c` | quit |

Board page, Normal mode, **Column focus** (whole column highlighted):
| Key | Action |
|---|---|
| `h` / `left` | previous column |
| `l` / `right` | next column |
| `j` / `k` | scroll the focused column's viewport by one line |

Board page, Normal mode, **Item focus** (one card highlighted):
| Key | Action |
|---|---|
| `j` / `down` | next task in column |
| `k` / `up` | previous task in column |
| `h` / `l` | move focus to adjacent column, clamping the selection index |
| `g` / `G` | first / last task in column |
| `a` | add task (opens input at bottom, new task lands in `todo`) |
| `e` | edit focused task title |
| `d` | delete focused task (asks `y/n`) |
| `m` | grab focused task → Move mode |

Move mode (task is "grabbed"):
| Key | Action |
|---|---|
| `h` / `l` | move the grabbed task to previous / next column, live |
| `enter` | drop it (persist) |
| `esc` | cancel, return task to its original column |

Input mode (add/edit): `enter` commits, `esc` cancels. All other keys go to the textinput.
Confirm-delete mode: `y` deletes, any other key cancels.

---

## File Structure

| File | Responsibility |
|---|---|
| `go.mod`, `go.sum` | module + the three deps |
| `main.go` | flag parsing (`-file`), load store, start Bubble Tea program |
| `internal/task/task.go` | `Status`, `Task`, `Transition`, ID generation, status helpers |
| `internal/task/task_test.go` | tests for the above |
| `internal/task/board.go` | `Board` aggregate: `Add`, `Move`, `Edit`, `Delete`, `ByStatus` |
| `internal/task/board_test.go` | tests for board ops |
| `internal/task/store.go` | `DefaultPath`, `Load`, `(*Board).Save` (atomic write) |
| `internal/task/store_test.go` | round-trip + atomicity tests |
| `internal/stats/stats.go` | `Counts`, `Throughput`, `Streak`, `HeatmapGrid`, `TimeInStatus`, `CycleTimes`, `BlockedReport` |
| `internal/stats/stats_test.go` | tests for all stats |
| `internal/ui/theme.go` | lipgloss palette + shared styles, per-column accent colors |
| `internal/ui/chart.go` | pure string renderers: `Sparkline`, `Heatmap`, `HBar` |
| `internal/ui/chart_test.go` | tests for chart renderers |
| `internal/ui/board.go` | Board page model: state, `Update`, `View` |
| `internal/ui/board_test.go` | key-routing tests driving `Update` directly |
| `internal/ui/analytics.go` | Analytics page model: `Update`, `View` |
| `internal/ui/app.go` | root model: page switching, help overlay, save-on-mutate |
| `internal/ui/app_test.go` | tab switching + quit tests |

---

## Task 1: Project scaffold and the Task model

**Files:**
- Create: `go.mod`
- Create: `internal/task/task.go`
- Test: `internal/task/task_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `task.Status` (string type) with constants `StatusTodo`, `StatusDoing`, `StatusBlocked`, `StatusDone`; `task.Statuses []Status` (ordered left→right); `task.Task` struct; `task.Transition` struct; `func (s Status) Label() string`; `func (s Status) Index() int`; `func task.CompletedAt(t Task) (time.Time, bool)`; `func task.NewTask(title string, now time.Time) Task`.

- [ ] **Step 1: Initialise the module and fetch dependencies**

```bash
cd "$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
go mod init gotodo
# Pinned: lipgloss v1.x deprecates Style.Copy(), which this plan's theme uses.
go get github.com/charmbracelet/bubbletea@v0.25.0
go get github.com/charmbracelet/lipgloss@v0.9.1
go get github.com/charmbracelet/bubbles@v0.18.0
git init 2>/dev/null || true
printf 'gotodo\n*.tmp\n' > .gitignore
```

Expected: `go.mod` exists with `go 1.22` and the three requires. If `go mod init` reports the module already exists, skip it.

- [ ] **Step 2: Write the failing test**

Create `internal/task/task_test.go`:

```go
package task

import (
	"testing"
	"time"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

func TestNewTaskDefaults(t *testing.T) {
	got := NewTask("[Task title]", ref)
	if got.Title != "[Task title]" {
		t.Errorf("Title = %q, want %q", got.Title, "[Task title]")
	}
	if got.Status != StatusTodo {
		t.Errorf("Status = %q, want %q", got.Status, StatusTodo)
	}
	if !got.CreatedAt.Equal(ref) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ref)
	}
	if got.ID == "" {
		t.Error("ID is empty")
	}
	if len(got.History) != 0 {
		t.Errorf("History = %v, want empty", got.History)
	}
}

func TestNewTaskIDsAreUnique(t *testing.T) {
	a := NewTask("[Task title]", ref)
	b := NewTask("[Task title]", ref)
	if a.ID == b.ID {
		t.Fatalf("IDs collided: %q", a.ID)
	}
}

func TestStatusIndex(t *testing.T) {
	cases := map[Status]int{StatusTodo: 0, StatusDoing: 1, StatusBlocked: 2, StatusDone: 3}
	for s, want := range cases {
		if got := s.Index(); got != want {
			t.Errorf("%q.Index() = %d, want %d", s, got, want)
		}
	}
	if got := Status("nope").Index(); got != 0 {
		t.Errorf("unknown status index = %d, want 0", got)
	}
}

func TestCompletedAtReturnsLastDoneTransition(t *testing.T) {
	first := ref.Add(-48 * time.Hour)
	last := ref.Add(-2 * time.Hour)
	tk := Task{
		Status: StatusDone,
		History: []Transition{
			{From: StatusTodo, To: StatusDone, At: first},
			{From: StatusDone, To: StatusDoing, At: ref.Add(-24 * time.Hour)},
			{From: StatusDoing, To: StatusDone, At: last},
		},
	}
	got, ok := CompletedAt(tk)
	if !ok {
		t.Fatal("CompletedAt returned ok=false, want true")
	}
	if !got.Equal(last) {
		t.Errorf("CompletedAt = %v, want %v", got, last)
	}
}

func TestCompletedAtNotDone(t *testing.T) {
	tk := Task{Status: StatusDoing, History: []Transition{{From: StatusTodo, To: StatusDoing, At: ref}}}
	if _, ok := CompletedAt(tk); ok {
		t.Error("CompletedAt returned ok=true for a non-done task")
	}
}
```

- [ ] **Step 3: Run the test and confirm it fails**

Run: `go test ./internal/task/ -v`
Expected: FAIL — `undefined: NewTask`, `undefined: StatusTodo`, etc.

- [ ] **Step 4: Write the implementation**

Create `internal/task/task.go`:

```go
// Package task holds the todo domain model and its JSON persistence.
package task

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Status is the kanban column a task sits in.
type Status string

const (
	StatusTodo    Status = "todo"
	StatusDoing   Status = "doing"
	StatusBlocked Status = "blocked"
	StatusDone    Status = "done"
)

// Statuses lists the columns in left-to-right board order.
var Statuses = []Status{StatusTodo, StatusDoing, StatusBlocked, StatusDone}

// Label is the human-readable column heading.
func (s Status) Label() string {
	switch s {
	case StatusTodo:
		return "TODO"
	case StatusDoing:
		return "DOING"
	case StatusBlocked:
		return "BLOCKED"
	case StatusDone:
		return "DONE"
	}
	return string(s)
}

// Index is the column position, 0-based. Unknown statuses report 0.
func (s Status) Index() int {
	for i, c := range Statuses {
		if c == s {
			return i
		}
	}
	return 0
}

// Transition records one status change, so analytics can reconstruct history.
type Transition struct {
	From Status    `json:"from"`
	To   Status    `json:"to"`
	At   time.Time `json:"at"`
}

// Task is one card on the board.
type Task struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Status    Status       `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	History   []Transition `json:"history,omitempty"`
}

// NewTask builds a task in the todo column with a fresh random ID.
func NewTask(title string, now time.Time) Task {
	return Task{
		ID:        newID(),
		Title:     title,
		Status:    StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means the process is broken; a time-based
		// fallback keeps the app usable rather than panicking on Add.
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b)
}

// CompletedAt reports when the task last entered done. ok is false unless the
// task is currently done, so reopened tasks are not counted as completions.
func CompletedAt(t Task) (time.Time, bool) {
	if t.Status != StatusDone {
		return time.Time{}, false
	}
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].To == StatusDone {
			return t.History[i].At, true
		}
	}
	return time.Time{}, false
}
```

- [ ] **Step 5: Run the test and confirm it passes**

Run: `go test ./internal/task/ -v`
Expected: PASS, 5 tests.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum .gitignore internal/task/task.go internal/task/task_test.go
git commit -m "feat: add task domain model with transition history"
```

---

## Task 2: Board aggregate — add, move, edit, delete, query

**Files:**
- Create: `internal/task/board.go`
- Test: `internal/task/board_test.go`

**Interfaces:**
- Consumes: `Task`, `Status`, `Transition`, `NewTask` from Task 1.
- Produces: `type Board struct { Tasks []Task }`; `func (b *Board) Add(title string, now time.Time) *Task`; `func (b *Board) Move(id string, to Status, now time.Time) error`; `func (b *Board) Edit(id, title string, now time.Time) error`; `func (b *Board) Delete(id string) error`; `func (b *Board) ByStatus(s Status) []Task`; `var ErrNotFound error`.

Note for the implementer: `Board.Tasks` is a flat slice, not per-column slices. `ByStatus` filters on demand and preserves insertion order. This keeps `Move` a one-field write instead of a splice across two slices.

- [ ] **Step 1: Write the failing test**

Create `internal/task/board_test.go`:

```go
package task

import (
	"errors"
	"testing"
	"time"
)

func TestBoardAddAppendsToTodo(t *testing.T) {
	var b Board
	got := b.Add("[Task title]", ref)
	if got == nil {
		t.Fatal("Add returned nil")
	}
	if len(b.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(b.Tasks))
	}
	if b.Tasks[0].Status != StatusTodo {
		t.Errorf("Status = %q, want %q", b.Tasks[0].Status, StatusTodo)
	}
}

func TestBoardMoveRecordsTransition(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", ref).ID
	later := ref.Add(time.Hour)

	if err := b.Move(id, StatusDoing, later); err != nil {
		t.Fatalf("Move returned %v, want nil", err)
	}

	got := b.Tasks[0]
	if got.Status != StatusDoing {
		t.Errorf("Status = %q, want %q", got.Status, StatusDoing)
	}
	if len(got.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(got.History))
	}
	tr := got.History[0]
	if tr.From != StatusTodo || tr.To != StatusDoing || !tr.At.Equal(later) {
		t.Errorf("History[0] = %+v, want {todo doing %v}", tr, later)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
	}
}

func TestBoardMoveToSameStatusIsNoOp(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", ref).ID
	if err := b.Move(id, StatusTodo, ref.Add(time.Hour)); err != nil {
		t.Fatalf("Move returned %v, want nil", err)
	}
	if len(b.Tasks[0].History) != 0 {
		t.Errorf("History = %v, want empty for a same-column move", b.Tasks[0].History)
	}
}

func TestBoardMoveUnknownID(t *testing.T) {
	var b Board
	if err := b.Move("missing", StatusDone, ref); !errors.Is(err, ErrNotFound) {
		t.Errorf("Move error = %v, want ErrNotFound", err)
	}
}

func TestBoardEdit(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", ref).ID
	later := ref.Add(time.Hour)
	if err := b.Edit(id, "[New title]", later); err != nil {
		t.Fatalf("Edit returned %v, want nil", err)
	}
	if b.Tasks[0].Title != "[New title]" {
		t.Errorf("Title = %q, want %q", b.Tasks[0].Title, "[New title]")
	}
	if !b.Tasks[0].UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", b.Tasks[0].UpdatedAt, later)
	}
}

func TestBoardDelete(t *testing.T) {
	var b Board
	keep := b.Add("[Keep]", ref).ID
	drop := b.Add("[Drop]", ref).ID

	if err := b.Delete(drop); err != nil {
		t.Fatalf("Delete returned %v, want nil", err)
	}
	if len(b.Tasks) != 1 || b.Tasks[0].ID != keep {
		t.Fatalf("Tasks = %+v, want only the kept task", b.Tasks)
	}
	if err := b.Delete(drop); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete error = %v, want ErrNotFound", err)
	}
}

func TestBoardByStatusPreservesOrder(t *testing.T) {
	var b Board
	first := b.Add("[First]", ref).ID
	second := b.Add("[Second]", ref).ID
	third := b.Add("[Third]", ref).ID
	if err := b.Move(second, StatusDoing, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	todo := b.ByStatus(StatusTodo)
	if len(todo) != 2 || todo[0].ID != first || todo[1].ID != third {
		t.Errorf("ByStatus(todo) = %+v, want [First Third]", todo)
	}
	if doing := b.ByStatus(StatusDoing); len(doing) != 1 || doing[0].ID != second {
		t.Errorf("ByStatus(doing) = %+v, want [Second]", doing)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/task/ -run TestBoard -v`
Expected: FAIL — `undefined: Board`.

- [ ] **Step 3: Write the implementation**

Create `internal/task/board.go`:

```go
package task

import (
	"errors"
	"strings"
	"time"
)

// ErrNotFound is returned when no task matches the given ID.
var ErrNotFound = errors.New("task not found")

// Board is the whole set of tasks. Columns are derived from Task.Status
// rather than stored separately, so a move is a single field write.
type Board struct {
	Tasks []Task `json:"tasks"`
}

// Add appends a new todo task and returns a pointer into b.Tasks.
func (b *Board) Add(title string, now time.Time) *Task {
	b.Tasks = append(b.Tasks, NewTask(strings.TrimSpace(title), now))
	return &b.Tasks[len(b.Tasks)-1]
}

func (b *Board) find(id string) (*Task, error) {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			return &b.Tasks[i], nil
		}
	}
	return nil, ErrNotFound
}

// Move changes a task's column and records the transition. Moving to the
// column it already occupies is a no-op, so the history stays meaningful.
func (b *Board) Move(id string, to Status, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	if t.Status == to {
		return nil
	}
	t.History = append(t.History, Transition{From: t.Status, To: to, At: now})
	t.Status = to
	t.UpdatedAt = now
	return nil
}

// Edit replaces the title. Blank titles are rejected so the board cannot
// grow unreadable empty cards.
func (b *Board) Edit(id, title string, now time.Time) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must not be blank")
	}
	t, err := b.find(id)
	if err != nil {
		return err
	}
	t.Title = title
	t.UpdatedAt = now
	return nil
}

// Delete removes a task permanently.
func (b *Board) Delete(id string) error {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			b.Tasks = append(b.Tasks[:i], b.Tasks[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

// ByStatus returns the tasks in one column, in insertion order.
func (b *Board) ByStatus(s Status) []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if t.Status == s {
			out = append(out, t)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run: `go test ./internal/task/ -v`
Expected: PASS, all tests including Task 1's.

- [ ] **Step 5: Commit**

```bash
git add internal/task/board.go internal/task/board_test.go
git commit -m "feat: add board aggregate with add/move/edit/delete"
```

---

## Task 3: JSON store with atomic writes

**Files:**
- Create: `internal/task/store.go`
- Test: `internal/task/store_test.go`

**Interfaces:**
- Consumes: `Board` from Task 2.
- Produces: `func DefaultPath() (string, error)`; `func Load(path string) (*Board, error)`; `func (b *Board) Save() error`; field `Board.path string` set by `Load`; `func (b *Board) SetPath(p string)`.

Note: `Load` on a missing file returns an empty board and no error — first run must not be an error path. `Save` writes to `path + ".tmp"` then `os.Rename`, so a crash mid-write cannot truncate the user's tasks.

- [ ] **Step 1: Write the failing test**

Create `internal/task/store_test.go`:

```go
package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsEmptyBoard(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v, want nil", err)
	}
	if len(b.Tasks) != 0 {
		t.Errorf("Tasks = %+v, want empty", b.Tasks)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")

	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	id := b.Add("[Task title]", ref).ID
	if err := b.Move(id, StatusBlocked, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}

	got, err := Load(p)
	if err != nil {
		t.Fatalf("reload returned %v", err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(got.Tasks))
	}
	if got.Tasks[0].Status != StatusBlocked {
		t.Errorf("Status = %q, want %q", got.Tasks[0].Status, StatusBlocked)
	}
	if len(got.Tasks[0].History) != 1 {
		t.Errorf("len(History) = %d, want 1", len(got.Tasks[0].History))
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	b.Add("[Task title]", ref)
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "tasks.json" {
		t.Errorf("dir contents = %v, want only tasks.json", entries)
	}
}

func TestLoadCorruptFileErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned %v", err)
	}
	if _, err := Load(p); err == nil {
		t.Error("Load returned nil error for corrupt JSON, want an error")
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/task/ -run 'TestLoad|TestSave' -v`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Write the implementation**

Create `internal/task/store.go`:

```go
package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultPath is the per-user tasks file, e.g. ~/.config/gotodo/tasks.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "gotodo", "tasks.json"), nil
}

// SetPath points the board at a file for later Save calls.
func (b *Board) SetPath(p string) { b.path = p }

// Load reads the board from disk. A missing file yields an empty board so
// the first run is not an error; malformed JSON is an error so a bad file is
// never silently overwritten with an empty board.
func Load(path string) (*Board, error) {
	b := &Board{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	b.path = path
	return b, nil
}

// Save writes the board atomically: temp file first, then rename.
func (b *Board) Save() error {
	if b.path == "" {
		return errors.New("board has no path; call SetPath or Load first")
	}
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encode board: %w", err)
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}
	if err := os.Rename(tmp, b.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}
```

Then add the unexported `path` field to the `Board` struct in `internal/task/board.go` — change the struct to:

```go
type Board struct {
	Tasks []Task `json:"tasks"`

	path string // where Save writes; unexported so it stays out of the JSON
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/task/ -v`
Expected: PASS, all task-package tests.

- [ ] **Step 5: Commit**

```bash
git add internal/task/store.go internal/task/store_test.go internal/task/board.go
git commit -m "feat: persist board to JSON with atomic writes"
```

---

## Task 4: Stats — counts, throughput, streak, heatmap

**Files:**
- Create: `internal/stats/stats.go`
- Test: `internal/stats/stats_test.go`

**Interfaces:**
- Consumes: `task.Task`, `task.Status`, `task.Statuses`, `task.CompletedAt`, `task.StatusDone`.
- Produces:
  - `func Counts(tasks []task.Task) map[task.Status]int`
  - `type DayCount struct { Day time.Time; N int }`
  - `func Throughput(tasks []task.Task, days int, now time.Time) []DayCount` — oldest first, exactly `days` entries ending on `now`'s day, zero-filled.
  - `func Streak(tasks []task.Task, now time.Time) (current, longest int)`
  - `func HeatmapGrid(tasks []task.Task, weeks int, now time.Time) [][]int` — `weeks` columns × 7 rows, `grid[row][col]`, row 0 = Monday.

Note: all day bucketing uses `now`'s location via `time.Date(y, m, d, 0,0,0,0, now.Location())`. Current streak counts back from today; if today has no completions but yesterday does, the streak still counts (the day is not over).

- [ ] **Step 1: Write the failing test**

Create `internal/stats/stats_test.go`:

```go
package stats

import (
	"testing"
	"time"

	"gotodo/internal/task"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

// done builds a task completed at the given time.
func done(id string, created, completed time.Time) task.Task {
	return task.Task{
		ID:        id,
		Title:     "[Task title]",
		Status:    task.StatusDone,
		CreatedAt: created,
		UpdatedAt: completed,
		History:   []task.Transition{{From: task.StatusTodo, To: task.StatusDone, At: completed}},
	}
}

func TestCounts(t *testing.T) {
	tasks := []task.Task{
		{Status: task.StatusTodo}, {Status: task.StatusTodo},
		{Status: task.StatusBlocked},
	}
	got := Counts(tasks)
	if got[task.StatusTodo] != 2 {
		t.Errorf("todo = %d, want 2", got[task.StatusTodo])
	}
	if got[task.StatusBlocked] != 1 {
		t.Errorf("blocked = %d, want 1", got[task.StatusBlocked])
	}
	if got[task.StatusDoing] != 0 {
		t.Errorf("doing = %d, want 0", got[task.StatusDoing])
	}
	if len(got) != len(task.Statuses) {
		t.Errorf("len(Counts) = %d, want %d (every column present)", len(got), len(task.Statuses))
	}
}

func TestThroughputZeroFillsAndOrders(t *testing.T) {
	tasks := []task.Task{
		done("a", ref.Add(-72*time.Hour), ref),                    // today
		done("b", ref.Add(-72*time.Hour), ref),                    // today
		done("c", ref.Add(-72*time.Hour), ref.Add(-48*time.Hour)), // two days ago
	}
	got := Throughput(tasks, 3, ref)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []int{1, 0, 2} // oldest first: -2d, -1d, today
	for i, w := range want {
		if got[i].N != w {
			t.Errorf("day %d (%v) N = %d, want %d", i, got[i].Day, got[i].N, w)
		}
	}
	if !got[2].Day.Equal(time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("last day = %v, want 30/07/2026 midnight", got[2].Day)
	}
}

func TestThroughputIgnoresUnfinished(t *testing.T) {
	tasks := []task.Task{{Status: task.StatusDoing, CreatedAt: ref, UpdatedAt: ref}}
	got := Throughput(tasks, 2, ref)
	for _, d := range got {
		if d.N != 0 {
			t.Errorf("day %v N = %d, want 0", d.Day, d.N)
		}
	}
}

func TestStreakCurrentAndLongest(t *testing.T) {
	day := func(offset int) time.Time { return ref.AddDate(0, 0, -offset) }
	tasks := []task.Task{
		done("a", day(10), day(0)), // today
		done("b", day(10), day(1)),
		done("c", day(10), day(2)),
		// gap at day 3
		done("d", day(10), day(5)),
		done("e", day(10), day(6)),
	}
	current, longest := Streak(tasks, ref)
	if current != 3 {
		t.Errorf("current = %d, want 3", current)
	}
	if longest != 3 {
		t.Errorf("longest = %d, want 3", longest)
	}
}

func TestStreakCountsYesterdayWhenTodayEmpty(t *testing.T) {
	day := func(offset int) time.Time { return ref.AddDate(0, 0, -offset) }
	tasks := []task.Task{done("a", day(5), day(1)), done("b", day(5), day(2))}
	current, _ := Streak(tasks, ref)
	if current != 2 {
		t.Errorf("current = %d, want 2 (today is not over yet)", current)
	}
}

func TestStreakEmpty(t *testing.T) {
	current, longest := Streak(nil, ref)
	if current != 0 || longest != 0 {
		t.Errorf("Streak(nil) = (%d, %d), want (0, 0)", current, longest)
	}
}

func TestHeatmapGridShapeAndPlacement(t *testing.T) {
	// 30/07/2026 is a Thursday -> weekday row 3 (Mon=0).
	tasks := []task.Task{done("a", ref.AddDate(0, 0, -20), ref)}
	grid := HeatmapGrid(tasks, 4, ref)
	if len(grid) != 7 {
		t.Fatalf("rows = %d, want 7", len(grid))
	}
	for i, row := range grid {
		if len(row) != 4 {
			t.Fatalf("row %d has %d cols, want 4", i, len(row))
		}
	}
	if grid[3][3] != 1 {
		t.Errorf("grid[3][3] = %d, want 1 (Thursday of the last week)", grid[3][3])
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/stats/ -v`
Expected: FAIL — `undefined: Counts`.

- [ ] **Step 3: Write the implementation**

Create `internal/stats/stats.go`:

```go
// Package stats computes board analytics. Every function takes an explicit
// now so results are deterministic and testable.
package stats

import (
	"time"

	"gotodo/internal/task"
)

// DayCount is one bucket of the throughput series.
type DayCount struct {
	Day time.Time
	N   int
}

// startOfDay truncates to midnight in t's own location. time.Truncate is
// wrong here: it works on absolute time, not calendar days.
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// Counts returns the number of tasks per column, with every column present
// (zeroes included) so callers can render a stable row of tiles.
func Counts(tasks []task.Task) map[task.Status]int {
	out := make(map[task.Status]int, len(task.Statuses))
	for _, s := range task.Statuses {
		out[s] = 0
	}
	for _, t := range tasks {
		if _, ok := out[t.Status]; ok {
			out[t.Status]++
		}
	}
	return out
}

// completionDays buckets every completed task by the day it was completed.
func completionDays(tasks []task.Task, loc *time.Location) map[time.Time]int {
	out := map[time.Time]int{}
	for _, t := range tasks {
		at, ok := task.CompletedAt(t)
		if !ok {
			continue
		}
		out[startOfDay(at.In(loc))]++
	}
	return out
}

// Throughput returns completions per day for the last `days` days, oldest
// first, ending on now's day. Empty days are present with N == 0.
func Throughput(tasks []task.Task, days int, now time.Time) []DayCount {
	if days < 1 {
		return nil
	}
	byDay := completionDays(tasks, now.Location())
	today := startOfDay(now)
	out := make([]DayCount, 0, days)
	for i := days - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		out = append(out, DayCount{Day: d, N: byDay[d]})
	}
	return out
}

// Streak reports the current and longest run of consecutive days with at
// least one completion. The current streak may end on yesterday, since today
// is not over yet.
func Streak(tasks []task.Task, now time.Time) (current, longest int) {
	byDay := completionDays(tasks, now.Location())
	if len(byDay) == 0 {
		return 0, 0
	}
	today := startOfDay(now)

	// Current: walk back from today, tolerating an empty today.
	start := today
	if byDay[start] == 0 {
		start = today.AddDate(0, 0, -1)
	}
	for d := start; byDay[d] > 0; d = d.AddDate(0, 0, -1) {
		current++
	}

	// Longest: walk back far enough to cover the oldest completion.
	oldest := today
	for d := range byDay {
		if d.Before(oldest) {
			oldest = d
		}
	}
	run := 0
	for d := oldest; !d.After(today); d = d.AddDate(0, 0, 1) {
		if byDay[d] > 0 {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return current, longest
}

// HeatmapGrid returns a 7-row (Mon..Sun) by `weeks`-column grid of
// completion counts. The rightmost column is the week containing now.
func HeatmapGrid(tasks []task.Task, weeks int, now time.Time) [][]int {
	if weeks < 1 {
		weeks = 1
	}
	grid := make([][]int, 7)
	for i := range grid {
		grid[i] = make([]int, weeks)
	}
	byDay := completionDays(tasks, now.Location())

	// Monday of the current week, then step back to the first shown week.
	today := startOfDay(now)
	monday := today.AddDate(0, 0, -weekdayIndex(today))
	first := monday.AddDate(0, 0, -7*(weeks-1))

	for col := 0; col < weeks; col++ {
		weekStart := first.AddDate(0, 0, 7*col)
		for row := 0; row < 7; row++ {
			grid[row][col] = byDay[weekStart.AddDate(0, 0, row)]
		}
	}
	return grid
}

// weekdayIndex maps Monday..Sunday to 0..6 (Go's Weekday puts Sunday at 0).
func weekdayIndex(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run: `go test ./internal/stats/ -v`
Expected: PASS, 7 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/stats/stats.go internal/stats/stats_test.go
git commit -m "feat: add counts, throughput, streak and heatmap analytics"
```

---

## Task 5: Stats — cycle time, time-in-status, blocked report

**Files:**
- Modify: `internal/stats/stats.go` (append; do not rewrite Task 4's code)
- Modify: `internal/stats/stats_test.go` (append)

**Interfaces:**
- Consumes: everything from Task 4 plus `task.Transition`, `task.StatusBlocked`.
- Produces:
  - `func TimeInStatus(t task.Task, now time.Time) map[task.Status]time.Duration`
  - `type CycleStats struct { Mean, Median time.Duration; N int; PerColumn map[task.Status]time.Duration }`
  - `func CycleTimes(tasks []task.Task, now time.Time) CycleStats` — `PerColumn` holds the **mean** time each completed task spent in that column.
  - `type BlockedItem struct { Title string; For time.Duration }`
  - `func BlockedReport(tasks []task.Task, now time.Time) []BlockedItem` — currently-blocked tasks only, longest-blocked first.

- [ ] **Step 1: Write the failing test**

Append to `internal/stats/stats_test.go`:

```go
func TestTimeInStatusWalksHistory(t *testing.T) {
	created := ref.Add(-10 * time.Hour)
	tk := task.Task{
		Status:    task.StatusDoing,
		CreatedAt: created,
		History: []task.Transition{
			{From: task.StatusTodo, To: task.StatusDoing, At: created.Add(2 * time.Hour)},
			{From: task.StatusDoing, To: task.StatusBlocked, At: created.Add(3 * time.Hour)},
			{From: task.StatusBlocked, To: task.StatusDoing, At: created.Add(7 * time.Hour)},
		},
	}
	got := TimeInStatus(tk, ref)
	want := map[task.Status]time.Duration{
		task.StatusTodo:    2 * time.Hour,
		task.StatusDoing:   1*time.Hour + 3*time.Hour, // 1h before block, 3h after, still open
		task.StatusBlocked: 4 * time.Hour,
	}
	for s, w := range want {
		if got[s] != w {
			t.Errorf("TimeInStatus[%q] = %v, want %v", s, got[s], w)
		}
	}
}

func TestTimeInStatusStopsAccruingWhenDone(t *testing.T) {
	created := ref.Add(-10 * time.Hour)
	tk := task.Task{
		Status:    task.StatusDone,
		CreatedAt: created,
		History:   []task.Transition{{From: task.StatusTodo, To: task.StatusDone, At: created.Add(4 * time.Hour)}},
	}
	got := TimeInStatus(tk, ref)
	if got[task.StatusTodo] != 4*time.Hour {
		t.Errorf("todo = %v, want 4h", got[task.StatusTodo])
	}
	if got[task.StatusDone] != 0 {
		t.Errorf("done = %v, want 0 (done tasks stop accruing)", got[task.StatusDone])
	}
}

func TestCycleTimesMeanAndMedian(t *testing.T) {
	tasks := []task.Task{
		done("a", ref.Add(-2*time.Hour), ref),
		done("b", ref.Add(-4*time.Hour), ref),
		done("c", ref.Add(-9*time.Hour), ref),
		{Status: task.StatusDoing, CreatedAt: ref.Add(-100 * time.Hour)}, // ignored
	}
	got := CycleTimes(tasks, ref)
	if got.N != 3 {
		t.Fatalf("N = %d, want 3", got.N)
	}
	if got.Mean != 5*time.Hour {
		t.Errorf("Mean = %v, want 5h", got.Mean)
	}
	if got.Median != 4*time.Hour {
		t.Errorf("Median = %v, want 4h", got.Median)
	}
}

func TestCycleTimesEmpty(t *testing.T) {
	got := CycleTimes(nil, ref)
	if got.N != 0 || got.Mean != 0 || got.Median != 0 {
		t.Errorf("CycleTimes(nil) = %+v, want zeroes", got)
	}
	if got.PerColumn == nil {
		t.Error("PerColumn is nil, want an empty non-nil map")
	}
}

func TestBlockedReportSortedLongestFirst(t *testing.T) {
	mk := func(title string, blockedAt time.Time) task.Task {
		return task.Task{
			Title:     title,
			Status:    task.StatusBlocked,
			CreatedAt: blockedAt.Add(-time.Hour),
			History:   []task.Transition{{From: task.StatusDoing, To: task.StatusBlocked, At: blockedAt}},
		}
	}
	tasks := []task.Task{
		mk("[Short]", ref.Add(-1*time.Hour)),
		mk("[Long]", ref.Add(-9*time.Hour)),
		{Title: "[Not blocked]", Status: task.StatusDoing, CreatedAt: ref.Add(-99 * time.Hour)},
	}
	got := BlockedReport(tasks, ref)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "[Long]" || got[0].For != 9*time.Hour {
		t.Errorf("got[0] = %+v, want {[Long] 9h}", got[0])
	}
	if got[1].Title != "[Short]" || got[1].For != 1*time.Hour {
		t.Errorf("got[1] = %+v, want {[Short] 1h}", got[1])
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/stats/ -run 'TestTimeInStatus|TestCycle|TestBlocked' -v`
Expected: FAIL — `undefined: TimeInStatus`.

- [ ] **Step 3: Write the implementation**

Append to `internal/stats/stats.go` (and add `"slices"` and `"sort"` are not both needed — use `slices` only):

```go
// TimeInStatus reconstructs how long a task has spent in each column by
// replaying its transition history. Done tasks stop accruing time.
func TimeInStatus(t task.Task, now time.Time) map[task.Status]time.Duration {
	out := map[task.Status]time.Duration{}
	cur := task.StatusTodo
	if len(t.History) > 0 {
		cur = t.History[0].From
	} else {
		cur = t.Status
	}
	start := t.CreatedAt
	for _, tr := range t.History {
		out[cur] += tr.At.Sub(start)
		cur, start = tr.To, tr.At
	}
	if cur != task.StatusDone {
		out[cur] += now.Sub(start)
	}
	return out
}

// CycleStats summarises how long completed tasks took end to end.
type CycleStats struct {
	Mean      time.Duration
	Median    time.Duration
	N         int
	PerColumn map[task.Status]time.Duration // mean time per column, completed tasks only
}

// CycleTimes measures created-to-done duration over completed tasks only.
func CycleTimes(tasks []task.Task, now time.Time) CycleStats {
	out := CycleStats{PerColumn: map[task.Status]time.Duration{}}
	var durations []time.Duration
	totals := map[task.Status]time.Duration{}

	for _, t := range tasks {
		at, ok := task.CompletedAt(t)
		if !ok {
			continue
		}
		durations = append(durations, at.Sub(t.CreatedAt))
		for s, d := range TimeInStatus(t, now) {
			totals[s] += d
		}
	}
	out.N = len(durations)
	if out.N == 0 {
		return out
	}

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	out.Mean = sum / time.Duration(out.N)
	out.Median = median(durations)
	for s, total := range totals {
		out.PerColumn[s] = total / time.Duration(out.N)
	}
	return out
}

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// BlockedItem is one currently-blocked task and how long it has been stuck.
type BlockedItem struct {
	Title string
	For   time.Duration
}

// BlockedReport lists currently-blocked tasks, longest-blocked first.
func BlockedReport(tasks []task.Task, now time.Time) []BlockedItem {
	var out []BlockedItem
	for _, t := range tasks {
		if t.Status != task.StatusBlocked {
			continue
		}
		since := t.CreatedAt
		for i := len(t.History) - 1; i >= 0; i-- {
			if t.History[i].To == task.StatusBlocked {
				since = t.History[i].At
				break
			}
		}
		out = append(out, BlockedItem{Title: t.Title, For: now.Sub(since)})
	}
	slices.SortStableFunc(out, func(a, b BlockedItem) int {
		switch {
		case a.For > b.For:
			return -1
		case a.For < b.For:
			return 1
		}
		return 0
	})
	return out
}
```

Add `"slices"` to the import block at the top of `internal/stats/stats.go`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/stats/ -v`
Expected: PASS, 12 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/stats/stats.go internal/stats/stats_test.go
git commit -m "feat: add cycle time, time-in-status and blocked analytics"
```

---

## Task 6: Theme and Unicode chart renderers

**Files:**
- Create: `internal/ui/theme.go`
- Create: `internal/ui/chart.go`
- Test: `internal/ui/chart_test.go`

**Interfaces:**
- Consumes: `task.Status`, `stats.DayCount`.
- Produces:
  - `theme.go`: `var ColBorder, ColMuted, ColText, ColAccent lipgloss.AdaptiveColor`; `func AccentFor(s task.Status) lipgloss.AdaptiveColor`; styles `TitleStyle`, `MutedStyle`, `HelpStyle`, `CardStyle`, `CardSelectedStyle`, `CardGrabbedStyle`, `ColumnStyle`, `ColumnFocusedStyle`, `HeaderStyle`, `StatTileStyle`; `func FormatDate(t time.Time) string`; `func FormatDuration(d time.Duration) string`.
  - `chart.go`: `func Sparkline(values []int) string`; `func Heatmap(grid [][]int) string`; `func HBar(label string, value, max, width int) string`.

Design intent: the board is dark-first but uses `lipgloss.AdaptiveColor` so it stays legible on light terminals. Column accents: todo = slate/blue, doing = amber, blocked = red, done = green. Borders are rounded. This is the whole "beautiful" budget — no gradients, no animation.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/chart_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"
)

func TestSparklineScalesToMax(t *testing.T) {
	got := Sparkline([]int{0, 1, 10})
	want := " ▁█"
	if got != want {
		t.Errorf("Sparkline = %q, want %q", got, want)
	}
}

func TestSparklineAllZeroes(t *testing.T) {
	got := Sparkline([]int{0, 0, 0})
	if got != "   " {
		t.Errorf("Sparkline = %q, want three spaces", got)
	}
}

func TestSparklineEmpty(t *testing.T) {
	if got := Sparkline(nil); got != "" {
		t.Errorf("Sparkline(nil) = %q, want empty", got)
	}
}

func TestHeatmapRowsAndRunes(t *testing.T) {
	grid := [][]int{
		{0, 4},
		{1, 0},
	}
	got := Heatmap(grid)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if lines[0] != "·█" {
		t.Errorf("line 0 = %q, want %q", lines[0], "·█")
	}
	if lines[1] != "▁·" {
		t.Errorf("line 1 = %q, want %q", lines[1], "▁·")
	}
}

func TestHBarFillsProportionally(t *testing.T) {
	got := HBar("todo", 5, 10, 10)
	if !strings.Contains(got, strings.Repeat("█", 5)) {
		t.Errorf("HBar = %q, want 5 filled blocks", got)
	}
	if !strings.HasPrefix(got, "todo") {
		t.Errorf("HBar = %q, want it to start with the label", got)
	}
}

func TestHBarZeroMax(t *testing.T) {
	got := HBar("todo", 0, 0, 10)
	if strings.Contains(got, "█") {
		t.Errorf("HBar = %q, want no filled blocks when max is 0", got)
	}
}

func TestFormatDateIsSingaporeFormat(t *testing.T) {
	got := FormatDate(time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	if got != "30/07/2026" {
		t.Errorf("FormatDate = %q, want %q", got, "30/07/2026")
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		45 * time.Second:     "0m",
		90 * time.Second:     "1m",
		3 * time.Hour:        "3h",
		30 * time.Hour:       "1d 6h",
		0:                    "0m",
	}
	for d, want := range cases {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -v`
Expected: FAIL — `undefined: Sparkline`.

- [ ] **Step 3: Write the theme**

Create `internal/ui/theme.go`:

```go
// Package ui holds the Bubble Tea models and all rendering.
package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// Palette. AdaptiveColor keeps the board legible on light terminals too.
var (
	ColText   = lipgloss.AdaptiveColor{Light: "#1f2430", Dark: "#e6e9ef"}
	ColMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#7a8290"}
	ColBorder = lipgloss.AdaptiveColor{Light: "#c8ccd4", Dark: "#3a4150"}
	ColAccent = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}

	colTodo    = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}
	colDoing   = lipgloss.AdaptiveColor{Light: "#b06f00", Dark: "#e0af68"}
	colBlocked = lipgloss.AdaptiveColor{Light: "#b02a37", Dark: "#f7768e"}
	colDone    = lipgloss.AdaptiveColor{Light: "#2f7a4f", Dark: "#9ece6a"}
)

// AccentFor is the signature colour of a column.
func AccentFor(s task.Status) lipgloss.AdaptiveColor {
	switch s {
	case task.StatusDoing:
		return colDoing
	case task.StatusBlocked:
		return colBlocked
	case task.StatusDone:
		return colDone
	}
	return colTodo
}

var (
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColAccent)
	MutedStyle = lipgloss.NewStyle().Foreground(ColMuted)
	HelpStyle  = lipgloss.NewStyle().Foreground(ColMuted).PaddingTop(1)

	ColumnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColBorder).
			Padding(0, 1)

	ColumnFocusedStyle = ColumnStyle.Copy().BorderForeground(ColAccent)

	CardStyle = lipgloss.NewStyle().
			Foreground(ColText).
			PaddingLeft(1)

	CardSelectedStyle = CardStyle.Copy().
				Bold(true).
				Foreground(ColAccent).
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(ColAccent).
				PaddingLeft(1)

	CardGrabbedStyle = CardSelectedStyle.Copy().
				Foreground(colDoing).
				BorderForeground(colDoing)

	StatTileStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColBorder).
			Padding(0, 2).
			Align(lipgloss.Center)
)

// HeaderStyle is the per-column heading, tinted with that column's accent.
func HeaderStyle(s task.Status) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(AccentFor(s))
}

// FormatDate renders Singapore-style DD/MM/YYYY.
func FormatDate(t time.Time) string { return t.Format("02/01/2006") }

// FormatDuration renders a compact human duration: "3h", "1d 6h", "12m".
func FormatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}
```

- [ ] **Step 4: Write the chart renderers**

Create `internal/ui/chart.go`:

```go
package ui

import (
	"fmt"
	"strings"
)

// sparkRunes index 0 is blank; 1..8 are increasing block heights.
var sparkRunes = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// heatRunes index 0 is "nothing happened"; 1..4 are increasing intensity.
var heatRunes = []rune{'·', '▁', '▄', '▓', '█'}

// Sparkline renders one block rune per value, scaled to the series max.
func Sparkline(values []int) string {
	if len(values) == 0 {
		return ""
	}
	max := 0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range values {
		if v <= 0 || max == 0 {
			b.WriteRune(sparkRunes[0])
			continue
		}
		b.WriteRune(sparkRunes[1+(v*7)/max])
	}
	return b.String()
}

// Heatmap renders a grid of counts as one line per row.
func Heatmap(grid [][]int) string {
	max := 0
	for _, row := range grid {
		for _, v := range row {
			if v > max {
				max = v
			}
		}
	}
	lines := make([]string, 0, len(grid))
	for _, row := range grid {
		var b strings.Builder
		for _, v := range row {
			b.WriteRune(heatRunes[heatLevel(v, max)])
		}
		lines = append(lines, b.String())
	}
	return strings.Join(lines, "\n")
}

func heatLevel(v, max int) int {
	if v <= 0 || max == 0 {
		return 0
	}
	l := 1 + (v*3)/max
	if l > 4 {
		l = 4
	}
	return l
}

// HBar renders "label ████░░░░ 5" scaled to max across width cells.
func HBar(label string, value, max, width int) string {
	filled := 0
	if max > 0 && value > 0 {
		filled = value * width / max
		if filled == 0 {
			filled = 1
		}
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + MutedStyle.Render(strings.Repeat("░", width-filled))
	return fmt.Sprintf("%-8s %s %d", label, bar, value)
}
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, 8 tests. If `TestHBarFillsProportionally` fails on the filled-block count, check the integer division: `5 * 10 / 10 == 5`.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/theme.go internal/ui/chart.go internal/ui/chart_test.go
git commit -m "feat: add theme palette and unicode chart renderers"
```

---

## Task 7: Board page — state and rendering

**Files:**
- Create: `internal/ui/board.go`
- Test: `internal/ui/board_test.go`

**Interfaces:**
- Consumes: `task.Board`, `task.Statuses`, theme styles from Task 6.
- Produces:
  - `type focusMode int` with `focusColumn`, `focusItem`
  - `type boardMode int` with `modeNormal`, `modeInput`, `modeMove`, `modeConfirm`
  - `type BoardModel struct { ... }` (fields listed below)
  - `func NewBoardModel(b *task.Board) BoardModel`
  - `func (m *BoardModel) SetSize(w, h int)`
  - `func (m BoardModel) View() string`
  - `func (m BoardModel) selectedTask() (task.Task, bool)`
  - `func (m *BoardModel) clampSelection()`

This task builds state + rendering only. Keys come in Task 8, mutations in Task 9.

Required `BoardModel` fields (later tasks depend on these exact names):

```go
type BoardModel struct {
	board  *task.Board
	col    int  // focused column index into task.Statuses
	sel    [4]int // selected item index per column
	focus  focusMode
	mode   boardMode
	input  textinput.Model
	editID string    // non-empty while editing an existing task
	grabID string    // non-empty while in modeMove
	grabFrom task.Status // where the grabbed task started, for esc
	width  int
	height int
	err    string
}
```

- [ ] **Step 1: Write the failing test**

Create `internal/ui/board_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

// seeded returns a board with one task per column.
func seeded(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	b.SetPath("")
	for _, s := range task.Statuses {
		id := b.Add("["+string(s)+" task]", ref).ID
		if err := b.Move(id, s, ref); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	return b
}

func TestNewBoardModelStartsInItemFocusOnTodo(t *testing.T) {
	m := NewBoardModel(seeded(t))
	if m.focus != focusItem {
		t.Errorf("focus = %v, want focusItem", m.focus)
	}
	if m.col != 0 {
		t.Errorf("col = %d, want 0", m.col)
	}
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
}

func TestViewRendersEveryColumnHeader(t *testing.T) {
	m := NewBoardModel(seeded(t))
	m.SetSize(120, 30)
	out := m.View()
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("View missing header %q", s.Label())
		}
	}
}

func TestViewRendersTaskTitles(t *testing.T) {
	m := NewBoardModel(seeded(t))
	m.SetSize(160, 30)
	out := m.View()
	if !strings.Contains(out, "todo task") {
		t.Errorf("View missing the todo task title:\n%s", out)
	}
	if !strings.Contains(out, "blocked task") {
		t.Errorf("View missing the blocked task title:\n%s", out)
	}
}

func TestViewShowsEmptyPlaceholder(t *testing.T) {
	m := NewBoardModel(&task.Board{})
	m.SetSize(120, 30)
	if !strings.Contains(m.View(), "empty") {
		t.Errorf("View of an empty board should say 'empty':\n%s", m.View())
	}
}

func TestSelectedTask(t *testing.T) {
	m := NewBoardModel(seeded(t))
	got, ok := m.selectedTask()
	if !ok {
		t.Fatal("selectedTask returned ok=false, want true")
	}
	if got.Status != task.StatusTodo {
		t.Errorf("selected status = %q, want todo", got.Status)
	}
}

func TestSelectedTaskEmptyColumn(t *testing.T) {
	m := NewBoardModel(&task.Board{})
	if _, ok := m.selectedTask(); ok {
		t.Error("selectedTask on an empty board returned ok=true, want false")
	}
}

func TestClampSelectionAfterShrink(t *testing.T) {
	b := seeded(t)
	m := NewBoardModel(b)
	m.sel[0] = 5 // stale index, e.g. after a delete
	m.clampSelection()
	if m.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (clamped to the single task)", m.sel[0])
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestNewBoardModel|TestView|TestSelected|TestClamp' -v`
Expected: FAIL — `undefined: NewBoardModel`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/board.go`:

```go
package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

type focusMode int

const (
	focusItem focusMode = iota
	focusColumn
)

type boardMode int

const (
	modeNormal boardMode = iota
	modeInput
	modeMove
	modeConfirm
)

// BoardModel is the kanban page.
type BoardModel struct {
	board *task.Board

	col   int    // focused column, index into task.Statuses
	sel   [4]int // selected item per column
	focus focusMode
	mode  boardMode

	input    textinput.Model
	editID   string      // set while editing an existing task
	grabID   string      // set while in modeMove
	grabFrom task.Status // original column of the grabbed task

	width  int
	height int
	err    string
}

// NewBoardModel wires a board into a fresh page model.
func NewBoardModel(b *task.Board) BoardModel {
	in := textinput.New()
	in.Placeholder = "task title"
	in.CharLimit = 200
	in.Prompt = "› "
	return BoardModel{board: b, focus: focusItem, mode: modeNormal, input: in}
}

// SetSize records the terminal size for layout.
func (m *BoardModel) SetSize(w, h int) { m.width, m.height = w, h }

func (m BoardModel) currentStatus() task.Status { return task.Statuses[m.col] }

// selectedTask is the card under the cursor, if the column is not empty.
func (m BoardModel) selectedTask() (task.Task, bool) {
	items := m.board.ByStatus(m.currentStatus())
	i := m.sel[m.col]
	if i < 0 || i >= len(items) {
		return task.Task{}, false
	}
	return items[i], true
}

// clampSelection keeps every column's cursor inside its item range.
func (m *BoardModel) clampSelection() {
	for i, s := range task.Statuses {
		n := len(m.board.ByStatus(s))
		if m.sel[i] >= n {
			m.sel[i] = n - 1
		}
		if m.sel[i] < 0 {
			m.sel[i] = 0
		}
	}
}

// columnWidth splits the terminal evenly across the four columns, leaving
// room for each column's border and padding.
func (m BoardModel) columnWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	per := w/len(task.Statuses) - 2
	if per < 12 {
		per = 12
	}
	return per
}

func (m BoardModel) columnHeight() int {
	h := m.height - 6 // header + footer + borders
	if h < 5 {
		h = 5
	}
	return h
}

// View renders the four columns side by side plus the footer line.
func (m BoardModel) View() string {
	cw := m.columnWidth()
	cols := make([]string, 0, len(task.Statuses))
	for i, s := range task.Statuses {
		cols = append(cols, m.renderColumn(i, s, cw))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.renderFooter())
}

func (m BoardModel) renderColumn(idx int, s task.Status, width int) string {
	items := m.board.ByStatus(s)

	header := HeaderStyle(s).Render(s.Label()) +
		MutedStyle.Render(" ("+strconv.Itoa(len(items))+")")

	var lines []string
	lines = append(lines, header, "")

	if len(items) == 0 {
		lines = append(lines, MutedStyle.Render("empty"))
	}
	for i, t := range items {
		lines = append(lines, m.renderCard(idx, i, t, width))
	}

	style := ColumnStyle
	if idx == m.col {
		style = ColumnFocusedStyle
		if m.focus == focusColumn {
			style = style.Copy().BorderForeground(AccentFor(s))
		}
	}
	return style.Width(width).Height(m.columnHeight()).
		Render(strings.Join(lines, "\n"))
}

func (m BoardModel) renderCard(colIdx, itemIdx int, t task.Task, width int) string {
	title := truncate(t.Title, width-4)
	selected := colIdx == m.col && itemIdx == m.sel[m.col]

	switch {
	case selected && m.mode == modeMove && t.ID == m.grabID:
		return CardGrabbedStyle.Render("⇄ " + title)
	case selected && m.focus == focusItem:
		return CardSelectedStyle.Render(title)
	default:
		return CardStyle.Render(title)
	}
}

func (m BoardModel) renderFooter() string {
	switch m.mode {
	case modeInput:
		return "\n" + m.input.View()
	case modeMove:
		return HelpStyle.Render("move: h/l reposition · enter drop · esc cancel")
	case modeConfirm:
		return HelpStyle.Render("delete this task? y / n")
	}
	if m.err != "" {
		return HelpStyle.Render("! " + m.err)
	}
	focusLabel := "item"
	if m.focus == focusColumn {
		focusLabel = "column"
	}
	return HelpStyle.Render(
		"focus: " + focusLabel + " (ctrl+t) · hjkl move · a add · e edit · d delete · m grab · tab analytics · ? help · q quit")
}

func truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}
```

Imports for `internal/ui/board.go` at this point: `strconv`, `strings`, `github.com/charmbracelet/bubbles/textinput`, `github.com/charmbracelet/lipgloss`, `gotodo/internal/task`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all ui tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/board.go internal/ui/board_test.go
git commit -m "feat: render kanban board columns and cards"
```

---

## Task 8: Board page — navigation keys and focus modes

**Files:**
- Modify: `internal/ui/board.go` (append the `Update` method and helpers)
- Modify: `internal/ui/board_test.go` (append)

**Interfaces:**
- Consumes: everything from Task 7.
- Produces: `func (m BoardModel) Update(msg tea.Msg) (BoardModel, tea.Cmd)` handling navigation only (mutations land in Task 9); `func (m *BoardModel) moveColumn(delta int)`; `func (m *BoardModel) moveItem(delta int)`.

Note: `Update` returns `BoardModel`, not `tea.Model` — the root model in Task 11 owns it as a struct field, so a concrete return type avoids a type assertion on every keypress.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/board_test.go`:

```go
func key(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "ctrl+t":
		return tea.KeyMsg{Type: tea.KeyCtrlT}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	panic("unhandled key in test helper: " + s)
}

// press feeds a sequence of keys through Update.
func press(m BoardModel, keys ...string) BoardModel {
	for _, k := range keys {
		m, _ = m.Update(key(k))
	}
	return m
}

func TestCtrlTTogglesFocus(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "ctrl+t")
	if m.focus != focusColumn {
		t.Fatalf("focus = %v, want focusColumn", m.focus)
	}
	m = press(m, "ctrl+t")
	if m.focus != focusItem {
		t.Errorf("focus = %v, want focusItem", m.focus)
	}
}

func TestHLMoveBetweenColumns(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "l", "l")
	if m.col != 2 {
		t.Errorf("col = %d, want 2", m.col)
	}
	m = press(m, "h")
	if m.col != 1 {
		t.Errorf("col = %d, want 1", m.col)
	}
}

func TestColumnNavigationClampsAtEdges(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "h", "h")
	if m.col != 0 {
		t.Errorf("col = %d, want 0 (clamped at the left edge)", m.col)
	}
	m = press(m, "l", "l", "l", "l", "l")
	if m.col != len(task.Statuses)-1 {
		t.Errorf("col = %d, want %d (clamped at the right edge)", m.col, len(task.Statuses)-1)
	}
}

func TestJKMoveBetweenItemsInItemFocus(t *testing.T) {
	b := &task.Board{}
	b.Add("[One]", ref)
	b.Add("[Two]", ref)
	b.Add("[Three]", ref)
	m := press(NewBoardModel(b), "j", "j")
	if m.sel[0] != 2 {
		t.Errorf("sel[0] = %d, want 2", m.sel[0])
	}
	m = press(m, "j") // clamp at the bottom
	if m.sel[0] != 2 {
		t.Errorf("sel[0] = %d, want 2 (clamped at the last item)", m.sel[0])
	}
	m = press(m, "k", "k", "k")
	if m.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (clamped at the first item)", m.sel[0])
	}
}

func TestJKDoNotMoveItemsInColumnFocus(t *testing.T) {
	b := &task.Board{}
	b.Add("[One]", ref)
	b.Add("[Two]", ref)
	m := press(NewBoardModel(b), "ctrl+t", "j")
	if m.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (column focus must not move the item cursor)", m.sel[0])
	}
}

func TestGAndShiftGJumpToEnds(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 4; i++ {
		b.Add("[Task title]", ref)
	}
	m := press(NewBoardModel(b), "G")
	if m.sel[0] != 3 {
		t.Errorf("sel[0] after G = %d, want 3", m.sel[0])
	}
	m = press(m, "g")
	if m.sel[0] != 0 {
		t.Errorf("sel[0] after g = %d, want 0", m.sel[0])
	}
}

func TestArrowKeysMirrorHJKL(t *testing.T) {
	m := press(NewBoardModel(seeded(t)), "right", "right")
	if m.col != 2 {
		t.Errorf("col = %d, want 2", m.col)
	}
}
```

Add `tea "github.com/charmbracelet/bubbletea"` to the test file's imports.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestCtrlT|TestHL|TestColumnNav|TestJK|TestGAnd|TestArrow' -v`
Expected: FAIL — `m.Update undefined`.

- [ ] **Step 3: Write the implementation**

Append to `internal/ui/board.go`:

```go
// Update handles board-page keys. Mode-specific keys (input, move, confirm)
// are added in the next task; this handles Normal-mode navigation.
func (m BoardModel) Update(msg tea.Msg) (BoardModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.err = ""

	if keyMsg.Type == tea.KeyCtrlT {
		if m.focus == focusItem {
			m.focus = focusColumn
		} else {
			m.focus = focusItem
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "h", "left":
		m.moveColumn(-1)
	case "l", "right":
		m.moveColumn(1)
	case "j", "down":
		if m.focus == focusItem {
			m.moveItem(1)
		}
	case "k", "up":
		if m.focus == focusItem {
			m.moveItem(-1)
		}
	case "g":
		if m.focus == focusItem {
			m.sel[m.col] = 0
		}
	case "G":
		if m.focus == focusItem {
			m.sel[m.col] = len(m.board.ByStatus(m.currentStatus())) - 1
		}
	}
	m.clampSelection()
	return m, nil
}

// moveColumn shifts the focused column, clamped at both edges.
func (m *BoardModel) moveColumn(delta int) {
	next := m.col + delta
	if next < 0 {
		next = 0
	}
	if next >= len(task.Statuses) {
		next = len(task.Statuses) - 1
	}
	m.col = next
}

// moveItem shifts the cursor within the focused column, clamped at both ends.
func (m *BoardModel) moveItem(delta int) {
	n := len(m.board.ByStatus(m.currentStatus()))
	if n == 0 {
		m.sel[m.col] = 0
		return
	}
	next := m.sel[m.col] + delta
	if next < 0 {
		next = 0
	}
	if next >= n {
		next = n - 1
	}
	m.sel[m.col] = next
}
```

Add `tea "github.com/charmbracelet/bubbletea"` to the imports in `internal/ui/board.go`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all ui tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/board.go internal/ui/board_test.go
git commit -m "feat: add hjkl navigation and ctrl+t focus toggle"
```

---

## Task 9: Board page — add, edit, delete, and grab-to-move

**Files:**
- Modify: `internal/ui/board.go` (extend `Update`, add mode handlers)
- Modify: `internal/ui/board_test.go` (append)

**Interfaces:**
- Consumes: everything from Task 8, plus `task.Board.Add/Edit/Delete/Move`.
- Produces:
  - `type dirtyMsg struct{}` — emitted as a `tea.Cmd` whenever the board mutates; the root model in Task 11 saves on it.
  - `func (m *BoardModel) Now() time.Time` — returns `m.now()`, an injectable clock.
  - `BoardModel.now func() time.Time` field, defaulting to `time.Now` in `NewBoardModel`, overridden in tests.
  - Extended `Update` handling `modeInput`, `modeMove`, `modeConfirm` and the `a` / `e` / `d` / `m` entry keys.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/board_test.go`:

```go
// fixedClock pins the model's clock so transitions are deterministic.
func fixedClock(m BoardModel) BoardModel {
	m.now = func() time.Time { return ref }
	return m
}

func TestAddOpensInputAndCommitsOnEnter(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want modeInput", m.mode)
	}
	m = press(m, "b", "u", "y", " ", "m", "i", "l", "k")
	m = press(m, "enter")

	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal after enter", m.mode)
	}
	todo := m.board.ByStatus(task.StatusTodo)
	if len(todo) != 1 {
		t.Fatalf("todo tasks = %d, want 1", len(todo))
	}
	if todo[0].Title != "buy milk" {
		t.Errorf("Title = %q, want %q", todo[0].Title, "buy milk")
	}
}

func TestAddEscCancels(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a", "x", "esc")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 after cancel", len(m.board.Tasks))
	}
}

func TestAddBlankTitleIsRejected(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a", " ", "enter")
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 for a blank title", len(m.board.Tasks))
	}
	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput (stay open on a blank title)", m.mode)
	}
}

func TestEditReplacesTitle(t *testing.T) {
	b := &task.Board{}
	b.Add("[Old title]", ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "e")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want modeInput", m.mode)
	}
	if m.input.Value() != "[Old title]" {
		t.Errorf("input prefilled with %q, want the existing title", m.input.Value())
	}
	// clear then type
	m.input.SetValue("new")
	m = press(m, "enter")
	if m.board.Tasks[0].Title != "new" {
		t.Errorf("Title = %q, want %q", m.board.Tasks[0].Title, "new")
	}
}

func TestDeleteAsksThenRemoves(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "d")
	if m.mode != modeConfirm {
		t.Fatalf("mode = %v, want modeConfirm", m.mode)
	}
	if len(m.board.Tasks) != 1 {
		t.Fatalf("Tasks = %d, want 1 before confirming", len(m.board.Tasks))
	}
	m = press(m, "y")
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 after confirming", len(m.board.Tasks))
	}
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
}

func TestDeleteCancelledByN(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "d", "n")
	if len(m.board.Tasks) != 1 {
		t.Errorf("Tasks = %d, want 1 after cancelling", len(m.board.Tasks))
	}
}

func TestGrabMoveAndDrop(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", ref)
	m := fixedClock(NewBoardModel(b))

	m = press(m, "m")
	if m.mode != modeMove {
		t.Fatalf("mode = %v, want modeMove", m.mode)
	}
	m = press(m, "l", "l") // todo -> doing -> blocked
	if got := m.board.Tasks[0].Status; got != task.StatusBlocked {
		t.Errorf("Status during move = %q, want blocked", got)
	}
	if m.col != 2 {
		t.Errorf("col = %d, want 2 (focus follows the grabbed task)", m.col)
	}
	m = press(m, "enter")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal after drop", m.mode)
	}
	if got := m.board.Tasks[0].Status; got != task.StatusBlocked {
		t.Errorf("Status after drop = %q, want blocked", got)
	}
}

func TestGrabEscRestoresOriginalColumn(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "m", "l", "l", "esc")
	if got := m.board.Tasks[0].Status; got != task.StatusTodo {
		t.Errorf("Status after esc = %q, want todo", got)
	}
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
}

func TestMutationEmitsDirtyCmd(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a", "x")
	m, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("commit returned a nil cmd, want a dirtyMsg cmd")
	}
	if _, ok := cmd().(dirtyMsg); !ok {
		t.Errorf("cmd produced %T, want dirtyMsg", cmd())
	}
	_ = m
}

func TestMoveKeysIgnoredOnEmptyColumn(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "m")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (nothing to grab)", m.mode)
	}
	m = press(m, "e")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (nothing to edit)", m.mode)
	}
	m = press(m, "d")
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal (nothing to delete)", m.mode)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestAdd|TestEdit|TestDelete|TestGrab|TestMutation|TestMoveKeys' -v`
Expected: FAIL — `m.now undefined` and `undefined: dirtyMsg`.

- [ ] **Step 3: Add the clock field and dirty message**

In `internal/ui/board.go`, add `now func() time.Time` to the `BoardModel` struct and set it in `NewBoardModel`:

```go
type BoardModel struct {
	board *task.Board

	col   int
	sel   [4]int
	focus focusMode
	mode  boardMode

	input    textinput.Model
	editID   string
	grabID   string
	grabFrom task.Status

	now func() time.Time // injectable clock; tests pin it

	width  int
	height int
	err    string
}
```

```go
func NewBoardModel(b *task.Board) BoardModel {
	in := textinput.New()
	in.Placeholder = "task title"
	in.CharLimit = 200
	in.Prompt = "› "
	return BoardModel{board: b, focus: focusItem, mode: modeNormal, input: in, now: time.Now}
}
```

Add `"time"` to the imports.

- [ ] **Step 4: Replace Update with the full mode-aware version**

Replace the `Update` method written in Task 8 with:

```go
// dirtyMsg signals that the board changed and should be persisted.
type dirtyMsg struct{}

func dirty() tea.Cmd { return func() tea.Msg { return dirtyMsg{} } }

// Update handles every board-page key, dispatching on the current mode.
func (m BoardModel) Update(msg tea.Msg) (BoardModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch m.mode {
	case modeInput:
		return m.updateInput(keyMsg)
	case modeMove:
		return m.updateMove(keyMsg)
	case modeConfirm:
		return m.updateConfirm(keyMsg)
	}
	return m.updateNormal(keyMsg)
}

func (m BoardModel) updateNormal(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.err = ""

	if k.Type == tea.KeyCtrlT {
		if m.focus == focusItem {
			m.focus = focusColumn
		} else {
			m.focus = focusItem
		}
		return m, nil
	}

	switch k.String() {
	case "h", "left":
		m.moveColumn(-1)
	case "l", "right":
		m.moveColumn(1)
	case "j", "down":
		if m.focus == focusItem {
			m.moveItem(1)
		}
	case "k", "up":
		if m.focus == focusItem {
			m.moveItem(-1)
		}
	case "g":
		if m.focus == focusItem {
			m.sel[m.col] = 0
		}
	case "G":
		if m.focus == focusItem {
			m.sel[m.col] = len(m.board.ByStatus(m.currentStatus())) - 1
		}
	case "a":
		m.mode = modeInput
		m.editID = ""
		m.input.SetValue("")
		m.input.Focus()
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.mode = modeInput
		m.editID = t.ID
		m.input.SetValue(t.Title)
		m.input.CursorEnd()
		m.input.Focus()
	case "d":
		if _, ok := m.selectedTask(); !ok {
			return m, nil
		}
		m.mode = modeConfirm
	case "m":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.mode = modeMove
		m.focus = focusItem
		m.grabID = t.ID
		m.grabFrom = t.Status
	}
	m.clampSelection()
	return m, nil
}

func (m BoardModel) updateInput(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		m.mode = modeNormal
		m.editID = ""
		m.input.Blur()
		return m, nil

	case tea.KeyEnter:
		title := strings.TrimSpace(m.input.Value())
		if title == "" {
			m.err = "title must not be blank"
			return m, nil // stay in input mode
		}
		if m.editID != "" {
			if err := m.board.Edit(m.editID, title, m.now()); err != nil {
				m.err = err.Error()
			}
		} else {
			m.board.Add(title, m.now())
			m.col = 0
			m.sel[0] = len(m.board.ByStatus(task.StatusTodo)) - 1
		}
		m.mode = modeNormal
		m.editID = ""
		m.input.Blur()
		m.clampSelection()
		return m, dirty()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

func (m BoardModel) updateMove(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.String() {
	case "h", "left":
		m.shiftGrabbed(-1)
	case "l", "right":
		m.shiftGrabbed(1)
	case "enter":
		m.mode = modeNormal
		m.grabID = ""
		m.clampSelection()
		return m, dirty()
	case "esc":
		if err := m.board.Move(m.grabID, m.grabFrom, m.now()); err != nil {
			m.err = err.Error()
		}
		m.col = m.grabFrom.Index()
		m.mode = modeNormal
		m.grabID = ""
		m.clampSelection()
		return m, dirty()
	}
	return m, nil
}

// shiftGrabbed moves the grabbed task one column over and follows it.
func (m *BoardModel) shiftGrabbed(delta int) {
	next := m.col + delta
	if next < 0 || next >= len(task.Statuses) {
		return
	}
	if err := m.board.Move(m.grabID, task.Statuses[next], m.now()); err != nil {
		m.err = err.Error()
		return
	}
	m.col = next
	m.sel[m.col] = len(m.board.ByStatus(task.Statuses[next])) - 1
	m.clampSelection()
}

func (m BoardModel) updateConfirm(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.mode = modeNormal
	if k.String() != "y" {
		return m, nil
	}
	t, ok := m.selectedTask()
	if !ok {
		return m, nil
	}
	if err := m.board.Delete(t.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.clampSelection()
	return m, dirty()
}
```

Note for the implementer: the esc-restore path calls `Move` back to `grabFrom`, which appends a second transition. That is intentional — the history is a factual log of what happened, including a mistaken grab.

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all ui tests. If `TestAddOpensInputAndCommitsOnEnter` fails with an empty title, check that `updateInput` forwards rune keys to `m.input.Update` — the textinput must be `Focus()`ed for it to accept runes.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/board.go internal/ui/board_test.go
git commit -m "feat: add task CRUD and grab-to-move on the board"
```

---

## Task 10: Analytics page

**Files:**
- Create: `internal/ui/analytics.go`
- Create: `internal/ui/analytics_test.go`

**Interfaces:**
- Consumes: `task.Board`, all of `internal/stats`, theme + charts from Task 6.
- Produces: `type AnalyticsModel struct { board *task.Board; now func() time.Time; width, height int }`; `func NewAnalyticsModel(b *task.Board) AnalyticsModel`; `func (m *AnalyticsModel) SetSize(w, h int)`; `func (m AnalyticsModel) View() string`.

Layout, top to bottom:
1. Row of four stat tiles (one per column) with the count and a coloured label.
2. **Throughput** — 14-day sparkline plus "N done in the last 14 days", with the first and last day labelled in `DD/MM/YYYY`.
3. **Cycle time** — mean and median created→done, plus an `HBar` per column showing mean time spent there.
4. **Blocked** — count currently blocked and the top 5 longest-blocked titles with durations.
5. **Streak** — current and longest, plus a 12-week heatmap.

The analytics page has no keys of its own; Tab and `q` are handled by the root model, so it needs no `Update`.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/analytics_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

func analyticsBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	// one done today, one done three days ago, one blocked for 9 hours
	a := b.Add("[Done today]", ref.Add(-30*time.Hour))
	if err := b.Move(a.ID, task.StatusDone, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	c := b.Add("[Done earlier]", ref.Add(-96*time.Hour))
	if err := b.Move(c.ID, task.StatusDone, ref.Add(-72*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	d := b.Add("[Stuck]", ref.Add(-20*time.Hour))
	if err := b.Move(d.ID, task.StatusBlocked, ref.Add(-9*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	b.Add("[Fresh]", ref.Add(-time.Hour))
	return b
}

func fixedAnalytics(b *task.Board) AnalyticsModel {
	m := NewAnalyticsModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(100, 40)
	return m
}

func TestAnalyticsViewHasAllSections(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	for _, want := range []string{"THROUGHPUT", "CYCLE TIME", "BLOCKED", "STREAK"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing section %q:\n%s", want, out)
		}
	}
}

func TestAnalyticsViewShowsColumnCounts(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("View missing the %q tile:\n%s", s.Label(), out)
		}
	}
}

func TestAnalyticsViewUsesSingaporeDates(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	if !strings.Contains(out, "30/07/2026") {
		t.Errorf("View missing the DD/MM/YYYY end date:\n%s", out)
	}
}

func TestAnalyticsViewListsBlockedTask(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	if !strings.Contains(out, "Stuck") {
		t.Errorf("View missing the blocked task title:\n%s", out)
	}
	if !strings.Contains(out, "9h") {
		t.Errorf("View missing the blocked duration '9h':\n%s", out)
	}
}

func TestAnalyticsViewEmptyBoardDoesNotPanic(t *testing.T) {
	out := fixedAnalytics(&task.Board{}).View()
	if out == "" {
		t.Error("View of an empty board returned an empty string")
	}
	if !strings.Contains(out, "THROUGHPUT") {
		t.Errorf("View of an empty board is missing its sections:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run TestAnalytics -v`
Expected: FAIL — `undefined: NewAnalyticsModel`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/analytics.go`:

```go
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/stats"
	"gotodo/internal/task"
)

const (
	throughputDays = 14
	heatmapWeeks   = 12
	blockedTopN    = 5
)

// AnalyticsModel is the stats page. It has no keys of its own.
type AnalyticsModel struct {
	board *task.Board
	now   func() time.Time

	width  int
	height int
}

// NewAnalyticsModel wires a board into the analytics page.
func NewAnalyticsModel(b *task.Board) AnalyticsModel {
	return AnalyticsModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *AnalyticsModel) SetSize(w, h int) { m.width, m.height = w, h }

// View renders the whole analytics page.
func (m AnalyticsModel) View() string {
	now := m.now()
	tasks := m.board.Tasks

	sections := []string{
		m.renderTiles(tasks),
		m.renderThroughput(tasks, now),
		m.renderCycle(tasks, now),
		m.renderBlocked(tasks, now),
		m.renderStreak(tasks, now),
	}
	return strings.Join(sections, "\n\n")
}

func (m AnalyticsModel) renderTiles(tasks []task.Task) string {
	counts := stats.Counts(tasks)
	tiles := make([]string, 0, len(task.Statuses))
	for _, s := range task.Statuses {
		body := lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(AccentFor(s)).
				Render(fmt.Sprintf("%d", counts[s])),
			MutedStyle.Render(s.Label()),
		)
		tiles = append(tiles, StatTileStyle.Render(body))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tiles...)
}

func (m AnalyticsModel) renderThroughput(tasks []task.Task, now time.Time) string {
	series := stats.Throughput(tasks, throughputDays, now)
	values := make([]int, len(series))
	total := 0
	for i, d := range series {
		values[i] = d.N
		total += d.N
	}

	line := lipgloss.NewStyle().Foreground(AccentFor(task.StatusDone)).
		Render(Sparkline(values))

	span := ""
	if len(series) > 0 {
		span = fmt.Sprintf("%s → %s",
			FormatDate(series[0].Day), FormatDate(series[len(series)-1].Day))
	}

	return strings.Join([]string{
		TitleStyle.Render("THROUGHPUT"),
		line,
		MutedStyle.Render(fmt.Sprintf("%d completed over %d days · %s",
			total, throughputDays, span)),
	}, "\n")
}

func (m AnalyticsModel) renderCycle(tasks []task.Task, now time.Time) string {
	c := stats.CycleTimes(tasks, now)
	lines := []string{TitleStyle.Render("CYCLE TIME")}

	if c.N == 0 {
		return strings.Join(append(lines, MutedStyle.Render("no completed tasks yet")), "\n")
	}

	lines = append(lines, fmt.Sprintf("mean %s · median %s · over %d completed",
		FormatDuration(c.Mean), FormatDuration(c.Median), c.N))
	lines = append(lines, MutedStyle.Render("mean time spent per column:"))

	maxMinutes := 0
	for _, s := range task.Statuses {
		if v := int(c.PerColumn[s].Minutes()); v > maxMinutes {
			maxMinutes = v
		}
	}
	for _, s := range task.Statuses {
		d := c.PerColumn[s]
		bar := HBar(strings.ToLower(s.Label()), int(d.Minutes()), maxMinutes, 24)
		// Replace the raw minute count with a human duration.
		bar = strings.TrimSuffix(bar, fmt.Sprintf(" %d", int(d.Minutes())))
		lines = append(lines, lipgloss.NewStyle().Foreground(AccentFor(s)).Render(bar)+
			" "+MutedStyle.Render(FormatDuration(d)))
	}
	return strings.Join(lines, "\n")
}

func (m AnalyticsModel) renderBlocked(tasks []task.Task, now time.Time) string {
	items := stats.BlockedReport(tasks, now)
	lines := []string{TitleStyle.Render("BLOCKED")}

	if len(items) == 0 {
		return strings.Join(append(lines, MutedStyle.Render("nothing is blocked")), "\n")
	}
	lines = append(lines, fmt.Sprintf("%d currently blocked", len(items)))

	n := len(items)
	if n > blockedTopN {
		n = blockedTopN
	}
	for _, it := range items[:n] {
		lines = append(lines, fmt.Sprintf("  %-40s %s",
			truncate(it.Title, 40),
			lipgloss.NewStyle().Foreground(AccentFor(task.StatusBlocked)).
				Render(FormatDuration(it.For))))
	}
	if len(items) > n {
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("  … and %d more", len(items)-n)))
	}
	return strings.Join(lines, "\n")
}

func (m AnalyticsModel) renderStreak(tasks []task.Task, now time.Time) string {
	current, longest := stats.Streak(tasks, now)
	grid := stats.HeatmapGrid(tasks, heatmapWeeks, now)

	heat := lipgloss.NewStyle().Foreground(AccentFor(task.StatusDone)).Render(Heatmap(grid))
	labels := []string{"Mon", "   ", "Wed", "   ", "Fri", "   ", "Sun"}
	rows := strings.Split(heat, "\n")
	for i := range rows {
		if i < len(labels) {
			rows[i] = MutedStyle.Render(labels[i]) + " " + rows[i]
		}
	}

	return strings.Join([]string{
		TitleStyle.Render("STREAK"),
		fmt.Sprintf("current %d days · longest %d days", current, longest),
		strings.Join(rows, "\n"),
		MutedStyle.Render(fmt.Sprintf("last %d weeks, ending %s", heatmapWeeks, FormatDate(now))),
	}, "\n")
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all ui tests. If `TestAnalyticsViewListsBlockedTask` fails on `"9h"`, verify `FormatDuration(9 * time.Hour) == "9h"`.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/analytics.go internal/ui/analytics_test.go
git commit -m "feat: add analytics page with throughput, cycle time, blocked and streak"
```

---

## Task 11: Root model, help overlay, persistence, and main

**Files:**
- Create: `internal/ui/app.go`
- Create: `internal/ui/app_test.go`
- Create: `main.go`
- Create: `README.md`

**Interfaces:**
- Consumes: `BoardModel`, `AnalyticsModel`, `dirtyMsg`, `task.Board`, `task.Load`, `task.DefaultPath`.
- Produces: `type page int` with `pageBoard`, `pageAnalytics`; `type AppModel struct{...}`; `func NewApp(b *task.Board) AppModel`; `func (m AppModel) Init() tea.Cmd`; `func (m AppModel) Update(tea.Msg) (tea.Model, tea.Cmd)`; `func (m AppModel) View() string`.

Routing rule: the root model intercepts `tab`, `?`, `q`, `ctrl+c`, and `tea.WindowSizeMsg`. Everything else goes to the active page. Crucially, `q` and `?` must **not** be intercepted while the board is in `modeInput` — otherwise the user cannot type the letter q in a task title.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/app_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
)

func app(t *testing.T) AppModel {
	t.Helper()
	a := NewApp(seeded(t))
	a.board.now = fixedClock(a.board).now
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m.(AppModel)
}

func TestTabSwitchesPages(t *testing.T) {
	a := app(t)
	if a.page != pageBoard {
		t.Fatalf("page = %v, want pageBoard", a.page)
	}
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.page != pageAnalytics {
		t.Fatalf("page = %v, want pageAnalytics", a.page)
	}
	if !strings.Contains(a.View(), "THROUGHPUT") {
		t.Errorf("analytics page view missing THROUGHPUT:\n%s", a.View())
	}

	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.page != pageBoard {
		t.Errorf("page = %v, want pageBoard after a second tab", a.page)
	}
}

func TestQuestionMarkTogglesHelp(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	if !a.showHelp {
		t.Fatal("showHelp = false, want true")
	}
	if !strings.Contains(a.View(), "ctrl+t") {
		t.Errorf("help overlay missing the ctrl+t binding:\n%s", a.View())
	}
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if m.(AppModel).showHelp {
		t.Error("showHelp = true, want false after a second ?")
	}
}

func TestQQuits(t *testing.T) {
	a := app(t)
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q returned a nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q produced %T, want tea.QuitMsg", cmd())
	}
}

func TestQIsTypableWhileAddingATask(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	a = m.(AppModel)
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	a = m.(AppModel)
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("q quit the app while the input was open")
		}
	}
	if a.board.input.Value() != "q" {
		t.Errorf("input value = %q, want %q", a.board.input.Value(), "q")
	}
}

func TestTabIgnoredWhileAddingATask(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	a = m.(AppModel)
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.(AppModel).page != pageBoard {
		t.Error("tab switched pages while the input was open, want it ignored")
	}
}

func TestDirtyMsgTriggersSave(t *testing.T) {
	b := &task.Board{}
	dir := t.TempDir()
	b.SetPath(dir + "/tasks.json")
	b.Add("[Task title]", ref)

	a := NewApp(b)
	if _, cmd := a.Update(dirtyMsg{}); cmd != nil {
		cmd() // drain any follow-up
	}
	if _, err := task.Load(dir + "/tasks.json"); err != nil {
		t.Fatalf("Load after dirtyMsg returned %v", err)
	}
	reloaded, _ := task.Load(dir + "/tasks.json")
	if len(reloaded.Tasks) != 1 {
		t.Errorf("saved tasks = %d, want 1", len(reloaded.Tasks))
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestTab|TestQuestion|TestQ|TestDirty' -v`
Expected: FAIL — `undefined: NewApp`.

- [ ] **Step 3: Write the root model**

Create `internal/ui/app.go`:

```go
package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

type page int

const (
	pageBoard page = iota
	pageAnalytics
)

// AppModel is the root Bubble Tea model: it owns page switching, the help
// overlay, and persistence.
type AppModel struct {
	board     BoardModel
	analytics AnalyticsModel
	store     *task.Board

	page     page
	showHelp bool
	saveErr  string

	width  int
	height int
}

// NewApp builds the root model around a loaded board.
func NewApp(b *task.Board) AppModel {
	return AppModel{
		board:     NewBoardModel(b),
		analytics: NewAnalyticsModel(b),
		store:     b,
	}
}

// Init satisfies tea.Model; nothing to do at startup.
func (m AppModel) Init() tea.Cmd { return nil }

// Update routes global keys itself and forwards the rest to the active page.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.board.SetSize(msg.Width, msg.Height)
		m.analytics.SetSize(msg.Width, msg.Height)
		return m, nil

	case dirtyMsg:
		if err := m.store.Save(); err != nil {
			m.saveErr = err.Error()
		} else {
			m.saveErr = ""
		}
		return m, nil

	case tea.KeyMsg:
		// While the board is capturing text, only ctrl+c is global —
		// everything else, including q and tab, belongs to the input.
		typing := m.page == pageBoard && m.board.mode == modeInput
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if !typing {
			switch msg.String() {
			case "tab":
				if m.page == pageBoard {
					m.page = pageAnalytics
				} else {
					m.page = pageBoard
				}
				return m, nil
			case "?":
				m.showHelp = !m.showHelp
				return m, nil
			case "q":
				return m, tea.Quit
			}
			if m.showHelp {
				m.showHelp = false // any other key dismisses the overlay
				return m, nil
			}
		}
		if m.page == pageBoard {
			var cmd tea.Cmd
			m.board, cmd = m.board.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

// View renders the tab bar plus the active page, or the help overlay.
func (m AppModel) View() string {
	if m.showHelp {
		return m.renderHelp()
	}
	body := m.board.View()
	if m.page == pageAnalytics {
		body = m.analytics.View()
	}
	parts := []string{m.renderTabs(), body}
	if m.saveErr != "" {
		parts = append(parts, HelpStyle.Render("save failed: "+m.saveErr))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m AppModel) renderTabs() string {
	active := lipgloss.NewStyle().Bold(true).Foreground(ColAccent).Padding(0, 2)
	inactive := MutedStyle.Copy().Padding(0, 2)

	board, analytics := active.Render("BOARD"), inactive.Render("ANALYTICS")
	if m.page == pageAnalytics {
		board, analytics = inactive.Render("BOARD"), active.Render("ANALYTICS")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, board, analytics,
		MutedStyle.Render("  tab to switch"))
}

func (m AppModel) renderHelp() string {
	rows := [][2]string{
		{"tab", "switch board ↔ analytics"},
		{"ctrl+t", "toggle column / item focus"},
		{"h l", "previous / next column"},
		{"j k", "previous / next task (item focus)"},
		{"g G", "first / last task in column"},
		{"a", "add a task"},
		{"e", "edit the selected task"},
		{"d", "delete the selected task (confirms)"},
		{"m", "grab the task, then h/l to move, enter to drop, esc to cancel"},
		{"?", "toggle this help"},
		{"q", "quit"},
	}
	lines := []string{TitleStyle.Render("KEYS"), ""}
	for _, r := range rows {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(ColAccent).Width(10).Render(r[0])+
				MutedStyle.Render(r[1]))
	}
	lines = append(lines, "", MutedStyle.Render("any key to close"))
	return ColumnStyle.Render(strings.Join(lines, "\n"))
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all ui tests. If `TestQIsTypableWhileAddingATask` fails, check the `typing` guard runs before the `q` case.

- [ ] **Step 5: Write main.go**

Create `main.go`:

```go
// Command gotodo is a keyboard-driven terminal kanban board.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
	"gotodo/internal/ui"
)

func main() {
	defaultPath, err := task.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotodo:", err)
		os.Exit(1)
	}
	path := flag.String("file", defaultPath, "path to the tasks JSON file")
	flag.Parse()

	board, err := task.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotodo:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(ui.NewApp(board), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gotodo:", err)
		os.Exit(1)
	}
	// Final save covers any state the dirty path missed.
	if err := board.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "gotodo: save failed:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Build and smoke-test by hand**

```bash
go vet ./...
go build -o gotodo .
./gotodo -file /tmp/gotodo-smoke.json
```

Manual checklist inside the app:
1. Press `a`, type `first task`, press `enter` — a card appears under TODO.
2. Press `m`, then `l` twice — the card follows into BLOCKED, highlighted in amber.
3. Press `enter` to drop it.
4. Press `ctrl+t` — the whole column highlights instead of one card. Press `ctrl+t` again.
5. Press `tab` — the analytics page renders with four tiles and four sections, no panic.
6. Press `tab` back, press `?`, confirm the key list, press any key to dismiss.
7. Press `q`. Re-run `./gotodo -file /tmp/gotodo-smoke.json` — the task is still in BLOCKED.

If any step fails, fix it before committing.

- [ ] **Step 7: Write the README**

Create `README.md`:

```markdown
# gotodo

A keyboard-driven terminal kanban board with a built-in analytics page.

## Install

```bash
go build -o gotodo .
```

## Run

```bash
./gotodo                        # uses ~/.config/gotodo/tasks.json
./gotodo -file ./mytasks.json   # or point it anywhere
```

## Keys

| Key | Action |
|---|---|
| `tab` | switch board ↔ analytics |
| `ctrl+t` | toggle column / item focus |
| `h` `l` | previous / next column |
| `j` `k` | previous / next task (item focus) |
| `g` `G` | first / last task in column |
| `a` | add a task |
| `e` | edit the selected task |
| `d` | delete the selected task (confirms with `y`) |
| `m` | grab a task; `h`/`l` to move it, `enter` to drop, `esc` to cancel |
| `?` | toggle help |
| `q` | quit |

## Storage

One JSON file, written atomically after every change. Every status change is
appended to the task's history, which is what the analytics page reads.

## Analytics

- Task counts per column
- 14-day completion throughput sparkline
- Cycle time: mean and median created→done, plus mean time per column
- Blocked report: how long each blocked task has been stuck
- Completion streak and a 12-week heatmap

Dates are shown as `DD/MM/YYYY`.
```

- [ ] **Step 8: Run the full test suite**

Run: `go test ./... && go vet ./...`
Expected: `ok` for all three packages, no vet warnings.

- [ ] **Step 9: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go main.go README.md
git commit -m "feat: wire root model, help overlay, persistence and CLI entrypoint"
```

---

## Done Criteria

- `go test ./...` passes for `internal/task`, `internal/stats`, `internal/ui`.
- `go vet ./...` is clean.
- `go build -o gotodo .` produces a working binary.
- The manual checklist in Task 11 Step 6 passes end to end.
- Every keybinding in the Keybinding Contract works as specified.
- Tasks survive a quit-and-restart.

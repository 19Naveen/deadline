# Task Template and Archive Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every task a title, a description and an optional deadline; render all three on the card with the deadline colour-coded by urgency; and add a third Archive page that done tasks fall into 14 days after completion.

**Architecture:** Three additive fields on `task.Task` (`Description`, `Deadline *time.Time`, `Archived`/`ArchivedAt`) keep old `tasks.json` files loading unchanged. Deadline urgency is a pure domain function in `internal/task` taking an explicit `now`, so the renderer only picks a colour. Archiving is a boolean sweep — not a fifth column — so the four-column layout and the `sel [4]int` cursor array are untouched. The add/edit prompt grows from one `textinput` to three in a small form.

**Tech Stack:** Go 1.22, bubbletea v0.25.0, lipgloss v0.9.1, bubbles v0.18.0. No new dependencies.

## Global Constraints

- Module `gotodo`, Go 1.22. Exactly three direct dependencies: `bubbletea` v0.25.0, `lipgloss` v0.9.1, `bubbles` v0.18.0. Adding a fourth is a defect.
- **All user-facing dates render as Singapore `DD/MM/YYYY`** (Go layout `"02/01/2006"`), via the existing `FormatDate`. Deadline *input* is parsed with the same layout.
- No PII in fixtures, comments, examples or the README — bracketed placeholders only (`[Task title]`, `[Ship the report]`).
- Every function in `internal/task` and `internal/stats` that depends on the current time takes `now time.Time` as a parameter. None may call `time.Now()` internally. The only `time.Now()` in non-test code lives in `main.go` and in the two UI models' injectable `now` fields.
- Every test uses the fixed reference time `time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)` (the package-level `ref`), never `time.Now()`.
- Old `tasks.json` files written before this change must load without error, with the new fields at their zero values. Never break that.
- Run `go test ./...`, `go vet ./...` and `gofmt -l .` before every commit. gofmt must print nothing.
- Commit with conventional prefixes (`feat:`, `fix:`, `test:`, `refactor:`).

## Frozen Decisions

**Card layout — three lines**, blank line between cards:

```
│ ▏Ship the Q3 report      │
│  Draft, review, send to  │
│  ● 02/08/2026            │
│                          │
│  Fix login redirect      │
│  Users bounce to /home   │
│  ● 28/07/2026 ✗          │
```

Description line is omitted entirely when the description is empty; the deadline line is omitted when there is no deadline. A card is therefore 1, 2 or 3 lines tall.

**Deadline colours** — by whole calendar days between today and the deadline day:

| Days remaining | Colour | Marker |
|---|---|---|
| more than 3 | green | `●` |
| 2 to 3 | yellow/amber | `●` |
| 0 or 1 (due today or tomorrow) | red | `●` |
| negative (deadline day has passed) | red | `● … ✗` |
| task is in `done` | muted grey | `●` |

A done task never shows urgency colour — the deadline is history at that point, not pressure.

**Add/edit form** — three fields, `tab`/`shift+tab` to move, `enter` saves from any field, `esc` cancels.

**Archive** — done tasks are swept into the archive 14 days after they entered done. Sweeps run at startup and hourly while running. The Archive page is **view-only**: no restore key. `tab` cycles Board → Analytics → Archive → Board.

**Analytics counts:** the four tiles count only non-archived tasks, so they agree with the board. Throughput, streak, heatmap, cycle time and blocked all keep counting archived tasks — archiving is a display concern and must not erase your history.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/task/task.go` | `Task` struct, `NewTask` | Modify — three new fields, new constructor signature |
| `internal/task/deadline.go` | `Urgency`, `DeadlineUrgency`, `DaysUntilDeadline` | **Create** |
| `internal/task/deadline_test.go` | tests for the above | **Create** |
| `internal/task/board.go` | `Add`/`Edit` signatures, `Active`, `ArchivedTasks`, `ByStatus` | Modify |
| `internal/task/archive.go` | `ArchiveAfter`, `(*Board).SweepArchive` | **Create** |
| `internal/task/archive_test.go` | tests for the sweep | **Create** |
| `internal/ui/theme.go` | `UrgencyColor`, `RenderDeadline` | Modify |
| `internal/ui/board.go` | three-line card, three-field form | Modify |
| `internal/ui/archive.go` | `ArchiveModel` — the third page | **Create** |
| `internal/ui/archive_test.go` | tests for the page | **Create** |
| `internal/ui/app.go` | third page, tab cycling, hourly tick | Modify |
| `internal/ui/analytics.go` | tiles count active tasks only | Modify |
| `main.go` | sweep at startup | Modify |
| `README.md` | document the fields, colours, Archive page | Modify |

---

## Task 1: Task model gains description, deadline and archive fields

**Files:**
- Modify: `internal/task/task.go`
- Modify: `internal/task/task_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `Task` with new fields `Description string`, `Deadline *time.Time`, `Archived bool`, `ArchivedAt *time.Time`; new signature `func NewTask(title, description string, deadline *time.Time, now time.Time) Task`.

`Deadline` is a pointer so "no deadline" is distinguishable from the zero time. Same for `ArchivedAt`.

- [ ] **Step 1: Write the failing test**

Append to `internal/task/task_test.go`:

```go
func TestNewTaskCarriesDescriptionAndDeadline(t *testing.T) {
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	got := NewTask("[Task title]", "[a short description]", &due, ref)

	if got.Description != "[a short description]" {
		t.Errorf("Description = %q, want %q", got.Description, "[a short description]")
	}
	if got.Deadline == nil {
		t.Fatal("Deadline is nil, want a value")
	}
	if !got.Deadline.Equal(due) {
		t.Errorf("Deadline = %v, want %v", *got.Deadline, due)
	}
	if got.Archived {
		t.Error("Archived = true, want false for a new task")
	}
	if got.ArchivedAt != nil {
		t.Errorf("ArchivedAt = %v, want nil", got.ArchivedAt)
	}
}

func TestNewTaskWithoutDeadline(t *testing.T) {
	got := NewTask("[Task title]", "", nil, ref)
	if got.Deadline != nil {
		t.Errorf("Deadline = %v, want nil", got.Deadline)
	}
	if got.Description != "" {
		t.Errorf("Description = %q, want empty", got.Description)
	}
}

func TestTaskJSONOmitsEmptyNewFields(t *testing.T) {
	data, err := json.Marshal(NewTask("[Task title]", "", nil, ref))
	if err != nil {
		t.Fatalf("Marshal returned %v", err)
	}
	for _, key := range []string{"description", "deadline", "archived", "archived_at"} {
		if strings.Contains(string(data), key) {
			t.Errorf("JSON contains %q for an empty field; want it omitted:\n%s", key, data)
		}
	}
}

func TestTaskJSONFromOlderVersionStillLoads(t *testing.T) {
	// A file written before deadlines existed must load with zero values.
	const old = `{"id":"abc","title":"[Task title]","status":"todo",
		"created_at":"2026-07-30T12:00:00Z","updated_at":"2026-07-30T12:00:00Z"}`
	var got Task
	if err := json.Unmarshal([]byte(old), &got); err != nil {
		t.Fatalf("Unmarshal returned %v", err)
	}
	if got.Title != "[Task title]" {
		t.Errorf("Title = %q, want %q", got.Title, "[Task title]")
	}
	if got.Deadline != nil || got.Description != "" || got.Archived {
		t.Errorf("new fields should be zero, got %+v", got)
	}
}
```

Add `"encoding/json"` and `"strings"` to the test file's imports.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/task/ -run 'TestNewTaskCarries|TestTaskJSON' -v`
Expected: FAIL — compile error, `NewTask` takes 2 args not 4, and `Description` is undefined.

- [ ] **Step 3: Update the struct and constructor**

In `internal/task/task.go`, replace the `Task` struct and `NewTask` with:

```go
// Task is one card on the board.
type Task struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Deadline    *time.Time   `json:"deadline,omitempty"`
	Status      Status       `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Archived    bool         `json:"archived,omitempty"`
	ArchivedAt  *time.Time   `json:"archived_at,omitempty"`
	History     []Transition `json:"history,omitempty"`
}

// NewTask builds a task in the todo column with a fresh random ID. A nil
// deadline means the task has none; description may be empty.
func NewTask(title, description string, deadline *time.Time, now time.Time) Task {
	return Task{
		ID:          newID(),
		Title:       title,
		Description: description,
		Deadline:    deadline,
		Status:      StatusTodo,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
```

- [ ] **Step 4: Fix every call site**

`go build ./...` will list them. The only non-test caller is `Board.Add` in `internal/task/board.go`; leave its own signature alone for now and pass through empty values so the tree compiles:

```go
b.Tasks = append(b.Tasks, NewTask(strings.TrimSpace(title), "", nil, now))
```

Task 3 changes `Add`'s signature properly. Do not touch it further here.

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./... -v`
Expected: PASS, all packages.

- [ ] **Step 6: Commit**

```bash
git add internal/task/task.go internal/task/task_test.go
git commit -m "feat: add description, deadline and archive fields to Task"
```

---

## Task 2: Deadline urgency

**Files:**
- Create: `internal/task/deadline.go`
- Create: `internal/task/deadline_test.go`

**Interfaces:**
- Consumes: `Task`, `StatusDone` from Task 1.
- Produces:
  - `type Urgency int` with `UrgencyNone`, `UrgencyDone`, `UrgencyFuture`, `UrgencySoon`, `UrgencyUrgent`, `UrgencyOverdue`
  - `func StartOfDay(t time.Time) time.Time`
  - `func DaysUntilDeadline(t Task, now time.Time) (int, bool)` — whole calendar days; `ok` false when no deadline
  - `func DeadlineUrgency(t Task, now time.Time) Urgency`

Bucketing is by **calendar day**, not elapsed hours — a deadline at 09:00 tomorrow and one at 23:00 tomorrow are both "1 day away". `time.Truncate` is wrong for this; it works on absolute time.

- [ ] **Step 1: Write the failing test**

Create `internal/task/deadline_test.go`:

```go
package task

import (
	"testing"
	"time"
)

// due builds a todo task whose deadline is the given day.
func due(day time.Time) Task {
	d := day
	return Task{Title: "[Task title]", Status: StatusTodo, CreatedAt: ref, Deadline: &d}
}

func TestDaysUntilDeadline(t *testing.T) {
	cases := []struct {
		name string
		day  time.Time
		want int
	}{
		{"later today", ref.Add(6 * time.Hour), 0},
		{"earlier today", ref.Add(-6 * time.Hour), 0},
		{"tomorrow", ref.AddDate(0, 0, 1), 1},
		{"in three days", ref.AddDate(0, 0, 3), 3},
		{"yesterday", ref.AddDate(0, 0, -1), -1},
		{"a week ago", ref.AddDate(0, 0, -7), -7},
	}
	for _, c := range cases {
		got, ok := DaysUntilDeadline(due(c.day), ref)
		if !ok {
			t.Errorf("%s: ok = false, want true", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("%s: days = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestDaysUntilDeadlineNoDeadline(t *testing.T) {
	if _, ok := DaysUntilDeadline(Task{Status: StatusTodo}, ref); ok {
		t.Error("ok = true for a task with no deadline, want false")
	}
}

func TestDeadlineUrgencyBuckets(t *testing.T) {
	cases := []struct {
		name string
		day  time.Time
		want Urgency
	}{
		{"four days out is future", ref.AddDate(0, 0, 4), UrgencyFuture},
		{"three days out is soon", ref.AddDate(0, 0, 3), UrgencySoon},
		{"two days out is soon", ref.AddDate(0, 0, 2), UrgencySoon},
		{"tomorrow is urgent", ref.AddDate(0, 0, 1), UrgencyUrgent},
		{"today is urgent", ref.Add(2 * time.Hour), UrgencyUrgent},
		{"yesterday is overdue", ref.AddDate(0, 0, -1), UrgencyOverdue},
	}
	for _, c := range cases {
		if got := DeadlineUrgency(due(c.day), ref); got != c.want {
			t.Errorf("%s: urgency = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDeadlineUrgencyNoDeadline(t *testing.T) {
	if got := DeadlineUrgency(Task{Status: StatusTodo}, ref); got != UrgencyNone {
		t.Errorf("urgency = %v, want UrgencyNone", got)
	}
}

func TestDeadlineUrgencyDoneTaskIsNeverUrgent(t *testing.T) {
	overdue := due(ref.AddDate(0, 0, -30))
	overdue.Status = StatusDone
	if got := DeadlineUrgency(overdue, ref); got != UrgencyDone {
		t.Errorf("urgency = %v, want UrgencyDone for a completed task", got)
	}
}

func TestDeadlineUrgencyDoneWithoutDeadlineIsNone(t *testing.T) {
	done := Task{Status: StatusDone, CreatedAt: ref}
	if got := DeadlineUrgency(done, ref); got != UrgencyNone {
		t.Errorf("urgency = %v, want UrgencyNone", got)
	}
}

func TestStartOfDayUsesCalendarNotElapsedTime(t *testing.T) {
	got := StartOfDay(time.Date(2026, 7, 30, 23, 59, 59, 0, time.UTC))
	want := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("StartOfDay = %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/task/ -run 'TestDays|TestDeadline|TestStartOfDay' -v`
Expected: FAIL — `undefined: DaysUntilDeadline`.

- [ ] **Step 3: Write the implementation**

Create `internal/task/deadline.go`:

```go
package task

import "time"

// Urgency is how much pressure a task's deadline is under, right now.
type Urgency int

const (
	UrgencyNone    Urgency = iota // no deadline set
	UrgencyDone                   // task is done; the deadline is history, not pressure
	UrgencyFuture                 // more than 3 days away
	UrgencySoon                   // 2 to 3 days away
	UrgencyUrgent                 // due today or tomorrow
	UrgencyOverdue                // the deadline day has passed
)

// Thresholds in whole calendar days remaining.
const (
	soonWithin   = 3 // at or under this many days: soon
	urgentWithin = 1 // at or under this many days: urgent
)

// StartOfDay truncates to midnight in t's own location. time.Truncate is
// wrong here: it operates on absolute time, not calendar days.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// DaysUntilDeadline is the number of whole calendar days from now's day to
// the deadline's day: 0 means today, 1 tomorrow, negative means past. ok is
// false when the task has no deadline.
func DaysUntilDeadline(t Task, now time.Time) (int, bool) {
	if t.Deadline == nil {
		return 0, false
	}
	from := StartOfDay(now)
	to := StartOfDay(t.Deadline.In(now.Location()))
	return int(to.Sub(from).Hours() / 24), true
}

// DeadlineUrgency buckets a task's deadline for display. A completed task
// always reports UrgencyDone: finishing late is not an ongoing emergency.
func DeadlineUrgency(t Task, now time.Time) Urgency {
	days, ok := DaysUntilDeadline(t, now)
	if !ok {
		return UrgencyNone
	}
	if t.Status == StatusDone {
		return UrgencyDone
	}
	switch {
	case days < 0:
		return UrgencyOverdue
	case days <= urgentWithin:
		return UrgencyUrgent
	case days <= soonWithin:
		return UrgencySoon
	}
	return UrgencyFuture
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/task/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add internal/task/deadline.go internal/task/deadline_test.go
git commit -m "feat: bucket task deadlines into urgency levels"
```

---

## Task 3: Board carries the new fields, and hides archived tasks

**Files:**
- Modify: `internal/task/board.go`
- Modify: `internal/task/board_test.go`

**Interfaces:**
- Consumes: `NewTask(title, description string, deadline *time.Time, now time.Time)` from Task 1.
- Produces:
  - `func (b *Board) Add(title, description string, deadline *time.Time, now time.Time) *Task`
  - `func (b *Board) Edit(id, title, description string, deadline *time.Time, now time.Time) error`
  - `func (b *Board) Active() []Task` — every non-archived task
  - `func (b *Board) ArchivedTasks() []Task` — every archived task, newest archived first
  - `ByStatus` now excludes archived tasks

This changes two widely-called signatures. Every call site in the repo must be updated — the compiler will list them. Do not add wrapper functions to avoid the churn.

- [ ] **Step 1: Write the failing test**

Append to `internal/task/board_test.go`:

```go
func TestBoardAddStoresDescriptionAndDeadline(t *testing.T) {
	var b Board
	dl := ref.AddDate(0, 0, 3)
	got := b.Add("[Task title]", "[a description]", &dl, ref)

	if got.Description != "[a description]" {
		t.Errorf("Description = %q, want %q", got.Description, "[a description]")
	}
	if got.Deadline == nil || !got.Deadline.Equal(dl) {
		t.Errorf("Deadline = %v, want %v", got.Deadline, dl)
	}
}

func TestBoardEditReplacesAllThreeFields(t *testing.T) {
	var b Board
	first := ref.AddDate(0, 0, 1)
	id := b.Add("[Old title]", "[old description]", &first, ref).ID

	second := ref.AddDate(0, 0, 9)
	later := ref.Add(time.Hour)
	if err := b.Edit(id, "[New title]", "[new description]", &second, later); err != nil {
		t.Fatalf("Edit returned %v", err)
	}

	got := b.Tasks[0]
	if got.Title != "[New title]" {
		t.Errorf("Title = %q, want %q", got.Title, "[New title]")
	}
	if got.Description != "[new description]" {
		t.Errorf("Description = %q, want %q", got.Description, "[new description]")
	}
	if got.Deadline == nil || !got.Deadline.Equal(second) {
		t.Errorf("Deadline = %v, want %v", got.Deadline, second)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
	}
}

func TestBoardEditCanClearTheDeadline(t *testing.T) {
	var b Board
	dl := ref.AddDate(0, 0, 3)
	id := b.Add("[Task title]", "", &dl, ref).ID

	if err := b.Edit(id, "[Task title]", "", nil, ref); err != nil {
		t.Fatalf("Edit returned %v", err)
	}
	if b.Tasks[0].Deadline != nil {
		t.Errorf("Deadline = %v, want nil after clearing", b.Tasks[0].Deadline)
	}
}

func TestByStatusExcludesArchived(t *testing.T) {
	var b Board
	keep := b.Add("[Visible]", "", nil, ref).ID
	gone := b.Add("[Archived]", "", nil, ref).ID
	for _, id := range []string{keep, gone} {
		if err := b.Move(id, StatusDone, ref); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	for i := range b.Tasks {
		if b.Tasks[i].ID == gone {
			b.Tasks[i].Archived = true
		}
	}

	done := b.ByStatus(StatusDone)
	if len(done) != 1 || done[0].ID != keep {
		t.Errorf("ByStatus(done) = %+v, want only the non-archived task", done)
	}
}

func TestActiveAndArchivedTasksPartitionTheBoard(t *testing.T) {
	var b Board
	b.Add("[Visible]", "", nil, ref)
	archivedID := b.Add("[Archived]", "", nil, ref).ID
	for i := range b.Tasks {
		if b.Tasks[i].ID == archivedID {
			b.Tasks[i].Archived = true
		}
	}

	if got := b.Active(); len(got) != 1 || got[0].Title != "[Visible]" {
		t.Errorf("Active() = %+v, want only the visible task", got)
	}
	if got := b.ArchivedTasks(); len(got) != 1 || got[0].Title != "[Archived]" {
		t.Errorf("ArchivedTasks() = %+v, want only the archived task", got)
	}
}

func TestArchivedTasksNewestFirst(t *testing.T) {
	var b Board
	older := b.Add("[Older]", "", nil, ref).ID
	newer := b.Add("[Newer]", "", nil, ref).ID
	oldStamp, newStamp := ref.Add(-48*time.Hour), ref.Add(-2*time.Hour)
	for i := range b.Tasks {
		switch b.Tasks[i].ID {
		case older:
			b.Tasks[i].Archived, b.Tasks[i].ArchivedAt = true, &oldStamp
		case newer:
			b.Tasks[i].Archived, b.Tasks[i].ArchivedAt = true, &newStamp
		}
	}

	got := b.ArchivedTasks()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "[Newer]" || got[1].Title != "[Older]" {
		t.Errorf("order = [%s %s], want [[Newer] [Older]]", got[0].Title, got[1].Title)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/task/ -run 'TestBoardAddStores|TestActive|TestByStatusExcludes' -v`
Expected: FAIL — compile error, `Add` takes 2 args not 4, `Active` undefined.

- [ ] **Step 3: Update `Add`, `Edit`, `ByStatus` and add the partition helpers**

In `internal/task/board.go`, replace `Add`, `Edit` and `ByStatus` with:

```go
// Add appends a new todo task and returns a pointer into b.Tasks. The
// pointer is only valid until the next Add — take the ID off it, never
// retain it.
func (b *Board) Add(title, description string, deadline *time.Time, now time.Time) *Task {
	b.Tasks = append(b.Tasks, NewTask(
		strings.TrimSpace(title), strings.TrimSpace(description), deadline, now))
	b.dirty = true
	return &b.Tasks[len(b.Tasks)-1]
}

// Edit replaces the title, description and deadline. Blank titles are
// rejected so the board cannot grow unreadable empty cards; a nil deadline
// clears any existing one.
func (b *Board) Edit(id, title, description string, deadline *time.Time, now time.Time) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must not be blank")
	}
	t, err := b.find(id)
	if err != nil {
		return err
	}
	t.Title = title
	t.Description = strings.TrimSpace(description)
	t.Deadline = deadline
	t.UpdatedAt = now
	b.dirty = true
	return nil
}

// ByStatus returns the non-archived tasks in one column, in insertion order.
// Archived tasks live on the Archive page and never appear on the board.
func (b *Board) ByStatus(s Status) []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if t.Status == s && !t.Archived {
			out = append(out, t)
		}
	}
	return out
}

// Active is every task still on the board.
func (b *Board) Active() []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if !t.Archived {
			out = append(out, t)
		}
	}
	return out
}

// ArchivedTasks is every archived task, most recently archived first.
func (b *Board) ArchivedTasks() []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if t.Archived {
			out = append(out, t)
		}
	}
	slices.SortStableFunc(out, func(x, y Task) int {
		return archivedStamp(y).Compare(archivedStamp(x)) // newest first
	})
	return out
}

// archivedStamp falls back to UpdatedAt for a task archived by an older
// build that did not record ArchivedAt.
func archivedStamp(t Task) time.Time {
	if t.ArchivedAt != nil {
		return *t.ArchivedAt
	}
	return t.UpdatedAt
}
```

Add `"slices"` to `board.go`'s import block. `errors`, `strings` and `time` are already there.

- [ ] **Step 4: Fix every call site**

Run `go build ./... && go vet ./...` and update each reported caller. The changes are mechanical — old `b.Add("[X]", ref)` becomes `b.Add("[X]", "", nil, ref)`, and old `b.Edit(id, "[X]", ref)` becomes `b.Edit(id, "[X]", "", nil, ref)`. Call sites live in:

- `internal/task/board_test.go`, `internal/task/store_test.go`
- `internal/ui/board_test.go` (the `seeded` helper), `internal/ui/analytics_test.go` (the `analyticsBoard` helper), `internal/ui/app_test.go`
- `internal/ui/board.go` — `updateInput` calls `m.board.Add(title, m.now())` and `m.board.Edit(m.editID, title, m.now())`. Pass `""` and `nil` for now; Task 7 replaces this whole function with the three-field form.

Do not change any test's assertions — only the argument lists.

- [ ] **Step 5: Run the whole suite and confirm it passes**

Run: `go test ./... -v`
Expected: PASS, every package.

- [ ] **Step 6: Commit**

```bash
git add internal/task/board.go internal/task/board_test.go internal/task/store_test.go internal/ui/
git commit -m "feat: carry description and deadline through the board, hide archived tasks"
```

---

## Task 4: The 14-day archive sweep

**Files:**
- Create: `internal/task/archive.go`
- Create: `internal/task/archive_test.go`
- Modify: `main.go`

**Interfaces:**
- Consumes: `Board`, `CompletedAt`, `StatusDone`.
- Produces: `const ArchiveAfter = 14 * 24 * time.Hour`; `func (b *Board) SweepArchive(now time.Time) int` — archives every eligible task and returns how many it moved.

Eligibility: the task is `done`, is not already archived, and entered done at least `ArchiveAfter` ago. "Entered done" is `CompletedAt`, which returns false unless the task is currently done — so a task that was reopened is never archived.

- [ ] **Step 1: Write the failing test**

Create `internal/task/archive_test.go`:

```go
package task

import (
	"testing"
	"time"
)

// completedAgo returns a done task that entered done the given duration before ref.
func completedAgo(b *Board, title string, ago time.Duration) string {
	id := b.Add(title, "", nil, ref.Add(-ago-time.Hour)).ID
	_ = b.Move(id, StatusDone, ref.Add(-ago))
	return id
}

func TestSweepArchiveMovesOldDoneTasks(t *testing.T) {
	var b Board
	old := completedAgo(&b, "[Long done]", 15*24*time.Hour)
	recent := completedAgo(&b, "[Just done]", 2*24*time.Hour)

	if n := b.SweepArchive(ref); n != 1 {
		t.Fatalf("SweepArchive = %d, want 1", n)
	}
	for _, tk := range b.Tasks {
		switch tk.ID {
		case old:
			if !tk.Archived {
				t.Error("the 15-day-old task was not archived")
			}
			if tk.ArchivedAt == nil || !tk.ArchivedAt.Equal(ref) {
				t.Errorf("ArchivedAt = %v, want %v", tk.ArchivedAt, ref)
			}
		case recent:
			if tk.Archived {
				t.Error("the 2-day-old task was archived, want it left on the board")
			}
		}
	}
}

func TestSweepArchiveBoundaryIsInclusive(t *testing.T) {
	var b Board
	completedAgo(&b, "[Exactly fourteen days]", ArchiveAfter)
	if n := b.SweepArchive(ref); n != 1 {
		t.Errorf("SweepArchive = %d, want 1 at exactly the threshold", n)
	}
}

func TestSweepArchiveJustUnderThreshold(t *testing.T) {
	var b Board
	completedAgo(&b, "[One minute short]", ArchiveAfter-time.Minute)
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("SweepArchive = %d, want 0 just under the threshold", n)
	}
}

func TestSweepArchiveIgnoresUnfinishedTasks(t *testing.T) {
	var b Board
	id := b.Add("[Still going]", "", nil, ref.Add(-100*24*time.Hour)).ID
	if err := b.Move(id, StatusBlocked, ref.Add(-99*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("SweepArchive = %d, want 0 for a blocked task", n)
	}
}

func TestSweepArchiveIgnoresReopenedTasks(t *testing.T) {
	var b Board
	id := completedAgo(&b, "[Reopened]", 30*24*time.Hour)
	if err := b.Move(id, StatusDoing, ref.Add(-time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("SweepArchive = %d, want 0 for a task moved back out of done", n)
	}
}

func TestSweepArchiveIsIdempotent(t *testing.T) {
	var b Board
	completedAgo(&b, "[Long done]", 20*24*time.Hour)
	if n := b.SweepArchive(ref); n != 1 {
		t.Fatalf("first sweep = %d, want 1", n)
	}
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("second sweep = %d, want 0 — already archived", n)
	}
}

func TestSweepArchiveMarksBoardDirtyOnlyWhenItMoves(t *testing.T) {
	var b Board
	completedAgo(&b, "[Just done]", time.Hour)
	b.dirty = false
	if b.SweepArchive(ref); b.Dirty() {
		t.Error("board is dirty after a no-op sweep, want clean")
	}

	completedAgo(&b, "[Long done]", 30*24*time.Hour)
	b.dirty = false
	if b.SweepArchive(ref); !b.Dirty() {
		t.Error("board is clean after archiving a task, want dirty")
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/task/ -run TestSweepArchive -v`
Expected: FAIL — `undefined: ArchiveAfter`.

- [ ] **Step 3: Write the implementation**

Create `internal/task/archive.go`:

```go
package task

import "time"

// ArchiveAfter is how long a task stays in the done column before it is
// swept off the board into the archive.
const ArchiveAfter = 14 * 24 * time.Hour

// SweepArchive archives every done task that has sat in done for at least
// ArchiveAfter, and reports how many it moved. It is idempotent: an already
// archived task is skipped. Tasks reopened out of done are never archived,
// because CompletedAt only reports a time for tasks currently in done.
func (b *Board) SweepArchive(now time.Time) int {
	moved := 0
	for i := range b.Tasks {
		t := &b.Tasks[i]
		if t.Archived || t.Status != StatusDone {
			continue
		}
		at, ok := CompletedAt(*t)
		if !ok || now.Sub(at) < ArchiveAfter {
			continue
		}
		stamp := now
		t.Archived = true
		t.ArchivedAt = &stamp
		t.UpdatedAt = now
		moved++
	}
	if moved > 0 {
		b.dirty = true
	}
	return moved
}
```

- [ ] **Step 4: Sweep at startup**

In `main.go`, immediately after the `task.Load` error check and before `tea.NewProgram`, add:

```go
	// Tidy the board before the first frame: anything done for two weeks
	// belongs in the archive, not in the done column.
	board.SweepArchive(time.Now())
```

Add `"time"` to `main.go`'s import block.

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./... -v && go build ./...`
Expected: PASS, and a clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/task/archive.go internal/task/archive_test.go main.go
git commit -m "feat: sweep done tasks into the archive after 14 days"
```

---

## Task 5: Deadline colours and the deadline line

**Files:**
- Modify: `internal/ui/theme.go`
- Modify: `internal/ui/chart_test.go`

**Interfaces:**
- Consumes: `task.Urgency` and its constants, `task.DeadlineUrgency`, `FormatDate`.
- Produces:
  - `func UrgencyColor(u task.Urgency) lipgloss.AdaptiveColor`
  - `func RenderDeadline(t task.Task, now time.Time) string` — the styled `● DD/MM/YYYY` line, with ` ✗` appended when overdue; empty string when the task has no deadline.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/chart_test.go`:

```go
func TestRenderDeadlineEmptyWithoutDeadline(t *testing.T) {
	got := RenderDeadline(task.Task{Status: task.StatusTodo}, chartRef)
	if got != "" {
		t.Errorf("RenderDeadline = %q, want empty for a task with no deadline", got)
	}
}

func TestRenderDeadlineShowsSingaporeDate(t *testing.T) {
	due := chartRef.AddDate(0, 0, 5)
	tk := task.Task{Status: task.StatusTodo, Deadline: &due}

	got := stripANSI(RenderDeadline(tk, chartRef))
	if !strings.Contains(got, "04/08/2026") {
		t.Errorf("RenderDeadline = %q, want it to contain 04/08/2026", got)
	}
	if !strings.Contains(got, "●") {
		t.Errorf("RenderDeadline = %q, want the ● marker", got)
	}
	if strings.Contains(got, "✗") {
		t.Errorf("RenderDeadline = %q, want no ✗ for a future deadline", got)
	}
}

func TestRenderDeadlineMarksOverdue(t *testing.T) {
	due := chartRef.AddDate(0, 0, -2)
	tk := task.Task{Status: task.StatusTodo, Deadline: &due}

	got := stripANSI(RenderDeadline(tk, chartRef))
	if !strings.Contains(got, "28/07/2026") {
		t.Errorf("RenderDeadline = %q, want it to contain 28/07/2026", got)
	}
	if !strings.Contains(got, "✗") {
		t.Errorf("RenderDeadline = %q, want the ✗ marker for an overdue task", got)
	}
}

func TestRenderDeadlineDoneTaskHasNoCross(t *testing.T) {
	due := chartRef.AddDate(0, 0, -30)
	tk := task.Task{Status: task.StatusDone, Deadline: &due}

	got := stripANSI(RenderDeadline(tk, chartRef))
	if strings.Contains(got, "✗") {
		t.Errorf("RenderDeadline = %q, want no ✗ on a completed task", got)
	}
}

func TestUrgencyColorsAreDistinct(t *testing.T) {
	seen := map[string]task.Urgency{}
	for _, u := range []task.Urgency{
		task.UrgencyFuture, task.UrgencySoon, task.UrgencyUrgent, task.UrgencyDone,
	} {
		c := UrgencyColor(u)
		key := c.Light + "/" + c.Dark
		if prev, dup := seen[key]; dup {
			t.Errorf("urgency %v and %v share colour %s; each level must be distinguishable", prev, u, key)
		}
		seen[key] = u
	}
	if UrgencyColor(task.UrgencyOverdue) != UrgencyColor(task.UrgencyUrgent) {
		t.Error("overdue and urgent should share the red; the ✗ is what distinguishes them")
	}
}
```

This test file needs a fixed clock and an ANSI stripper. Add these near the top of `internal/ui/chart_test.go`, after the imports:

```go
// chartRef is this file's fixed clock. board_test.go owns the package-level
// `ref`; this name avoids colliding with it.
var chartRef = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes lipgloss's colour escapes so assertions can match the
// visible text.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }
```

Add `"regexp"` and `"gotodo/internal/task"` to the test file's imports (`strings`, `testing` and `time` are already there).

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestRenderDeadline|TestUrgencyColors' -v`
Expected: FAIL — `undefined: RenderDeadline`.

- [ ] **Step 3: Write the implementation**

Append to `internal/ui/theme.go`:

```go
// Deadline palette. Overdue reuses the urgent red — the ✗ marker, not the
// colour, is what says "you missed it".
var (
	colDeadlineFuture = colDone    // green: plenty of time
	colDeadlineSoon   = colDoing   // amber: 2-3 days
	colDeadlineUrgent = colBlocked // red: today or tomorrow, or passed
)

// UrgencyColor is the colour a deadline renders in at each urgency level.
func UrgencyColor(u task.Urgency) lipgloss.AdaptiveColor {
	switch u {
	case task.UrgencySoon:
		return colDeadlineSoon
	case task.UrgencyUrgent, task.UrgencyOverdue:
		return colDeadlineUrgent
	case task.UrgencyFuture:
		return colDeadlineFuture
	}
	return ColMuted // UrgencyDone and UrgencyNone
}

// RenderDeadline is the card's deadline line: a coloured bullet, the date in
// DD/MM/YYYY, and a ✗ when the deadline has passed. Empty when the task has
// no deadline.
func RenderDeadline(t task.Task, now time.Time) string {
	if t.Deadline == nil {
		return ""
	}
	u := task.DeadlineUrgency(t, now)
	line := "● " + FormatDate(*t.Deadline)
	if u == task.UrgencyOverdue {
		line += " ✗"
	}
	return lipgloss.NewStyle().Foreground(UrgencyColor(u)).Render(line)
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all UI tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/theme.go internal/ui/chart_test.go
git commit -m "feat: colour-code deadlines by urgency"
```

---

## Task 6: Three-line cards

**Files:**
- Modify: `internal/ui/board.go`
- Modify: `internal/ui/board_test.go`

**Interfaces:**
- Consumes: `RenderDeadline` from Task 5, `truncate` and `BoardModel.now` from the existing code.
- Produces: `renderCard` renders up to three lines; `renderColumn` puts a blank line between cards.

The selected card keeps its left accent bar; lipgloss draws that border down every line of the block, so a selected three-line card gets a full-height bar. That is intended.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/board_test.go`:

```go
// withFields returns a board holding one fully-populated todo task.
func withFields(t *testing.T, desc string, deadline *time.Time) *task.Board {
	t.Helper()
	b := &task.Board{}
	b.Add("[Ship the report]", desc, deadline, ref)
	return b
}

func TestCardShowsTitleDescriptionAndDeadline(t *testing.T) {
	due := ref.AddDate(0, 0, 5)
	m := NewBoardModel(withFields(t, "[draft, review, send]", &due))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	out := stripANSI(m.View())
	for _, want := range []string{"Ship the report", "draft, review, send", "04/08/2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
}

func TestCardOmitsDescriptionLineWhenEmpty(t *testing.T) {
	m := NewBoardModel(withFields(t, "", nil))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	card := stripANSI(m.renderCard(0, 0, m.board.ByStatus(task.StatusTodo)[0], 30))
	if got := len(strings.Split(card, "\n")); got != 1 {
		t.Errorf("card has %d lines, want 1 for a task with no description and no deadline:\n%q", got, card)
	}
}

func TestCardHasThreeLinesWhenFullyPopulated(t *testing.T) {
	due := ref.AddDate(0, 0, 5)
	m := NewBoardModel(withFields(t, "[a description]", &due))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	card := stripANSI(m.renderCard(0, 0, m.board.ByStatus(task.StatusTodo)[0], 30))
	if got := len(strings.Split(card, "\n")); got != 3 {
		t.Errorf("card has %d lines, want 3:\n%q", got, card)
	}
}

func TestCardMarksOverdueDeadline(t *testing.T) {
	due := ref.AddDate(0, 0, -3)
	m := NewBoardModel(withFields(t, "", &due))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	out := stripANSI(m.View())
	if !strings.Contains(out, "27/07/2026 ✗") {
		t.Errorf("View missing the overdue marker:\n%s", out)
	}
}

func TestCardTruncatesLongDescription(t *testing.T) {
	long := strings.Repeat("x", 200)
	m := NewBoardModel(withFields(t, long, nil))
	m.now = func() time.Time { return ref }
	m.SetSize(160, 40)

	card := stripANSI(m.renderCard(0, 0, m.board.ByStatus(task.StatusTodo)[0], 30))
	for _, line := range strings.Split(card, "\n") {
		if len([]rune(line)) > 30 {
			t.Errorf("line is %d runes, wider than the 30-wide column: %q", len([]rune(line)), line)
		}
	}
}
```

`board_test.go` already imports `strings`, `testing`, `time` and `gotodo/internal/task`. `stripANSI` comes from `chart_test.go` in the same package — do not redeclare it.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run TestCard -v`
Expected: FAIL — the description and deadline are not rendered, so `TestCardShowsTitleDescriptionAndDeadline` reports the missing substrings and the line-count tests report 1 line where 3 are wanted.

- [ ] **Step 3: Rewrite `renderCard`**

In `internal/ui/board.go`, replace `renderCard` with:

```go
// renderCard draws one card: title, optional description, optional deadline.
// A card is 1 to 3 lines tall depending on which fields are set.
func (m BoardModel) renderCard(colIdx, itemIdx int, t task.Task, width int) string {
	inner := width - 4
	selected := colIdx == m.col && itemIdx == m.sel[m.col]
	grabbed := selected && m.mode == modeMove && t.ID == m.grabID

	title := truncate(t.Title, inner)
	if grabbed {
		title = "⇄ " + truncate(t.Title, inner-2)
	}

	lines := []string{title}
	if t.Description != "" {
		lines = append(lines, MutedStyle.Render(truncate(t.Description, inner)))
	}
	if dl := RenderDeadline(t, m.now()); dl != "" {
		lines = append(lines, dl)
	}
	body := strings.Join(lines, "\n")

	switch {
	case grabbed:
		return CardGrabbedStyle.Render(body)
	case selected && m.focus == focusItem:
		return CardSelectedStyle.Render(body)
	default:
		return CardStyle.Render(body)
	}
}
```

- [ ] **Step 4: Space the cards apart in `renderColumn`**

In `renderColumn`, replace the card loop with one that inserts a blank line between cards:

```go
	for i, t := range items {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderCard(idx, i, t, width))
	}
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, including every pre-existing board test.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/board.go internal/ui/board_test.go
git commit -m "feat: render title, description and colour-coded deadline on each card"
```

---

## Task 7: Three-field add/edit form

**Files:**
- Modify: `internal/ui/board.go`
- Modify: `internal/ui/board_test.go`

**Interfaces:**
- Consumes: `Board.Add`/`Board.Edit` (four-argument forms) from Task 3.
- Produces:
  - `BoardModel.inputs [3]textinput.Model` replacing the single `input` field
  - `BoardModel.field int` — which field has focus
  - constants `fieldTitle = 0`, `fieldDesc = 1`, `fieldDeadline = 2`
  - `func parseDeadline(s string) (*time.Time, error)`
  - `func (m BoardModel) renderForm() string`

Keys inside the form: `tab` / `shift+tab` cycle fields (wrapping), `enter` saves from any field, `esc` cancels. Everything else goes to the focused input.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/board_test.go`:

```go
func TestParseDeadline(t *testing.T) {
	got, err := parseDeadline("02/08/2026")
	if err != nil {
		t.Fatalf("parseDeadline returned %v", err)
	}
	if got == nil {
		t.Fatal("parseDeadline returned nil for a valid date")
	}
	want := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseDeadline = %v, want %v", *got, want)
	}
}

func TestParseDeadlineBlankIsNoDeadline(t *testing.T) {
	got, err := parseDeadline("   ")
	if err != nil {
		t.Fatalf("parseDeadline returned %v", err)
	}
	if got != nil {
		t.Errorf("parseDeadline = %v, want nil for blank input", got)
	}
}

func TestParseDeadlineRejectsUSFormatAndGarbage(t *testing.T) {
	for _, in := range []string{"2026-08-02", "notadate", "13/13/2026"} {
		if _, err := parseDeadline(in); err == nil {
			t.Errorf("parseDeadline(%q) returned nil error, want a rejection", in)
		}
	}
}

func TestFormTabCyclesFields(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	if m.field != fieldTitle {
		t.Fatalf("field = %d, want fieldTitle on open", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.field != fieldDesc {
		t.Errorf("field = %d, want fieldDesc after tab", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.field != fieldDeadline {
		t.Errorf("field = %d, want fieldDeadline after two tabs", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.field != fieldTitle {
		t.Errorf("field = %d, want it to wrap back to fieldTitle", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.field != fieldDeadline {
		t.Errorf("field = %d, want fieldDeadline after shift+tab from the first field", m.field)
	}
}

func TestFormSavesAllThreeFields(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldTitle].SetValue("[Ship the report]")
	m.inputs[fieldDesc].SetValue("[draft, review, send]")
	m.inputs[fieldDeadline].SetValue("02/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal after save", m.mode)
	}
	todo := m.board.ByStatus(task.StatusTodo)
	if len(todo) != 1 {
		t.Fatalf("todo tasks = %d, want 1", len(todo))
	}
	got := todo[0]
	if got.Title != "[Ship the report]" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Description != "[draft, review, send]" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Deadline == nil || !got.Deadline.Equal(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Deadline = %v, want 02/08/2026", got.Deadline)
	}
}

func TestFormRejectsBadDeadlineAndStaysOpen(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldTitle].SetValue("[Ship the report]")
	m.inputs[fieldDeadline].SetValue("tomorrow")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput — the form must stay open on a bad date", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 — nothing should be saved", len(m.board.Tasks))
	}
	if m.err == "" {
		t.Error("err is empty, want a message explaining the date format")
	}
}

func TestFormBlankTitleStillRejected(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldDesc].SetValue("[a description]")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0", len(m.board.Tasks))
	}
}

func TestEditPrefillsAllThreeFields(t *testing.T) {
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	b := &task.Board{}
	b.Add("[Old title]", "[old description]", &due, ref)

	m := fixedClock(NewBoardModel(b))
	m = press(m, "e")
	if m.mode != modeInput {
		t.Fatalf("mode = %v, want modeInput", m.mode)
	}
	if got := m.inputs[fieldTitle].Value(); got != "[Old title]" {
		t.Errorf("title field = %q", got)
	}
	if got := m.inputs[fieldDesc].Value(); got != "[old description]" {
		t.Errorf("description field = %q", got)
	}
	if got := m.inputs[fieldDeadline].Value(); got != "02/08/2026" {
		t.Errorf("deadline field = %q, want 02/08/2026", got)
	}
}

func TestEditWithClearedDeadlineRemovesIt(t *testing.T) {
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	b := &task.Board{}
	b.Add("[Task title]", "", &due, ref)

	m := fixedClock(NewBoardModel(b))
	m = press(m, "e")
	m.inputs[fieldDeadline].SetValue("")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.board.Tasks[0].Deadline != nil {
		t.Errorf("Deadline = %v, want nil after clearing the field", m.board.Tasks[0].Deadline)
	}
}

func TestFormEscCancels(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.inputs[fieldTitle].SetValue("[Task title]")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != modeNormal {
		t.Errorf("mode = %v, want modeNormal", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 after cancel", len(m.board.Tasks))
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestParseDeadline|TestForm|TestEdit' -v`
Expected: FAIL — `undefined: parseDeadline`, and `m.inputs` / `m.field` undefined.

- [ ] **Step 3: Replace the single input with three**

In `internal/ui/board.go`, change the `BoardModel` struct: replace the `input textinput.Model` field with these two, leaving every other field exactly as it is:

```go
	inputs [3]textinput.Model
	field  int // which of inputs has focus
```

Add the field constants next to the mode constants:

```go
// Fields of the add/edit form, in tab order.
const (
	fieldTitle = iota
	fieldDesc
	fieldDeadline
)
```

Replace `NewBoardModel` with:

```go
// NewBoardModel wires a board into a fresh page model.
func NewBoardModel(b *task.Board) BoardModel {
	m := BoardModel{board: b, focus: focusItem, mode: modeNormal, now: time.Now}
	placeholders := [3]string{"task title", "description (optional)", "DD/MM/YYYY (optional)"}
	limits := [3]int{200, 500, 10}
	for i := range m.inputs {
		in := textinput.New()
		in.Placeholder = placeholders[i]
		in.CharLimit = limits[i]
		in.Prompt = "› "
		m.inputs[i] = in
	}
	return m
}
```

- [ ] **Step 4: Add the deadline parser and the form helpers**

Append to `internal/ui/board.go`:

```go
// parseDeadline reads a DD/MM/YYYY date. Blank means "no deadline", which is
// not an error. The layout matches FormatDate, so what the card shows is
// exactly what you type back in.
func parseDeadline(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	d, err := time.Parse("02/01/2006", s)
	if err != nil {
		return nil, errors.New("deadline must look like 02/08/2026, or be left blank")
	}
	return &d, nil
}

// openForm puts the model into the add/edit form, seeded with the given
// values and focused on the title.
func (m *BoardModel) openForm(editID, title, desc string, deadline *time.Time) {
	m.mode = modeInput
	m.editID = editID
	m.field = fieldTitle
	m.err = ""

	dl := ""
	if deadline != nil {
		dl = FormatDate(*deadline)
	}
	for i, v := range [3]string{title, desc, dl} {
		m.inputs[i].SetValue(v)
		m.inputs[i].CursorEnd()
		m.inputs[i].Blur()
	}
	m.inputs[fieldTitle].Focus()
}

// closeForm returns to normal mode and drops focus from every field.
func (m *BoardModel) closeForm() {
	m.mode = modeNormal
	m.editID = ""
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

// focusField moves form focus by delta, wrapping at both ends.
func (m *BoardModel) focusField(delta int) {
	m.inputs[m.field].Blur()
	m.field = (m.field + delta + len(m.inputs)) % len(m.inputs)
	m.inputs[m.field].Focus()
	m.inputs[m.field].CursorEnd()
}

// renderForm draws the three-field entry panel shown at the bottom.
func (m BoardModel) renderForm() string {
	heading := "new task"
	if m.editID != "" {
		heading = "edit task"
	}
	labels := [3]string{"Title", "Description", "Deadline"}

	rows := []string{TitleStyle.Render(heading)}
	for i, label := range labels {
		name := MutedStyle.Render(fmt.Sprintf("%-12s", label))
		if i == m.field {
			name = lipgloss.NewStyle().Foreground(ColAccent).Bold(true).
				Render(fmt.Sprintf("%-12s", label))
		}
		rows = append(rows, name+m.inputs[i].View())
	}
	if m.err != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(AccentFor(task.StatusBlocked)).
			Render("! "+m.err))
	}
	rows = append(rows, MutedStyle.Render("tab/shift+tab field · enter save · esc cancel"))
	return ColumnStyle.Render(strings.Join(rows, "\n"))
}
```

Add `"errors"` and `"fmt"` to `board.go`'s import block.

- [ ] **Step 5: Rewrite `updateInput` and the entry keys**

Replace `updateInput` with:

```go
func (m BoardModel) updateInput(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		m.closeForm()
		return m, nil

	case tea.KeyTab:
		m.focusField(1)
		return m, nil

	case tea.KeyShiftTab:
		m.focusField(-1)
		return m, nil

	case tea.KeyEnter:
		title := strings.TrimSpace(m.inputs[fieldTitle].Value())
		if title == "" {
			m.err = "title must not be blank"
			return m, nil // stay in the form
		}
		deadline, err := parseDeadline(m.inputs[fieldDeadline].Value())
		if err != nil {
			m.err = err.Error()
			return m, nil // stay in the form
		}
		desc := strings.TrimSpace(m.inputs[fieldDesc].Value())

		if m.editID != "" {
			if err := m.board.Edit(m.editID, title, desc, deadline, m.now()); err != nil {
				m.err = err.Error()
				return m, nil
			}
		} else {
			m.board.Add(title, desc, deadline, m.now())
			m.col = 0
			m.sel[0] = len(m.board.ByStatus(task.StatusTodo)) - 1
		}
		m.closeForm()
		m.clampSelection()
		return m, dirty()
	}

	var cmd tea.Cmd
	m.inputs[m.field], cmd = m.inputs[m.field].Update(k)
	return m, cmd
}
```

In `updateNormal`, replace the bodies of `case "a":` and `case "e":` with:

```go
	case "a":
		m.openForm("", "", "", nil)
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.openForm(t.ID, t.Title, t.Description, t.Deadline)
```

In `renderFooter`, replace the `case modeInput:` branch with:

```go
	case modeInput:
		return "\n" + m.renderForm()
```

- [ ] **Step 6: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS. Two pre-existing tests reference `m.input` (singular) — `TestAddOpensInputAndCommitsOnEnter` and `TestEditReplacesTitle` in `board_test.go`, and `TestQIsTypableWhileAddingATask` in `app_test.go`. Update those references to `m.inputs[fieldTitle]`; do not weaken what they assert.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/board.go internal/ui/board_test.go internal/ui/app_test.go
git commit -m "feat: three-field add/edit form with title, description and deadline"
```

---

## Task 8: The Archive page

**Files:**
- Create: `internal/ui/archive.go`
- Create: `internal/ui/archive_test.go`

**Interfaces:**
- Consumes: `task.Board.ArchivedTasks()`, `RenderDeadline`, `FormatDate`, `truncate`, theme styles.
- Produces:
  - `type ArchiveModel struct { board *task.Board; now func() time.Time; sel, width, height int }`
  - `func NewArchiveModel(b *task.Board) ArchiveModel`
  - `func (m *ArchiveModel) SetSize(w, h int)`
  - `func (m ArchiveModel) Update(msg tea.Msg) (ArchiveModel, tea.Cmd)` — `j`/`k`/arrows only
  - `func (m ArchiveModel) View() string`

View-only: no restore, no delete, no edit.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/archive_test.go`:

```go
package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
)

// archivedBoard returns a board with two archived tasks and one live one.
func archivedBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	due := ref.AddDate(0, 0, -20)

	older := b.Add("[Older archived]", "[first description]", &due, ref.Add(-40*24*time.Hour)).ID
	newer := b.Add("[Newer archived]", "", nil, ref.Add(-30*24*time.Hour)).ID
	for _, id := range []string{older, newer} {
		if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	if n := b.SweepArchive(ref); n != 2 {
		t.Fatalf("SweepArchive = %d, want 2", n)
	}
	b.Add("[Still on the board]", "", nil, ref)
	return b
}

func fixedArchive(b *task.Board) ArchiveModel {
	m := NewArchiveModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(100, 40)
	return m
}

func TestArchiveViewListsArchivedTasksOnly(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())

	for _, want := range []string{"Older archived", "Newer archived"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Still on the board") {
		t.Errorf("View shows a live task; the archive must list archived tasks only:\n%s", out)
	}
}

func TestArchiveViewShowsDescriptionAndDeadline(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, "first description") {
		t.Errorf("View missing the description:\n%s", out)
	}
	if !strings.Contains(out, "10/07/2026") {
		t.Errorf("View missing the deadline date:\n%s", out)
	}
}

func TestArchiveViewShowsArchivedDate(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, FormatDate(ref)) {
		t.Errorf("View missing the archived-on date %s:\n%s", FormatDate(ref), out)
	}
}

func TestArchiveViewCountsEntries(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, "2") || !strings.Contains(out, "ARCHIVE") {
		t.Errorf("View missing the ARCHIVE heading and its count:\n%s", out)
	}
}

func TestArchiveViewEmptyState(t *testing.T) {
	out := stripANSI(fixedArchive(&task.Board{}).View())
	if !strings.Contains(out, "ARCHIVE") {
		t.Errorf("empty archive lost its heading:\n%s", out)
	}
	if !strings.Contains(out, "nothing archived yet") {
		t.Errorf("empty archive missing its placeholder:\n%s", out)
	}
}

func TestArchiveJKMovesSelection(t *testing.T) {
	m := fixedArchive(archivedBoard(t))
	if m.sel != 0 {
		t.Fatalf("sel = %d, want 0", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 1 {
		t.Errorf("sel = %d, want 1 after j", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 1 {
		t.Errorf("sel = %d, want it clamped at the last entry", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0 after k", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want it clamped at the first entry", m.sel)
	}
}

func TestArchiveSelectionSurvivesEmptyBoard(t *testing.T) {
	m := fixedArchive(&task.Board{})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0 on an empty archive", m.sel)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run TestArchive -v`
Expected: FAIL — `undefined: NewArchiveModel`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/archive.go`:

```go
package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// ArchiveModel is the third page: done tasks that have aged off the board.
// It is view-only — nothing here mutates the board.
type ArchiveModel struct {
	board *task.Board
	now   func() time.Time

	sel    int
	width  int
	height int
}

// NewArchiveModel wires a board into the archive page.
func NewArchiveModel(b *task.Board) ArchiveModel {
	return ArchiveModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *ArchiveModel) SetSize(w, h int) { m.width, m.height = w, h }

// Update handles selection movement. The archive is read-only, so no key
// here changes any task.
func (m ArchiveModel) Update(msg tea.Msg) (ArchiveModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	n := len(m.board.ArchivedTasks())
	switch k.String() {
	case "j", "down":
		m.sel++
	case "k", "up":
		m.sel--
	case "g":
		m.sel = 0
	case "G":
		m.sel = n - 1
	}
	if m.sel >= n {
		m.sel = n - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	return m, nil
}

// View renders the archive list.
func (m ArchiveModel) View() string {
	items := m.board.ArchivedTasks()
	width := m.width
	if width <= 0 {
		width = 80
	}

	head := TitleStyle.Render("ARCHIVE") +
		MutedStyle.Render(" ("+strconv.Itoa(len(items))+")")
	lines := []string{head, ""}

	if len(items) == 0 {
		lines = append(lines,
			MutedStyle.Render("nothing archived yet"),
			"",
			MutedStyle.Render(fmt.Sprintf(
				"done tasks move here %d days after you finish them",
				int(task.ArchiveAfter.Hours()/24))))
		return strings.Join(lines, "\n")
	}

	for i, t := range items {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderEntry(i, t, width))
	}
	lines = append(lines, "",
		MutedStyle.Render("j/k move · tab back to the board · read-only"))
	return strings.Join(lines, "\n")
}

func (m ArchiveModel) renderEntry(i int, t task.Task, width int) string {
	inner := width - 6

	rows := []string{truncate(t.Title, inner)}
	if t.Description != "" {
		rows = append(rows, MutedStyle.Render(truncate(t.Description, inner)))
	}

	meta := MutedStyle.Render("archived " + FormatDate(archivedOn(t)))
	if dl := RenderDeadline(t, m.now()); dl != "" {
		meta = dl + MutedStyle.Render("  ·  archived "+FormatDate(archivedOn(t)))
	}
	rows = append(rows, meta)

	body := strings.Join(rows, "\n")
	if i == m.sel {
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColAccent).
			PaddingLeft(1).Bold(true).Render(body)
	}
	return CardStyle.Render(body)
}

// archivedOn falls back to UpdatedAt for entries archived by an older build
// that did not stamp ArchivedAt.
func archivedOn(t task.Task) time.Time {
	if t.ArchivedAt != nil {
		return *t.ArchivedAt
	}
	return t.UpdatedAt
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all UI tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/archive.go internal/ui/archive_test.go
git commit -m "feat: add the read-only Archive page"
```

---

## Task 9: Wire the third page, the hourly sweep, and the docs

**Files:**
- Modify: `internal/ui/app.go`
- Modify: `internal/ui/app_test.go`
- Modify: `internal/ui/analytics.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `ArchiveModel` from Task 8, `task.Board.SweepArchive` from Task 4, `task.Board.Active` from Task 3.
- Produces:
  - `pageArchive` added to the `page` enum
  - `AppModel.archive ArchiveModel`
  - `type archiveTickMsg time.Time` and `func archiveTick() tea.Cmd`
  - `AppModel.Init` returns `archiveTick()`
  - Tab cycles Board → Analytics → Archive → Board

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/app_test.go`:

```go
func TestTabCyclesThreePages(t *testing.T) {
	a := app(t)
	if a.page != pageBoard {
		t.Fatalf("page = %v, want pageBoard", a.page)
	}
	for _, want := range []page{pageAnalytics, pageArchive, pageBoard} {
		m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
		a = m.(AppModel)
		if a.page != want {
			t.Fatalf("page = %v, want %v", a.page, want)
		}
	}
}

func TestArchivePageRenders(t *testing.T) {
	a := app(t)
	for i := 0; i < 2; i++ {
		m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
		a = m.(AppModel)
	}
	if !strings.Contains(stripANSI(a.View()), "ARCHIVE") {
		t.Errorf("archive page did not render:\n%s", a.View())
	}
}

func TestArchiveKeysRouteToArchivePage(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 3; i++ {
		id := b.Add("[Archived task]", "", nil, ref.Add(-40*24*time.Hour)).ID
		if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	if n := b.SweepArchive(ref); n != 3 {
		t.Fatalf("SweepArchive = %d, want 3", n)
	}

	a := NewApp(b)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = m.(AppModel)
	for i := 0; i < 2; i++ {
		m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
		a = m.(AppModel)
	}
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	a = m.(AppModel)
	if a.archive.sel != 1 {
		t.Errorf("archive sel = %d, want 1 — j must reach the archive page", a.archive.sel)
	}
}

func TestArchiveTickSweepsAndSaves(t *testing.T) {
	b := &task.Board{}
	b.SetPath(t.TempDir() + "/tasks.json")
	id := b.Add("[Long done]", "", nil, ref.Add(-40*24*time.Hour)).ID
	if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	a := NewApp(b)
	a.now = func() time.Time { return ref }
	m, cmd := a.Update(archiveTickMsg(ref))
	a = m.(AppModel)

	if !a.store.Tasks[0].Archived {
		t.Error("the tick did not archive an eligible task")
	}
	if cmd == nil {
		t.Fatal("the tick returned a nil cmd, want at least the next tick scheduled")
	}
}

func TestArchiveTickWithNothingToDoStillReschedules(t *testing.T) {
	b := &task.Board{}
	b.SetPath(t.TempDir() + "/tasks.json")
	b.Add("[Fresh]", "", nil, ref)

	a := NewApp(b)
	a.now = func() time.Time { return ref }
	_, cmd := a.Update(archiveTickMsg(ref))
	if cmd == nil {
		t.Error("cmd is nil, want the next tick rescheduled even when nothing was archived")
	}
}
```

`app_test.go` needs `"strings"` in its imports if it is not already there.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestTabCycles|TestArchive' -v`
Expected: FAIL — `undefined: pageArchive`, `undefined: archiveTickMsg`.

- [ ] **Step 3: Add the third page and the tick**

In `internal/ui/app.go`, extend the page enum:

```go
const (
	pageBoard page = iota
	pageAnalytics
	pageArchive
)
```

Add fields to `AppModel` — keep every existing field:

```go
	archive ArchiveModel
	now     func() time.Time
```

Replace `NewApp` and `Init`:

```go
// NewApp builds the root model around a loaded board.
func NewApp(b *task.Board) AppModel {
	return AppModel{
		board:     NewBoardModel(b),
		analytics: NewAnalyticsModel(b),
		archive:   NewArchiveModel(b),
		store:     b,
		now:       time.Now,
	}
}

// archiveTickMsg fires periodically so a long-running session still tidies
// itself instead of only archiving at startup.
type archiveTickMsg time.Time

func archiveTick() tea.Cmd {
	return tea.Tick(time.Hour, func(t time.Time) tea.Msg { return archiveTickMsg(t) })
}

// Init starts the hourly archive sweep.
func (m AppModel) Init() tea.Cmd { return archiveTick() }
```

Add `"time"` to `app.go`'s import block.

- [ ] **Step 4: Handle the tick and route keys to the archive**

In `AppModel.Update`, add this case alongside the existing `dirtyMsg` case:

```go
	case archiveTickMsg:
		if m.store.SweepArchive(m.now()) > 0 {
			return m, tea.Batch(dirty(), archiveTick())
		}
		return m, archiveTick()
```

Replace the `case "tab":` body so it cycles three pages:

```go
			case "tab":
				m.page = (m.page + 1) % 3
				return m, nil
```

At the end of the `tea.KeyMsg` branch, where keys are forwarded to the active page, replace the board-only forwarding with:

```go
		switch m.page {
		case pageBoard:
			var cmd tea.Cmd
			m.board, cmd = m.board.Update(msg)
			return m, cmd
		case pageArchive:
			var cmd tea.Cmd
			m.archive, cmd = m.archive.Update(msg)
			return m, cmd
		}
		return m, nil
```

In the `tea.WindowSizeMsg` case, add the archive alongside the other two:

```go
		m.archive.SetSize(msg.Width, msg.Height)
```

In `View`, add the archive branch:

```go
	body := m.board.View()
	switch m.page {
	case pageAnalytics:
		body = m.analytics.View()
	case pageArchive:
		body = m.archive.View()
	}
```

Replace `renderTabs` so all three tabs show:

```go
func (m AppModel) renderTabs() string {
	active := lipgloss.NewStyle().Bold(true).Foreground(ColAccent).Padding(0, 2)
	inactive := MutedStyle.Copy().Padding(0, 2)

	names := [3]string{"BOARD", "ANALYTICS", "ARCHIVE"}
	tabs := make([]string, 0, len(names))
	for i, n := range names {
		if page(i) == m.page {
			tabs = append(tabs, active.Render(n))
			continue
		}
		tabs = append(tabs, inactive.Render(n))
	}
	tabs = append(tabs, MutedStyle.Render("  tab to switch"))
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}
```

- [ ] **Step 5: Update the help overlay**

In `renderHelp`, replace the `{"tab", ...}` row and add the deadline note, keeping every other row:

```go
		{"tab", "cycle board → analytics → archive"},
```

and append these two rows just before the closing of the `rows` literal:

```go
		{"", ""},
		{"deadlines", "green >3 days · amber ≤3 · red ≤1 · red ✗ overdue"},
```

- [ ] **Step 6: Make the analytics tiles count active tasks only**

In `internal/ui/analytics.go`, `View` currently starts with `tasks := m.board.Tasks`. Keep that for the history-based sections, but pass only active tasks to the tiles so they match the board's column counts:

```go
	sections := []string{
		m.renderTiles(m.board.Active()),
		m.renderThroughput(tasks, now),
		m.renderCycle(tasks, now),
		m.renderBlocked(tasks, now),
		m.renderStreak(tasks, now),
	}
```

Add a test to `internal/ui/analytics_test.go`:

```go
func TestAnalyticsTilesExcludeArchivedButHistoryDoesNot(t *testing.T) {
	b := &task.Board{}
	id := b.Add("[Long done]", "", nil, ref.Add(-40*24*time.Hour)).ID
	if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if n := b.SweepArchive(ref); n != 1 {
		t.Fatalf("SweepArchive = %d, want 1", n)
	}

	out := stripANSI(fixedAnalytics(b).View())

	// The DONE tile must read 0 — the task left the board.
	lines := strings.Split(out, "\n")
	var labelIdx = -1
	for i, l := range lines {
		if strings.Contains(l, "TODO") && strings.Contains(l, "DONE") {
			labelIdx = i
			break
		}
	}
	if labelIdx < 1 {
		t.Fatalf("could not find the tile label row:\n%s", out)
	}
	if got := strings.Fields(lines[labelIdx-1]); len(got) != 4 || got[3] != "0" {
		t.Errorf("tile counts = %v, want the DONE tile to read 0", got)
	}

	// But throughput must still count the completion.
	if !strings.Contains(out, "1 completed over 14 days") {
		t.Errorf("archiving erased the completion from throughput:\n%s", out)
	}
}
```

- [ ] **Step 7: Run the whole suite**

Run: `go test ./... -v && go vet ./... && gofmt -l .`
Expected: every package PASS, vet clean, gofmt silent.

- [ ] **Step 8: Update the README**

In `README.md`, replace the Keys table's `tab` row and add the new rows:

```markdown
| `tab` | cycle Board → Analytics → Archive |
```

Add these two sections before the Storage section:

```markdown
## Task fields

Each task has a title, an optional one-line description, and an optional
deadline. Press `a` to open the form, `tab` and `shift+tab` to move between
the three fields, `enter` to save from anywhere, `esc` to cancel.

Deadlines are typed and displayed as `DD/MM/YYYY` — leave the field blank for
no deadline. The date on the card is colour-coded by how long you have left:

| Colour | Meaning |
|---|---|
| green | more than 3 days left |
| amber | 3 days or less |
| red | due today or tomorrow |
| red with ✗ | the deadline has passed |

A completed task shows its deadline in grey — finishing late is history, not
an ongoing emergency.

## Archive

Done tasks move to the Archive 14 days after you finish them, so the board
stays clean without losing anything. Sweeps run at startup and hourly while
gotodo is open. Press `tab` twice to browse the Archive; it is read-only.

Archived tasks still count toward every analytic — throughput, streak, cycle
time and the heatmap all keep their full history. Only the four column tiles
and the board itself hide them.
```

- [ ] **Step 9: Build and smoke-test by hand**

```bash
go vet ./...
go build -o gotodo .
./gotodo -file /tmp/gotodo-template-smoke.json
```

Manual checklist:
1. Press `a`. The three-field form appears with Title focused.
2. Type `Ship the report`, `tab`, type `draft and send`, `tab`, type a date three days out, `enter`. The card shows all three lines with an amber date.
3. Press `a` again, enter a title and a date already in the past, `enter`. That card's date is red with a ✗.
4. Press `a`, enter a title and the deadline `notadate`, `enter`. The form stays open and shows the format hint.
5. Press `e` on a card — all three fields are prefilled. Clear the deadline field, `enter`. The date line disappears.
6. Press `tab` three times: Analytics, Archive, back to Board.
7. Press `?` — the help lists the colour legend.
8. Press `q`, relaunch, confirm the description and deadline survived.

If any step fails, fix it before committing.

- [ ] **Step 10: Commit**

```bash
git add internal/ui/app.go internal/ui/app_test.go internal/ui/analytics.go internal/ui/analytics_test.go README.md
git commit -m "feat: add the Archive page, hourly sweep and deadline docs"
```

---

## Done Criteria

- `go test ./...`, `go vet ./...` pass; `gofmt -l .` prints nothing.
- A `tasks.json` written by the previous version loads without error, with every task showing no description and no deadline.
- Cards show title, description and deadline; the date is green / amber / red / red-with-✗ per the frozen thresholds, and grey once the task is done.
- The add/edit form has three fields with `tab` navigation; a malformed date keeps the form open with a readable message.
- `tab` cycles all three pages; the Archive lists tasks done more than 14 days ago and nothing else.
- Archived tasks are absent from the board and from the four analytics tiles, but still counted in throughput, streak, cycle time and the heatmap.
- The manual checklist in Task 9 Step 9 passes end to end.

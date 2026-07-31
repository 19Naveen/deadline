# Deadline Date Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user pick a deadline from a small month calendar instead of typing `DD/MM/YYYY` by hand.

**Architecture:** A self-contained `datePicker` value type in `internal/ui`, with pure navigation functions and a pure renderer, opened from the form's Deadline field with `ctrl+d`. The picker lives inside the existing `modeInput` rather than becoming a new board mode, so the root model's modal guard already covers it and `app.go` needs no changes at all.

**Tech Stack:** Go 1.22, lipgloss for styling. No new dependencies. Typing a date by hand keeps working exactly as it does today — the picker is additive.

## Global Constraints

- Module `gotodo`, Go 1.22. EXACTLY three direct dependencies: `bubbletea` v0.25.0, `lipgloss` v0.9.1, `bubbles` v0.18.0. A fourth is a defect.
- **Dates render as Singapore `DD/MM/YYYY`** — the picker writes into the field via the existing `FormatDate`, and the field is still read back by the existing `parseDeadline`. Never introduce a second date layout.
- **The picker never calls `time.Now()`.** It receives `today` as a parameter, and the form passes `m.now()`. This is what keeps the tests deterministic.
- No PII in fixtures or comments — bracketed placeholders only.
- Tests use the package-level `ref` (= `time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)`), never `time.Now()`.
- Do not redeclare any existing package-level test helper: `ref`, `chartRef`, `stripANSI`, `key`, `press`, `seeded`, `fixedClock`, `withFields`, `analyticsBoard`, `fixedAnalytics`, `archivedBoard`, `fixedArchive`.
- Run `go test ./...`, `go vet ./...` and `gofmt -l .` before every commit. gofmt must print nothing.

## Frozen Decisions

- **`ctrl+d` opens the picker**, and only while the Deadline field has focus. It must be a ctrl-key: any plain letter would be swallowed by the focused `textinput` and typed into the field.
- **Week starts Monday**, matching the analytics heatmap's existing `Mon..Sun` row order.
- **It opens on the date already in the field** if that parses, otherwise on today.
- **Keys inside the picker:** `h`/`l` or `←`/`→` move one day; `j`/`k` or `↓`/`↑` move one week; `[`/`]` move one month; `t` jumps to today; `enter` writes the highlighted date into the field and closes; `esc` closes and changes nothing; `x` clears the field and closes.
- **The picker stays inside `modeInput`.** Do not add a new `boardMode`. `AppModel`'s guard is `m.board.mode != modeNormal`, so `q`/`tab`/`?` are already modal while the picker is up, and `app.go` needs no edit.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `internal/ui/datepicker.go` | `datePicker` state, navigation, month grid, renderer | **Create** |
| `internal/ui/datepicker_test.go` | tests for all of the above | **Create** |
| `internal/ui/board.go` | `picker` field, `ctrl+d`, key routing while open, footer hint | Modify |
| `internal/ui/board_test.go` | integration tests for the form + picker | Modify |
| `README.md` | document `ctrl+d` and the picker keys | Modify |

---

## Task 1: Picker state and navigation

**Files:**
- Create: `internal/ui/datepicker.go`
- Create: `internal/ui/datepicker_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type datePicker struct { cursor time.Time; open bool }`
  - `func newDatePicker(seed time.Time) datePicker`
  - `func (p datePicker) move(days int) datePicker`
  - `func (p datePicker) addMonths(n int) datePicker`
  - `func daysInMonth(year int, m time.Month) int`
  - `func monthGrid(cursor time.Time) [6][7]time.Time` — Monday-first; a zero `time.Time` marks a cell outside the month

The one genuinely tricky part is month arithmetic. Go's `AddDate(0, 1, 0)` on 31 January yields 3 March, because it normalises the overflow rather than clamping. `addMonths` must clamp to the last day of the target month instead. There is a test for exactly this.

- [ ] **Step 1: Write the failing test**

Create `internal/ui/datepicker_test.go`:

```go
package ui

import (
	"testing"
	"time"
)

// day is a terse constructor for a midnight UTC date.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestNewDatePickerSeedsOnTheGivenDayAndOpens(t *testing.T) {
	p := newDatePicker(day(2026, time.August, 9))
	if !p.open {
		t.Error("open = false, want true")
	}
	if !p.cursor.Equal(day(2026, time.August, 9)) {
		t.Errorf("cursor = %v, want 09/08/2026", p.cursor)
	}
}

func TestNewDatePickerTruncatesTheSeedToMidnight(t *testing.T) {
	p := newDatePicker(time.Date(2026, time.August, 9, 17, 45, 3, 0, time.UTC))
	if !p.cursor.Equal(day(2026, time.August, 9)) {
		t.Errorf("cursor = %v, want midnight on 09/08/2026", p.cursor)
	}
}

func TestMoveByDaysCrossesMonthAndYearBoundaries(t *testing.T) {
	cases := []struct {
		name string
		from time.Time
		by   int
		want time.Time
	}{
		{"forward one day", day(2026, time.August, 9), 1, day(2026, time.August, 10)},
		{"back one day", day(2026, time.August, 9), -1, day(2026, time.August, 8)},
		{"forward one week", day(2026, time.August, 9), 7, day(2026, time.August, 16)},
		{"over the month end", day(2026, time.August, 31), 1, day(2026, time.September, 1)},
		{"back over the month start", day(2026, time.August, 1), -1, day(2026, time.July, 31)},
		{"over the year end", day(2026, time.December, 31), 1, day(2027, time.January, 1)},
	}
	for _, c := range cases {
		got := datePicker{cursor: c.from, open: true}.move(c.by).cursor
		if !got.Equal(c.want) {
			t.Errorf("%s: move(%d) = %v, want %v", c.name, c.by, got, c.want)
		}
	}
}

func TestAddMonthsClampsToTheLastDayOfTheTargetMonth(t *testing.T) {
	cases := []struct {
		name string
		from time.Time
		by   int
		want time.Time
	}{
		// Go's AddDate would give 03/03 here. Clamping is the whole point.
		{"31 Jan forward one month", day(2026, time.January, 31), 1, day(2026, time.February, 28)},
		{"31 Mar back one month", day(2026, time.March, 31), -1, day(2026, time.February, 28)},
		{"31 Aug forward one month", day(2026, time.August, 31), 1, day(2026, time.September, 30)},
		{"leap year February", day(2024, time.January, 31), 1, day(2024, time.February, 29)},
		{"ordinary month keeps its day", day(2026, time.August, 9), 1, day(2026, time.September, 9)},
		{"across the year end", day(2026, time.December, 15), 1, day(2027, time.January, 15)},
		{"across the year start", day(2026, time.January, 15), -1, day(2025, time.December, 15)},
	}
	for _, c := range cases {
		got := datePicker{cursor: c.from, open: true}.addMonths(c.by).cursor
		if !got.Equal(c.want) {
			t.Errorf("%s: addMonths(%d) = %v, want %v", c.name, c.by, got, c.want)
		}
	}
}

func TestDaysInMonth(t *testing.T) {
	cases := []struct {
		year int
		mon  time.Month
		want int
	}{
		{2026, time.January, 31},
		{2026, time.February, 28},
		{2024, time.February, 29},
		{2000, time.February, 29},
		{1900, time.February, 28},
		{2026, time.April, 30},
	}
	for _, c := range cases {
		if got := daysInMonth(c.year, c.mon); got != c.want {
			t.Errorf("daysInMonth(%d, %v) = %d, want %d", c.year, c.mon, got, c.want)
		}
	}
}

func TestMonthGridPlacesDaysMondayFirst(t *testing.T) {
	// 01/08/2026 is a Saturday, so the first row is blank until column 5.
	g := monthGrid(day(2026, time.August, 9))

	if !g[0][5].Equal(day(2026, time.August, 1)) {
		t.Errorf("g[0][5] = %v, want 01/08/2026 (Saturday)", g[0][5])
	}
	if !g[0][6].Equal(day(2026, time.August, 2)) {
		t.Errorf("g[0][6] = %v, want 02/08/2026 (Sunday)", g[0][6])
	}
	for col := 0; col < 5; col++ {
		if !g[0][col].IsZero() {
			t.Errorf("g[0][%d] = %v, want a blank cell before the 1st", col, g[0][col])
		}
	}
	if !g[1][0].Equal(day(2026, time.August, 3)) {
		t.Errorf("g[1][0] = %v, want 03/08/2026 (the first Monday)", g[1][0])
	}
}

func TestMonthGridHoldsEveryDayExactlyOnce(t *testing.T) {
	g := monthGrid(day(2026, time.August, 9))
	seen := map[int]int{}
	for _, row := range g {
		for _, cell := range row {
			if cell.IsZero() {
				continue
			}
			if cell.Month() != time.August {
				t.Errorf("grid contains %v, from outside the month", cell)
			}
			seen[cell.Day()]++
		}
	}
	if len(seen) != 31 {
		t.Errorf("grid holds %d distinct days, want 31", len(seen))
	}
	for d, n := range seen {
		if n != 1 {
			t.Errorf("day %d appears %d times, want once", d, n)
		}
	}
}

func TestMonthGridFitsAMonthStartingSunday(t *testing.T) {
	// 01/02/2026 is a Sunday — the largest possible lead for a Monday-first
	// grid, pushing the 1st all the way into column 6.
	g := monthGrid(day(2026, time.February, 1))
	if !g[0][6].Equal(day(2026, time.February, 1)) {
		t.Errorf("g[0][6] = %v, want 01/02/2026", g[0][6])
	}
	// Lead 6 plus 28 days ends at index 33, so the last day sits at row 4,
	// column 5. 28/02/2026 is a Saturday, not a Sunday.
	if !g[4][5].Equal(day(2026, time.February, 28)) {
		t.Errorf("g[4][5] = %v, want 28/02/2026 in the last populated cell", g[4][5])
	}
	if !g[4][6].IsZero() {
		t.Errorf("g[4][6] = %v, want blank — February 2026 ends on the Saturday", g[4][6])
	}
}

func TestMonthGridHandlesTheSixRowWorstCase(t *testing.T) {
	// A 31-day month starting on a Sunday needs all six rows: lead 6 plus
	// 31 days ends at index 36, which is row 5.
	g := monthGrid(day(2026, time.March, 1)) // 01/03/2026 is a Sunday
	if !g[0][6].Equal(day(2026, time.March, 1)) {
		t.Errorf("g[0][6] = %v, want 01/03/2026", g[0][6])
	}
	if !g[5][1].Equal(day(2026, time.March, 31)) {
		t.Errorf("g[5][1] = %v, want 31/03/2026 in the sixth row", g[5][1])
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestNewDatePicker|TestMove|TestAddMonths|TestDaysInMonth|TestMonthGrid' -v`
Expected: FAIL — `undefined: newDatePicker`.

- [ ] **Step 3: Write the implementation**

Create `internal/ui/datepicker.go`:

```go
package ui

import "time"

// datePicker is the little month calendar shown under the Deadline field.
// It holds only a cursor: the month on screen is whichever month the cursor
// is in, so navigation and display can never disagree.
type datePicker struct {
	cursor time.Time
	open   bool
}

// newDatePicker opens a picker on the given day, truncated to midnight.
func newDatePicker(seed time.Time) datePicker {
	y, m, d := seed.Date()
	return datePicker{
		cursor: time.Date(y, m, d, 0, 0, 0, 0, seed.Location()),
		open:   true,
	}
}

// move shifts the cursor by whole days, rolling across months and years.
func (p datePicker) move(days int) datePicker {
	p.cursor = p.cursor.AddDate(0, 0, days)
	return p
}

// addMonths shifts the cursor by whole months, clamping the day to the end
// of the target month. Go's AddDate normalises instead of clamping — it
// turns 31 January plus one month into 3 March — which is never what a
// calendar should do.
func (p datePicker) addMonths(n int) datePicker {
	y, m, d := p.cursor.Date()
	// Stepping from the 1st avoids the same overflow while we find the month.
	target := time.Date(y, m, 1, 0, 0, 0, 0, p.cursor.Location()).AddDate(0, n, 0)
	if last := daysInMonth(target.Year(), target.Month()); d > last {
		d = last
	}
	p.cursor = time.Date(target.Year(), target.Month(), d, 0, 0, 0, 0, p.cursor.Location())
	return p
}

// daysInMonth is the length of the given month, leap years included. Day 0
// of the following month is the last day of this one.
func daysInMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// monthGrid lays the cursor's month out as six Monday-first weeks. Cells
// outside the month are the zero time, which the renderer draws as blank.
// Six rows always suffice: the worst case is a 31-day month starting on a
// Sunday, which needs 6 leading blanks plus 31 days — 37 of 42 cells.
func monthGrid(cursor time.Time) [6][7]time.Time {
	var g [6][7]time.Time
	loc := cursor.Location()
	first := time.Date(cursor.Year(), cursor.Month(), 1, 0, 0, 0, 0, loc)
	lead := (int(first.Weekday()) + 6) % 7 // Go puts Sunday at 0; we want Monday at 0.

	for d := 1; d <= daysInMonth(cursor.Year(), cursor.Month()); d++ {
		i := lead + d - 1
		g[i/7][i%7] = time.Date(cursor.Year(), cursor.Month(), d, 0, 0, 0, 0, loc)
	}
	return g
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS, all new tests plus every pre-existing one.

- [ ] **Step 5: Commit**

```bash
go vet ./... && gofmt -l .
git add internal/ui/datepicker.go internal/ui/datepicker_test.go
git commit -m "feat: add date picker state and month arithmetic"
```

---

## Task 2: Rendering the calendar

**Files:**
- Modify: `internal/ui/datepicker.go` (append)
- Modify: `internal/ui/datepicker_test.go` (append)

**Interfaces:**
- Consumes: `datePicker`, `monthGrid` from Task 1; `MutedStyle`, `TitleStyle`, `ColAccent`, `AccentFor` from `theme.go`.
- Produces: `func (p datePicker) View(today time.Time) string`

`View` takes `today` rather than reading the clock, so the "today" marker is deterministic in tests. Output is 21 columns wide, which fits under the form at every terminal width the board allows.

**Every cell is exactly three columns: a marker slot, then the day right-aligned in two.** The marker is `>` for the cursor and a space otherwise. This is what keeps the grid aligned — a bracketed `[9]` would be wider than a plain ` 9` and would shove its whole row one column right of the header. Seven three-column cells give a 21-column row, and the weekday header is padded to match.

Layout:

```
    August 2026
 Mo Tu We Th Fr Sa Su
                 1  2
  3  4  5  6  7  8> 9
 10 11 12 13 14 15 16
 17 18 19 20 21 22 23
 24 25 26 27 28 29 30
 31
```

The cursor day renders in the accent colour and bold, with the `>` marker so it is findable without relying on colour. Today renders in the "done" green when it is not the cursor. Everything else is plain.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/datepicker_test.go`:

```go
func TestPickerViewShowsMonthYearAndWeekdayHeader(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 9)))

	if !strings.Contains(out, "August 2026") {
		t.Errorf("View missing the month heading:\n%s", out)
	}
	if !strings.Contains(out, "Mo Tu We Th Fr Sa Su") {
		t.Errorf("View missing the Monday-first weekday header:\n%s", out)
	}
}

func TestPickerViewMarksTheCursor(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 1)))
	if !strings.Contains(out, "> 9") {
		t.Errorf("View does not mark the cursor day:\n%s", out)
	}
}

func TestPickerViewHasOneCursorOnly(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 1)))
	if got := strings.Count(out, ">"); got != 1 {
		t.Errorf("View marks %d days, want exactly 1:\n%s", got, out)
	}
}

func TestPickerViewRowsAllHaveTheSameWidth(t *testing.T) {
	// The cursor marker must not widen its row. Put the cursor on a day in
	// the middle of a full week so any extra column would show up.
	out := newDatePicker(day(2026, time.August, 12)).View(day(2026, time.August, 1))
	lines := strings.Split(out, "\n")

	header := lipgloss.Width(lines[1]) // the Mo Tu We … row
	for i, line := range lines[2:] {
		if w := lipgloss.Width(line); w != header && strings.TrimSpace(stripANSI(line)) != "" {
			t.Errorf("week row %d is %d columns, want %d to match the header: %q",
				i, w, header, stripANSI(line))
		}
	}
}

func TestPickerViewShowsEveryDayOfTheMonth(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 9)))
	for _, d := range []string{"1", "15", "31"} {
		if !strings.Contains(out, d) {
			t.Errorf("View missing day %s:\n%s", d, out)
		}
	}
	if strings.Contains(out, "32") {
		t.Errorf("View shows a day past the end of the month:\n%s", out)
	}
}

func TestPickerViewFitsTwentyOneColumns(t *testing.T) {
	// February 2026 starts on a Sunday — the widest leading-blank case.
	out := newDatePicker(day(2026, time.February, 1)).View(day(2026, time.February, 1))
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 21 {
			t.Errorf("line is %d columns, want at most 21: %q", w, stripANSI(line))
		}
	}
}

func TestPickerViewRendersEveryMonthWithoutPanicking(t *testing.T) {
	for m := time.January; m <= time.December; m++ {
		for _, y := range []int{2024, 2026} { // one leap year, one not
			out := newDatePicker(day(y, m, 1)).View(day(2026, time.July, 30))
			if out == "" {
				t.Errorf("%v %d rendered empty", m, y)
			}
		}
	}
}
```

`datepicker_test.go` needs `"strings"` and `"github.com/charmbracelet/lipgloss"` added to its imports. `stripANSI` already exists in `chart_test.go` — do not redeclare it.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run TestPickerView -v`
Expected: FAIL — `p.View undefined`.

- [ ] **Step 3: Write the implementation**

Append to `internal/ui/datepicker.go`:

```go
// pickerWidth is the rendered width: seven cells of three columns each.
const pickerWidth = 21

// View renders the month as a Monday-first grid. today is passed in rather
// than read from the clock so tests stay deterministic.
//
// Every cell is exactly three columns — one marker slot plus the day right
// aligned in two — so the cursor marker can never shift a row out of line
// with the header. Rows are padded, not trimmed, for the same reason.
func (p datePicker) View(today time.Time) string {
	title := p.cursor.Format("January 2006")
	pad := (pickerWidth - len(title)) / 2
	if pad < 0 {
		pad = 0
	}

	rows := []string{
		strings.Repeat(" ", pad) + TitleStyle.Render(title),
		MutedStyle.Render(" Mo Tu We Th Fr Sa Su"),
	}

	for _, week := range monthGrid(p.cursor) {
		var b strings.Builder
		blank := true
		for _, cell := range week {
			if cell.IsZero() {
				b.WriteString("   ")
				continue
			}
			blank = false
			day := fmt.Sprintf("%2d", cell.Day())
			switch {
			case sameDay(cell, p.cursor):
				b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColAccent).
					Render(">" + day))
			case sameDay(cell, today):
				b.WriteString(" " + lipgloss.NewStyle().
					Foreground(AccentFor(task.StatusDone)).Render(day))
			default:
				b.WriteString(" " + day)
			}
		}
		if blank {
			continue // a wholly empty trailing week
		}
		rows = append(rows, b.String())
	}
	return strings.Join(rows, "\n")
}

// sameDay reports whether two times fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
```

Add `"fmt"`, `"strings"`, `"github.com/charmbracelet/lipgloss"` and `"gotodo/internal/task"` to `datepicker.go`'s imports.

Note the marker sits **inside** each cell's three columns rather than replacing a separator, which is what makes `TestPickerViewRowsAllHaveTheSameWidth` pass. If that test fails, the cell width is wrong — fix the rendering, not the test.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/ui/ -v`
Expected: PASS. If `TestPickerViewFitsTwentyColumns` fails, the bracketed cursor is widening its row — fix the padding, not the test.

- [ ] **Step 5: Commit**

```bash
go vet ./... && gofmt -l .
git add internal/ui/datepicker.go internal/ui/datepicker_test.go
git commit -m "feat: render the date picker month grid"
```

---

## Task 3: Wire the picker into the form

**Files:**
- Modify: `internal/ui/board.go`
- Modify: `internal/ui/board_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `datePicker`, `newDatePicker`, `(datePicker).move`, `(datePicker).addMonths`, `(datePicker).View` from Tasks 1-2; the existing `parseDeadline`, `FormatDate`, `openForm`, `closeForm`.
- Produces: `BoardModel.picker datePicker`; `func (m BoardModel) updatePicker(k tea.KeyMsg) (BoardModel, tea.Cmd)`; `ctrl+d` handling in `updateInput`.

Routing rule: at the very top of `updateInput`, if the picker is open, every key goes to `updatePicker` and nothing else runs. That keeps `enter` inside the picker from saving the task.

The picker must also be closed by `closeForm`, or it would still be open the next time the form opens.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/board_test.go`:

```go
func TestCtrlDOpensThePickerOnlyOnTheDeadlineField(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.picker.open {
		t.Error("picker opened from the Title field, want it to stay closed")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // now on Deadline
	if m.field != fieldDeadline {
		t.Fatalf("field = %d, want fieldDeadline", m.field)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !m.picker.open {
		t.Error("picker did not open on the Deadline field")
	}
}

func TestPickerOpensOnTodayWhenTheFieldIsEmpty(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if !sameDay(m.picker.cursor, ref) {
		t.Errorf("cursor = %v, want today (%v)", m.picker.cursor, ref)
	}
}

func TestPickerOpensOnTheDateAlreadyTyped(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m.inputs[fieldDeadline].SetValue("09/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	if m.picker.cursor.Day() != 9 || m.picker.cursor.Month() != time.August {
		t.Errorf("cursor = %v, want 09/08/2026 from the field", m.picker.cursor)
	}
}

func TestPickerEnterWritesTheDateAndCloses(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	// ref is 30/07/2026; two days forward is 01/08/2026.
	m, _ = m.Update(key("l"))
	m, _ = m.Update(key("l"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.picker.open {
		t.Error("picker still open after enter")
	}
	if got := m.inputs[fieldDeadline].Value(); got != "01/08/2026" {
		t.Errorf("deadline field = %q, want %q", got, "01/08/2026")
	}
	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput — enter picks a date, it must not save the task", m.mode)
	}
	if len(m.board.Tasks) != 0 {
		t.Errorf("Tasks = %d, want 0 — enter in the picker must not save", len(m.board.Tasks))
	}
}

func TestPickerEscClosesAndChangesNothing(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m.inputs[fieldDeadline].SetValue("09/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = m.Update(key("l"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.picker.open {
		t.Error("picker still open after esc")
	}
	if got := m.inputs[fieldDeadline].Value(); got != "09/08/2026" {
		t.Errorf("deadline field = %q, want it untouched", got)
	}
	if m.mode != modeInput {
		t.Errorf("mode = %v, want modeInput — esc closes the picker, not the form", m.mode)
	}
}

func TestPickerXClearsTheField(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m.inputs[fieldDeadline].SetValue("09/08/2026")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = m.Update(key("x"))

	if m.picker.open {
		t.Error("picker still open after x")
	}
	if got := m.inputs[fieldDeadline].Value(); got != "" {
		t.Errorf("deadline field = %q, want empty after clearing", got)
	}
}

func TestPickerNavigationKeys(t *testing.T) {
	// ref is Thursday 30/07/2026.
	cases := []struct {
		keys  []string
		want  time.Time
		label string
	}{
		{[]string{"l"}, time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC), "l is one day forward"},
		{[]string{"h"}, time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC), "h is one day back"},
		{[]string{"j"}, time.Date(2026, time.August, 6, 0, 0, 0, 0, time.UTC), "j is one week forward"},
		{[]string{"k"}, time.Date(2026, time.July, 23, 0, 0, 0, 0, time.UTC), "k is one week back"},
		{[]string{"]"}, time.Date(2026, time.August, 30, 0, 0, 0, 0, time.UTC), "] is one month forward"},
		{[]string{"["}, time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC), "[ is one month back"},
		{[]string{"]", "t"}, time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC), "t returns to today"},
	}
	for _, c := range cases {
		m := fixedClock(NewBoardModel(&task.Board{}))
		m = press(m, "a")
		m.field = fieldDeadline
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
		for _, k := range c.keys {
			m, _ = m.Update(key(k))
		}
		if !sameDay(m.picker.cursor, c.want) {
			t.Errorf("%s: cursor = %v, want %v", c.label, m.picker.cursor, c.want)
		}
	}
}

func TestPickerAppearsInTheFormView(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m.SetSize(120, 40)
	m = press(m, "a")
	m.field = fieldDeadline

	before := stripANSI(m.View())
	if strings.Contains(before, "Mo Tu We Th Fr Sa Su") {
		t.Error("the calendar is visible before ctrl+d")
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	after := stripANSI(m.View())
	if !strings.Contains(after, "Mo Tu We Th Fr Sa Su") {
		t.Errorf("the calendar did not appear after ctrl+d:\n%s", after)
	}
	if !strings.Contains(after, "July 2026") {
		t.Errorf("the calendar is not showing the current month:\n%s", after)
	}
}

func TestClosingTheFormAlsoClosesThePicker(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m = press(m, "a")
	m.field = fieldDeadline
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // closes the picker
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc}) // closes the form

	if m.mode != modeNormal {
		t.Fatalf("mode = %v, want modeNormal", m.mode)
	}
	m = press(m, "a")
	if m.picker.open {
		t.Error("the picker is open on a freshly opened form")
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test ./internal/ui/ -run 'TestCtrlD|TestPicker|TestClosingTheForm' -v`
Expected: FAIL — `m.picker undefined`.

- [ ] **Step 3: Add the field and the routing**

In `internal/ui/board.go`, add one field to `BoardModel`, immediately after `field int`:

```go
	picker datePicker // the deadline calendar, when open
```

In `closeForm`, add one line so the picker never survives the form:

```go
	m.picker = datePicker{}
```

At the very top of `updateInput`, before its `switch`, add:

```go
	// While the calendar is up it owns every key — otherwise enter would
	// save the task instead of picking a date.
	if m.picker.open {
		return m.updatePicker(k)
	}
```

Add a case to `updateInput`'s `switch k.Type`, alongside the existing `tea.KeyEsc` and `tea.KeyTab` cases:

```go
	case tea.KeyCtrlD:
		if m.field != fieldDeadline {
			return m, nil
		}
		seed := m.now()
		if d, err := parseDeadline(m.inputs[fieldDeadline].Value()); err == nil && d != nil {
			seed = *d
		}
		m.picker = newDatePicker(seed)
		m.err = ""
		return m, nil
```

- [ ] **Step 4: Add the picker's key handler**

Append to `internal/ui/board.go`:

```go
// updatePicker handles keys while the deadline calendar is open. It never
// saves the task: enter picks a date and hands control back to the form.
func (m BoardModel) updatePicker(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		m.picker = datePicker{}
		return m, nil

	case tea.KeyEnter:
		m.inputs[fieldDeadline].SetValue(FormatDate(m.picker.cursor))
		m.inputs[fieldDeadline].CursorEnd()
		m.picker = datePicker{}
		return m, nil

	case tea.KeyLeft:
		m.picker = m.picker.move(-1)
		return m, nil
	case tea.KeyRight:
		m.picker = m.picker.move(1)
		return m, nil
	case tea.KeyUp:
		m.picker = m.picker.move(-7)
		return m, nil
	case tea.KeyDown:
		m.picker = m.picker.move(7)
		return m, nil
	}

	switch k.String() {
	case "h":
		m.picker = m.picker.move(-1)
	case "l":
		m.picker = m.picker.move(1)
	case "k":
		m.picker = m.picker.move(-7)
	case "j":
		m.picker = m.picker.move(7)
	case "[":
		m.picker = m.picker.addMonths(-1)
	case "]":
		m.picker = m.picker.addMonths(1)
	case "t":
		m.picker = newDatePicker(m.now())
	case "x":
		m.inputs[fieldDeadline].SetValue("")
		m.picker = datePicker{}
	}
	return m, nil
}
```

- [ ] **Step 5: Show the calendar and its hint in the form**

In `renderForm`, replace the footer line and add the calendar. The existing final two statements are the `rows = append(rows, MutedStyle.Render("tab/shift+tab field · enter save · esc cancel"))` line and the `return`. Replace them with:

```go
	if m.picker.open {
		rows = append(rows, "", m.picker.View(m.now()))
		rows = append(rows, MutedStyle.Render(
			"hjkl day/week · [ ] month · t today · enter pick · x clear · esc close"))
	} else {
		hint := "tab/shift+tab field · enter save · esc cancel"
		if m.field == fieldDeadline {
			hint = "ctrl+d calendar · " + hint
		}
		rows = append(rows, MutedStyle.Render(hint))
	}
	return ColumnStyle.Render(strings.Join(rows, "\n"))
```

- [ ] **Step 6: Run the tests and confirm they pass**

Run: `go test ./... -v`
Expected: PASS across all three packages, including every pre-existing form test.

- [ ] **Step 7: Check the whole tree**

```bash
go vet ./... && gofmt -l . && go build -o /tmp/gotodo-dp . && rm -f /tmp/gotodo-dp
```

Expected: vet clean, gofmt silent, build succeeds.

- [ ] **Step 8: Update the README**

In `README.md`, in the paragraph under the Keys table that begins "Dates go in as `DD/MM/YYYY`", add a sentence after it:

```markdown
Or do not type them at all. With the cursor in the Deadline field, `ctrl+d`
opens a small calendar: `hjkl` moves a day or a week, `[` and `]` change
month, `t` jumps to today, `enter` picks, `x` clears the date, `esc` closes.
```

Also add a row to the Keys table, directly under the `m` row:

```markdown
| `ctrl+d` | open the calendar (Deadline field only) |
```

- [ ] **Step 9: Commit**

```bash
git add internal/ui/board.go internal/ui/board_test.go README.md
git commit -m "feat: pick deadlines from a calendar with ctrl+d"
```

---

## Done Criteria

- `go test ./...`, `go vet ./...` pass; `gofmt -l .` prints nothing.
- `ctrl+d` on the Deadline field opens a month calendar; it does nothing on the other two fields.
- The calendar opens on the date already typed, or today when the field is empty.
- `hjkl` and the arrow keys move by day and week; `[`/`]` move by month with the day clamped to the month's length; `t` returns to today.
- `enter` writes `DD/MM/YYYY` into the field and closes the calendar **without** saving the task; `esc` closes it and changes nothing; `x` clears the field.
- Typing a date by hand still works exactly as before.
- The calendar never survives the form closing.

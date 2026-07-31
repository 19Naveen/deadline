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

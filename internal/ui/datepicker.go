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

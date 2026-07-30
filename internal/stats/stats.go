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

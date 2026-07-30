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

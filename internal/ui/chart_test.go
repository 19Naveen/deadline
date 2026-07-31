package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

// chartRef is this file's fixed clock. board_test.go owns the package-level
// `ref`; this name avoids colliding with it.
var chartRef = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes lipgloss's colour escapes so assertions can match the
// visible text.
func stripANSI(s string) string { return ansiRE.ReplaceAllString(s, "") }

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
		45 * time.Second:  "0m",
		90 * time.Second:  "1m",
		3 * time.Hour:     "3h",
		30 * time.Hour:    "1d 6h",
		0:                 "0m",
		-90 * time.Minute: "-1h",
		-30 * time.Hour:   "-1d 6h",
		minDuration:       "-106751d 23h",
	}
	for d, want := range cases {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

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

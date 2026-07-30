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
		45 * time.Second: "0m",
		90 * time.Second: "1m",
		3 * time.Hour:    "3h",
		30 * time.Hour:   "1d 6h",
		0:                "0m",
	}
	for d, want := range cases {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

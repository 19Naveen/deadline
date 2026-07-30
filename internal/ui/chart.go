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

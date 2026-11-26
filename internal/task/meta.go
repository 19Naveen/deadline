package task

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// String is the priority's CLI name: none, low, medium or high.
func (p Priority) String() string {
	switch p {
	case PriorityNone:
		return "none"
	case PriorityLow:
		return "low"
	case PriorityMedium:
		return "medium"
	case PriorityHigh:
		return "high"
	}
	return fmt.Sprintf("Priority(%d)", int(p))
}

// ParsePriority reads a priority name, case-insensitively. It accepts the
// full names (none, low, medium, high), their initials (l, m, h), "med", and
// the P-scale where p0 and p1 are high, p2 medium and p3 low. Blank is none.
func ParsePriority(s string) (Priority, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "none", "-":
		return PriorityNone, nil
	case "low", "l", "p3":
		return PriorityLow, nil
	case "medium", "med", "m", "p2":
		return PriorityMedium, nil
	case "high", "h", "p1", "p0":
		return PriorityHigh, nil
	}
	return PriorityNone, fmt.Errorf("bad priority %q (want none, low, medium or high)", s)
}

// valid reports whether p is one of the four defined priorities.
func (p Priority) valid() bool { return p >= PriorityNone && p <= PriorityHigh }

// NormalizeLabel trims a label, drops one leading '#' and lowercases it.
// The result is "" for a blank label.
func NormalizeLabel(label string) string {
	label = strings.TrimSpace(label)
	label = strings.TrimPrefix(label, "#")
	return strings.ToLower(strings.TrimSpace(label))
}

// NormalizeLabels normalizes each label, drops blanks and duplicates, and
// keeps first-seen order. It returns nil when nothing is left.
func NormalizeLabels(labels []string) []string {
	var out []string
	for _, l := range labels {
		if l = NormalizeLabel(l); l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// HasLabel reports whether the task carries label (compared normalized).
func (t Task) HasLabel(label string) bool {
	return slices.Contains(t.Labels, NormalizeLabel(label))
}

// SetPriority sets a task's priority; PriorityNone clears it. Unchanged
// values are a no-op.
func (b *Board) SetPriority(id string, p Priority, now time.Time) error {
	if !p.valid() {
		return fmt.Errorf("bad priority %d", int(p))
	}
	t, err := b.find(id)
	if err != nil {
		return err
	}
	if t.Priority == p {
		return nil
	}
	t.Priority = p
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

// SetLabels replaces a task's labels with their normalized form (see
// NormalizeLabels); an empty list clears them. Unchanged values are a no-op.
func (b *Board) SetLabels(id string, labels []string, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	labels = NormalizeLabels(labels)
	if slices.Equal(t.Labels, labels) {
		return nil
	}
	t.Labels = labels
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

// SortByPriority orders tasks highest priority first, in place. The sort
// is stable, so equal priorities keep their existing (insertion) order.
func SortByPriority(tasks []Task) {
	slices.SortStableFunc(tasks, func(x, y Task) int {
		return int(y.Priority) - int(x.Priority)
	})
}

// WIPLimit is the column's card cap, or 0 for no limit.
func (b *Board) WIPLimit(s Status) int {
	if n := b.WIPLimits[s]; n > 0 {
		return n
	}
	return 0
}

// OverWIP reports a column's active (non-archived) card count, its WIP
// limit (0 for none), and whether the count exceeds the limit. Moving into
// a full column is allowed; callers use this to warn.
func (b *Board) OverWIP(s Status) (count, limit int, over bool) {
	count = len(b.ByStatus(s))
	limit = b.WIPLimit(s)
	return count, limit, limit > 0 && count > limit
}

// Package report builds paste-ready summaries of board activity. Functions
// take an explicit now so output is deterministic.
package report

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gotodo/internal/task"
)

const (
	dateLayout     = "02/01/2006"
	dateTimeLayout = "02/01/2006 15:04"
	timeLayout     = "15:04"
)

// ParseSince parses a look-back window: any time.ParseDuration value
// ("24h", "90m") or whole days ("7d").
func ParseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	} else if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d, nil
	}
	return 0, fmt.Errorf("bad -since %q (want e.g. 24h, 90m or 7d)", s)
}

// Item is one standup line.
type Item struct {
	ID    string
	Title string
	Text  string // the detail after the title, e.g. "todo → indev by claude"

	at     time.Time // sort key
	column int       // sort key for In progress
}

// Section is a titled group of items.
type Section struct {
	Title string
	Items []Item
}

// Standup is the report for one window.
type Standup struct {
	Now, Since time.Time
	Sections   []Section // only non-empty sections, in display order
}

// Empty reports whether there is nothing to say.
func (s Standup) Empty() bool { return len(s.Sections) == 0 }

func who(agent string) string {
	if agent == "" {
		return "you"
	}
	return agent
}

// stamp renders a time compactly: HH:MM today, else DD/MM/YYYY HH:MM.
func stamp(at, now time.Time) string {
	at = at.In(now.Location())
	if task.StartOfDay(at).Equal(task.StartOfDay(now)) {
		return at.Format(timeLayout)
	}
	return at.Format(dateTimeLayout)
}

// isActive reports whether s is an in-progress column: not the first,
// blocked or terminal column.
func isActive(b *task.Board, s task.Status) bool {
	return b.ColumnIndex(s) > 0 && s != task.StatusBlocked && s != b.DoneStatus()
}

// lastEntered is when the task last moved into its current column, falling
// back to UpdatedAt for tasks without history.
func lastEntered(t task.Task) time.Time {
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].To == t.Status {
			return t.History[i].At
		}
	}
	return t.UpdatedAt
}

// FormatDuration renders "12m", "3h" or "1d 6h".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days, hours := int(d.Hours())/24, int(d.Hours())%24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}

// BuildStandup reports activity since `since`: Done, Moved, Blocked now,
// Due today/overdue and In progress. Empty sections are omitted.
func BuildStandup(b *task.Board, now, since time.Time) Standup {
	done := b.DoneStatus()
	var finished, moved, blocked, due, inProgress []Item
	for _, t := range b.Tasks {
		completedAt, isDone := task.CompletedAt(t, done)
		countedDone := isDone && !completedAt.Before(since) && !completedAt.After(now)
		if countedDone {
			text := ""
			for i := len(t.History) - 1; i >= 0; i-- {
				if t.History[i].To == done {
					text = "by " + who(t.History[i].Agent) + ", " + stamp(completedAt, now)
					break
				}
			}
			finished = append(finished, Item{ID: t.ID, Title: t.Title, Text: text, at: completedAt})
		}
		for _, tr := range t.History {
			if tr.At.Before(since) || tr.At.After(now) {
				continue
			}
			if countedDone && tr.To == done && tr.At.Equal(completedAt) {
				continue
			}
			moved = append(moved, Item{ID: t.ID, Title: t.Title, at: tr.At,
				Text: fmt.Sprintf("%s → %s by %s, %s", tr.From, tr.To, who(tr.Agent), stamp(tr.At, now))})
		}
		if t.Archived {
			continue
		}
		if t.Status == task.StatusBlocked {
			entered := lastEntered(t)
			blocked = append(blocked, Item{ID: t.ID, Title: t.Title, at: entered,
				Text: "blocked for " + FormatDuration(now.Sub(entered))})
		}
		if t.Status != done {
			if days, ok := task.DaysUntilDeadline(t, now); ok && days <= 0 {
				text := "due today"
				if days < 0 {
					text = fmt.Sprintf("overdue %dd (due %s)", -days, t.Deadline.In(now.Location()).Format(dateLayout))
				}
				due = append(due, Item{ID: t.ID, Title: t.Title, Text: text, at: *t.Deadline})
			}
		}
		if isActive(b, t.Status) {
			text := "[" + string(t.Status) + "]"
			if t.ClaimedBy != "" && (t.ClaimUntil == nil || t.ClaimUntil.After(now)) {
				text += " claimed by " + t.ClaimedBy
				if t.ClaimUntil != nil {
					text += " until " + stamp(*t.ClaimUntil, now)
				}
			}
			if t.PR != "" {
				text += " · PR " + t.PR
			}
			inProgress = append(inProgress, Item{ID: t.ID, Title: t.Title, Text: text, column: b.ColumnIndex(t.Status)})
		}
	}
	byTime := func(x, y Item) int { return x.at.Compare(y.at) }
	slices.SortStableFunc(finished, byTime)
	slices.SortStableFunc(moved, byTime)
	slices.SortStableFunc(blocked, byTime) // longest blocked first
	slices.SortStableFunc(due, byTime)     // most overdue first
	slices.SortStableFunc(inProgress, func(x, y Item) int { return x.column - y.column })

	s := Standup{Now: now, Since: since}
	for _, sec := range []Section{
		{"Done", finished},
		{"Moved", moved},
		{"Blocked now", blocked},
		{"Due today/overdue", due},
		{"In progress", inProgress},
	} {
		if len(sec.Items) > 0 {
			s.Sections = append(s.Sections, sec)
		}
	}
	return s
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// oneLine keeps a title on one line so pasted reports don't break apart.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func (s Standup) header() string {
	return fmt.Sprintf("Standup %s (since %s)", s.Now.Format(dateTimeLayout), s.Since.In(s.Now.Location()).Format(dateTimeLayout))
}

// Text renders plain text, paste-ready for chat.
func (s Standup) Text() string {
	var sb strings.Builder
	sb.WriteString(s.header() + "\n")
	if s.Empty() {
		sb.WriteString("\nNothing to report\n")
		return sb.String()
	}
	for _, sec := range s.Sections {
		sb.WriteString("\n" + sec.Title + "\n")
		for _, it := range sec.Items {
			fmt.Fprintf(&sb, "- %s (%s)", oneLine(it.Title), shortID(it.ID))
			if it.Text != "" {
				sb.WriteString(": " + it.Text)
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// Markdown renders Markdown for Teams, Slack or a PR comment.
func (s Standup) Markdown() string {
	var sb strings.Builder
	sb.WriteString("**" + s.header() + "**\n")
	if s.Empty() {
		sb.WriteString("\n_Nothing to report_\n")
		return sb.String()
	}
	for _, sec := range s.Sections {
		sb.WriteString("\n**" + sec.Title + "**\n")
		for _, it := range sec.Items {
			fmt.Fprintf(&sb, "- %s `%s`", oneLine(it.Title), shortID(it.ID))
			if it.Text != "" {
				sb.WriteString(": " + it.Text)
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

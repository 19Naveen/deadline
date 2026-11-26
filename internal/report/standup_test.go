package report

import (
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

var now = time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC)

func at(h float64) time.Time { return now.Add(time.Duration(h * float64(time.Hour))) }

func TestParseSince(t *testing.T) {
	ok := map[string]time.Duration{"24h": 24 * time.Hour, "90m": 90 * time.Minute, "7d": 7 * 24 * time.Hour}
	for in, want := range ok {
		if got, err := ParseSince(in); err != nil || got != want {
			t.Errorf("ParseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0h", "-1h", "xd", "0d", "soon"} {
		if _, err := ParseSince(bad); err == nil {
			t.Errorf("ParseSince(%q) accepted", bad)
		}
	}
}

// fixture builds a dev board with one task per standup section.
func fixture(t *testing.T) (*task.Board, map[string]string) {
	t.Helper()
	b := &task.Board{}
	b.SetColumns(task.DevColumns)
	ids := map[string]string{}
	add := func(name string, created time.Time) string {
		id := b.Add(name, "", nil, created).ID
		ids[name] = id
		return id
	}
	move := func(id string, to task.Status, when time.Time, agent string) {
		b.Actor = agent
		if err := b.Move(id, to, when); err != nil {
			t.Fatal(err)
		}
		b.Actor = ""
	}
	// Shipped yesterday within the window, by an agent.
	shipped := add("Ship parser", at(-72))
	move(shipped, task.StatusInDev, at(-48), "claude")
	move(shipped, task.StatusShipped, at(-3), "claude")
	// Shipped before the window: not Done, no Moved lines.
	old := add("Old work", at(-100))
	move(old, task.StatusShipped, at(-30), "")
	// Moved by a human in the window, now in review.
	review := add("Review docs", at(-50))
	move(review, task.StatusInDev, at(-40), "")
	move(review, task.StatusTestingReview, at(-2), "")
	// Blocked for 2 days.
	blocked := add("Waiting on API", at(-80))
	move(blocked, task.StatusBlocked, at(-48), "codex")
	// Due today and overdue, untouched.
	today := now
	dueID := add("Due today", at(-100))
	_ = b.Edit(dueID, "Due today", "", &today, at(-100))
	lateDay := now.Add(-72 * time.Hour)
	lateID := add("Late one", at(-100))
	_ = b.Edit(lateID, "Late one", "", &lateDay, at(-100))
	// Claimed in-progress task (claim set directly: the claim API belongs to another slice).
	until := at(2)
	for i := range b.Tasks {
		if b.Tasks[i].ID == review {
			b.Tasks[i].ClaimedBy, b.Tasks[i].ClaimUntil, b.Tasks[i].PR = "claude", &until, "42"
		}
	}
	return b, ids
}

func sectionTitles(s Standup) []string {
	var out []string
	for _, sec := range s.Sections {
		out = append(out, sec.Title)
	}
	return out
}

func TestStandupSections(t *testing.T) {
	b, _ := fixture(t)
	s := BuildStandup(b, now, now.Add(-24*time.Hour))
	if got, want := strings.Join(sectionTitles(s), "|"), "Done|Moved|Blocked now|Due today/overdue|In progress"; got != want {
		t.Fatalf("sections = %s, want %s", got, want)
	}
	text := s.Text()
	for _, want := range []string{
		"Standup 26/11/2026 09:00 (since 25/11/2026 09:00)",
		"Done\n- Ship parser (",
		"): by claude, 06:00",
		"Moved\n- Review docs (",
		"indev → testing-review by you, 07:00",
		"Blocked now\n- Waiting on API (",
		"blocked for 2d",
		"Late one (",
		"overdue 3d (due 23/11/2026)",
		"Due today (",
		"due today",
		"In progress\n- Review docs (",
		"[testing-review] claimed by claude until 11:00 · PR 42",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Old work", "→ shipped by claude", "todo → indev by claude"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("text should not contain %q:\n%s", unwanted, text)
		}
	}
	// Most overdue first.
	if strings.Index(text, "Late one") > strings.Index(text, "Due today (") {
		t.Error("overdue task should precede the one due today")
	}
}

func TestStandupCutoffWidens(t *testing.T) {
	b, _ := fixture(t)
	text := BuildStandup(b, now, now.Add(-7*24*time.Hour)).Text()
	for _, want := range []string{"Old work", "todo → indev by claude, 24/11/2026 09:00", "todo → blocked by codex"} {
		if !strings.Contains(text, want) {
			t.Errorf("7d window missing %q:\n%s", want, text)
		}
	}
	// The shipping move is listed under Done, not repeated under Moved.
	if strings.Count(text, "→ shipped") != 0 {
		t.Errorf("done transition duplicated in Moved:\n%s", text)
	}
}

func TestStandupReopenedShowsAsMove(t *testing.T) {
	b := &task.Board{}
	id := b.Add("Bounce", "", nil, at(-10)).ID
	_ = b.Move(id, task.StatusDone, at(-5))
	_ = b.Move(id, task.StatusTodo, at(-4))
	s := BuildStandup(b, now, now.Add(-24*time.Hour))
	if got := strings.Join(sectionTitles(s), "|"); got != "Moved" {
		t.Fatalf("sections = %s, want only Moved", got)
	}
	if text := s.Text(); !strings.Contains(text, "todo → done") || !strings.Contains(text, "done → todo") {
		t.Errorf("reopen not shown:\n%s", text)
	}
}

func TestStandupEmptyAndExpiredClaim(t *testing.T) {
	b := &task.Board{}
	s := BuildStandup(b, now, now.Add(-24*time.Hour))
	if !s.Empty() || !strings.Contains(s.Text(), "Nothing to report") || !strings.Contains(s.Markdown(), "Nothing to report") {
		t.Fatalf("empty board: %q", s.Text())
	}
	id := b.Add("Stale claim", "", nil, at(-100)).ID
	_ = b.Move(id, task.StatusDoing, at(-90))
	past := at(-1)
	b.Tasks[0].ClaimedBy, b.Tasks[0].ClaimUntil = "ghost", &past
	text := BuildStandup(b, now, now.Add(-24*time.Hour)).Text()
	if !strings.Contains(text, "In progress\n- Stale claim") || strings.Contains(text, "ghost") {
		t.Errorf("expired claim shown or task missing:\n%s", text)
	}
}

func TestStandupMarkdown(t *testing.T) {
	b, _ := fixture(t)
	md := BuildStandup(b, now, now.Add(-24*time.Hour)).Markdown()
	for _, want := range []string{"**Standup 26/11/2026 09:00", "**Done**\n- Ship parser `", "**Blocked now**"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

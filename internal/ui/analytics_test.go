package ui

import (
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

func analyticsBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	// one done today, one done three days ago, one blocked for 9 hours
	a := b.Add("[Done today]", ref.Add(-30*time.Hour))
	if err := b.Move(a.ID, task.StatusDone, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	c := b.Add("[Done earlier]", ref.Add(-96*time.Hour))
	if err := b.Move(c.ID, task.StatusDone, ref.Add(-72*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	d := b.Add("[Stuck]", ref.Add(-20*time.Hour))
	if err := b.Move(d.ID, task.StatusBlocked, ref.Add(-9*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	b.Add("[Fresh]", ref.Add(-time.Hour))
	return b
}

func fixedAnalytics(b *task.Board) AnalyticsModel {
	m := NewAnalyticsModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(100, 40)
	return m
}

func TestAnalyticsViewHasAllSections(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	for _, want := range []string{"THROUGHPUT", "CYCLE TIME", "BLOCKED", "STREAK"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing section %q:\n%s", want, out)
		}
	}
}

func TestAnalyticsViewShowsColumnCounts(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("View missing the %q tile:\n%s", s.Label(), out)
		}
	}
}

func TestAnalyticsViewUsesSingaporeDates(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	if !strings.Contains(out, "30/07/2026") {
		t.Errorf("View missing the DD/MM/YYYY end date:\n%s", out)
	}
}

func TestAnalyticsViewListsBlockedTask(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	if !strings.Contains(out, "Stuck") {
		t.Errorf("View missing the blocked task title:\n%s", out)
	}
	if !strings.Contains(out, "9h") {
		t.Errorf("View missing the blocked duration '9h':\n%s", out)
	}
}

func TestAnalyticsViewEmptyBoardDoesNotPanic(t *testing.T) {
	out := fixedAnalytics(&task.Board{}).View()
	if out == "" {
		t.Error("View of an empty board returned an empty string")
	}
	if !strings.Contains(out, "THROUGHPUT") {
		t.Errorf("View of an empty board is missing its sections:\n%s", out)
	}
}

package task

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestParsePriority(t *testing.T) {
	cases := map[string]Priority{
		"": PriorityNone, "none": PriorityNone, "NONE": PriorityNone,
		"low": PriorityLow, "l": PriorityLow, "p3": PriorityLow,
		"medium": PriorityMedium, "Med": PriorityMedium, "m": PriorityMedium, "P2": PriorityMedium,
		"high": PriorityHigh, " h ": PriorityHigh, "p1": PriorityHigh, "p0": PriorityHigh,
	}
	for in, want := range cases {
		got, err := ParsePriority(in)
		if err != nil || got != want {
			t.Errorf("ParsePriority(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"urgent", "p4", "3"} {
		if _, err := ParsePriority(bad); err == nil {
			t.Errorf("ParsePriority(%q) = nil error, want rejection", bad)
		}
	}
	for p, want := range map[Priority]string{PriorityNone: "none", PriorityLow: "low", PriorityMedium: "medium", PriorityHigh: "high"} {
		if p.String() != want {
			t.Errorf("%d.String() = %q, want %q", int(p), p.String(), want)
		}
		if back, _ := ParsePriority(p.String()); back != p {
			t.Errorf("round trip %v → %v", p, back)
		}
	}
}

func TestSetPriority(t *testing.T) {
	var b Board
	id := b.Add("x", "", nil, ref).ID
	later := ref.Add(time.Hour)
	if err := b.SetPriority(id, PriorityHigh, later); err != nil {
		t.Fatal(err)
	}
	got, _ := b.TaskByID(id)
	if got.Priority != PriorityHigh || !got.UpdatedAt.Equal(later) {
		t.Fatalf("task = %+v, want high priority updated at %v", got, later)
	}
	if err := b.SetPriority(id, Priority(9), later); err == nil {
		t.Error("SetPriority(9) = nil, want an error")
	}
	if err := b.SetPriority("missing", PriorityLow, later); err == nil {
		t.Error("SetPriority(missing) = nil, want ErrNotFound")
	}
}

func TestSetLabelsNormalizes(t *testing.T) {
	var b Board
	id := b.Add("x", "", nil, ref).ID
	if err := b.SetLabels(id, []string{" Bug ", "#backend", "", "bug", "BACKEND", "  ", "ui"}, ref); err != nil {
		t.Fatal(err)
	}
	got, _ := b.TaskByID(id)
	if want := []string{"bug", "backend", "ui"}; !slices.Equal(got.Labels, want) {
		t.Fatalf("Labels = %q, want %q", got.Labels, want)
	}
	if !got.HasLabel("#UI") || got.HasLabel("frontend") {
		t.Errorf("HasLabel mismatch for %q", got.Labels)
	}
	if err := b.SetLabels(id, []string{""}, ref); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.TaskByID(id); got.Labels != nil {
		t.Errorf("Labels after clear = %q, want nil", got.Labels)
	}
}

func TestSetLabelsNoOpKeepsBoardClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	b, _ := Load(path)
	id := b.Add("x", "", nil, ref).ID
	b.SetLabels(id, []string{"a"}, ref)
	b.SetPriority(id, PriorityLow, ref)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	b.SetLabels(id, []string{"A"}, ref.Add(time.Hour))
	b.SetPriority(id, PriorityLow, ref.Add(time.Hour))
	if b.Dirty() {
		t.Error("unchanged labels/priority marked the board dirty")
	}
	re, _ := Load(path)
	if got := re.Tasks[0]; got.Priority != PriorityLow || !slices.Equal(got.Labels, []string{"a"}) {
		t.Errorf("persisted = %+v", got)
	}
}

func TestSortByPriorityIsStable(t *testing.T) {
	tasks := []Task{
		{ID: "a", Priority: PriorityLow},
		{ID: "b"},
		{ID: "c", Priority: PriorityHigh},
		{ID: "d", Priority: PriorityLow},
		{ID: "e", Priority: PriorityHigh},
	}
	SortByPriority(tasks)
	var ids []string
	for _, x := range tasks {
		ids = append(ids, x.ID)
	}
	if want := []string{"c", "e", "a", "d", "b"}; !slices.Equal(ids, want) {
		t.Errorf("order = %v, want %v", ids, want)
	}
}

func TestOverWIP(t *testing.T) {
	var b Board
	b.SetColumns(DevColumns)
	for range 4 {
		id := b.Add("x", "", nil, ref).ID
		b.Move(id, StatusInDev, ref)
	}
	if count, limit, over := b.OverWIP(StatusInDev); count != 4 || limit != 0 || over {
		t.Errorf("no limit: OverWIP = %d, %d, %v", count, limit, over)
	}
	b.SetWIPLimit(StatusInDev, 4)
	if _, _, over := b.OverWIP(StatusInDev); over {
		t.Error("at the limit counts as over, want only strictly above")
	}
	b.SetWIPLimit(StatusInDev, 3)
	if count, limit, over := b.OverWIP(StatusInDev); count != 4 || limit != 3 || !over {
		t.Errorf("OverWIP = %d, %d, %v; want 4, 3, true", count, limit, over)
	}
	b.Tasks[0].Archived = true
	if count, _, over := b.OverWIP(StatusInDev); count != 3 || over {
		t.Errorf("archived card counted: %d, %v", count, over)
	}
}

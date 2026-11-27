package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"gotodo/internal/task"
)

// fullBoard is a dev board whose card sets every detail section: a parent,
// a subtask, a blocker, a worktree, documents and agent fields.
func fullBoard(t *testing.T) (*task.Board, string) {
	t.Helper()
	b := &task.Board{Columns: task.DevColumns}
	b.SetPath("")
	parent := b.Add("[Billing epic]", "", nil, ref).ID
	blocker := b.Add("[Blocker]", "", nil, ref).ID
	id := b.Add("[Card]", "the description", nil, ref).ID
	if _, err := b.AddChild(id, "[Child]", "", nil, ref); err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	until := ref.Add(time.Hour)
	setTask(b, id, func(t *task.Task) {
		t.ParentID = parent
		t.Deadline = &due
		t.Priority = task.PriorityHigh
		t.Labels = []string{"billing-ode", "api"}
		t.BlockedBy = []string{blocker}
		t.Worktree = "/home/dev/Projects/Personal/deadline-detail"
		t.Attachments = []string{"brief.pdf"}
		t.WorkingNotes = []string{"/home/dev/Projects/Personal/deadline-detail/docs/plans/detail-popup-notes.md"}
		t.ClaimedBy = "claude"
		t.ClaimUntil = &until
		t.PR = "https://example.test/org/repo/pull/42"
		t.LastAgent = "claude"
	})
	return b, id
}

func openFull(t *testing.T, w, h int) (BoardModel, string) {
	t.Helper()
	b, id := fullBoard(t)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(w, h)
	if !m.selectTaskID(id) {
		t.Fatal("card not found")
	}
	return press(m, "enter"), id
}

func TestDetailSectionOrder(t *testing.T) {
	m, id := openFull(t, 120, 40)
	tk, _ := m.board.TaskByID(id)
	body := stripANSI(strings.Join(m.detailBodyLines(tk, 100), "\n"))
	last := -1
	for _, want := range []string{
		"Description", "the description",
		"Subtasks", "[Child]",
		"Blocked by", "[Blocker]",
		"Parent", "[Billing epic]",
		"Worktree", "deadline-detail",
		"Attachments", "brief.pdf",
		"Working notes", "detail-popup-notes.md",
		"Agent", "Claimed by", "PR", "Last agent",
	} {
		i := strings.Index(body[last+1:], want)
		if i < 0 {
			t.Fatalf("%q missing or out of order after offset %d:\n%s", want, last, body)
		}
		last += 1 + i
	}
	// One blank row between sections, none inside one.
	if strings.Contains(body, "\n\n\n") {
		t.Errorf("sections are more than one blank row apart:\n%s", body)
	}
}

// The Agent rows align their values in one column.
func TestDetailAgentRowsAlignKeys(t *testing.T) {
	m, id := openFull(t, 120, 40)
	tk, _ := m.board.TaskByID(id)
	col := -1
	for _, row := range m.agentRows(tk, 100) {
		plain := stripANSI(row)
		v := strings.IndexFunc(plain[agentKeyWidth-1:], func(r rune) bool { return r != ' ' })
		if col >= 0 && v+agentKeyWidth-1 != col {
			t.Errorf("value column %d, want %d: %q", v+agentKeyWidth-1, col, plain)
		}
		col = v + agentKeyWidth - 1
	}
	if col != agentKeyWidth {
		t.Errorf("values start at %d, want %d", col, agentKeyWidth)
	}
}

func TestDetailFactsLine(t *testing.T) {
	m, _ := openFull(t, 120, 40)
	lines := strings.Split(stripANSI(m.renderDetail()), "\n")
	meta := -1
	for i, l := range lines {
		if strings.Contains(l, "created 0m ago") {
			meta = i
		}
	}
	if meta < 0 {
		t.Fatalf("no meta line:\n%s", strings.Join(lines, "\n"))
	}
	facts := lines[meta+1]
	if !strings.Contains(facts, "● high   due 02/08/2026 · 3d   #billing-ode #api") {
		t.Errorf("facts line = %q", facts)
	}
	if !strings.Contains(lines[meta+2], "────") {
		t.Errorf("rule does not follow the facts line: %q", lines[meta+2])
	}

	// Nothing set: the rule follows the meta line directly.
	b := &task.Board{}
	b.Add("[Plain]", "", nil, ref)
	pm := press(fixedClock(NewBoardModel(b)), "enter")
	pm.SetSize(120, 40)
	plain := strings.Split(stripANSI(pm.renderDetail()), "\n")
	for i, l := range plain {
		if strings.Contains(l, "created 0m ago") && !strings.Contains(plain[i+1], "────") {
			t.Errorf("empty facts left a row: %q", plain[i+1])
		}
	}

	// Only the set facts appear.
	setTask(b, b.Tasks[0].ID, func(t *task.Task) { t.Priority = task.PriorityLow })
	if out := stripANSI(pm.renderDetail()); !strings.Contains(out, "● low") || strings.Contains(out, "due ") || strings.Contains(out, "#") {
		t.Errorf("low-only facts line wrong:\n%s", out)
	}
}

// Each priority's dot and word render in its colour.
func TestDetailFactsColourPriority(t *testing.T) {
	withColor(t)
	for p, c := range map[task.Priority]lipgloss.AdaptiveColor{
		task.PriorityHigh: colBlocked, task.PriorityMedium: colDoing, task.PriorityLow: colTodo,
	} {
		want := lipgloss.NewStyle().Bold(p == task.PriorityHigh).Foreground(c).Render("● " + priorityWord(p))
		if got := priorityFact(p); got != want {
			t.Errorf("%d: fact = %q, want %q", p, got, want)
		}
	}
	if priorityFact(task.PriorityNone) != "" || priorityDot(task.PriorityNone) != "" {
		t.Error("an unset priority rendered something")
	}
}

// Priority reads by colour on cards: a coloured ● in colour, a letter in
// plain ASCII, and no exclamation marks anywhere.
func TestCardPriorityByColourNotBang(t *testing.T) {
	b := &task.Board{}
	for _, p := range []task.Priority{task.PriorityHigh, task.PriorityMedium, task.PriorityLow, task.PriorityNone} {
		name := priorityWord(p)
		if name == "" {
			name = "none"
		}
		id := b.Add("[P "+name+"]", "", nil, ref).ID
		setTask(b, id, func(t *task.Task) { t.Priority = p })
	}
	colours := []lipgloss.AdaptiveColor{colBlocked, colDoing, colTodo}

	withColor(t)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	for i, tk := range b.Tasks {
		card := m.renderCard(0, i, tk, 30)
		if i < 3 {
			if dot := lipgloss.NewStyle().Foreground(colours[i]).Render("●"); !strings.Contains(card, dot) {
				t.Errorf("%d: card lacks the coloured dot %q: %q", tk.Priority, dot, card)
			}
		} else if strings.Contains(card, "●") {
			t.Errorf("no-priority card has a dot: %q", card)
		}
	}
	if out := stripANSI(m.View()); strings.Contains(out, "!") {
		t.Errorf("board still shows a ! marker:\n%s", out)
	}

	lipgloss.SetColorProfile(termenv.Ascii)
	for i, want := range []string{"H [P high]", "M [P medium]", "L [P low]", "  [P none]"} {
		card := stripANSI(m.renderCard(0, i, b.Tasks[i], 30))
		if !strings.Contains(card, want) {
			t.Errorf("ASCII card = %q, want %q", card, want)
		}
	}
}

func TestDetailHasNoBangMarkers(t *testing.T) {
	m, _ := openFull(t, 120, 40)
	if out := stripANSI(m.View()); strings.Contains(out, "!") {
		t.Errorf("detail shows a ! marker:\n%s", out)
	}
}

func TestWrapPathBreaksAtSlash(t *testing.T) {
	path := "/home/dev/Projects/Personal/deadline-detail/docs/notes.md"
	rows := wrapPath(path, 20)
	if strings.Join(rows, "") != path {
		t.Fatalf("wrapping lost text: %q", rows)
	}
	for i, r := range rows {
		if lipgloss.Width(r) > 20 {
			t.Errorf("row %q wider than 20", r)
		}
		if i < len(rows)-1 && !strings.HasSuffix(r, "/") {
			t.Errorf("row %q breaks mid-segment", r)
		}
	}
	// A segment wider than the row breaks hard.
	if rows := wrapPath("abcdefghij/k", 4); strings.Join(rows, "") != "abcdefghij/k" || rows[0] != "abcd" {
		t.Errorf("long segment = %q", rows)
	}
	// Spaces are break points too, and are not carried to the next row.
	if rows := wrapPath("notes for the plan", 10); rows[0] != "notes for" || rows[1] != "the plan" {
		t.Errorf("spaced path = %q", rows)
	}
}

// A long worktree path wraps at "/" with its continuation under the first
// character.
func TestDetailWrapsLongPathUnderFirstCharacter(t *testing.T) {
	m, id := openFull(t, 120, 40)
	tk, _ := m.board.TaskByID(id)
	body := m.detailBodyLines(tk, 30)
	var rows []string
	in := false
	for _, l := range body {
		plain := stripANSI(l)
		switch {
		case strings.HasPrefix(plain, "Working notes"):
			in = true
		case in && plain == "":
			in = false
		case in:
			rows = append(rows, plain)
		}
	}
	if len(rows) < 2 {
		t.Fatalf("working note did not wrap: %q", rows)
	}
	for i, r := range rows {
		if !strings.HasPrefix(r, "  ") || r[2] == ' ' {
			t.Errorf("row %d not indented under the first character: %q", i, r)
		}
		if i < len(rows)-1 && !strings.HasSuffix(r, "/") {
			t.Errorf("row %d breaks mid-segment: %q", i, r)
		}
		if lipgloss.Width(r) > 30 {
			t.Errorf("row %d wider than the body: %q", i, r)
		}
	}
}

// Every section set still fits each terminal size.
func TestDetailFullCardFitsEverySize(t *testing.T) {
	for _, w := range []int{60, 80, 110, 160} {
		for _, h := range []int{8, 10, 14, 18, 24, 40} {
			b, id := fullBoard(t)
			b.RequireApproval = true
			a := NewApp(b)
			a.now = func() time.Time { return ref }
			a.board.now = func() time.Time { return ref }
			up, _ := a.Update(tea.WindowSizeMsg{Width: w, Height: h})
			a = up.(AppModel)
			a.board.selectTaskID(id)
			up, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
			a = up.(AppModel)
			view := a.View()
			if got := lipgloss.Height(view); got > h {
				t.Errorf("%dx%d: detail is %d rows:\n%s", w, h, got, stripANSI(view))
			}
			for _, line := range strings.Split(view, "\n") {
				if got := lipgloss.Width(line); got > w {
					t.Errorf("%dx%d: line is %d wide: %q", w, h, got, stripANSI(line))
				}
			}
		}
	}
}

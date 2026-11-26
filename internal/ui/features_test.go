package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// agentBoard is a dev board with one card carrying every agent field, a
// blocker, and an in-progress card claimed by another agent.
func agentBoard(t *testing.T) (*task.Board, string) {
	t.Helper()
	b := &task.Board{Columns: task.DevColumns}
	b.SetPath("")
	blocker := b.Add("[Blocker]", "", nil, ref).ID
	id := b.Add("[Agent card]", "", nil, ref).ID
	until := time.Date(2026, 7, 30, 15, 30, 0, 0, time.UTC)
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			b.Tasks[i].Priority = task.PriorityHigh
			b.Tasks[i].Labels = []string{"api", "ui"}
			b.Tasks[i].BlockedBy = []string{blocker}
			b.Tasks[i].ClaimedBy = "claude"
			b.Tasks[i].ClaimUntil = &until
			b.Tasks[i].PR = "https://example.test/pr/42"
			b.Tasks[i].LastAgent = "claude"
		}
	}
	return b, id
}

// setTask mutates one task in place for test setup.
func setTask(b *task.Board, id string, f func(*task.Task)) {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			f(&b.Tasks[i])
		}
	}
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// --- cards and headers ---

// Priority and claim ride on the title row: the card does not grow.
func TestCardShowsPriorityAndClaimWithoutGrowing(t *testing.T) {
	b, id := agentBoard(t)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(160, 40)
	tt, _ := b.TaskByID(id)
	card := stripANSI(m.renderCard(1, 1, tt, 30))
	if strings.Count(card, "\n") != 0 {
		t.Errorf("card grew past one row:\n%s", card)
	}
	for _, want := range []string{"!!!", "◆ claude", "Agent"} {
		if !strings.Contains(card, want) {
			t.Errorf("card is missing %q: %q", want, card)
		}
	}
	for _, w := range []int{minColumnWidth, 22, 30} {
		for _, line := range strings.Split(stripANSI(m.renderCard(0, 1, tt, w)), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("width %d: card line is %d wide: %q", w, got, line)
			}
		}
	}
	// A narrow card keeps the diamond but drops the name before the title.
	narrow := stripANSI(m.renderCard(1, 1, tt, minColumnWidth+2))
	if !strings.Contains(narrow, "◆") || strings.Contains(narrow, "claude") {
		t.Errorf("narrow card should shorten the claim to a bare diamond: %q", narrow)
	}
}

func TestCardHidesExpiredClaim(t *testing.T) {
	b, id := agentBoard(t)
	past := ref.Add(-time.Hour)
	setTask(b, id, func(t *task.Task) { t.ClaimUntil = &past })
	m := fixedClock(NewBoardModel(b))
	tt, _ := b.TaskByID(id)
	if card := stripANSI(m.renderCard(1, 1, tt, 30)); strings.Contains(card, "◆") {
		t.Errorf("expired claim still on the card: %q", card)
	}
}

// A WIP limit shows cards/limit, in the error colour once over.
func TestColumnHeaderShowsWIPLimit(t *testing.T) {
	withColor(t)
	b := &task.Board{Columns: task.DevColumns}
	for i := 0; i < 4; i++ {
		id := b.Add("[Work]", "", nil, ref).ID
		_ = b.Move(id, task.StatusInDev, ref)
	}
	b.SetWIPLimit(task.StatusInDev, 3)
	b.SetWIPLimit(task.StatusTodo, 5)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(200, 30)
	out := m.View()
	plain := stripANSI(out)
	if !strings.Contains(plain, "IN DEV (4/3)") || !strings.Contains(plain, "TODO (0/5)") {
		t.Errorf("headers missing WIP counts:\n%s", plain)
	}
	if !strings.Contains(out, ErrorStyle.Render("4/3")) {
		t.Error("over-limit count is not in the error colour")
	}
	if strings.Contains(out, ErrorStyle.Render("0/5")) {
		t.Error("under-limit count is in the error colour")
	}
}

// A long header is shortened, never wrapped onto a second row.
func TestColumnHeaderNeverWraps(t *testing.T) {
	b := &task.Board{Columns: task.DevColumns}
	b.SetWIPLimit(task.StatusTestingReview, 10)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(100, 30)
	h := m.columnHeader(task.StatusTestingReview, 12, m.columnWidth())
	if got := lipgloss.Width(h); got > m.columnWidth()-2 {
		t.Errorf("header is %d wide, column content is %d: %q", got, m.columnWidth()-2, stripANSI(h))
	}
}

// --- detail props ---

func TestDetailShowsAgentFields(t *testing.T) {
	b, id := agentBoard(t)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	if !m.selectTaskID(id) {
		t.Fatal("card not found")
	}
	out := stripANSI(press(m, "enter").View())
	blocker := shortTaskID(b.Tasks[0].ID)
	for _, want := range []string{
		"Priority", "!!! high",
		"Labels", "#api #ui",
		"Blocked by", blocker, "[Blocker]", "todo",
		"Claimed by", "◆ claude", "until 30/07/2026 15:30",
		"PR", "https://example.test/pr/42",
		"Last agent",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("detail is missing %q:\n%s", want, out)
		}
	}
}

func TestDetailOmitsUnsetAgentFields(t *testing.T) {
	m := fixedClock(NewBoardModel(seeded(t)))
	m.SetSize(120, 40)
	out := stripANSI(press(m, "enter").View())
	for _, absent := range []string{"Priority", "Labels", "Blocked by", "Claimed by", "Last agent", "awaiting approval", "o Open", "y Approve"} {
		if strings.Contains(out, absent) {
			t.Errorf("detail shows unset %q:\n%s", absent, out)
		}
	}
}

// Every field set still fits short terminals: props give way to the frame.
func TestDetailWithAgentFieldsFitsShortTerminals(t *testing.T) {
	for _, h := range []int{8, 10, 14, 18, 24} {
		b, id := agentBoard(t)
		b.RequireApproval = true
		setTask(b, id, func(t *task.Task) {
			t.Description = "one\ntwo"
			t.Attachments = []string{"a.md", "b.pdf"}
			t.BlockedBy = append(t.BlockedBy, "aaaa", "bbbb", "cccc", "dddd")
		})
		a := NewApp(b)
		a.board.now = func() time.Time { return ref }
		up, _ := a.Update(tea.WindowSizeMsg{Width: 110, Height: h})
		a = up.(AppModel)
		a.board.selectTaskID(id)
		up, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnter})
		a = up.(AppModel)
		if got := lipgloss.Height(a.View()); got > h {
			t.Errorf("height=%d: detail is %d rows:\n%s", h, got, stripANSI(a.View()))
		}
	}
}

// --- search ---

func TestMatchesQuery(t *testing.T) {
	tk := task.Task{ID: "abcdef1234", Title: "Fix login", Description: "OAuth redirect",
		Labels: []string{"auth", "Backend"}, ClaimedBy: "claude", LastAgent: "codex"}
	for _, c := range []struct {
		q    string
		want bool
	}{
		{"", true}, {"login", true}, {"LOGIN", true}, {"oauth", true}, {"abcd", true},
		{"#auth", true}, {"#back", true}, {"#front", false}, {"backend", true},
		{"@claude", true}, {"@codex", true}, {"@gem", false},
		{"fix #auth @cla", true}, {"fix #nope", false}, {"cdef", false},
	} {
		if got := matchesQuery(tk, c.q); got != c.want {
			t.Errorf("matchesQuery(%q) = %v, want %v", c.q, got, c.want)
		}
	}
}

// / filters live as you type, enter keeps it, esc clears it.
func TestSearchFiltersAcrossColumnsAndClears(t *testing.T) {
	b := &task.Board{}
	b.Add("Alpha login", "", nil, ref)
	b.Add("Beta", "", nil, ref)
	d := b.Add("Gamma login", "", nil, ref).ID
	_ = b.Move(d, task.StatusDoing, ref)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 30)
	m = press(m, "j") // select Beta, which the filter will hide

	m = press(m, "/")
	if m.mode != modeSearch {
		t.Fatalf("mode = %v, want modeSearch", m.mode)
	}
	m = typeInto(m, "login")
	if got := len(m.visibleByStatus(task.StatusTodo)); got != 1 {
		t.Errorf("todo shows %d cards while filtering, want 1", got)
	}
	if got := len(m.visibleByStatus(task.StatusDoing)); got != 1 {
		t.Errorf("doing shows %d cards, want 1", got)
	}
	if sel, ok := m.selectedTask(); !ok || !strings.Contains(sel.Title, "login") {
		t.Errorf("selection did not clamp onto a match: %+v", sel)
	}
	if footer := stripANSI(m.renderFooter()); !strings.Contains(footer, "/ login") {
		t.Errorf("footer does not show the typed filter: %q", footer)
	}

	m = press(m, "enter")
	if m.mode != modeNormal || m.query != "login" {
		t.Fatalf("enter did not keep the filter: mode=%v query=%q", m.mode, m.query)
	}
	out := stripANSI(m.View())
	if strings.Contains(out, "Beta") || !strings.Contains(out, "Search login") || !strings.Contains(out, "TODO (1/2)") {
		t.Errorf("kept filter not applied or not shown:\n%s", out)
	}

	m = press(m, "esc")
	if m.query != "" || len(m.visibleByStatus(task.StatusTodo)) != 2 {
		t.Errorf("esc did not clear the filter: query=%q", m.query)
	}
}

// Search narrows inside the family filter rather than replacing it.
func TestSearchCombinesWithFamilyFilter(t *testing.T) {
	b := &task.Board{}
	p := b.Add("Parent", "", nil, ref).ID
	b.AddChild(p, "Child login", "", nil, ref)
	b.AddChild(p, "Child other", "", nil, ref)
	b.Add("Stranger login", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 30)
	m = press(m, "f", "/")
	m = typeInto(m, "login")
	m = press(m, "enter")
	items := m.visibleByStatus(task.StatusTodo)
	if len(items) != 1 || items[0].Title != "Child login" {
		t.Errorf("family+search shows %v, want only the matching child", items)
	}
}

func TestSearchByLabelAgentAndID(t *testing.T) {
	b, id := agentBoard(t)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(200, 30)
	for _, q := range []string{"#api", "@claude", id[:6]} {
		mm := typeInto(press(m, "/"), q)
		items := mm.visibleByStatus(task.StatusTodo)
		if len(items) != 1 || items[0].ID != id {
			t.Errorf("query %q matched %d cards, want the agent card", q, len(items))
		}
	}
}

// --- open documents ---

// boardInProject puts a board at <root>/.deadline/board.json with one task
// pointing at the given documents.
func boardInProject(t *testing.T, attachments, notes []string) (*task.Board, string) {
	t.Helper()
	root := t.TempDir()
	b := &task.Board{}
	b.SetPath(filepath.Join(root, ".deadline", "board.json"))
	id := b.Add("[Docs]", "", nil, ref).ID
	_ = b.SetDocuments(id, attachments, notes, ref)
	return b, root
}

func recordOpener(m *BoardModel) *[]string {
	var opened []string
	m.opener = func(path string) tea.Cmd {
		opened = append(opened, path)
		return nil
	}
	return &opened
}

func TestOpenSingleDocumentResolvesAgainstProjectRoot(t *testing.T) {
	b, root := boardInProject(t, []string{"docs/spec.md"}, nil)
	os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	os.WriteFile(filepath.Join(root, "docs", "spec.md"), []byte("x"), 0o644)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	opened := recordOpener(&m)

	m = press(m, "enter")
	if out := stripANSI(m.View()); !strings.Contains(out, "o Open") {
		t.Errorf("detail footer lacks the o hint:\n%s", out)
	}
	m = press(m, "o")
	if want := filepath.Join(root, "docs", "spec.md"); len(*opened) != 1 || (*opened)[0] != want {
		t.Errorf("opened %v, want [%s]", *opened, want)
	}
	if m.mode != modeDetail {
		t.Errorf("mode = %v, want the detail popup to stay open", m.mode)
	}
}

func TestOpenChooserPicksAmongDocuments(t *testing.T) {
	b, root := boardInProject(t, []string{"a.md"}, []string{"notes/b.txt"})
	os.WriteFile(filepath.Join(root, "a.md"), nil, 0o644)
	os.MkdirAll(filepath.Join(root, "notes"), 0o755)
	os.WriteFile(filepath.Join(root, "notes", "b.txt"), nil, 0o644)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	opened := recordOpener(&m)

	m = press(m, "enter", "o")
	if m.mode != modeOpen {
		t.Fatalf("mode = %v, want the chooser", m.mode)
	}
	out := stripANSI(m.View())
	for _, want := range []string{"Open document", "a.md", "attachment", "notes/b.txt", "note"} {
		if !strings.Contains(out, want) {
			t.Errorf("chooser is missing %q:\n%s", want, out)
		}
	}
	m = press(m, "j", "enter")
	if len(*opened) != 1 || (*opened)[0] != filepath.Join(root, "notes", "b.txt") {
		t.Errorf("opened %v, want the note", *opened)
	}
	if m.mode != modeDetail {
		t.Errorf("mode = %v, want back on the detail popup", m.mode)
	}
	m = press(m, "o", "esc")
	if m.mode != modeDetail {
		t.Errorf("esc from the chooser: mode = %v, want detail", m.mode)
	}
}

func TestOpenMissingDocumentShowsError(t *testing.T) {
	b, _ := boardInProject(t, []string{"gone.md"}, nil)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	opened := recordOpener(&m)
	m = press(m, "enter", "o")
	if len(*opened) != 0 {
		t.Errorf("a missing file was handed to the opener: %v", *opened)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "! not found: gone.md") {
		t.Errorf("missing file error not shown:\n%s", out)
	}
}

func TestOpenWithoutDocumentsSaysSo(t *testing.T) {
	m := fixedClock(NewBoardModel(seeded(t)))
	m.SetSize(120, 40)
	m = press(m, "enter", "o")
	if !strings.Contains(m.err, "no attachments") {
		t.Errorf("err = %q, want a no-documents message", m.err)
	}
}

// A failed editor or opener run lands in the board footer.
func TestDocOpenedErrorReachesFooter(t *testing.T) {
	a := app(t)
	m, _ := a.Update(docOpenedMsg{Err: errors.New("exec: \"nvim\": not found")})
	if out := stripANSI(m.(AppModel).View()); !strings.Contains(out, "open failed") {
		t.Errorf("open error not shown:\n%s", out)
	}
}

func TestDocumentOpenHelpers(t *testing.T) {
	if got := projectRoot("/p/app/.deadline/board.json"); got != "/p/app" {
		t.Errorf("projectRoot = %q", got)
	}
	if got := projectRoot("/home/u/.config/gotodo/tasks.json"); got != "" {
		t.Errorf("projectRoot outside a project = %q, want empty", got)
	}
	if got := resolveDocPath("docs/a.md", "/p/app"); got != filepath.Join("/p/app", "docs/a.md") {
		t.Errorf("resolveDocPath relative = %q", got)
	}
	if got := resolveDocPath("/abs/a.md", "/p/app"); got != "/abs/a.md" {
		t.Errorf("resolveDocPath absolute = %q", got)
	}
	for path, want := range map[string]bool{"a.md": true, "main.go": true, "Makefile": true, "x.pdf": false, "x.png": false} {
		if got := isTextDoc(path); got != want {
			t.Errorf("isTextDoc(%q) = %v", path, got)
		}
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := editorCommand(env(map[string]string{"EDITOR": "code -w"})); strings.Join(got, " ") != "code -w" {
		t.Errorf("editorCommand = %v", got)
	}
	if got := editorCommand(env(nil)); got[0] != "vi" {
		t.Errorf("editorCommand fallback = %v", got)
	}
	found := func(string) (string, error) { return "/usr/bin/wslview", nil }
	missing := func(string) (string, error) { return "", errors.New("no") }
	for _, c := range []struct {
		goos string
		wsl  bool
		look func(string) (string, error)
		want string
	}{
		{"darwin", false, missing, "open"},
		{"linux", false, found, "xdg-open"},
		{"linux", true, found, "wslview"},
		{"linux", true, missing, "xdg-open"},
	} {
		if got := launcher(c.goos, c.wsl, c.look); got != c.want {
			t.Errorf("launcher(%s, wsl=%v) = %q, want %q", c.goos, c.wsl, got, c.want)
		}
	}
}

// --- approval ---

// reviewBoard is a dev board needing approval with one card in review.
func reviewBoard(t *testing.T) (*task.Board, string) {
	t.Helper()
	b := &task.Board{Columns: task.DevColumns, RequireApproval: true}
	id := b.Add("[Review me]", "", nil, ref).ID
	if err := b.Move(id, task.StatusTestingReview, ref); err != nil {
		t.Fatal(err)
	}
	return b, id
}

func TestApproveShipsReviewedCardAsHuman(t *testing.T) {
	b, id := reviewBoard(t)
	b.Actor = "claude" // even a TUI started with GOTODO_AGENT set approves as a human
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m.selectTaskID(id)
	m = press(m, "enter")
	out := stripANSI(m.View())
	for _, want := range []string{"awaiting approval", "y Approve"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail is missing %q:\n%s", want, out)
		}
	}
	var cmd tea.Cmd
	m, cmd = m.Update(runes("y"))
	got, _ := b.TaskByID(id)
	if got.Status != task.StatusShipped {
		t.Fatalf("status = %s, want shipped", got.Status)
	}
	if last := got.History[len(got.History)-1]; last.Agent != "" {
		t.Errorf("approval recorded agent %q, want a human", last.Agent)
	}
	if b.Actor != "claude" {
		t.Errorf("Actor not restored: %q", b.Actor)
	}
	if cmd == nil || m.mode != modeNormal || !strings.Contains(m.info, "approved") {
		t.Errorf("approve: cmd=%v mode=%v info=%q", cmd != nil, m.mode, m.info)
	}
	if sel, _ := m.selectedTask(); sel.ID != id {
		t.Error("selection did not follow the shipped card")
	}
}

func TestApproveOnlyWhenGateOnAndInReview(t *testing.T) {
	b, id := reviewBoard(t)
	b.RequireApproval = false
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m.selectTaskID(id)
	m = press(m, "enter", "y")
	if got, _ := b.TaskByID(id); got.Status != task.StatusTestingReview {
		t.Errorf("y shipped a card with the gate off: %s", got.Status)
	}

	personal := &task.Board{RequireApproval: true}
	pid := personal.Add("[Doing]", "", nil, ref).ID
	_ = personal.Move(pid, task.StatusDoing, ref)
	pm := fixedClock(NewBoardModel(personal))
	if pm.reviewStatus() != task.StatusDoing {
		t.Errorf("personal review column = %s, want doing", pm.reviewStatus())
	}
	tk, _ := personal.TaskByID(pid)
	if !pm.awaitingApproval(tk) {
		t.Error("doing card on a gated personal board is not awaiting approval")
	}
}

func TestShiftATogglesApprovalWithNotice(t *testing.T) {
	b := &task.Board{Columns: task.DevColumns}
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m, cmd := m.Update(runes("A"))
	if !b.RequireApproval || cmd == nil {
		t.Fatalf("A did not turn the gate on (cmd=%v)", cmd != nil)
	}
	if footer := stripANSI(m.renderFooter()); !strings.Contains(footer, "approval on") || !strings.Contains(footer, "SHIPPED") {
		t.Errorf("footer lacks the confirmation: %q", footer)
	}
	m = press(m, "A")
	if b.RequireApproval || !strings.Contains(m.info, "approval off") {
		t.Errorf("second A: gate=%v info=%q", b.RequireApproval, m.info)
	}
	m = press(m, "j")
	if m.info != "" {
		t.Errorf("notice survived the next key: %q", m.info)
	}
}

// --- undo ---

func TestUndoMove(t *testing.T) {
	b := &task.Board{}
	b.Add("[Mover]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m = press(m, "m", "l", "l", "enter")
	if b.Tasks[0].Status != task.StatusBlocked {
		t.Fatalf("setup: status = %s", b.Tasks[0].Status)
	}
	m, cmd := m.Update(runes("u"))
	if b.Tasks[0].Status != task.StatusTodo || len(b.Tasks[0].History) != 0 {
		t.Errorf("undo left %s with %d moves", b.Tasks[0].Status, len(b.Tasks[0].History))
	}
	if cmd == nil {
		t.Error("undo did not ask for a save")
	}
	if footer := stripANSI(m.renderFooter()); !strings.Contains(footer, "undone: move [Mover]") {
		t.Errorf("footer = %q", footer)
	}
	if sel, _ := m.selectedTask(); sel.ID != b.Tasks[0].ID {
		t.Error("selection did not follow the restored card")
	}
	m = press(m, "u")
	if !strings.Contains(m.info, "nothing to undo") {
		t.Errorf("empty stack info = %q", m.info)
	}
}

// A grab that ends where it started records nothing.
func TestGrabWithoutMoveRecordsNoUndo(t *testing.T) {
	b := &task.Board{}
	b.Add("[Still]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m = press(m, "m", "enter", "m", "l", "esc")
	if len(m.undo) != 0 {
		t.Errorf("undo stack = %d entries, want 0", len(m.undo))
	}
}

func TestUndoEditAndAdd(t *testing.T) {
	b := &task.Board{}
	b.Add("[Original]", "", nil, ref)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m = press(m, "e")
	m.title.SetValue("[Renamed]")
	m = press(m, "enter")
	m = press(m, "a")
	m = typeInto(m, "[Added]")
	m = press(m, "enter")
	if len(b.Tasks) != 2 {
		t.Fatalf("setup: %d tasks", len(b.Tasks))
	}
	m = press(m, "u")
	if len(b.Tasks) != 1 || !strings.Contains(m.info, "undone: add [Added]") {
		t.Errorf("undo add: %d tasks, info %q", len(b.Tasks), m.info)
	}
	m = press(m, "u")
	if b.Tasks[0].Title != "[Original]" || !strings.Contains(m.info, "undone: edit [Renamed]") {
		t.Errorf("undo edit: title %q, info %q", b.Tasks[0].Title, m.info)
	}
}

// Undoing a delete brings the task back with its subtasks re-attached, and
// it survives a save and reload.
func TestUndoDeleteRestoresFamilyAndSaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b := &task.Board{}
	b.SetPath(path)
	p := b.Add("[Parent]", "", nil, ref).ID
	c, _ := b.AddChild(p, "[Child]", "", nil, ref)
	child := c.ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m = press(m, "d", "y")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if _, ok := b.TaskByID(p); ok {
		t.Fatal("setup: parent not deleted")
	}
	m = press(m, "u")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.TaskByID(child)
	if _, pok := reloaded.TaskByID(p); !pok || !ok || got.ParentID != p {
		t.Errorf("after undo+save: parent present=%v, child parent=%q", pok, got.ParentID)
	}
	if !strings.Contains(m.info, "undone: delete [Parent]") {
		t.Errorf("info = %q", m.info)
	}
}

func TestUndoStackIsCapped(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	for i := 0; i < undoLimit+5; i++ {
		m.pushUndo(undoEntry{what: "x"})
	}
	if len(m.undo) != undoLimit {
		t.Errorf("stack = %d, want %d", len(m.undo), undoLimit)
	}
}

// Snapshots are deep copies: later in-place edits never leak into them.
func TestCloneTaskIsDeep(t *testing.T) {
	d := ref
	orig := task.Task{Labels: []string{"a"}, History: []task.Transition{{To: task.StatusDoing}}, Deadline: &d}
	c := cloneTask(orig)
	orig.Labels[0] = "z"
	orig.History[0].To = task.StatusDone
	*orig.Deadline = ref.Add(time.Hour)
	if c.Labels[0] != "a" || c.History[0].To != task.StatusDoing || !c.Deadline.Equal(ref) {
		t.Errorf("clone shares memory with the original: %+v", c)
	}
}

// --- agents page ---

// activityBoard has two agents and a human move.
func activityBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{Columns: task.DevColumns}
	b.SetPath("")
	b.Actor = "claude"
	a := b.Add("[Claude work]", "", nil, ref.Add(-48*time.Hour)).ID
	_ = b.Move(a, task.StatusInDev, ref.Add(-2*time.Hour))
	b.Actor = "codex"
	c := b.Add("[Codex work]", "", nil, ref.Add(-48*time.Hour)).ID
	_ = b.Move(c, task.StatusInDev, ref.Add(-10*24*time.Hour)) // outside the week
	b.Actor = ""
	h := b.Add("[Human work]", "", nil, ref).ID
	_ = b.Move(h, task.StatusInDev, ref.Add(-time.Hour))
	return b
}

func TestAgentsPageSummarisesAgentsAndFeed(t *testing.T) {
	b := activityBoard(t)
	m := NewAgentsModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(120, 40)
	out := stripANSI(m.View())
	for _, want := range []string{
		"Agents (2)", "◆ claude", "1 active", "last 2h ago", "1 move this week",
		"◆ codex", "0 moves this week", "[Claude work]", "in dev",
		"Activity", "30/07 11:00", "you", "todo → in dev",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("agents page is missing %q:\n%s", want, out)
		}
	}
	// Newest move first: the human's (1h ago) leads the feed.
	feed := out[strings.Index(out, "Activity"):]
	if strings.Index(feed, "[Human work]") > strings.Index(feed, "[Claude work]") {
		t.Errorf("feed is not newest first:\n%s", feed)
	}
}

func TestAgentsPageEmptyState(t *testing.T) {
	m := NewAgentsModel(&task.Board{})
	m.SetSize(100, 30)
	out := stripANSI(m.View())
	if !strings.Contains(out, "no agent activity yet") || !strings.Contains(out, "no moves yet") {
		t.Errorf("empty agents page:\n%s", out)
	}
}

func TestAgentsFeedCapsAtTwenty(t *testing.T) {
	b := &task.Board{}
	id := b.Add("[Busy]", "", nil, ref).ID
	for i := 0; i < 30; i++ {
		to := task.StatusDoing
		if i%2 == 1 {
			to = task.StatusTodo
		}
		_ = b.Move(id, to, ref.Add(time.Duration(i)*time.Minute))
	}
	if got := len(recentActivity(b, activityFeedSize)); got != activityFeedSize {
		t.Errorf("feed has %d rows, want %d", got, activityFeedSize)
	}
}

// The page reads the shared board, so a live reload shows a new agent.
func TestAgentsPageRefreshesOnLiveReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b := &task.Board{}
	b.SetPath(path)
	b.Add("[Mine]", "", nil, ref)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	a := NewApp(b)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	for i := 0; i < int(pageAgents); i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if m.(AppModel).page != pageAgents {
		t.Fatalf("page = %v, want agents", m.(AppModel).page)
	}

	agent, _ := task.Load(path)
	agent.Actor = "gemini"
	id := agent.Add("[Agent card]", "", nil, ref).ID
	_ = agent.Move(id, task.StatusDoing, time.Now())
	if err := agent.Save(); err != nil {
		t.Fatal(err)
	}
	m, _ = m.Update(refreshTickMsg(time.Now()))
	if out := stripANSI(m.(AppModel).View()); !strings.Contains(out, "◆ gemini") || !strings.Contains(out, "[Agent card]") {
		t.Errorf("agents page did not pick up the reload:\n%s", out)
	}
}

func TestAgentsPageFitsTerminal(t *testing.T) {
	withColor(t)
	b := activityBoard(t)
	for _, sz := range [][2]int{{80, 15}, {80, 24}, {120, 40}} {
		a := NewApp(b)
		var m tea.Model = a
		m, _ = m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		for i := 0; i < int(pageAgents); i++ {
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		}
		out := m.View()
		if h := lipgloss.Height(out); h > sz[1] {
			t.Errorf("%dx%d: %d rows", sz[0], sz[1], h)
		}
		for _, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w > sz[0] {
				t.Errorf("%dx%d: line %d wide", sz[0], sz[1], w)
				break
			}
		}
	}
}

// --- frame fit for the new modes ---

func TestNewModesFitTerminal(t *testing.T) {
	withColor(t)
	for _, sz := range [][2]int{{80, 15}, {80, 24}, {110, 40}} {
		b, root := boardInProject(t, []string{"a.md", "b.md", "c.md", "d.md", "e.md", "f.md", "g.md", "h.md", "i.md", "j.md"}, []string{"n.md"})
		_ = root
		a := NewApp(b)
		var m tea.Model = a
		m, _ = m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		check := func(name string) {
			out := m.View()
			if h := lipgloss.Height(out); h > sz[1] {
				t.Errorf("%dx%d %s: %d rows:\n%s", sz[0], sz[1], name, h, stripANSI(out))
			}
			for _, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w > sz[0] {
					t.Errorf("%dx%d %s: line %d wide", sz[0], sz[1], name, w)
					break
				}
			}
		}
		m, _ = m.Update(runes("/"))
		m, _ = m.Update(runes("doc"))
		check("search typing")
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		check("search kept")
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m, _ = m.Update(runes("o"))
		check("chooser")
		m, _ = m.Update(runes("G"))
		for i := 0; i < 12; i++ {
			m, _ = m.Update(runes("j"))
		}
		check("chooser scrolled")
		if !strings.Contains(stripANSI(m.View()), "n.md") {
			t.Errorf("%dx%d: chooser did not scroll to the last entry:\n%s", sz[0], sz[1], stripANSI(m.View()))
		}
	}
}

// The help lists the new keys and still fits 80x24.
func TestHelpListsNewKeys(t *testing.T) {
	a := NewApp(seeded(t))
	m, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(runes("?"))
	out := stripANSI(m.View())
	for _, want := range []string{"search", "undo the last change", "approval gate", "open attachment", "approve"} {
		if !strings.Contains(out, want) {
			t.Errorf("help is missing %q:\n%s", want, out)
		}
	}
	if h := lipgloss.Height(m.View()); h > 24 {
		t.Errorf("help is %d rows on a 24-row terminal", h)
	}
}

// Dropping a blocker in the terminal column unblocks its dependants.
func TestDroppingBlockerInDoneUnblocksDependants(t *testing.T) {
	b := &task.Board{}
	b.SetColumns(task.DevColumns)
	blocker := b.Add("Blocker", "", nil, ref).ID
	dep := b.Add("Dependant", "", nil, ref).ID
	if err := b.SetBlockedBy(dep, []string{blocker}, ref); err != nil {
		t.Fatal(err)
	}
	b.SyncBlocked(ref)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(140, 40)
	m = press(m, "m")
	for range b.Statuses()[1:] {
		m = press(m, "l")
	}
	m = press(m, "enter")
	if got, _ := b.TaskByID(blocker); got.Status != task.StatusShipped {
		t.Fatalf("blocker = %s, want shipped", got.Status)
	}
	if got, _ := b.TaskByID(dep); got.Status != task.StatusTodo {
		t.Errorf("dependant = %s, want todo after its blocker shipped", got.Status)
	}
	if !strings.Contains(stripANSI(m.View()), "dependencies:") {
		t.Errorf("footer does not report the unblock:\n%s", stripANSI(m.View()))
	}
}

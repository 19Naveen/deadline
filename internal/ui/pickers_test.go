package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/project"
	"gotodo/internal/task"
)

// --- detail order ---

func TestDetailBodyOrderDescriptionSubtasksDocuments(t *testing.T) {
	b := &task.Board{}
	parent := b.Add("Parent", "the description", nil, ref).ID
	if _, err := b.AddChild(parent, "Child", "", nil, ref); err != nil {
		t.Fatal(err)
	}
	setTask(b, parent, func(t *task.Task) {
		t.Attachments = []string{"brief.pdf"}
		t.WorkingNotes = []string{"docs/notes.md"}
	})
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 44)
	tk, _ := b.TaskByID(parent)
	body := stripANSI(strings.Join(m.detailBodyLines(tk, 70), "\n"))
	order := []string{"Description", "the description", "Subtasks", "Child", "Attachments", "brief.pdf", "Working notes", "docs/notes.md"}
	last := -1
	for _, want := range order {
		i := strings.Index(body, want)
		if i < 0 || i < last {
			t.Fatalf("%q out of order (at %d, previous at %d):\n%s", want, i, last, body)
		}
		last = i
	}
}

// --- verify & ship ---

func TestVerifyShipsFromReviewWithGateOffAndUndoRestores(t *testing.T) {
	b, id := reviewBoard(t)
	b.RequireApproval = false
	b.Actor = "claude"
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m.selectTaskID(id)
	if footer := stripANSI(m.renderFooter()); !strings.Contains(footer, "v Verify") {
		t.Errorf("board footer lacks v Verify on a review card: %q", footer)
	}
	if n := len(strings.Fields(strings.TrimSpace(stripANSI(m.renderFooter())))); strings.Count(strings.TrimSpace(stripANSI(m.renderFooter())), "\n") != 0 || n == 0 {
		t.Errorf("footer wrapped at 120 columns: %q", stripANSI(m.renderFooter()))
	}
	m = press(m, "enter")
	out := stripANSI(m.View())
	for _, want := range []string{"ready to verify", "v Verify & ship"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "awaiting approval") {
		t.Errorf("gate is off but the detail says awaiting approval:\n%s", out)
	}
	m = press(m, "y") // y stays gate-only
	if got, _ := b.TaskByID(id); got.Status != task.StatusTestingReview {
		t.Fatalf("y shipped with the gate off: %s", got.Status)
	}
	var cmd tea.Cmd
	m, cmd = m.Update(runes("v"))
	got, _ := b.TaskByID(id)
	if got.Status != task.StatusShipped || cmd == nil || m.mode != modeNormal {
		t.Fatalf("v: status=%s cmd=%v mode=%v", got.Status, cmd != nil, m.mode)
	}
	if last := got.History[len(got.History)-1]; last.Agent != "" || b.Actor != "claude" {
		t.Errorf("verify recorded agent %q, Actor now %q", last.Agent, b.Actor)
	}
	m = press(m, "u")
	if got, _ := b.TaskByID(id); got.Status != task.StatusTestingReview {
		t.Errorf("undo left the card in %s", got.Status)
	}
}

func TestVerifyFromBoardWithGateOn(t *testing.T) {
	b, id := reviewBoard(t)
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	m.selectTaskID(id)
	detail := stripANSI(press(m, "enter").View())
	if !strings.Contains(detail, "awaiting approval") || strings.Contains(detail, "ready to verify") {
		t.Errorf("gate on: want awaiting approval only:\n%s", detail)
	}
	m = press(m, "v")
	if got, _ := b.TaskByID(id); got.Status != task.StatusShipped {
		t.Fatalf("v on the board left the card in %s", got.Status)
	}
	m = press(m, "u")
	if got, _ := b.TaskByID(id); got.Status != task.StatusTestingReview {
		t.Errorf("undo left the card in %s", got.Status)
	}
}

func TestVerifyIgnoresCardsOutsideReview(t *testing.T) {
	b := &task.Board{Columns: task.DevColumns}
	id := b.Add("[Todo]", "", nil, ref).ID
	m := fixedClock(NewBoardModel(b))
	m.SetSize(120, 40)
	if strings.Contains(stripANSI(m.renderFooter()), "v Verify") {
		t.Error("v Verify shown on a todo card")
	}
	m = press(m, "v", "enter", "v")
	if got, _ := b.TaskByID(id); got.Status != task.StatusTodo {
		t.Errorf("v moved a todo card to %s", got.Status)
	}
	if strings.Contains(stripANSI(m.View()), "Verify") {
		t.Errorf("detail offers Verify outside review:\n%s", stripANSI(m.View()))
	}

	personal := &task.Board{}
	pid := personal.Add("[Doing]", "", nil, ref).ID
	_ = personal.Move(pid, task.StatusDoing, ref)
	pm := fixedClock(NewBoardModel(personal))
	pm.SetSize(100, 30)
	pm.selectTaskID(pid)
	pm = press(pm, "v")
	if got, _ := personal.TaskByID(pid); got.Status != personal.DoneStatus() {
		t.Errorf("personal board: v left the doing card in %s", got.Status)
	}
}

// --- temp repositories ---

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, rel string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tempRepo is a repository named "repo" with branches main, feat/picker and
// fix/login, a linked worktree repo-wt on wt-branch, tracked docs, an
// untracked file and an ignored one. It skips without git.
func tempRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "init", "-q", "-b", "main")
	for _, f := range []string{"README.md", "docs/plan.md", "docs/design/form.md", "internal/ui/board.go", ".gitignore"} {
		writeFile(t, dir, f)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("secret.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", ".")
	gitCmd(t, dir, "commit", "-q", "-m", "init")
	gitCmd(t, dir, "branch", "feat/picker")
	gitCmd(t, dir, "branch", "fix/login")
	gitCmd(t, dir, "worktree", "add", "-q", "-b", "wt-branch", filepath.Join(filepath.Dir(dir), "repo-wt"))
	writeFile(t, dir, "notes/untracked.md")
	writeFile(t, dir, "secret.log")
	return dir
}

// recorder wraps the real git runner and remembers every call.
type recorder struct{ calls [][]string }

func (r *recorder) run(dir string, args ...string) (string, error) {
	r.calls = append(r.calls, args)
	return project.Git(dir, args...)
}

func (r *recorder) ran(sub string) bool {
	for _, c := range r.calls {
		if strings.HasPrefix(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

// repoForm opens the add form on a board whose project is dir.
func repoForm(t *testing.T, dir string, w, h int) (BoardModel, *recorder) {
	t.Helper()
	rec := &recorder{}
	m := fixedClock(NewBoardModel(&task.Board{}))
	m.git, m.root = rec.run, dir
	m.SetSize(w, h)
	return press(m, "a"), rec
}

func focus(m BoardModel, field int) BoardModel {
	m.field = field
	m.applyFieldFocus()
	return m
}

func typeText(m BoardModel, s string) BoardModel {
	m, _ = m.Update(runes(s))
	return m
}

func keyType(m BoardModel, k tea.KeyType) BoardModel {
	m, _ = m.Update(tea.KeyMsg{Type: k})
	return m
}

func rowLabels(rows []pickRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.label
	}
	return out
}

// --- worktree picker ---

func TestWorktreePickerListsWorktreesAndBranches(t *testing.T) {
	dir := tempRepo(t)
	m, _ := repoForm(t, dir, 120, 44)
	m = focus(m, fieldWorktree)
	if !m.wt.repo {
		t.Fatal("temp repo not detected")
	}
	labels := rowLabels(m.wt.rows)
	want := []string{"repo", "repo-wt", "feat/picker", "fix/login", "main", "wt-branch"}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("rows = %v, want %v", labels, want)
	}
	out := stripANSI(m.renderForm())
	for _, s := range []string{"6 matches", "repo-wt  ⎇ wt-branch", "feat/picker  branch", "↑↓ Choose", "type to search"} {
		if !strings.Contains(out, s) {
			t.Errorf("form missing %q:\n%s", s, out)
		}
	}

	// Ranking: prefix beats word boundary beats scattered.
	m = typeText(m, "fi")
	if got := rowLabels(m.wt.rows); len(got) == 0 || got[0] != "fix/login" {
		t.Errorf("'fi' ranking = %v", got)
	}
	m = typeText(m, "x")
	if m.wt.index != 0 || !m.wt.choosing {
		t.Errorf("first match not highlighted: index=%d choosing=%v", m.wt.index, m.wt.choosing)
	}
	m = keyType(m, tea.KeyEnter)
	if m.mode != modeInput || m.worktree.Value() != "fix/login" {
		t.Fatalf("enter did not pick the branch: mode=%v value=%q", m.mode, m.worktree.Value())
	}
	// Picking a worktree fills its path.
	m.worktree.SetValue("")
	m = typeText(m, "repo-wt")
	m = keyType(m, tea.KeyEnter)
	if want := filepath.Join(filepath.Dir(dir), "repo-wt"); m.worktree.Value() != want {
		t.Errorf("worktree pick = %q, want %q", m.worktree.Value(), want)
	}
	if hasCreateRow(m.wt.rows) {
		t.Errorf("create row offered for an exact match: %v", rowLabels(m.wt.rows))
	}
}

func hasCreateRow(rows []pickRow) bool { return len(rows) > 0 && rows[len(rows)-1].create }

func TestWorktreeCreateRowOnlyOnNonExactAndNeedsExplicitPick(t *testing.T) {
	dir := tempRepo(t)
	for _, exact := range []string{"main", "feat/picker", "repo-wt"} {
		m, _ := repoForm(t, dir, 120, 44)
		m = typeText(focus(m, fieldWorktree), exact)
		if hasCreateRow(m.wt.rows) {
			t.Errorf("%q: create row on an exact match", exact)
		}
	}

	// Typing a new name offers the create row last, unhighlighted; enter
	// alone saves the task with the free text and creates nothing.
	m, rec := repoForm(t, dir, 120, 44)
	m.title.SetValue("Task")
	m = typeText(focus(m, fieldWorktree), "feat/nu")
	if !hasCreateRow(m.wt.rows) || m.wt.index != -1 {
		t.Fatalf("create row missing or highlighted: %v index=%d", rowLabels(m.wt.rows), m.wt.index)
	}
	out := stripANSI(m.renderForm())
	if !strings.Contains(out, "+ create worktree ../repo-feat-nu on new branch feat/nu") {
		t.Errorf("create row text:\n%s", out)
	}
	m = keyType(m, tea.KeyEnter)
	if m.mode != modeNormal || rec.ran("worktree add") {
		t.Fatalf("enter without a pick: mode=%v, calls=%v", m.mode, rec.calls)
	}
	if got := m.board.Tasks[0].Worktree; got != "feat/nu" {
		t.Errorf("free text worktree = %q", got)
	}

	// ↓ onto the create row, then enter, runs git worktree add.
	m, rec = repoForm(t, dir, 120, 44)
	m = typeText(focus(m, fieldWorktree), "feat/nu")
	m = keyType(m, tea.KeyCtrlN)
	if m.wt.index != len(m.wt.rows)-1 {
		t.Fatalf("ctrl+n did not reach the create row: %d of %d", m.wt.index, len(m.wt.rows))
	}
	m = keyType(m, tea.KeyEnter)
	path := filepath.Join(filepath.Dir(dir), "repo-feat-nu")
	if m.err != "" || m.mode != modeInput || m.worktree.Value() != path {
		t.Fatalf("create: err=%q mode=%v value=%q", m.err, m.mode, m.worktree.Value())
	}
	if !rec.ran("check-ref-format --branch feat/nu") || !rec.ran("worktree add -b feat/nu") {
		t.Errorf("runner calls = %v", rec.calls)
	}
	if _, err := os.Stat(filepath.Join(path, "README.md")); err != nil {
		t.Errorf("worktree not checked out: %v", err)
	}
	if hasCreateRow(m.wt.rows) {
		t.Error("the new worktree is still offered for creation")
	}
}

func TestWorktreeCreateErrorsShowInForm(t *testing.T) {
	dir := tempRepo(t)
	m, rec := repoForm(t, dir, 120, 44)
	m = typeText(focus(m, fieldWorktree), "bad..name")
	m = keyType(m, tea.KeyUp) // ↑ from nothing lands on the last row
	m = keyType(m, tea.KeyEnter)
	if !strings.Contains(m.err, "not a valid branch name") || rec.ran("worktree add") {
		t.Errorf("err=%q calls=%v", m.err, rec.calls)
	}
	if out := stripANSI(m.renderForm()); !strings.Contains(out, "! not a valid branch name") {
		t.Errorf("error not in the form:\n%s", out)
	}
}

func TestWorktreeOutsideGitStaysPlainText(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m.SetSize(120, 40)
	m = focus(press(m, "a"), fieldWorktree)
	m = typeText(m, "some/branch")
	if m.wt.repo || m.computeFormLayout().dropdown != 0 {
		t.Errorf("picker active outside git")
	}
	out := stripANSI(m.renderForm())
	if strings.Contains(out, "↑↓ Choose") || !strings.Contains(out, "path or branch") {
		t.Errorf("outside git the field should be plain:\n%s", out)
	}
}

// --- document picker ---

func TestDocumentPickerListsOnlyInRootFiles(t *testing.T) {
	dir := tempRepo(t)
	m, _ := repoForm(t, dir, 120, 44)
	m = focus(m, fieldAttachments)
	want := []string{".gitignore", "README.md", "docs/design/form.md", "docs/plan.md", "internal/ui/board.go", "notes/untracked.md"}
	if !reflect.DeepEqual(m.doc.files, want) {
		t.Errorf("files = %v, want %v (ignored excluded)", m.doc.files, want)
	}
	// A project root below the repository top lists only its own files.
	sub, _ := repoForm(t, filepath.Join(dir, "docs"), 120, 44)
	sub = focus(sub, fieldWorkingNotes)
	if !reflect.DeepEqual(sub.doc.files, []string{"design/form.md", "plan.md"}) {
		t.Errorf("subdir files = %v", sub.doc.files)
	}
	// Outside git: a walk of the root, never above it.
	plain := t.TempDir()
	writeFile(t, plain, "a.md")
	writeFile(t, plain, ".git-like/hidden.md")
	writeFile(t, plain, "node_modules/x.js")
	pm := fixedClock(NewBoardModel(&task.Board{}))
	pm.root = plain
	pm = focus(press(pm, "a"), fieldAttachments)
	if !reflect.DeepEqual(pm.doc.files, []string{"a.md"}) {
		t.Errorf("walk files = %v", pm.doc.files)
	}
}

func TestDocumentChipsAddRemoveAndRoundTrip(t *testing.T) {
	dir := tempRepo(t)
	m, _ := repoForm(t, dir, 120, 44)
	m.title.SetValue("Docs")
	m = typeText(focus(m, fieldAttachments), "plan")
	if got := rowLabels(m.doc.rows); len(got) == 0 || got[0] != "docs/plan.md" {
		t.Fatalf("'plan' rows = %v", got)
	}
	m = keyType(m, tea.KeyEnter)
	if m.mode != modeInput || !reflect.DeepEqual(m.doc.paths[0], []string{"docs/plan.md"}) || m.attachments.Value() != "" {
		t.Fatalf("enter did not add a chip: mode=%v paths=%v query=%q", m.mode, m.doc.paths[0], m.attachments.Value())
	}
	if out := stripANSI(m.renderForm()); !strings.Contains(out, "docs/plan.md ×") {
		t.Errorf("chip not drawn:\n%s", out)
	}
	for _, r := range m.doc.rows {
		if r.value == "docs/plan.md" {
			t.Error("a chosen file is still offered")
		}
	}
	// ↓ then enter on an empty query adds the highlighted file.
	m = keyType(m, tea.KeyDown)
	picked := m.doc.rows[m.doc.index].value
	m = keyType(m, tea.KeyEnter)
	if len(m.doc.paths[0]) != 2 || m.doc.paths[0][1] != picked {
		t.Fatalf("arrow pick = %v", m.doc.paths[0])
	}
	// Backspace on the empty query removes the last chip.
	m = keyType(m, tea.KeyBackspace)
	if !reflect.DeepEqual(m.doc.paths[0], []string{"docs/plan.md"}) {
		t.Fatalf("backspace left %v", m.doc.paths[0])
	}
	// A path outside the root is offered as typed, never matched.
	m = typeText(m, "~/Downloads/brief, final.pdf")
	if len(m.doc.rows) != 1 || !m.doc.rows[0].create {
		t.Fatalf("outside path rows = %v", rowLabels(m.doc.rows))
	}
	m = keyType(m, tea.KeyEnter)
	// Working notes: something that matches nothing is still addable.
	m = typeText(focus(m, fieldWorkingNotes), "zzz-new-note.md")
	if out := stripANSI(m.renderForm()); !strings.Contains(out, "+ add zzz-new-note.md") {
		t.Errorf("no add row for an unmatched path:\n%s", out)
	}
	m = keyType(m, tea.KeyEnter)
	m = keyType(m, tea.KeyEnter) // empty query: save
	if m.mode != modeNormal {
		t.Fatalf("form did not save: err=%q", m.err)
	}
	saved := m.board.Tasks[0]
	if !reflect.DeepEqual(saved.Attachments, []string{"docs/plan.md", "~/Downloads/brief, final.pdf"}) ||
		!reflect.DeepEqual(saved.WorkingNotes, []string{"zzz-new-note.md"}) {
		t.Fatalf("saved attachments=%v notes=%v", saved.Attachments, saved.WorkingNotes)
	}
	// Editing prefills the chips and saves them back unchanged.
	m = press(m, "e")
	if !reflect.DeepEqual(m.doc.paths[0], saved.Attachments) || !reflect.DeepEqual(m.doc.paths[1], saved.WorkingNotes) {
		t.Errorf("edit chips = %v", m.doc.paths)
	}
	m = keyType(m, tea.KeyEnter)
	if got := m.board.Tasks[0]; !reflect.DeepEqual(got.Attachments, saved.Attachments) || !reflect.DeepEqual(got.WorkingNotes, saved.WorkingNotes) {
		t.Errorf("round trip changed documents: %v %v", got.Attachments, got.WorkingNotes)
	}
}

func TestDocumentLeftoverQueryBlocksSave(t *testing.T) {
	m := fixedClock(NewBoardModel(&task.Board{}))
	m.SetSize(120, 40)
	m = press(m, "a")
	m.title.SetValue("Task")
	m = typeText(focus(m, fieldAttachments), "half-typed")
	m = focus(m, fieldTitle)
	m = keyType(m, tea.KeyEnter)
	if m.mode != modeInput || m.field != fieldAttachments || !strings.Contains(m.err, "enter to add") {
		t.Errorf("leftover query: mode=%v field=%d err=%q", m.mode, m.field, m.err)
	}
}

func TestChipLineFitsAndCounts(t *testing.T) {
	paths := []string{"docs/one.md", "docs/two.md", "docs/three.md", "docs/four.md"}
	for _, avail := range []int{6, 12, 20, 40, 80} {
		for _, fromEnd := range []bool{false, true} {
			line := chipLine(paths, avail, fromEnd)
			if w := lipgloss.Width(line); w > avail {
				t.Errorf("avail %d fromEnd %v: width %d %q", avail, fromEnd, w, stripANSI(line))
			}
		}
	}
	if got := stripANSI(chipLine(paths, 30, true)); !strings.HasPrefix(got, "+") || !strings.HasSuffix(got, "docs/four.md ×") {
		t.Errorf("fromEnd chips = %q", got)
	}
	if got := stripANSI(chipLine(paths, 30, false)); !strings.HasPrefix(got, "docs/one.md ×") || !strings.Contains(got, "+") {
		t.Errorf("from start chips = %q", got)
	}
}

// --- heights ---

// Every dropdown open, on every size from 80x15 to 120x44: the form never
// outgrows the terminal or its width, and the focused field stays visible.
func TestFormWithDropdownsFitsEverySize(t *testing.T) {
	dir := tempRepo(t)
	for i := 0; i < 8; i++ {
		gitCmd(t, dir, "branch", "feat/extra-"+string(rune('a'+i)))
	}
	b := &task.Board{}
	parent := b.Add("A parent task", "", nil, ref).ID
	b.Add("Another parent", "", nil, ref)
	for _, w := range []int{80, 100, 120} {
		for h := 15; h <= 44; h++ {
			for _, c := range []struct {
				field int
				typed string
				label string
			}{
				{fieldWorktree, "", "Worktree"},
				{fieldWorktree, "feat/new-thing", "Worktree"},
				{fieldParent, "", "Parent"},
				{fieldAttachments, "", "Attachments"},
				{fieldAttachments, "md", "Attachments"},
				{fieldWorkingNotes, "zzz", "Working notes"},
			} {
				m := fixedClock(NewBoardModel(b))
				m.git, m.root = project.Git, dir
				m.SetSize(w, h)
				m.selectTaskID(parent)
				m = press(m, "s") // a subtask form: relationships row too
				m.desc.SetValue("line one\nline two\nline three")
				m.doc.paths = [2][]string{{"docs/plan.md", "README.md"}, {"notes/untracked.md"}}
				m = focus(m, c.field)
				if c.typed != "" {
					m = typeText(m, c.typed)
				}
				m.sizeForm()
				view := m.View()
				if got := lipgloss.Height(view) + 1; got > h { // +1 for the app tab row
					t.Errorf("%dx%d field %d %q: form needs %d rows:\n%s", w, h, c.field, c.typed, got, stripANSI(view))
				}
				for _, line := range strings.Split(view, "\n") {
					if lw := lipgloss.Width(line); lw > w {
						t.Errorf("%dx%d field %d: line %d wide: %q", w, h, c.field, lw, stripANSI(line))
						break
					}
				}
				out := stripANSI(view)
				l := m.computeFormLayout()
				lines := strings.Split(stripANSI(m.renderForm()), "\n")
				for i, line := range lines {
					for _, label := range []string{"Parent", "Attachments"} {
						if strings.HasPrefix(strings.Trim(line, "│ "), label) {
							if blank := strings.Trim(lines[i-1], "│ ") == ""; blank != l.gaps {
								t.Errorf("%dx%d field %d: gap before %s = %v, layout gaps = %v:\n%s", w, h, c.field, label, blank, l.gaps, out)
							}
						}
					}
				}
				if !strings.Contains(out, c.label) {
					t.Errorf("%dx%d: focused field %s hidden:\n%s", w, h, c.label, out)
				}
				if h >= 30 && m.computeFormLayout().dropdown == 0 {
					t.Errorf("%dx%d field %d: no dropdown on a tall terminal", w, h, c.field)
				}
			}
		}
	}
}

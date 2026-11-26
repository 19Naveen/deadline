package project

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func names(items []string, ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = items[m.Index]
	}
	return out
}

func TestFuzzyRanksPrefixBoundaryContiguousScattered(t *testing.T) {
	items := []string{
		"xfoxrxm",       // scattered f-o-r-m
		"platform",      // contiguous "form" mid-word
		"docs/form.md",  // "form" at a word boundary
		"formula",       // prefix
		"unrelated.txt", // no match
	}
	got := names(items, Fuzzy("FORM", items))
	want := []string{"formula", "docs/form.md", "platform", "xfoxrxm"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ranking = %v, want %v", got, want)
	}
}

func TestFuzzyTiesPreferShorterThenOrder(t *testing.T) {
	items := []string{"feat/longer-name", "feat/x", "feat/y"}
	got := names(items, Fuzzy("feat", items))
	want := []string{"feat/x", "feat/y", "feat/longer-name"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ties = %v, want %v", got, want)
	}
}

func TestFuzzyEmptyQueryKeepsOrderAndNoMatchIsEmpty(t *testing.T) {
	items := []string{"b", "a"}
	if got := names(items, Fuzzy("  ", items)); !reflect.DeepEqual(got, items) {
		t.Errorf("empty query = %v", got)
	}
	if got := Fuzzy("zzz", items); len(got) != 0 {
		t.Errorf("no-match query = %v", got)
	}
	if got := Fuzzy("abc", []string{"ab"}); len(got) != 0 {
		t.Errorf("longer query than item matched: %v", got)
	}
}

func TestFuzzyScatteredPrefersWordStarts(t *testing.T) {
	items := []string{"abxxcd", "a-b-c-d"}
	got := names(items, Fuzzy("abcd", items))
	if got[0] != "a-b-c-d" {
		t.Errorf("word-start letters should win: %v", got)
	}
}

func TestSlugAndNewWorktreePath(t *testing.T) {
	if Slug("feat/ui/form") != "feat-ui-form" {
		t.Error(Slug("feat/ui/form"))
	}
	if got := NewWorktreePath("/src/repo", "feat/x"); got != filepath.Join("/src", "repo-feat-x") {
		t.Error(got)
	}
}

// gitRepo makes a temp repository with one commit; it skips without git.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	write(t, dir, "README.md", "hi")
	write(t, dir, "docs/plan.md", "plan")
	write(t, dir, ".gitignore", "secret.log\nbuild/\n")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	run("branch", "feat/picker")
	write(t, dir, "untracked.txt", "u")
	write(t, dir, "secret.log", "s")
	write(t, dir, "build/out.bin", "b")
	return dir
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitListsWorktreesBranchesAndFiles(t *testing.T) {
	dir := gitRepo(t)
	p := New(dir)
	top, ok := p.TopLevel()
	if !ok {
		t.Fatal("temp repo not detected")
	}
	branches, err := p.Branches()
	if err != nil || !reflect.DeepEqual(branches, []string{"feat/picker", "main"}) {
		t.Errorf("branches = %v, %v", branches, err)
	}
	path := NewWorktreePath(top, "feat/new")
	if err := p.AddWorktree("feat/new", path); err != nil {
		t.Fatal(err)
	}
	wts, err := p.Worktrees()
	if err != nil || len(wts) != 2 {
		t.Fatalf("worktrees = %+v, %v", wts, err)
	}
	if !wts[0].Main || wts[0].Path != top || wts[0].Branch != "main" {
		t.Errorf("main worktree = %+v (top %s)", wts[0], top)
	}
	if wts[1].Main || wts[1].Branch != "feat/new" || filepath.Base(wts[1].Path) != "repo-feat-new" {
		t.Errorf("new worktree = %+v", wts[1])
	}

	files := p.Files()
	want := []string{".gitignore", "README.md", "docs/plan.md", "untracked.txt"}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("files = %v, want %v (ignored files must stay out)", files, want)
	}
	// From a subdirectory only that directory's files are listed.
	if got := New(filepath.Join(dir, "docs")).Files(); !reflect.DeepEqual(got, []string{"plan.md"}) {
		t.Errorf("subdir files = %v", got)
	}
}

func TestAddWorktreeRejectsBadBranch(t *testing.T) {
	dir := gitRepo(t)
	p := New(dir)
	for _, bad := range []string{"has space", "-x", "a..b", ""} {
		if err := p.AddWorktree(bad, filepath.Join(t.TempDir(), "wt")); err == nil {
			t.Errorf("branch %q accepted", bad)
		}
	}
}

func TestFilesOutsideGitWalksAndSkips(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.md", "")
	write(t, dir, "sub/b.txt", "")
	write(t, dir, ".hidden/c.md", "")
	write(t, dir, "node_modules/d.js", "")
	write(t, dir, "vendor/e.go", "")
	write(t, dir, ".env", "")
	notGit := func(string, ...string) (string, error) { return "", errors.New("not a git repository") }
	got := Project{Root: dir, Run: notGit}.Files()
	want := []string{".env", "a.md", "sub/b.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("walk = %v, want %v", got, want)
	}
	if got := WalkFiles(dir, 2); len(got) != 2 {
		t.Errorf("limit ignored: %v", got)
	}
}

func TestWorktreesParsesPorcelainAndSkipsBare(t *testing.T) {
	porcelain := strings.Join([]string{
		"worktree /src/repo.git", "bare", "",
		"worktree /src/repo", "HEAD abc", "branch refs/heads/main", "",
		"worktree /src/repo-x", "HEAD def", "detached", "",
	}, "\n")
	fake := func(dir string, args ...string) (string, error) { return porcelain, nil }
	wts, err := Project{Root: "/src/repo", Run: fake}.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	want := []Worktree{{Path: "/src/repo", Branch: "main", Main: true}, {Path: "/src/repo-x"}}
	if !reflect.DeepEqual(wts, want) {
		t.Errorf("worktrees = %+v, want %+v", wts, want)
	}
}

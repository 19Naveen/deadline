package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotodo/internal/mcp"
	"gotodo/internal/task"
)

// isolateBoards points the personal board and HOME at temp dirs and clears
// the agent and loop variables, so nothing touches the real boards. It
// returns the personal board path.
func isolateBoards(t *testing.T) string {
	t.Helper()
	config := realTempDir(t)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", realTempDir(t))
	t.Setenv("GOTODO_BOARD", "")
	t.Setenv(task.AgentEnv, "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return filepath.Join(config, "gotodo", "tasks.json")
}

// realTempDir is t.TempDir with symlinks resolved, so paths compare equal
// to what git reports.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.invalid",
		"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// newRepo creates <parent>/repo with one commit (including sub/) and a
// linked worktree at <parent>/wt, and returns both roots.
func newRepo(t *testing.T) (repo, wt string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	parent := realTempDir(t)
	repo = filepath.Join(parent, "repo")
	wt = filepath.Join(parent, "wt")
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "sub", "file.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "init")
	git(t, repo, "worktree", "add", "-q", "-b", "feature", wt)
	return repo, wt
}

// captureOutput returns what fn wrote to stdout and stderr.
func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	oldErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	func() {
		defer func() { os.Stderr = oldErr }()
		stdout = captureStdout(t, fn)
	}()
	w.Close()
	return stdout, <-done
}

func runOK(t *testing.T, argv ...string) {
	t.Helper()
	if err := run(argv); err != nil {
		t.Fatalf("run %v = %v", argv, err)
	}
}

func taskTitles(t *testing.T, path string) []string {
	t.Helper()
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, tk := range b.Tasks {
		titles = append(titles, tk.Title)
	}
	return titles
}

func assertNoProjectBoard(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, projectDirName)); !os.IsNotExist(err) {
			t.Errorf("%s/.deadline exists (err %v), want no project board created", dir, err)
		}
	}
}

func boardFile(root string) string { return filepath.Join(root, projectDirName, projectBoardFile) }

// A linked worktree has no .deadline (it is gitignored), so resolution asks
// git for the main checkout and uses its board.
func TestWorktreeResolvesMainCheckoutBoard(t *testing.T) {
	isolateBoards(t)
	repo, wt := newRepo(t)
	chdir(t, repo)
	captureStdout(t, func() { runOK(t, "init") })

	for _, dir := range []string{wt, filepath.Join(wt, "sub")} {
		chdir(t, dir)
		got, err := resolveBoardPath()
		if err != nil {
			t.Fatal(err)
		}
		if got != boardFile(repo) {
			t.Errorf("resolveBoardPath() from %s = %q, want the main checkout board %q", dir, got, boardFile(repo))
		}
	}
}

// gotodo init inside a linked worktree initialises the main checkout.
func TestInitInWorktreeInitialisesMainCheckout(t *testing.T) {
	isolateBoards(t)
	repo, wt := newRepo(t)
	chdir(t, wt)
	out := captureStdout(t, func() { runOK(t, "init") })
	if !strings.Contains(out, "initialised "+boardFile(repo)) {
		t.Errorf("init output = %q, want the main checkout board", out)
	}
	if _, err := os.Stat(boardFile(repo)); err != nil {
		t.Fatalf("main checkout board: %v", err)
	}
	assertNoProjectBoard(t, wt)
	if data, _ := os.ReadFile(filepath.Join(repo, ".gitignore")); !strings.Contains(string(data), ".deadline/") {
		t.Errorf("main checkout .gitignore = %q, want .deadline/", data)
	}
	// A second init from the worktree sees the main board.
	out = captureStdout(t, func() { runOK(t, "init") })
	if !strings.Contains(out, "already initialised: "+boardFile(repo)) {
		t.Errorf("second init output = %q", out)
	}
}

// An agent's first change inside a git repo without a board creates the
// project board at the main checkout root, even from a worktree subdir.
func TestAgentAutoInitsProjectBoardInRepo(t *testing.T) {
	for _, from := range []string{"repo/sub", "wt/sub"} {
		t.Run(from, func(t *testing.T) {
			personal := isolateBoards(t)
			repo, wt := newRepo(t)
			t.Setenv(task.AgentEnv, "claude")
			dir := filepath.Join(filepath.Dir(repo), from)
			chdir(t, dir)
			stdout, stderr := captureOutput(t, func() {
				runOK(t, "add", "Fix parser", "-desc", "goal")
			})
			want := "initialised project board " + boardFile(repo) + " (agents always use a project board inside a git repo)"
			if strings.TrimSpace(stderr) != want {
				t.Errorf("stderr = %q, want exactly %q", stderr, want)
			}
			if strings.Contains(stdout, "initialised") || strings.Contains(stdout, ".gitignore") {
				t.Errorf("stdout = %q, want only the add receipt", stdout)
			}
			if got := taskTitles(t, boardFile(repo)); len(got) != 1 || got[0] != "Fix parser" {
				t.Errorf("project board tasks = %v, want [Fix parser]", got)
			}
			b, err := task.Load(boardFile(repo))
			if err != nil {
				t.Fatal(err)
			}
			if b.DoneStatus() != task.StatusShipped {
				t.Errorf("auto-initialised board done column = %q, want shipped (dev columns)", b.DoneStatus())
			}
			if data, _ := os.ReadFile(filepath.Join(repo, ".gitignore")); !strings.Contains(string(data), ".deadline/") {
				t.Errorf(".gitignore = %q, want .deadline/", data)
			}
			assertNoProjectBoard(t, wt)
			if _, err := os.Stat(personal); !os.IsNotExist(err) {
				t.Errorf("personal board written (err %v), want it untouched", err)
			}
		})
	}
}

// Humans in a repo without a board keep the personal board, with a note.
func TestHumanInRepoUsesPersonalBoardWithNote(t *testing.T) {
	personal := isolateBoards(t)
	repo, _ := newRepo(t)
	chdir(t, repo)
	note := "note: you are in git repo " + repo + ", which has no project board; agents will create one on their first change (or run gotodo init)"

	_, stderr := captureOutput(t, func() { runOK(t, "add", "Buy milk") })
	if !strings.Contains(stderr, note) {
		t.Errorf("add stderr = %q, want the note", stderr)
	}
	if got := taskTitles(t, personal); len(got) != 1 || got[0] != "Buy milk" {
		t.Errorf("personal board tasks = %v, want [Buy milk]", got)
	}
	stdout, stderr := captureOutput(t, func() { runOK(t, "board") })
	if !strings.Contains(stdout, "path: "+personal+" (personal)") {
		t.Errorf("board stdout = %q, want the personal path", stdout)
	}
	if !strings.Contains(stderr, note) {
		t.Errorf("board stderr = %q, want the note", stderr)
	}
	assertNoProjectBoard(t, repo)
}

// Outside a git repo the personal board is the right one: no auto-init and
// no note, even for agents.
func TestNoAutoInitOutsideGitRepo(t *testing.T) {
	personal := isolateBoards(t)
	t.Setenv(task.AgentEnv, "claude")
	dir := realTempDir(t)
	chdir(t, dir)
	_, stderr := captureOutput(t, func() {
		runOK(t, "add", "Book dentist")
		runOK(t, "board")
	})
	if strings.Contains(stderr, "note:") || strings.Contains(stderr, "initialised") {
		t.Errorf("stderr = %q, want no note or init outside a repo", stderr)
	}
	if got := taskTitles(t, personal); len(got) != 1 {
		t.Errorf("personal board tasks = %v, want the one card", got)
	}
	assertNoProjectBoard(t, dir)
}

// A dotfiles repository at $HOME is not a project.
func TestNoAutoInitWhenRepoIsHome(t *testing.T) {
	personal := isolateBoards(t)
	repo, _ := newRepo(t)
	t.Setenv("HOME", repo)
	t.Setenv(task.AgentEnv, "claude")
	chdir(t, filepath.Join(repo, "sub"))
	captureOutput(t, func() { runOK(t, "add", "Tidy dotfiles") })
	assertNoProjectBoard(t, repo)
	if got := taskTitles(t, personal); len(got) != 1 {
		t.Errorf("personal board tasks = %v, want the one card", got)
	}
}

// Read-only commands never create a board, even for agents; board says
// which board it is and adds the note.
func TestReadOnlyCommandsCreateNothing(t *testing.T) {
	personal := isolateBoards(t)
	repo, wt := newRepo(t)
	t.Setenv(task.AgentEnv, "claude")
	chdir(t, wt)
	stdout, stderr := captureOutput(t, func() {
		runOK(t, "board")
		runOK(t, "list")
		runOK(t, "standup")
	})
	assertNoProjectBoard(t, repo, wt)
	if !strings.Contains(stdout, "path: "+personal+" (personal)") {
		t.Errorf("board stdout = %q, want the personal path", stdout)
	}
	if !strings.Contains(stderr, "note: you are in git repo "+repo+", which has no project board") {
		t.Errorf("board stderr = %q, want the note naming the main checkout", stderr)
	}
	if strings.Count(stderr, "note:") != 1 {
		t.Errorf("stderr = %q, want the note from board only", stderr)
	}
}

// -file and GOTODO_BOARD beat auto-init.
func TestOverridesBeatAutoInit(t *testing.T) {
	isolateBoards(t)
	repo, _ := newRepo(t)
	t.Setenv(task.AgentEnv, "claude")
	chdir(t, repo)

	file := filepath.Join(realTempDir(t), "board.json")
	captureOutput(t, func() { runOK(t, "-file", file, "add", "Via file") })
	if got := taskTitles(t, file); len(got) != 1 {
		t.Errorf("-file board tasks = %v", got)
	}

	env := filepath.Join(realTempDir(t), "loop.json")
	if err := os.WriteFile(env, []byte(`{"tasks":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOTODO_BOARD", env)
	captureOutput(t, func() { runOK(t, "add", "Via env") })
	if got := taskTitles(t, env); len(got) != 1 || got[0] != "Via env" {
		t.Errorf("GOTODO_BOARD tasks = %v", got)
	}
	assertNoProjectBoard(t, repo)
}

// mcpSession runs one MCP session of tools/call requests (after the
// handshake and tools/list) and returns each tool's text.
func mcpSession(t *testing.T, calls ...string) []string {
	t.Helper()
	var in strings.Builder
	in.WriteString(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"test"}}}` + "\n")
	in.WriteString(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n")
	in.WriteString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n")
	for i, c := range calls {
		in.WriteString(`{"jsonrpc":"2.0","id":` + jsonNum(i+2) + `,"method":"tools/call","params":` + c + "}\n")
	}
	srv := mcp.NewServer(mcp.Config{Resolve: mcpResolver("", io.Discard), Agent: "claude"})
	var out bytes.Buffer
	if err := srv.Serve(strings.NewReader(in.String()), &out); err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n")[2:] {
		var r struct {
			Result struct {
				Content []struct{ Text string } `json:"content"`
				IsError bool                    `json:"isError"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if r.Result.IsError || len(r.Result.Content) != 1 {
			t.Fatalf("tool reply = %s", line)
		}
		texts = append(texts, r.Result.Content[0].Text)
	}
	return texts
}

func jsonNum(n int) string {
	data, _ := json.Marshal(n)
	return string(data)
}

// The MCP server resolves per call: opening a session and reading creates
// nothing; the first write creates the project board.
func TestMCPAutoInitsOnFirstWrite(t *testing.T) {
	personal := isolateBoards(t)
	repo, wt := newRepo(t)
	chdir(t, wt)

	got := mcpSession(t, `{"name":"board"}`, `{"name":"list_tasks"}`)
	assertNoProjectBoard(t, repo, wt)
	if !strings.Contains(got[0], "path: "+personal+" (personal)") ||
		!strings.Contains(got[0], "note: you are in git repo "+repo) {
		t.Errorf("board before any write = %q, want personal path and note", got[0])
	}

	got = mcpSession(t, `{"name":"add_task","arguments":{"title":"Fix parser"}}`, `{"name":"board"}`)
	if !strings.HasPrefix(got[0], "initialised project board "+boardFile(repo)) {
		t.Errorf("first add_task = %q, want the init notice", got[0])
	}
	if !strings.Contains(got[1], "path: "+boardFile(repo)+" (project)") || strings.Contains(got[1], "note:") {
		t.Errorf("board after add = %q, want the project path and no note", got[1])
	}
	if titles := taskTitles(t, boardFile(repo)); len(titles) != 1 || titles[0] != "Fix parser" {
		t.Errorf("project board tasks = %v", titles)
	}
	assertNoProjectBoard(t, wt)
}

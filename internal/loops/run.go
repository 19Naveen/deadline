package loops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gotodo/internal/task"
)

// Command is one agent CLI invocation. It is executed directly (never via a
// shell), so the prompt is a single argv entry whatever it contains.
type Command struct {
	Name  string
	Args  []string
	Dir   string
	Env   []string // full environment
	Notes []string // caveats written to the run log (e.g. permissions the CLI cannot express)
}

// String renders the command for the log, quoting arguments with spaces.
func (c Command) String() string {
	parts := []string{c.Name}
	for _, a := range c.Args {
		if a == "" || strings.ContainsAny(a, " \t\n'\"\\$`*?()[]{}<>|&;") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// claudeAllowed lets a Claude Code run keep its card current without the
// prompts that -p cannot answer.
var claudeAllowed = []string{"Bash(gotodo *)", "mcp__gotodo"}

// BuildCommand maps a stage to its agent CLI's headless flags (checked
// against claude 2.1, codex-cli 0.171 and opencode 2.4 --help):
//
//	claude   -p [--allowedTools ...] --permission-mode plan|acceptEdits | --dangerously-skip-permissions [--model M] PROMPT
//	codex    exec --skip-git-repo-check --sandbox read-only|workspace-write | --dangerously-bypass-approvals-and-sandbox [-m M] PROMPT
//	opencode run --title T [-m provider/model] [--auto] PROMPT
//
// dir is the working directory and boardPath the board the card is on.
func BuildCommand(stageName string, st Stage, prompt, dir, boardPath string) Command {
	if strings.HasPrefix(prompt, "-") {
		prompt = " " + prompt // never let a prompt parse as a flag
	}
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		abs = boardPath
	}
	c := Command{Name: st.Agent, Dir: dir, Env: agentEnv(os.Environ(), stageName, abs)}
	perm := st.Perm()
	switch st.Agent {
	case "claude":
		c.Args = []string{"-p"}
		switch perm {
		case PermFull:
			c.Args = append(c.Args, "--dangerously-skip-permissions")
		case PermReadOnly:
			c.Args = append(c.Args, "--allowedTools")
			c.Args = append(c.Args, claudeAllowed...)
			c.Args = append(c.Args, "--permission-mode", "plan")
		default:
			c.Args = append(c.Args, "--allowedTools")
			c.Args = append(c.Args, claudeAllowed...)
			c.Args = append(c.Args, "--permission-mode", "acceptEdits")
			c.Notes = append(c.Notes, "claude acceptEdits: file edits are allowed; shell commands other than gotodo are denied (use permissions full to allow them)")
		}
		if st.Model != "" {
			c.Args = append(c.Args, "--model", st.Model)
		}
	case "codex":
		c.Args = []string{"exec", "--skip-git-repo-check"}
		switch perm {
		case PermFull:
			c.Args = append(c.Args, "--dangerously-bypass-approvals-and-sandbox")
		case PermReadOnly:
			c.Args = append(c.Args, "--sandbox", "read-only")
			c.Notes = append(c.Notes, "codex read-only sandbox: the agent cannot write the board, so gotodo updates from it will fail")
		default:
			c.Args = append(c.Args, "--sandbox", "workspace-write")
			if boardDir := filepath.Dir(abs); !within(dir, boardDir) {
				c.Args = append(c.Args, "--add-dir", boardDir) // so gotodo can save the board
			}
		}
		if st.Model != "" {
			c.Args = append(c.Args, "-m", st.Model)
		}
	case "opencode":
		c.Args = []string{"run", "--title", "gotodo loop " + stageName}
		if st.Model != "" {
			c.Args = append(c.Args, "-m", st.Model)
		}
		switch perm {
		case PermFull:
			c.Args = append(c.Args, "--auto")
		case PermReadOnly:
			c.Notes = append(c.Notes, "opencode run has no read-only flag: permissions come from your opencode config (set edit and bash to deny for a read-only agent)")
		default:
			c.Notes = append(c.Notes, "opencode run uses your opencode config's permissions; anything set to ask is not auto-approved (use permissions full for --auto)")
		}
	}
	c.Args = append(c.Args, prompt)
	return c
}

func within(dir, target string) bool {
	rel, err := filepath.Rel(dir, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// agentEnv is base with the loop's identity set and nested-session markers
// removed (Claude Code refuses to start inside another session's env).
func agentEnv(base []string, stage, boardAbs string) []string {
	drop := map[string]bool{task.AgentEnv: true, LoopEnv: true, BoardEnv: true,
		"CLAUDECODE": true, "CLAUDE_CODE_ENTRYPOINT": true}
	env := make([]string, 0, len(base)+3)
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i > 0 && drop[kv[:i]] {
			continue
		}
		env = append(env, kv)
	}
	return append(env, task.AgentEnv+"="+AgentName(stage), LoopEnv+"=1", BoardEnv+"="+boardAbs)
}

// Run is the body of `gotodo loops run <task> <stage>`: claim the card as
// loop:<stage>, run the stage's agent in the task's worktree (else the
// project root) with a timeout, log everything, record the result, release
// the claim and drain the queue. out receives a one-line summary.
func Run(boardPath, taskID, stage string, out io.Writer) error {
	cfg, _, err := Load(boardPath)
	if err != nil {
		return err
	}
	st, ok := cfg.Stages[stage]
	if !ok {
		return fmt.Errorf("no loop configured for stage %q", stage)
	}
	b, err := task.Load(boardPath)
	if err != nil {
		return err
	}
	t, err := findTask(b, taskID)
	if err != nil {
		return err
	}
	dir := StateDir(boardPath)
	rec, err := registerRun(boardPath, dir, t.ID, stage, st, time.Now())
	if err != nil {
		return err
	}
	logf, err := os.OpenFile(rec.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	agent := AgentName(stage)
	timeout := cfg.Timeout()
	now := time.Now()
	fmt.Fprintf(logf, "== gotodo loop · %s · run %d · task %s (%s)\n", stage, rec.N, shortID(t.ID), t.Title)
	fmt.Fprintf(logf, "started:  %s\n", now.Format(stampLayout))

	finish := func(status string, exit *int, note string) {
		end := time.Now()
		withState(boardPath, func(s *state) (bool, error) {
			for i := range s.Runs {
				r := &s.Runs[i]
				if r.TaskID == rec.TaskID && r.Stage == rec.Stage && r.N == rec.N && r.Status == RunRunning {
					s.finish(r, status, exit, note, end)
					return true, nil
				}
			}
			return false, nil
		})
		code := "-"
		if exit != nil {
			code = fmt.Sprint(*exit)
		}
		fmt.Fprintf(logf, "== ended %s · %s · exit %s · %s\n", end.Format(stampLayout), status, code,
			end.Sub(rec.Started).Round(time.Second))
		if note != "" {
			fmt.Fprintf(logf, "note: %s\n", note)
		}
		if out != nil {
			fmt.Fprintf(out, "loop %s run %d for %s: %s (log %s)\n", stage, rec.N, shortID(t.ID), status, rec.Log)
		}
	}

	err = task.UpdateLocked(boardPath, agent, func(b *task.Board) error {
		return b.Claim(t.ID, agent, timeout+10*time.Minute, now)
	})
	if err != nil {
		msg := "claim: " + err.Error()
		fmt.Fprintln(logf, msg)
		eventLog(dir, now, "loop skipped: %s %s: %s", shortID(t.ID), stage, msg)
		finish(RunSkipped, nil, msg)
		return errors.New(msg)
	}
	defer func() {
		if err := releaseClaim(boardPath, t.ID, stage, time.Now()); err != nil {
			fmt.Fprintf(logf, "release claim: %v\n", err)
		}
		if _, err := Dispatch(boardPath, time.Now()); err != nil {
			fmt.Fprintf(logf, "dispatch: %v\n", err)
		}
	}()

	cwd, wtNote := workDir(boardPath, t)
	cols := make([]string, 0, len(b.Statuses()))
	for _, c := range b.Statuses() {
		cols = append(cols, string(c))
	}
	prompt := st.PromptFor(shortID(t.ID), t.Title, stage, NextStage(cols, stage), absPath(boardPath)) + boardHint(cwd, boardPath)
	cmd := BuildCommand(stage, st, prompt, cwd, boardPath)
	fmt.Fprintf(logf, "agent:    %s · model %s · permissions %s · skill %s\n", st.Agent, orDash(st.Model), st.Perm(), orDash(st.Skill))
	fmt.Fprintf(logf, "cwd:      %s\n", cwd)
	fmt.Fprintf(logf, "timeout:  %s\n", timeout)
	fmt.Fprintf(logf, "command:  %s\n", cmd)
	if wtNote != "" {
		cmd.Notes = append([]string{wtNote}, cmd.Notes...)
	}
	for _, n := range cmd.Notes {
		fmt.Fprintf(logf, "note:     %s\n", n)
	}
	fmt.Fprintln(logf, "---")

	if _, err := Hooks.LookPath(cmd.Name); err != nil {
		exit := -1
		finish(RunFailed, &exit, fmt.Sprintf("%s not found on PATH", cmd.Name))
		return fmt.Errorf("%s not found on PATH", cmd.Name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	exit, runErr := Hooks.ExecAgent(ctx, cmd, logf, func(pid int) {
		withState(boardPath, func(s *state) (bool, error) {
			for i := range s.Runs {
				r := &s.Runs[i]
				if r.TaskID == rec.TaskID && r.Stage == rec.Stage && r.N == rec.N {
					r.AgentPID = pid
					return true, nil
				}
			}
			return false, nil
		})
	})
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		finish(RunTimeout, &exit, fmt.Sprintf("stopped after %s", timeout))
	case runErr != nil:
		finish(RunFailed, &exit, runErr.Error())
	case exit != 0:
		finish(RunFailed, &exit, "")
	default:
		finish(RunDone, &exit, "")
	}
	return nil
}

// registerRun finds the record Dispatch made for this runner (same task,
// stage and pid), or creates one for a manual `gotodo loops run`.
func registerRun(boardPath, dir, taskID, stage string, st Stage, now time.Time) (RunRecord, error) {
	var rec RunRecord
	pid := os.Getpid()
	err := withState(boardPath, func(s *state) (bool, error) {
		for _, r := range s.running() {
			if r.TaskID == taskID && r.Stage == stage && r.PID == pid {
				rec = *r
				return false, nil
			}
		}
		for _, r := range s.running() {
			if r.TaskID == taskID {
				return false, fmt.Errorf("task %s already has a live %s run (pid %d); stop it first", shortID(taskID), r.Stage, r.PID)
			}
		}
		if s.Counts == nil {
			s.Counts = map[string]int{}
		}
		key := countKey(taskID, stage)
		s.Counts[key]++
		rec = RunRecord{N: s.Counts[key], TaskID: taskID, Stage: stage, Agent: st.Agent, Model: st.Model,
			PID: pid, Started: now, Status: RunRunning,
			Log: filepath.Join(dir, fmt.Sprintf("%s-%s-%d.log", shortID(taskID), stage, s.Counts[key]))}
		s.Runs = append(s.Runs, rec)
		return true, nil
	})
	return rec, err
}

// findTask matches an exact ID, else a unique prefix.
func findTask(b *task.Board, id string) (task.Task, error) {
	if t, ok := b.TaskByID(id); ok {
		return t, nil
	}
	var match []task.Task
	for _, t := range b.Tasks {
		if id != "" && strings.HasPrefix(t.ID, id) {
			match = append(match, t)
		}
	}
	switch len(match) {
	case 1:
		return match[0], nil
	case 0:
		return task.Task{}, fmt.Errorf("no task id starts with %q", id)
	}
	return task.Task{}, fmt.Errorf("ambiguous id %q", id)
}

// workDir is the task's worktree when it names an existing directory
// (relative paths resolve against the project root), else the project root.
func workDir(boardPath string, t task.Task) (dir, note string) {
	root := ProjectRoot(boardPath)
	wt := strings.TrimSpace(t.Worktree)
	if wt == "" {
		return root, ""
	}
	if strings.HasPrefix(wt, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			wt = filepath.Join(home, wt[2:])
		}
	}
	if !filepath.IsAbs(wt) {
		wt = filepath.Join(root, wt)
	}
	if st, err := os.Stat(wt); err == nil && st.IsDir() {
		return wt, ""
	}
	return root, fmt.Sprintf("worktree %q is not a directory here; running in %s", t.Worktree, root)
}

// boardHint tells the agent how to reach the board when gotodo would not
// find it from cwd on its own (e.g. a worktree outside the project).
func boardHint(cwd, boardPath string) string {
	abs := absPath(boardPath)
	if discoverBoard(cwd) == abs {
		return ""
	}
	return fmt.Sprintf(" This card's board is not the one gotodo finds from %s: run every gotodo command as `gotodo -file %s ...`.", cwd, abs)
}

// discoverBoard mirrors gotodo's own lookup: the nearest
// .deadline/board.json above dir, else the personal board.
func discoverBoard(dir string) string {
	for {
		candidate := filepath.Join(dir, ".deadline", "board.json")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return absPath(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	p, err := task.DefaultPath()
	if err != nil {
		return ""
	}
	return absPath(p)
}

func absPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// execAgent is the real ExecAgent: the agent runs in its own process group
// with stdin closed; on ctx end the group gets SIGTERM, then SIGKILL.
func execAgent(ctx context.Context, c Command, out io.Writer, started func(int)) (int, error) {
	cmd := exec.Command(c.Name, c.Args...)
	cmd.Dir, cmd.Env = c.Dir, c.Env
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = groupAttr()
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	pid := cmd.Process.Pid
	started(pid)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return exitCode(cmd, err)
	case <-ctx.Done():
		killGroup(pid, syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			killGroup(pid, syscall.SIGKILL)
			<-done
		}
		code, _ := exitCode(cmd, nil)
		return code, ctx.Err()
	}
}

func exitCode(cmd *exec.Cmd, err error) (int, error) {
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return cmd.ProcessState.ExitCode(), nil
	}
	if err == nil {
		err = errors.New("agent was killed")
	}
	return -1, err
}

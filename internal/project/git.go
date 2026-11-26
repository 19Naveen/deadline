package project

import (
	"bytes"
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Runner runs git with args in dir and returns its standard output. It is
// injectable so tests can fake git or point it at a temporary repository.
type Runner func(dir string, args ...string) (string, error)

// Git is the real Runner: it executes the git binary directly (no shell),
// folding stderr into the error so callers can show what git said.
func Git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), errors.New(firstLine(msg))
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimPrefix(line, "fatal: ")
}

// Project is a project root plus the git runner used to inspect it.
type Project struct {
	Root string
	Run  Runner
}

// New is a Project over root using the real git binary.
func New(root string) Project { return Project{Root: root, Run: Git} }

func (p Project) git(args ...string) (string, error) {
	run := p.Run
	if run == nil {
		run = Git
	}
	return run(p.Root, args...)
}

// TopLevel is the working tree root git reports for Root; ok is false
// outside a repository or when git is missing.
func (p Project) TopLevel() (string, bool) {
	out, err := p.git("rev-parse", "--show-toplevel")
	top := strings.TrimSpace(out)
	if err != nil || top == "" {
		return "", false
	}
	return filepath.Clean(top), true
}

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path   string // absolute
	Branch string // short name; empty when detached
	Main   bool   // the repository's main worktree (listed first)
}

// Worktrees lists the repository's worktrees, the main one first; bare
// entries are left out.
func (p Project) Worktrees() ([]Worktree, error) {
	out, err := p.git("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var list []Worktree
	var cur *Worktree
	bare := false
	flush := func() {
		if cur != nil && !bare {
			list = append(list, *cur)
		}
		cur, bare = nil, false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: filepath.Clean(strings.TrimPrefix(line, "worktree ")), Main: len(list) == 0}
		case cur == nil:
		case strings.HasPrefix(line, "branch "):
			cur.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "bare":
			bare = true
		}
	}
	flush()
	return list, nil
}

// Branches lists local branch names in git's order.
func (p Project) Branches() ([]string, error) {
	out, err := p.git("for-each-ref", "refs/heads", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// CheckBranch validates a new branch name with `git check-ref-format
// --branch`, returning git's normalised form.
func (p Project) CheckBranch(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "-") {
		return "", errors.New("not a valid branch name: " + name)
	}
	out, err := p.git("check-ref-format", "--branch", name)
	if err != nil {
		return "", errors.New("not a valid branch name: " + name)
	}
	return strings.TrimSpace(out), nil
}

// Slug turns a branch name into a directory suffix: feat/x → feat-x.
func Slug(branch string) string { return strings.ReplaceAll(branch, "/", "-") }

// NewWorktreePath is where a worktree for branch goes: a sibling of the main
// worktree named <repo>-<slug>. main is the main worktree's path.
func NewWorktreePath(main, branch string) string {
	return filepath.Join(filepath.Dir(main), filepath.Base(main)+"-"+Slug(branch))
}

// AddWorktree runs `git worktree add -b branch path`, creating the branch
// at HEAD. The branch is validated first; path should be absolute.
func (p Project) AddWorktree(branch, path string) error {
	b, err := p.CheckBranch(branch)
	if err != nil {
		return err
	}
	_, err = p.git("worktree", "add", "-b", b, "--", path)
	return err
}

// FileLimit caps how many files Files returns.
const FileLimit = 20000

// Files lists the files under Root as slash-separated paths relative to it.
// In a git repository that is `git ls-files --cached --others
// --exclude-standard` (so ignored files stay out); elsewhere a directory
// walk that skips dot-directories, node_modules and vendor. Never more than
// FileLimit, and never anything outside Root.
func (p Project) Files() []string {
	if _, ok := p.TopLevel(); ok {
		if out, err := p.git("ls-files", "-z", "--cached", "--others", "--exclude-standard"); err == nil {
			seen := map[string]bool{}
			var files []string
			for _, f := range strings.Split(out, "\x00") {
				if f == "" || seen[f] || strings.HasPrefix(f, "../") {
					continue
				}
				seen[f] = true
				files = append(files, f)
				if len(files) == FileLimit {
					break
				}
			}
			sort.Strings(files)
			return files
		}
	}
	return WalkFiles(p.Root, FileLimit)
}

// WalkFiles walks root for at most limit files, skipping dot-directories,
// node_modules and vendor, and returns sorted slash-separated relative paths.
func WalkFiles(root string, limit int) []string {
	var files []string
	stop := errors.New("limit")
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && path != root {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= limit {
			return stop
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// Package-level helpers over the real git binary.

// Worktrees lists root's repository worktrees.
func Worktrees(root string) ([]Worktree, error) { return New(root).Worktrees() }

// Branches lists root's repository's local branches.
func Branches(root string) ([]string, error) { return New(root).Branches() }

// Files lists the files inside root.
func Files(root string) []string { return New(root).Files() }

// AddWorktree creates a worktree at path on a new branch.
func AddWorktree(root, branch, path string) error { return New(root).AddWorktree(branch, path) }

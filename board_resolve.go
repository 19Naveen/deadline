package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gotodo/internal/loops"
	"gotodo/internal/task"
)

// gitTimeout bounds every git query the resolver makes, so a slow or hung
// git never stalls a board command.
const gitTimeout = 2 * time.Second

// boardChoice is a resolved board plus what the caller should be told.
type boardChoice struct {
	Path string
	// RepoRoot is the main checkout root of the git repository the working
	// directory is in when that repository has no project board (so the
	// personal board was chosen), else "".
	RepoRoot string
	// Created reports that this resolution initialised the project board.
	Created bool
}

// note is the stderr hint shown when a git repository has no project board.
func (c boardChoice) note() string {
	if c.RepoRoot == "" {
		return ""
	}
	return fmt.Sprintf("note: you are in git repo %s, which has no project board; "+
		"agents will create one on their first change (or run gotodo init)", c.RepoRoot)
}

// boardScope names the board at path for `gotodo board` output: "personal"
// for the global board, "project" for a .deadline/board.json, else "file"
// (an arbitrary -file or GOTODO_BOARD path).
func boardScope(path string) string {
	if personal, err := task.DefaultPath(); err == nil && filepath.Clean(path) == filepath.Clean(personal) {
		return "personal"
	}
	if filepath.Base(path) == projectBoardFile && filepath.Base(filepath.Dir(path)) == projectDirName {
		return "project"
	}
	return "file"
}

// resolveBoardPath is the read-only resolution: it never creates a board.
func resolveBoardPath() (string, error) {
	c, err := resolveBoard(false, nil)
	return c.Path, err
}

// resolveBoard picks the board for a command, in order:
//
//  1. GOTODO_BOARD, when it names an existing file (stage loops set it);
//  2. the nearest .deadline/board.json walking up from the working directory;
//  3. inside a git repository, the main checkout's board (a linked worktree
//     has no .deadline of its own, since .deadline/ is gitignored);
//  4. inside a git repository with no board, when autoInit is set (an agent
//     making a change), a new project board at the main checkout root;
//  5. the global personal board.
//
// log receives the one-line notice when a board is auto-initialised.
func resolveBoard(autoInit bool, log io.Writer) (boardChoice, error) {
	if env := strings.TrimSpace(os.Getenv(loops.BoardEnv)); env != "" {
		if st, err := os.Stat(env); err == nil && !st.IsDir() {
			return boardChoice{Path: env}, nil
		}
	}
	if board, err := findProjectBoard(); err != nil {
		return boardChoice{}, err
	} else if board != "" {
		return boardChoice{Path: board}, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return boardChoice{}, fmt.Errorf("locate working directory: %w", err)
	}
	repo, inRepo := findGitRepo(cwd)
	if inRepo {
		if board, err := findBoardIn(repo.mainPathFor(cwd), repo.Root); err != nil {
			return boardChoice{}, err
		} else if board != "" {
			return boardChoice{Path: board}, nil
		}
		if autoInit {
			// One notice line only: the .gitignore update stays quiet.
			path, err := initProjectBoard(repo.Root, nil)
			if err != nil {
				return boardChoice{}, err
			}
			if log != nil {
				fmt.Fprintf(log, "initialised project board %s (agents always use a project board inside a git repo)\n", path)
			}
			return boardChoice{Path: path, Created: true}, nil
		}
	}
	personal, err := task.DefaultPath()
	if err != nil {
		return boardChoice{}, err
	}
	c := boardChoice{Path: personal}
	if inRepo {
		c.RepoRoot = repo.Root
	}
	return c, nil
}

// findProjectBoard returns the nearest ancestor's .deadline/board.json, or
// "". A .deadline directory without a board file does not count.
func findProjectBoard() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("locate working directory: %w", err)
	}
	return findBoardIn(dir, "")
}

// findBoardIn walks up from dir looking for .deadline/board.json, stopping
// after stop (or at the filesystem root when stop is "").
func findBoardIn(dir, stop string) (string, error) {
	for {
		candidate := filepath.Join(dir, projectDirName, projectBoardFile)
		if st, err := os.Stat(candidate); err == nil {
			if st.IsDir() {
				return "", fmt.Errorf("%s is a directory, want a board file", candidate)
			}
			return candidate, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if dir == stop || parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// gitRepo locates the working directory inside a git repository.
type gitRepo struct {
	// Root is the main checkout's root: the directory that holds the
	// shared .git, which every linked worktree of the repository shares.
	Root string
	// Toplevel is the root of the checkout the working directory is in;
	// it differs from Root inside a linked worktree.
	Toplevel string
}

// mainPathFor maps dir inside the current checkout to the same relative
// path inside the main checkout, so a board in a monorepo subdirectory is
// found from the matching subdirectory of a linked worktree.
func (r gitRepo) mainPathFor(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	rel, err := filepath.Rel(r.Toplevel, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return r.Root
	}
	return filepath.Join(r.Root, rel)
}

// findGitRepo asks git which repository dir belongs to. It reports false
// when git is missing, slow, or dir is not inside a work tree, and when the
// repository's root is the home directory (a dotfiles repository is not a
// project, and the personal board serves it).
func findGitRepo(dir string) (gitRepo, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse",
		"--path-format=absolute", "--git-common-dir", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return gitRepo{}, false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		return gitRepo{}, false
	}
	common, top := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	if !filepath.IsAbs(top) {
		return gitRepo{}, false
	}
	root := top
	// The main checkout holds the shared .git; a bare repository or a
	// submodule keeps it elsewhere, so fall back to this checkout's root.
	if filepath.Base(common) == ".git" {
		root = filepath.Dir(common)
	}
	root, top = filepath.Clean(root), filepath.Clean(top)
	if home, err := os.UserHomeDir(); err == nil && sameDir(root, home) {
		return gitRepo{}, false
	}
	return gitRepo{Root: root, Toplevel: top}, true
}

// sameDir compares two directories, resolving symlinks where possible.
func sameDir(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// initProjectBoard creates root/.deadline/board.json with the dev columns
// (or keeps an existing one) and gitignores .deadline/. Progress lines go
// to log. It returns the board path.
func initProjectBoard(root string, log io.Writer) (string, error) {
	if log == nil {
		log = io.Discard
	}
	path := filepath.Join(root, projectDirName, projectBoardFile)
	board, err := task.Load(path)
	if err != nil {
		return "", err
	}
	if len(board.Columns) > 0 || len(board.Tasks) > 0 {
		return path, nil
	}
	board.SetColumns(task.DevColumns)
	if err := board.Save(); err != nil {
		return "", err
	}
	ensureGitignored(root, log)
	return path, nil
}

// initRoot is where `gotodo init` creates a board: the main checkout root
// when run inside a linked git worktree (whose .deadline would be invisible
// to the main checkout and lost with the worktree), else the working
// directory.
func initRoot(cwd string) string {
	if repo, ok := findGitRepo(cwd); ok && !sameDir(repo.Root, repo.Toplevel) {
		return repo.Root
	}
	return cwd
}

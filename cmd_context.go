package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gotodo/internal/task"
)

const contextUsage = `usage: gotodo context <id>`

// runContext prints a markdown resume pack for one task (see Board.Context).
// Relative working-note paths resolve against the project root (the
// directory holding .deadline/) first, then the working directory.
// Read-only: nothing is saved.
func runContext(path string, args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println(contextUsage)
		return nil
	}
	if len(args) != 1 {
		return errors.New(contextUsage)
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	now := time.Now()
	b.SweepArchive(now)
	t, err := resolveID(b, args[0])
	if err != nil {
		return err
	}
	var roots []string
	if root := projectRootOf(path); root != "" {
		roots = append(roots, root)
	}
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, cwd)
	}
	out, err := b.Context(t.ID, task.ContextOptions{Now: now, NoteRoots: roots})
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// projectRootOf is the project directory for a <root>/.deadline/board.json
// path, or "" for the personal board, which has no project.
func projectRootOf(boardPath string) string {
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) != projectDirName {
		return ""
	}
	return filepath.Dir(dir)
}

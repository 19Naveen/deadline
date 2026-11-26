package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"gotodo/internal/gh"
)

const prUsage = `usage: gotodo pr <id> <url|number>   (link; "" clears)
       gotodo pr <id>                  (show the linked PR)
       gotodo pr sync [-dry-run]       (update tasks from GitHub via gh)`

// newGHRunner builds the gh runner for a board; tests replace it with a fake
// so they never call the real gh.
var newGHRunner = func(boardPath string) gh.Runner { return gh.CLI{Dir: prProjectDir(boardPath)} }

// prProjectDir is the repository a project board belongs to (the parent of
// .deadline/), so bare PR numbers resolve against it; "" (the working
// directory) for personal boards.
func prProjectDir(boardPath string) string {
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		return ""
	}
	if dir := filepath.Dir(abs); filepath.Base(dir) == projectDirName {
		return filepath.Dir(dir)
	}
	return ""
}

// runPR links a task to a pull request, shows the link, or syncs PR state.
func runPR(path string, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(prUsage)
		return nil
	}
	if args[0] == "sync" {
		return runPRSync(path, args[1:])
	}
	if len(args) > 2 {
		return errors.New(prUsage)
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	b.SweepArchive(time.Now())
	t, err := resolveID(b, args[0])
	if err != nil {
		return err
	}
	if len(args) == 1 {
		if t.PR == "" {
			fmt.Printf("%s %s has no linked PR\n", shortID(t.ID), t.Title)
		} else {
			fmt.Printf("%s %s · PR %s\n", shortID(t.ID), t.Title, t.PR)
		}
		return nil
	}
	if err := b.SetPR(t.ID, args[1], time.Now()); err != nil {
		return err
	}
	if err := b.Save(); err != nil {
		return err
	}
	linked, _ := b.TaskByID(t.ID)
	if linked.PR == "" {
		fmt.Printf("cleared PR on %s %s\n", shortID(t.ID), t.Title)
	} else {
		fmt.Printf("linked %s %s → PR %s\n", shortID(t.ID), t.Title, linked.PR)
	}
	return nil
}

// runPRSync asks gh about every open task's PR: merged ships the task,
// failing CI blocks it. -dry-run reports without saving.
func runPRSync(path string, args []string) error {
	fs := flag.NewFlagSet("pr sync", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "report what would change without saving")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (usage: gotodo pr sync [-dry-run])", fs.Arg(0))
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	now := time.Now()
	b.SweepArchive(now)
	linked := 0
	for _, t := range b.Active() {
		if t.PR != "" && t.Status != b.DoneStatus() {
			linked++
		}
	}
	if linked == 0 {
		fmt.Println("no open tasks with a linked PR")
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	runner := newGHRunner(path)
	if err := gh.CheckAuth(ctx, runner); err != nil {
		return err
	}
	actions := gh.Sync(ctx, b, runner, now)
	failed := 0
	for _, a := range actions {
		fmt.Println(a.String(*dryRun))
		if a.Kind == gh.ActionError {
			failed++
		}
	}
	if len(actions) == 0 {
		fmt.Printf("checked %d PR(s): nothing to change\n", linked)
	}
	if !*dryRun && b.Dirty() {
		if err := b.Save(); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d PR(s) could not be checked", failed, linked)
	}
	return nil
}

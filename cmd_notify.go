package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gotodo/internal/notify"
)

// minNotifyInterval stops -watch from hammering the disk.
const minNotifyInterval = 10 * time.Second

// newNotifySender builds the desktop sender; tests replace it with a fake.
var newNotifySender = func() notifySender { return notify.NewDesktop(os.Stdout) }

type notifySender interface {
	notify.Sender
	Method() string
}

// notifyBoardName labels notifications: the project directory for a project
// board, else "personal".
func notifyBoardName(boardPath string) string {
	abs, err := filepath.Abs(boardPath)
	if err != nil {
		return ""
	}
	if dir := filepath.Dir(abs); filepath.Base(dir) == projectDirName {
		return filepath.Base(filepath.Dir(dir))
	}
	return "personal"
}

// runNotify sends desktop notifications for new board events: one pass, or
// a loop with -watch. Dedupe state lives in the user cache dir.
func runNotify(path string, args []string) error {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	watch := fs.Bool("watch", false, "keep checking until interrupted")
	interval := fs.Duration("interval", time.Minute, "how often -watch checks")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (usage: gotodo notify [-watch] [-interval 1m])", fs.Arg(0))
	}
	if *interval < minNotifyInterval {
		return fmt.Errorf("-interval must be at least %s", minNotifyInterval)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("locate cache dir: %w", err)
	}
	sender := newNotifySender()
	n := &notify.Notifier{
		BoardPath: path,
		BoardName: notifyBoardName(path),
		StateFile: notify.StatePath(cache, path),
		Sender:    sender,
	}
	if !*watch {
		sent, err := n.Pass()
		switch {
		case len(sent) > 0:
			fmt.Printf("sent %d notification(s) via %s\n", len(sent), sender.Method())
		case err == nil:
			fmt.Println("no new events")
		}
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	fmt.Printf("watching every %s, notifying via %s (Ctrl+C to stop)\n", *interval, sender.Method())
	n.Watch(ctx, ticker.C, func(_ []notify.Event, err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "gotodo: notify:", err)
		}
	})
	return nil
}

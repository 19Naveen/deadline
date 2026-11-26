package main

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"gotodo/internal/report"
)

const standupUsage = "usage: gotodo standup [-since 24h|7d] [-format text|markdown]"

// runStandup prints a paste-ready summary of recent board activity.
// Read-only: it never saves.
func runStandup(path string, args []string) error {
	fs := flag.NewFlagSet("standup", flag.ContinueOnError)
	sinceStr := fs.String("since", "24h", "look-back window, e.g. 24h, 90m or 7d")
	format := fs.String("format", "text", "output format: text or markdown")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (%s)", fs.Arg(0), standupUsage)
	}
	window, err := report.ParseSince(*sinceStr)
	if err != nil {
		return err
	}
	if *format != "text" && *format != "markdown" && *format != "md" {
		return fmt.Errorf("bad -format %q (want text or markdown)", *format)
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	now := time.Now()
	s := report.BuildStandup(b, now, now.Add(-window))
	if *format == "text" {
		fmt.Print(s.Text())
	} else {
		fmt.Print(s.Markdown())
	}
	return nil
}

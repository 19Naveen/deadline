package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpOverviewAndAliases(t *testing.T) {
	for _, argv := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		out := captureStdout(t, func() {
			if err := run(argv); err != nil {
				t.Fatalf("%v = %v", argv, err)
			}
		})
		for _, want := range []string{"usage:", "tasks:", "agents:", "next", "gotodo help <command>"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v output missing %q:\n%s", argv, want, out)
			}
		}
	}
}

func TestHelpForOneCommand(t *testing.T) {
	out := captureStdout(t, func() {
		if err := run([]string{"help", "add"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"usage: gotodo add", "-working-note", "examples:"} {
		if !strings.Contains(out, want) {
			t.Errorf("help add missing %q:\n%s", want, out)
		}
	}
	if err := run([]string{"help", "nope"}); err == nil {
		t.Error("help nope = nil, want an error")
	}
}

// Every command the help documents is one the dispatcher knows, and an
// unknown command points at help.
func TestHelpCommandsAreAllDispatched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	for _, g := range helpGroups {
		for _, e := range g.commands {
			if e.name == "help" || e.name == "init" || e.name == "mcp" || e.name == "notify" {
				continue // run elsewhere, or long-running
			}
			captureStdout(t, func() {
				err := run([]string{"-file", path, e.name, "-h"})
				if err != nil && strings.Contains(err.Error(), "unknown command") {
					t.Errorf("help documents %q but run does not know it", e.name)
				}
			})
		}
	}
	err := run([]string{"-file", path, "nope"})
	if err == nil || !strings.Contains(err.Error(), "gotodo help") {
		t.Errorf("unknown command error = %v, want a pointer to gotodo help", err)
	}
}

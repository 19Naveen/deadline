package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gotodo/internal/agentsetup"
	"gotodo/internal/wizard"
)

const setupUsage = "usage: gotodo setup [-yes] [-loops | -no-loops] [-remove] [-dry-run]"

// runSetup is the setup wizard in a terminal, and with -yes (or without a
// terminal) the scripted path: register gotodo's MCP server with every
// installed coding agent. -remove and -dry-run skip the wizard.
func runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	remove := fs.Bool("remove", false, "unregister the MCP server instead")
	dryRun := fs.Bool("dry-run", false, "report what would change without writing")
	yes := fs.Bool("yes", false, "no questions: register the MCP server with every agent")
	loopsOn := fs.Bool("loops", false, "set up stage loops (with -yes: write defaults if no config exists)")
	noLoops := fs.Bool("no-loops", false, "skip stage loops")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (%s)", fs.Arg(0), setupUsage)
	}
	if *loopsOn && *noLoops {
		return fmt.Errorf("-loops and -no-loops conflict (%s)", setupUsage)
	}
	if *remove || *dryRun {
		return setupMCPOnly(*remove, *dryRun)
	}
	d, err := wizard.DefaultDeps()
	if err != nil {
		return err
	}
	var o wizard.Options
	switch {
	case *loopsOn:
		o.Loops = boolPtr(true)
	case *noLoops:
		o.Loops = boolPtr(false)
	}
	if *yes || !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return wizard.NonInteractive(d, o, os.Stdout)
	}
	m, err := wizard.Run(d, o)
	if err != nil {
		return err
	}
	for _, line := range m.Report() {
		fmt.Println(line)
	}
	return nil
}

func boolPtr(b bool) *bool { return &b }

// isTerminal reports whether f is a character device (a terminal).
func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// setupMCPOnly is -remove and -dry-run: the MCP registration alone.
func setupMCPOnly(remove, dryRun bool) error {
	env, err := agentsetup.DefaultEnv()
	if err != nil {
		return err
	}
	env.DryRun = dryRun
	results := agentsetup.Install
	if remove {
		results = agentsetup.Remove
	}
	failed := 0
	for _, r := range results(env) {
		fmt.Println(r)
		if r.Action == "failed" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d agent(s) could not be set up", failed)
	}
	if !remove && !dryRun {
		fmt.Println("Restart open agent sessions to load the gotodo MCP server.")
	}
	return nil
}

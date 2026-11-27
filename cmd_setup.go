package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gotodo/internal/agentsetup"
	"gotodo/internal/wizard"
)

const setupUsage = "usage: gotodo setup [-yes] [-loops | -no-loops] [-no-agents] [-remove] [-dry-run]"

// noAgentsNote is what setup says when the user keeps gotodo to themselves.
const noAgentsNote = "Agents: not connected (run gotodo setup any time to connect them)."

// runSetup is the setup wizard in a terminal, and with -yes (or without a
// terminal) the scripted path: connect every installed coding agent (MCP
// server, gotodo skill, instruction block). -no-agents connects nothing;
// -remove and -dry-run skip the wizard.
func runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	remove := fs.Bool("remove", false, "disconnect every agent: MCP server, skill and instruction block")
	dryRun := fs.Bool("dry-run", false, "report what would change without writing")
	yes := fs.Bool("yes", false, "no questions: connect every installed agent")
	noAgents := fs.Bool("no-agents", false, "personal use only: no agent setup at all")
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
	if *noAgents && (*remove || *loopsOn) {
		return fmt.Errorf("-no-agents conflicts with -remove and -loops (%s)", setupUsage)
	}
	if *noAgents {
		fmt.Println(noAgentsNote)
		return nil
	}
	if *remove || *dryRun {
		return setupScripted(*remove, *dryRun)
	}
	d, err := wizard.DefaultDeps()
	if err != nil {
		return err
	}
	d.MCP.Assets = agentAssets()
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

// setupScripted is -remove (disconnect every agent) and -dry-run (preview
// connecting them), with no wizard.
func setupScripted(remove, dryRun bool) error {
	env, err := agentsetup.DefaultEnv()
	if err != nil {
		return err
	}
	env.DryRun = dryRun
	env.Assets = agentAssets()
	var results []agentsetup.Result
	if remove {
		results = append(agentsetup.Remove(env), agentsetup.Unwire(env)...)
	} else {
		results = append(agentsetup.Install(env), agentsetup.Wire(env)...)
	}
	failed := 0
	for _, r := range results {
		fmt.Println(r)
		if r.Action == "failed" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d agent step(s) failed", failed)
	}
	return nil
}

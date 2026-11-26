package main

import (
	"errors"
	"flag"
	"fmt"

	"gotodo/internal/agentsetup"
)

const setupUsage = "usage: gotodo setup [-remove] [-dry-run]"

// runSetup registers (or with -remove, unregisters) gotodo's MCP server with
// every installed coding agent. install.sh runs it, so users never edit
// agent configs by hand.
func runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	remove := fs.Bool("remove", false, "unregister the MCP server instead")
	dryRun := fs.Bool("dry-run", false, "report what would change without writing")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (%s)", fs.Arg(0), setupUsage)
	}
	env, err := agentsetup.DefaultEnv()
	if err != nil {
		return err
	}
	env.DryRun = *dryRun
	results := agentsetup.Install(env)
	if *remove {
		results = agentsetup.Remove(env)
	}
	failed := 0
	for _, r := range results {
		fmt.Println(r)
		if r.Action == "failed" {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d agent(s) could not be set up", failed)
	}
	if !*remove && !*dryRun {
		fmt.Println("Restart open agent sessions to load the gotodo MCP server.")
	}
	return nil
}

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gotodo/internal/mcp"
	"gotodo/internal/task"
)

// runMCP serves the resolved board to an MCP client over stdio until stdin
// closes. Stdout carries only protocol messages; diagnostics go to stderr.
// Moves are attributed to GOTODO_AGENT, else the client's name.
func runMCP(path string, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (usage: gotodo mcp)", fs.Arg(0))
	}
	srv := mcp.NewServer(mcp.Config{
		Path:    path,
		Version: version,
		Agent:   task.AgentFromEnv(),
		Log:     os.Stderr,
	})
	return srv.Serve(os.Stdin, os.Stdout)
}

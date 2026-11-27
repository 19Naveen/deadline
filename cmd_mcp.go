package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"gotodo/internal/mcp"
	"gotodo/internal/task"
)

// runMCP serves the board to an MCP client over stdio until stdin closes.
// Stdout carries only protocol messages; diagnostics go to stderr. Moves are
// attributed to GOTODO_AGENT, else the client's name. A non-blank path
// (-file) pins the board; otherwise every tool call resolves it like the
// CLI does for an agent, so the first write in a git repository without a
// board creates its project board.
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
		Resolve: mcpResolver(path, os.Stderr),
		Version: version,
		Agent:   task.AgentFromEnv(),
		Log:     os.Stderr,
	})
	return srv.Serve(os.Stdin, os.Stdout)
}

// mcpResolver resolves the board per tool call. An MCP client is always an
// agent, so write calls may auto-initialise the repository's project board.
// Progress lines go to log, never stdout (the protocol stream).
func mcpResolver(fixed string, log io.Writer) func(write bool) (mcp.BoardRef, error) {
	return func(write bool) (mcp.BoardRef, error) {
		if fixed != "" {
			return mcp.BoardRef{Path: fixed, Scope: boardScope(fixed)}, nil
		}
		c, err := resolveBoard(write, log)
		if err != nil {
			return mcp.BoardRef{}, err
		}
		return mcp.BoardRef{Path: c.Path, Scope: boardScope(c.Path), Note: c.note(), Created: c.Created}, nil
	}
}

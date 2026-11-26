package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"gotodo/internal/task"
)

const claimUsage = `usage: gotodo claim <id> [-ttl 2h]`

const releaseUsage = `usage: gotodo release <id>`

// runClaim claims one task for the GOTODO_AGENT agent. The check and the
// write happen under the board lock so two agents cannot both win.
func runClaim(path string, args []string) error {
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	ttl := fs.Duration("ttl", task.DefaultClaimTTL, "how long the claim lasts (e.g. 30m, 2h)")
	id, help, err := coordIDArgs(fs, args, claimUsage)
	if help || err != nil {
		return err
	}
	agent := task.AgentFromEnv()
	if agent == "" {
		return fmt.Errorf("claim needs an agent name: export %s=<name> (e.g. claude)", task.AgentEnv)
	}
	now := time.Now()
	var claimed task.Task
	err = task.UpdateLocked(path, agent, func(b *task.Board) error {
		b.ReleaseFinishedClaims(now)
		t, err := resolveID(b, id)
		if err != nil {
			return err
		}
		if err := b.Claim(t.ID, agent, *ttl, now); err != nil {
			return fmt.Errorf("claim %s: %w", shortID(t.ID), err)
		}
		claimed, _ = b.TaskByID(t.ID)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("claimed %s [%s] %s for %s until %s\n", shortID(claimed.ID), claimed.Status,
		claimed.Title, agent, claimed.ClaimUntil.In(time.Local).Format(dateLayout+" 15:04"))
	return nil
}

// runRelease drops a claim. Agents release their own (or expired) claims;
// a human (no GOTODO_AGENT) may release anyone's.
func runRelease(path string, args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	id, help, err := coordIDArgs(fs, args, releaseUsage)
	if help || err != nil {
		return err
	}
	agent := task.AgentFromEnv()
	now := time.Now()
	var t task.Task
	var released bool
	err = task.UpdateLocked(path, agent, func(b *task.Board) error {
		b.ReleaseFinishedClaims(now)
		var err error
		if t, err = resolveID(b, id); err != nil {
			return err
		}
		if released, err = b.Release(t.ID, agent, now); err != nil {
			return fmt.Errorf("release %s: %w", shortID(t.ID), err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !released {
		fmt.Printf("%s %s is not claimed\n", shortID(t.ID), t.Title)
		return nil
	}
	fmt.Printf("released %s [%s] %s (was %s)\n", shortID(t.ID), t.Status, t.Title, t.ClaimedBy)
	return nil
}

// coordIDArgs parses "<id> [flags]" or "[flags] <id>" for the coordination
// commands. help is true when usage was printed and the caller should stop.
func coordIDArgs(fs *flag.FlagSet, args []string, usage string) (id string, help bool, err error) {
	if len(args) == 0 {
		return "", false, errors.New(usage)
	}
	if args[0] == "-h" || args[0] == "--help" {
		fmt.Println(usage)
		return "", true, nil
	}
	rest := args
	if !strings.HasPrefix(args[0], "-") {
		id, rest = args[0], args[1:]
	}
	if err := fs.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", true, nil
		}
		return "", false, err
	}
	if id == "" {
		if fs.NArg() == 0 {
			return "", false, errors.New(usage)
		}
		id = fs.Arg(0)
		if fs.NArg() > 1 {
			return "", false, fmt.Errorf("unexpected argument %q (%s)", fs.Arg(1), usage)
		}
	} else if fs.NArg() > 0 {
		return "", false, fmt.Errorf("unexpected argument %q (%s)", fs.Arg(0), usage)
	}
	return id, false, nil
}

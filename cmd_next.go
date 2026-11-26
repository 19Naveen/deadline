package main

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"gotodo/internal/task"
)

const nextUsage = `usage: gotodo next [-claim] [-ttl 2h] [-start]`

// runNext prints the task an agent should pick up next (see Board.NextTask)
// in show's format. -claim claims it for GOTODO_AGENT; -start also moves it
// to the active column (doing/indev), claiming it when run by an agent.
// Pick, claim and move happen in one locked transaction, so two agents
// running next at once get different cards.
func runNext(path string, args []string) error {
	fs := flag.NewFlagSet("next", flag.ContinueOnError)
	claim := fs.Bool("claim", false, "claim the task for $"+task.AgentEnv)
	ttl := fs.Duration("ttl", task.DefaultClaimTTL, "claim duration (e.g. 30m, 2h)")
	start := fs.Bool("start", false, "also move the task to the active column")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (%s)", fs.Arg(0), nextUsage)
	}
	agent := task.AgentFromEnv()
	if *claim && agent == "" {
		return fmt.Errorf("-claim needs an agent name: export %s=<name> (e.g. claude)", task.AgentEnv)
	}
	// An agent that starts a card always claims it, so others skip it.
	doClaim := *claim || (*start && agent != "")
	now := time.Now()

	var picked task.Task
	var found bool
	var startedIn task.Status
	if !doClaim && !*start {
		// Read-only peek: no lock, no write.
		b, err := loadBoard(path)
		if err != nil {
			return err
		}
		b.SweepArchive(now)
		picked, found = b.NextTask(agent, now)
	} else {
		err := task.UpdateLocked(path, agent, func(b *task.Board) error {
			b.SweepArchive(now)
			b.ReleaseFinishedClaims(now)
			picked, found = b.NextTask(agent, now)
			if !found {
				return nil
			}
			if doClaim {
				if err := b.Claim(picked.ID, agent, *ttl, now); err != nil {
					return err
				}
			}
			if *start {
				cols := b.Statuses()
				if len(cols) < 2 {
					return errors.New("board has no active column to start in")
				}
				startedIn = cols[1]
				if err := b.Move(picked.ID, startedIn, now); err != nil {
					return fmt.Errorf("start %s: %w", shortID(picked.ID), err)
				}
			}
			picked, _ = b.TaskByID(picked.ID)
			return nil
		})
		if err != nil {
			return err
		}
	}
	if !found {
		fmt.Println("no available tasks")
		return nil
	}
	if doClaim {
		fmt.Printf("claimed by %s until %s\n", agent,
			picked.ClaimUntil.In(time.Local).Format(dateLayout+" 15:04"))
	}
	if startedIn != "" {
		fmt.Printf("started: moved to %s\n", startedIn)
	}
	return runShow(path, []string{picked.ID})
}

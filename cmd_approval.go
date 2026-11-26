package main

import (
	"errors"
	"fmt"

	"gotodo/internal/task"
)

const approvalUsage = `usage: gotodo approval on|off|status`

// runApproval shows or sets the human ship gate. Only a human may change
// it: with GOTODO_AGENT set, on/off are refused so an agent can never lift
// the gate that exists to review agents.
func runApproval(path string, args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println(approvalUsage)
		return nil
	}
	if len(args) != 1 {
		return errors.New(approvalUsage)
	}
	mode := args[0]
	if mode != "on" && mode != "off" && mode != "status" {
		return fmt.Errorf("unknown approval mode %q (%s)", mode, approvalUsage)
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	if mode != "status" {
		if b.Actor != "" {
			return fmt.Errorf("agents cannot change the approval gate (%s=%s); ask a human to run it", task.AgentEnv, b.Actor)
		}
		b.SetRequireApproval(mode == "on")
		if err := b.Save(); err != nil {
			return err
		}
	}
	if b.RequireApproval {
		fmt.Printf("approval: on (only a human can move cards to %s)\n", b.DoneStatus())
	} else {
		fmt.Printf("approval: off (agents may move cards to %s)\n", b.DoneStatus())
	}
	return nil
}

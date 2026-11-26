package mcp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gotodo/internal/task"
)

// These mirror the headless CLI helpers in package main, so MCP output and
// errors read the same as `gotodo list` / `gotodo show`.

// dateLayout is the only accepted deadline shape (DD/MM/YYYY).
const dateLayout = "02/01/2006"

// stampLayout formats history and claim times (DD/MM/YYYY HH:MM).
const stampLayout = "02/01/2006 15:04"

// shortLen is how much of a task ID output prints; tools accept any unique
// prefix.
const shortLen = 8

func shortID(id string) string {
	if len(id) > shortLen {
		return id[:shortLen]
	}
	return id
}

// parseDeadline parses DD/MM/YYYY at local midnight. Blank means no deadline.
func parseDeadline(value string, loc *time.Location) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	d, err := time.ParseInLocation(dateLayout, value, loc)
	if err != nil {
		return nil, fmt.Errorf("bad deadline %q (want DD/MM/YYYY)", value)
	}
	return &d, nil
}

// resolveID finds the one active task whose ID starts with arg; a match on
// an archived task is reported as an error.
func resolveID(b *task.Board, arg string) (task.Task, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return task.Task{}, errors.New("task id must not be blank")
	}
	var active, archived []task.Task
	for _, t := range b.Tasks {
		if strings.HasPrefix(t.ID, arg) {
			if t.Archived {
				archived = append(archived, t)
			} else {
				active = append(active, t)
			}
		}
	}
	switch len(active) {
	case 0:
		if len(archived) > 0 {
			return task.Task{}, fmt.Errorf("task %s is archived", shortID(archived[0].ID))
		}
		return task.Task{}, fmt.Errorf("no task id starts with %q", arg)
	case 1:
		return active[0], nil
	default:
		ids := make([]string, 0, len(active))
		for _, t := range active {
			ids = append(ids, t.ID)
		}
		return task.Task{}, fmt.Errorf("ambiguous id %q (%d tasks match: %s)", arg, len(active), strings.Join(ids, ", "))
	}
}

// boardStatus parses a status name against this board's own columns.
func boardStatus(b *task.Board, name string) (task.Status, error) {
	s := task.Status(strings.TrimSpace(name))
	if !b.HasStatus(s) {
		return "", fmt.Errorf("unknown status %q for this board (want %s)", name, statusNames(b.Statuses()))
	}
	return s, nil
}

func statusNames(cols []task.Status) string {
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, string(c))
	}
	return strings.Join(names, ", ")
}

// priorityName is the display name of a priority, "" when unset.
func priorityName(p task.Priority) string {
	switch p {
	case task.PriorityNone:
		return ""
	case task.PriorityLow:
		return "low"
	case task.PriorityMedium:
		return "medium"
	case task.PriorityHigh:
		return "high"
	}
	return fmt.Sprintf("%d", p)
}

// Package gh talks to GitHub only through the user's installed `gh` CLI. It
// never reads or stores tokens and never calls the GitHub API directly.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrNotInstalled means the gh binary is not on PATH.
var ErrNotInstalled = errors.New("the GitHub CLI (gh) is not installed; install it from https://cli.github.com and run `gh auth login`")

// ErrNotAuthenticated means gh is installed but not logged in.
var ErrNotAuthenticated = errors.New("the GitHub CLI is not authenticated; run `gh auth login` first")

// Runner runs one gh invocation and returns its stdout. Tests inject a fake;
// production uses CLI.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// CLI runs the real gh binary, optionally from Dir (so bare PR numbers
// resolve against the project's repository).
type CLI struct {
	Dir string
}

// Run executes `gh args...`. Errors include gh's stderr.
func (c CLI) Run(ctx context.Context, args ...string) ([]byte, error) {
	bin, err := exec.LookPath("gh")
	if err != nil {
		return nil, ErrNotInstalled
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = c.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

// CheckAuth confirms gh is installed and logged in.
func CheckAuth(ctx context.Context, r Runner) error {
	if _, err := r.Run(ctx, "auth", "status"); err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return err
		}
		return ErrNotAuthenticated
	}
	return nil
}

// Check is one entry of a PR's statusCheckRollup: a CheckRun (Conclusion)
// or a commit StatusContext (State).
type Check struct {
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// PRStatus is the subset of `gh pr view --json` that sync needs.
type PRStatus struct {
	State    string  `json:"state"`    // OPEN, CLOSED or MERGED
	MergedAt string  `json:"mergedAt"` // RFC 3339, or null/blank when unmerged
	URL      string  `json:"url"`
	Checks   []Check `json:"statusCheckRollup"`
}

// Merged reports whether the PR has been merged.
func (s PRStatus) Merged() bool {
	return strings.EqualFold(s.State, "MERGED") || (s.MergedAt != "" && !strings.HasPrefix(s.MergedAt, "0001-"))
}

// failing are the check outcomes that count as broken CI.
var failing = map[string]bool{
	"FAILURE": true, "ERROR": true, "CANCELLED": true,
	"TIMED_OUT": true, "STARTUP_FAILURE": true,
}

// Failing reports whether any check failed, errored or was cancelled.
func (s PRStatus) Failing() bool {
	for _, c := range s.Checks {
		if failing[strings.ToUpper(c.Conclusion)] || failing[strings.ToUpper(c.State)] {
			return true
		}
	}
	return false
}

// View fetches one PR's state, merge time, checks and URL.
func View(ctx context.Context, r Runner, pr string) (PRStatus, error) {
	out, err := r.Run(ctx, "pr", "view", pr, "--json", "state,mergedAt,statusCheckRollup,url")
	if err != nil {
		return PRStatus{}, err
	}
	var s PRStatus
	if err := json.Unmarshal(out, &s); err != nil {
		return PRStatus{}, fmt.Errorf("parse gh output for PR %s: %w", pr, err)
	}
	return s, nil
}

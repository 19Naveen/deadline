// Package notify turns board changes into desktop notifications. Events are
// deduplicated through a small state file in the user cache directory (never
// in the board), so each event fires once.
package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gotodo/internal/task"
)

const dateLayout = "02/01/2006"

// freshWindow is how far back a blocked/review transition still counts as
// new on the very first run, so starting notify doesn't replay old history.
const freshWindow = 24 * time.Hour

// Kind is the type of event.
type Kind string

const (
	KindBlocked  Kind = "blocked"
	KindReview   Kind = "review"
	KindDueToday Kind = "due"
	KindOverdue  Kind = "overdue"
)

// Event is one notification-worthy change.
type Event struct {
	Key    string // dedupe key: kind:taskID:stamp
	Kind   Kind
	TaskID string
	Title  string // notification title
	Body   string // notification body
}

// ReviewStatus is the column that means "ready for review": testing-review
// on a development board, doing on a personal one.
func ReviewStatus(b *task.Board) task.Status {
	if b.HasStatus(task.StatusTestingReview) {
		return task.StatusTestingReview
	}
	return task.StatusDoing
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// lastEntered is when t last moved into its current column; UpdatedAt for
// tasks without a matching transition.
func lastEntered(t task.Task) time.Time {
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].To == t.Status {
			return t.History[i].At
		}
	}
	return t.UpdatedAt
}

func by(agent string) string {
	if agent == "" {
		return ""
	}
	return " by " + agent
}

// Events lists every current event on the board. Blocked and review events
// only count transitions at or after `fresh` (the first-run horizon).
func Events(b *task.Board, now, fresh time.Time, boardName string) []Event {
	done := b.DoneStatus()
	review := ReviewStatus(b)
	prefix := "gotodo"
	if boardName != "" {
		prefix += " · " + boardName
	}
	var out []Event
	for _, t := range b.Active() {
		label := fmt.Sprintf("%s (%s)", t.Title, shortID(t.ID))
		agent := ""
		if n := len(t.History); n > 0 && t.History[n-1].To == t.Status {
			agent = t.History[n-1].Agent
		}
		if t.Status == task.StatusBlocked || t.Status == review {
			entered := lastEntered(t)
			if !entered.Before(fresh) {
				kind, verb := KindBlocked, "blocked"
				if t.Status == review {
					kind, verb = KindReview, "ready for review"
				}
				out = append(out, Event{
					Key:  fmt.Sprintf("%s:%s:%d", kind, t.ID, entered.Unix()),
					Kind: kind, TaskID: t.ID,
					Title: prefix + ": " + verb,
					Body:  label + by(agent),
				})
			}
		}
		if t.Status == done {
			continue
		}
		days, ok := task.DaysUntilDeadline(t, now)
		if !ok || days > 0 {
			continue
		}
		dueDay := t.Deadline.In(now.Location()).Format("2006-01-02")
		if days == 0 {
			out = append(out, Event{
				Key:  fmt.Sprintf("%s:%s:%s", KindDueToday, t.ID, dueDay),
				Kind: KindDueToday, TaskID: t.ID,
				Title: prefix + ": due today",
				Body:  label,
			})
		} else {
			out = append(out, Event{
				Key:  fmt.Sprintf("%s:%s:%s", KindOverdue, t.ID, dueDay),
				Kind: KindOverdue, TaskID: t.ID,
				Title: prefix + ": overdue",
				Body:  fmt.Sprintf("%s was due %s", label, t.Deadline.In(now.Location()).Format(dateLayout)),
			})
		}
	}
	return out
}

// State is the dedupe record kept outside the board.
type State struct {
	Board     string               `json:"board"`
	CreatedAt time.Time            `json:"created_at"`
	Sent      map[string]time.Time `json:"sent"`
}

// StatePath is <cacheDir>/gotodo/notify-<hash of the board path>.json.
func StatePath(cacheDir, boardPath string) string {
	if abs, err := filepath.Abs(boardPath); err == nil {
		boardPath = abs
	}
	sum := sha256.Sum256([]byte(boardPath))
	return filepath.Join(cacheDir, "gotodo", "notify-"+hex.EncodeToString(sum[:8])+".json")
}

// LoadState reads the state file; a missing file is a fresh state.
func LoadState(path, board string, now time.Time) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &State{Board: board, CreatedAt: now, Sent: map[string]time.Time{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		// A corrupt cache is not worth failing over: start again.
		return &State{Board: board, CreatedAt: now, Sent: map[string]time.Time{}}, nil
	}
	if s.Sent == nil {
		s.Sent = map[string]time.Time{}
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	return &s, nil
}

// Save writes the state atomically.
func (s *State) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "notify-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// prune drops records for tasks no longer on the board.
func (s *State) prune(b *task.Board) {
	live := map[string]bool{}
	for _, t := range b.Active() {
		live[t.ID] = true
	}
	for key := range s.Sent {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 || !live[parts[1]] {
			delete(s.Sent, key)
		}
	}
}

// Sender delivers one notification.
type Sender interface {
	Send(title, body string) error
}

// Notifier runs dedupe passes over one board.
type Notifier struct {
	BoardPath string
	BoardName string // shown in notification titles; may be blank
	StateFile string
	Sender    Sender
	Now       func() time.Time
}

// Pass loads the board, sends any events not sent before, records them, and
// returns what it sent. The board is never written.
func (n *Notifier) Pass() ([]Event, error) {
	now := time.Now()
	if n.Now != nil {
		now = n.Now()
	}
	b, err := task.Load(n.BoardPath)
	if err != nil {
		return nil, err
	}
	b.SweepArchive(now) // in memory only
	st, err := LoadState(n.StateFile, n.BoardPath, now)
	if err != nil {
		return nil, err
	}
	var sent []Event
	var sendErrs []error
	for _, ev := range Events(b, now, st.CreatedAt.Add(-freshWindow), n.BoardName) {
		if _, done := st.Sent[ev.Key]; done {
			continue
		}
		if err := n.Sender.Send(ev.Title, ev.Body); err != nil {
			sendErrs = append(sendErrs, err)
			continue // retry next pass
		}
		st.Sent[ev.Key] = now
		sent = append(sent, ev)
	}
	st.prune(b)
	if err := st.Save(n.StateFile); err != nil {
		return sent, fmt.Errorf("save notify state: %w", err)
	}
	return sent, errors.Join(sendErrs...)
}

// Watch runs Pass now and on every tick until ctx is cancelled, calling
// report after each pass. Pass errors are reported, not fatal.
func (n *Notifier) Watch(ctx context.Context, tick <-chan time.Time, report func([]Event, error)) {
	for {
		sent, err := n.Pass()
		report(sent, err)
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
	}
}

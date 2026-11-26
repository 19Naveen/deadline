package task

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// NormalizePR validates a pull request reference for SetPR: an http(s) URL,
// or a PR number with or without a leading "#". Blank is allowed (it clears).
func NormalizePR(pr string) (string, error) {
	pr = strings.TrimSpace(pr)
	if pr == "" {
		return "", nil
	}
	if n := strings.TrimPrefix(pr, "#"); n != "" && isDigits(n) {
		return n, nil
	}
	u, err := url.Parse(pr)
	if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
		return pr, nil
	}
	return "", fmt.Errorf("bad PR %q (want a PR URL or number)", pr)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// SetPR links a task to a pull request URL or number; blank clears it.
// Unchanged values are a no-op.
func (b *Board) SetPR(id, pr string, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	pr, err = NormalizePR(pr)
	if err != nil {
		return err
	}
	if t.Archived {
		return errors.New("task is archived")
	}
	if t.PR == pr {
		return nil
	}
	t.PR = pr
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

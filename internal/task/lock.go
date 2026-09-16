package task

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// lockAttempts and lockDelay bound how long a writer waits for a competing
// session: about two seconds total. Brief headless writes never hold it
// longer than milliseconds; a stuck holder fails the waiter with a retry
// message instead of hanging an agent forever.
const (
	lockAttempts = 40
	lockDelay    = 50 * time.Millisecond
)

// BoardLock is an advisory exclusive lock on one board file, so two agents
// (or an agent plus an open TUI) cannot load-modify-save over each other
// and silently drop tasks. Reads take no lock: Save's atomic rename means
// readers never see a half-written board. Best-effort on filesystems
// without flock (e.g. some NFS mounts), where writers fall back to the
// old last-writer-wins behaviour.
type BoardLock struct {
	f *os.File
}

// LockBoard takes the lock for boardPath, via a sidecar lock file that
// never disturbs the board itself. Hold it across the whole
// load-modify-save and Close it afterwards; the TUI holds it for the
// session instead.
func LockBoard(boardPath string) (*BoardLock, error) {
	// The board's directory may not exist yet (init creates it via Save).
	if err := os.MkdirAll(filepath.Dir(boardPath), 0o755); err != nil {
		return nil, fmt.Errorf("lock board: %w", err)
	}
	f, err := os.OpenFile(boardPath+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock board: %w", err)
	}
	for i := 0; ; i++ {
		if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			return &BoardLock{f: f}, nil
		}
		if i >= lockAttempts {
			f.Close()
			return nil, fmt.Errorf("board is busy (another session holds it); wait a moment and retry")
		}
		time.Sleep(lockDelay)
	}
}

// Close releases the lock.
func (l *BoardLock) Close() error {
	defer l.f.Close()
	return unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
}

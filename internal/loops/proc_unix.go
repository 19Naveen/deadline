//go:build unix

package loops

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// detachedAttr starts the runner in a new session, so it outlives the
// command (or TUI) that dispatched it and ignores that terminal's hangup.
func detachedAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }

// groupAttr puts the agent in its own process group, so a timeout or stop
// kills the agent and everything it spawned without killing the runner.
func groupAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

// killGroup signals a whole process group (pgid == leader pid).
func killGroup(pgid int, sig syscall.Signal) error {
	if pgid <= 0 {
		return nil
	}
	err := syscall.Kill(-pgid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// processAlive reports whether pid exists (EPERM means it does, owned by
// someone else).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// lockFile takes an exclusive flock on path, waiting up to wait.
func lockFile(path string, wait time.Duration) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("loops lock: %w", err)
	}
	deadline := time.Now().Add(wait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			f.Close()
			return nil, fmt.Errorf("loops lock: %w", err)
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, errors.New("loops are busy (another dispatcher holds the lock); retry in a moment")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

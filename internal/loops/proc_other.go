//go:build !unix

package loops

import (
	"os"
	"syscall"
	"time"
)

// Non-unix builds have no sessions, process groups or flock: runs start
// attached, stop kills only the leader, and the lock is a no-op.

func detachedAttr() *syscall.SysProcAttr { return nil }

func groupAttr() *syscall.SysProcAttr { return nil }

func killGroup(pid int, _ syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	return p.Kill()
}

func processAlive(pid int) bool { return pid > 0 }

func lockFile(string, time.Duration) (func(), error) { return func() {}, nil }

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gotodo/internal/loops"
)

// lockedBuffer is a bytes.Buffer safe to read while a follower writes.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// writeLoopState writes the board's loop state.json by hand, as the
// engine would, so no runner ever starts.
func writeLoopState(t *testing.T, board string, runs ...loops.RunRecord) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"primed": testRef, "processed": map[string]int{}, "runs": runs})
	if err != nil {
		t.Fatal(err)
	}
	dir := loops.StateDir(board)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func liveRun(board, id string) loops.RunRecord {
	return loops.RunRecord{N: 1, TaskID: id, Stage: "indev", Agent: "claude", Started: testRef, Status: loops.RunRunning,
		Log: filepath.Join(loops.StateDir(board), shortID(id)+"-indev-1.log")}
}

// -f prints the log so far, then appended bytes, and exits once the run
// has ended and its last line (written after the state) is out.
func TestLoopsLogsFollowPrintsAppendedDataUntilTheRunEnds(t *testing.T) {
	board, id := loopsEnv(t)
	rec := liveRun(board, id)
	writeLoopState(t, board, rec)
	appendFile(t, rec.Log, "== gotodo loop · indev · run 1\n")

	var out lockedBuffer
	done := make(chan error, 1)
	go func() { done <- followLoopLog(context.Background(), board, rec, &out, 5*time.Millisecond) }()

	waitFor(t, "the header", func() bool { return strings.Contains(out.String(), "== gotodo loop") })
	appendFile(t, rec.Log, "agent: working\n")
	waitFor(t, "appended output", func() bool { return strings.Contains(out.String(), "agent: working") })
	select {
	case err := <-done:
		t.Fatalf("follow exited while the run is live: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// The runner records the end first, then writes the last line.
	ended := rec
	end := testRef.Add(time.Minute)
	ended.Status, ended.Ended = loops.RunDone, &end
	writeLoopState(t, board, ended)
	appendFile(t, rec.Log, "== ended · done · exit 0\n")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not exit after the run ended")
	}
	if got := out.String(); got != "== gotodo loop · indev · run 1\nagent: working\n== ended · done · exit 0\n" {
		t.Errorf("followed output = %q", got)
	}
}

// Ctrl+C (a cancelled context) ends the follow of a run that never ends.
func TestLoopsLogsFollowStopsOnInterrupt(t *testing.T) {
	board, id := loopsEnv(t)
	rec := liveRun(board, id)
	writeLoopState(t, board, rec)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var out lockedBuffer
	go func() { done <- followLoopLog(ctx, board, rec, &out, 5*time.Millisecond) }()
	time.Sleep(30 * time.Millisecond) // no log file yet: nothing to print, still waiting
	cancel()
	select {
	case err := <-done:
		if err != nil || out.String() != "" {
			t.Errorf("err=%v out=%q", err, out.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow ignored the interrupt")
	}
}

// A run whose runner died is lost, which ends the follow too.
func TestLoopsLogsFollowEndsForALostRun(t *testing.T) {
	board, id := loopsEnv(t)
	rec := liveRun(board, id)
	rec.PID = 4242
	writeLoopState(t, board, rec)
	appendFile(t, rec.Log, "partial\n")
	loops.Hooks.Alive = func(int) bool { return false }
	var out lockedBuffer
	if err := followLoopLog(context.Background(), board, rec, &out, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if out.String() != "partial\n" {
		t.Errorf("out = %q", out.String())
	}
}

// The CLI wires -f and -stage; a finished run prints and returns.
func TestLoopsLogsCLIFollowFinishedRun(t *testing.T) {
	board, id := loopsEnv(t)
	end := testRef.Add(time.Minute)
	rec := liveRun(board, id)
	rec.Status, rec.Ended = loops.RunDone, &end
	writeLoopState(t, board, rec)
	appendFile(t, rec.Log, "whole log\n")
	out, err := runOut(t, "-file", board, "loops", "logs", shortID(id), "-stage", "indev", "-f")
	if err != nil || out != "whole log\n" {
		t.Fatalf("logs -f = %q, %v", out, err)
	}
	if _, err := runOut(t, "-file", board, "loops", "logs", shortID(id), "-stage", "testing-review", "-f"); err == nil {
		t.Error("logs -f for a stage with no runs did not fail")
	}
}

// A log that shrank (rotated or rewritten) is read again from the start.
func TestCopyLogFromRestartsAfterTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.log")
	appendFile(t, path, "first line\n")
	var off int64
	var out bytes.Buffer
	if _, err := copyLogFrom(path, &off, &out); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := copyLogFrom(path, &off, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "first line\nnew\n" || off != 4 {
		t.Errorf("out=%q off=%d", out.String(), off)
	}
	if n, err := copyLogFrom(filepath.Join(t.TempDir(), "missing.log"), new(int64), &out); n != 0 || err != nil {
		t.Errorf("missing file: n=%d err=%v", n, err)
	}
}

package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// DefaultPath is the per-user tasks file, e.g. ~/.config/gotodo/tasks.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "gotodo", "tasks.json"), nil
}

// SetPath points the board at a file for later Save calls.
func (b *Board) SetPath(p string) { b.path = p }

// Load reads the board from disk. A missing file yields an empty board so
// the first run is not an error; malformed JSON is an error so a bad file is
// never silently overwritten with an empty board.
func Load(path string) (*Board, error) {
	b := &Board{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	b.path = path
	return b, nil
}

// Save writes the board atomically: temp file first, then rename. Each
// writer gets a unique temp file (via os.CreateTemp) so two concurrent
// processes never race over the same intermediate name.
func (b *Board) Save() error {
	if b.path == "" {
		return errors.New("board has no path; call SetPath or Load first")
	}
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encode board: %w", err)
	}
	f, err := os.CreateTemp(dir, "tasks-*.json")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := os.Rename(tmp, b.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp: %w", err)
	}
	b.dirty = false
	return nil
}

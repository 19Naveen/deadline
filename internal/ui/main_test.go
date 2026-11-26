package ui

import (
	"errors"
	"os"
	"testing"
)

// TestMain keeps every test away from the real repository: git reports "not
// a repository" and a board outside a project searches an empty directory.
// Picker tests opt back in with a temp repository.
func TestMain(m *testing.M) {
	empty, err := os.MkdirTemp("", "gotodo-ui-test")
	if err != nil {
		panic(err)
	}
	// Loop config is read from XDG_CONFIG_HOME; never the user's own.
	os.Setenv("XDG_CONFIG_HOME", empty)
	defaultGit = func(string, ...string) (string, error) { return "", errors.New("not a git repository") }
	workingDir = func() (string, error) { return empty, nil }
	code := m.Run()
	os.RemoveAll(empty)
	os.Exit(code)
}

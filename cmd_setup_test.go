package main

import (
	"strings"
	"testing"
)

func TestSetupRejectsConflictingLoopFlags(t *testing.T) {
	err := runSetup([]string{"-loops", "-no-loops"})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("err = %v", err)
	}
	if err := runSetup([]string{"extra"}); err == nil {
		t.Fatal("stray argument should fail")
	}
}

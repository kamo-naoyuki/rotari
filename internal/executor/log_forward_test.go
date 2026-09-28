package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestMain lets wrapper integration tests execute the package test binary as
// the internal helper, matching the hidden command dispatch in cmd/rotari.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "__log-forward" {
		os.Exit(RunLogForward(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func installTestRotari(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A coverage-instrumented binary warns on stderr unless GOCOVERDIR is set.
	script := fmt.Sprintf("#!/bin/sh\nGOCOVERDIR=%q exec %q \"$@\"\n", t.TempDir(), executable)
	if err := os.WriteFile(filepath.Join(binDir, "rotari"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binDir
}

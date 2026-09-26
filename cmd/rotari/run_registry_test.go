package main

import (
	"github.com/kamo-naoyuki/rotari/internal/state"
	"testing"
)

func TestDeleteRunRejectsUnsafeRunIDs(t *testing.T) {
	paths := state.ProjectPaths{RunsDir: t.TempDir()}
	for _, runID := range []string{"", "../run-1", "nested/run-1", "run/..", "run/."} {
		if err := deleteRun(paths, runID); err == nil {
			t.Fatalf("deleteRun accepted unsafe run ID %q", runID)
		}
	}
}

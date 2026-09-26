package queueops

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestDeleteRunRejectsUnsafeRunIDs(t *testing.T) {
	paths := state.ProjectPaths{RunsDir: t.TempDir()}
	for _, runID := range []string{"", "../run-1", "nested/run-1", "run/..", "run/."} {
		if err := (Editor{}).deleteRun(paths, runID); err == nil {
			t.Fatalf("deleteRun accepted unsafe run ID %q", runID)
		}
	}
}

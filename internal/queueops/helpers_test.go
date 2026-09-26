package queueops

import (
	"fmt"
	"sync/atomic"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

var testIDs atomic.Int64

// testEditor edits queues with the built-in executors and distinct job IDs.
func testEditor() Editor {
	store := state.NewStore(0o755, 0o644)
	return Editor{
		Store: store, Executors: executor.NewRegistry(store, func(string, ...any) {}),
		NewJobID:      func() string { return fmt.Sprintf("id%07d", testIDs.Add(1)) },
		UnregisterRun: func(string) error { return nil },
	}
}

// testRunID returns a distinct run ID in the format rotari generates.
func testRunID() string {
	return fmt.Sprintf("20260927-000000-%08x", testIDs.Add(1))
}

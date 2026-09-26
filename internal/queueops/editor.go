package queueops

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Editor edits queues and run history on disk. Its fields are the
// process-wide settings the operations need.
type Editor struct {
	Store state.Store
	// Executors decides which executor names an added or changed job may
	// use.
	Executors executor.Registry
	// NewJobID returns a fresh job or matrix group ID.
	NewJobID func() string
	// UnregisterRun removes a deleted run from the run registry.
	UnregisterRun func(runID string) error
}

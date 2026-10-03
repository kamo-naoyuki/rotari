package queueops

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/project"
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
	// RegisterBaseDir records a basedir in which an applied add or copy
	// wrote a queue, so that discovery across basedirs finds the project.
	RegisterBaseDir func(baseDir string) error
	// Guard previews the operations or applies them only at a given project
	// revision; see project.Guard. The zero Guard applies them.
	Guard project.Guard
}

// registerBaseDir registers baseDir after an applied edit; a dry run writes
// nothing, the registry included.
func (editor Editor) registerBaseDir(baseDir string) error {
	if editor.Guard.DryRun || editor.RegisterBaseDir == nil {
		return nil
	}
	if err := editor.RegisterBaseDir(baseDir); err != nil {
		return fmt.Errorf("failed to register state directory: %w", err)
	}
	return nil
}

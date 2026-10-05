package project

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ResetResult is what Reset did or, for a dry run, would do.
type ResetResult struct {
	// Cleared counts the queued jobs removed.
	Cleared int
}

// Reset empties the queue of a project under guard, keeping its defaults and
// run history. A project that does not exist yet is created empty; running or
// interrupted runs are left untouched. `rotari reset` and the MCP reset tools
// use it.
func Reset(paths state.ProjectPaths, guard Guard) (ResetResult, error) {
	cleared := 0
	err := CreateQueueGuarded(paths, guard, func(queue *model.Queue) error {
		cleared = len(queue.Commands)
		queue.Commands = nil
		return nil
	})
	if err != nil {
		return ResetResult{}, fmt.Errorf("failed to reset queue: %w", err)
	}
	return ResetResult{Cleared: cleared}, nil
}

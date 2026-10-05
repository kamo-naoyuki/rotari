package project

import (
	"errors"
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ErrInterruptedRun reports that a reset would recover an interrupted run,
// whose jobs may still be running, without the caller having confirmed it.
var ErrInterruptedRun = errors.New("project has an interrupted run; resetting it recovers the run and needs confirmation")

// ResetResult is what Reset did or, for a dry run, would do.
type ResetResult struct {
	// Cleared counts the queued jobs removed.
	Cleared int
	// RecoveredRunID is the interrupted run recovered, if any.
	RecoveredRunID string
}

// Reset empties the queue of an idle project under guard, keeping its
// defaults and run history; a project that does not exist yet is created
// empty. An interrupted run is recovered with it only when
// recoverInterrupted confirms it, since its jobs may still be running. A
// running project is refused. `rotari reset` and the MCP reset tools use it.
func Reset(paths state.ProjectPaths, recoverInterrupted bool, guard Guard) (ResetResult, error) {
	inspection, err := InspectConsistent(paths, !guard.DryRun)
	if err != nil {
		return ResetResult{}, fmt.Errorf("failed to check project state: %w", err)
	}
	switch inspection.State {
	case Running:
		return ResetResult{}, fmt.Errorf("project %q is running run %q; cancel it before resetting", paths.ProjectName, inspection.RunID)
	case Interrupted:
		if !recoverInterrupted {
			detail, _ := InterruptedRunDetail(paths, inspection.RunID)
			return ResetResult{}, fmt.Errorf("%w: run %q%s", ErrInterruptedRun, inspection.RunID, detail)
		}
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			return ResetResult{}, fmt.Errorf("failed to load queue: %w", err)
		}
		if err := RecoverInterruptedGuarded(paths, inspection.RunID, true, guard); err != nil {
			return ResetResult{}, fmt.Errorf("failed to recover interrupted run: %w", err)
		}
		return ResetResult{Cleared: len(queue.Commands), RecoveredRunID: inspection.RunID}, nil
	}
	cleared := 0
	err = CreateQueueGuarded(paths, guard, func(queue *model.Queue) error {
		cleared = len(queue.Commands)
		queue.Commands = nil
		return nil
	})
	if err != nil {
		return ResetResult{}, fmt.Errorf("failed to reset queue: %w", err)
	}
	return ResetResult{Cleared: cleared}, nil
}

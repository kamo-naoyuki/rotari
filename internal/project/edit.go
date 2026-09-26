package project

import (
	"fmt"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Edit runs edit while holding the project's state lock, after checking that
// the project is idle. operation names the command in the rejection message.
func Edit(paths state.ProjectPaths, operation string, edit func() error) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	if err := EnsureIdle(paths, operation); err != nil {
		return err
	}
	return edit()
}

// EditQueue applies edit to the project's queue under Edit and saves the
// result with WriteIdleQueue. When edit returns an error, nothing is written.
func EditQueue(paths state.ProjectPaths, operation string, edit func(queue *model.Queue) error) error {
	return Edit(paths, operation, func() error {
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			return fmt.Errorf("failed to load queue: %w", err)
		}
		if err := edit(&queue); err != nil {
			return err
		}
		return WriteIdleQueue(paths, queue)
	})
}

// WriteIdleQueue saves a queue edited while the project is idle and marks the
// project as collecting. The two files cannot be replaced atomically together,
// so the metadata is written first: marking an idle project as collecting is
// harmless on its own, and a failed queue write then leaves the previous queue
// in place. Callers must hold the state lock and have checked that the project
// is idle.
func WriteIdleQueue(paths state.ProjectPaths, queue model.Queue) error {
	return writeIdleQueueWith(state.WriteJSON, paths, &queue)
}

// MarkCollecting updates only the metadata, for idle operations that leave
// the queue file untouched.
func MarkCollecting(paths state.ProjectPaths) error {
	return writeIdleQueueWith(state.WriteJSON, paths, nil)
}

func writeIdleQueueWith(writeJSON func(string, any) error, paths state.ProjectPaths, queue *model.Queue) error {
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	if queue == nil {
		return nil
	}
	if err := writeJSON(paths.QueueFile, *queue); err != nil {
		return fmt.Errorf("failed to write queue: %w", err)
	}
	return nil
}

// RecoverInterrupted returns an interrupted project to collecting, keeping or
// discarding the retained queue. It fails unless runID is still the
// project's interrupted run.
func RecoverInterrupted(paths state.ProjectPaths, runID string, discardQueue bool) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	inspection, err := InspectConsistent(paths, true)
	if err != nil {
		return err
	}
	if inspection.State != Interrupted || inspection.RunID != runID {
		return fmt.Errorf("project %q no longer has interrupted run %q", paths.ProjectName, runID)
	}
	// Queue first: a failed metadata write leaves the project interrupted and
	// recoverable instead of idle with a stale queue.
	if discardQueue {
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			return err
		}
		queue.Commands = nil
		queue.WorkflowImport = false
		if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
			return err
		}
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return err
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return state.WriteJSON(paths.MetaFile, meta)
}

package project

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Guard conditions an idle edit. The zero Guard applies the edit
// unconditionally.
type Guard struct {
	// DryRun computes the edit without writing anything.
	DryRun bool
	// IfRevision, when set, refuses the edit unless the project is still at
	// this revision; see Revision.
	IfRevision string
	// Report, when set, receives the edit's outcome once it succeeds.
	Report func(Outcome)
}

// Outcome describes a guarded edit.
type Outcome struct {
	// Revision is the project's revision before the edit.
	Revision string
	// Queue is the queue the edit produced, or nil for an edit of the run
	// history that leaves the queue alone.
	Queue *model.Queue
	// Applied reports that the edit was written; it is false for a dry run.
	Applied bool
	// NewRevision is the project's revision after an applied edit.
	NewRevision string
}

// ErrRevisionChanged reports that a guarded edit found the project at
// another revision than the one it was planned on.
var ErrRevisionChanged = errors.New("project changed since the planned revision")

// Revision identifies the project's queue and metadata as they are now: it
// changes whenever either file is written, including by a run or an earlier
// edit. A caller that previews an edit passes it back as Guard.IfRevision.
func Revision(paths state.ProjectPaths) (string, error) {
	hash := sha256.New()
	for _, path := range []string{paths.QueueFile, paths.MetaFile} {
		data, err := os.ReadFile(path) // NOSONAR: paths are the resolved project's own state files.
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("failed to read project state: %w", err)
		}
		fmt.Fprintf(hash, "%d\n", len(data))
		hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

// Edit runs edit while holding the project's state lock, after checking that
// the project is idle. operation names the command in the rejection message.
func Edit(paths state.ProjectPaths, operation string, edit func() error) error {
	return EditGuarded(paths, operation, Guard{}, func(bool) error { return edit() })
}

// EditGuarded is Edit under guard. It checks guard.IfRevision under the lock,
// and passes guard.DryRun to edit, which must not write when it is set.
func EditGuarded(paths state.ProjectPaths, operation string, guard Guard, edit func(dryRun bool) error) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	if err := EnsureIdle(paths, operation); err != nil {
		return err
	}
	revision, err := CheckRevision(paths, guard)
	if err != nil {
		return err
	}
	if err := edit(guard.DryRun); err != nil {
		return err
	}
	return report(paths, guard, Outcome{Revision: revision})
}

// EditQueue applies edit to the project's queue under Edit and saves the
// result with WriteIdleQueue. When edit returns an error, nothing is written.
func EditQueue(paths state.ProjectPaths, operation string, edit func(queue *model.Queue) error) error {
	return EditQueueGuarded(paths, operation, Guard{}, edit)
}

// EditQueueGuarded is EditQueue under guard: a dry run computes the queue
// edit reports, without writing it.
func EditQueueGuarded(paths state.ProjectPaths, operation string, guard Guard, edit func(queue *model.Queue) error) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	if err := EnsureIdle(paths, operation); err != nil {
		return err
	}
	revision, err := CheckRevision(paths, guard)
	if err != nil {
		return err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}
	if err := edit(&queue); err != nil {
		return err
	}
	if !guard.DryRun {
		if err := WriteIdleQueue(paths, queue); err != nil {
			return err
		}
	}
	return report(paths, guard, Outcome{Revision: revision, Queue: &queue})
}

// CreateQueueGuarded is EditQueueGuarded for an edit that may create the
// project, such as add or import. Applying it creates the project directory
// first. A dry run of a project that does not exist yet edits an empty queue
// without locking or creating anything, and reports the revision a new
// project has.
func CreateQueueGuarded(paths state.ProjectPaths, operation string, guard Guard, edit func(queue *model.Queue) error) error {
	if _, err := os.Stat(paths.ProjectDir); errors.Is(err, os.ErrNotExist) && guard.DryRun {
		revision, err := CheckRevision(paths, guard)
		if err != nil {
			return err
		}
		var queue model.Queue
		if err := edit(&queue); err != nil {
			return err
		}
		return report(paths, guard, Outcome{Revision: revision, Queue: &queue})
	}
	if !guard.DryRun {
		if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
			return err
		}
	}
	return EditQueueGuarded(paths, operation, guard, edit)
}

// CheckRevision returns the project's revision, refusing the edit when it is
// not guard.IfRevision. Callers hold the state lock, unless the project does
// not exist yet.
func CheckRevision(paths state.ProjectPaths, guard Guard) (string, error) {
	revision, err := Revision(paths)
	if err != nil {
		return "", err
	}
	if guard.IfRevision != "" && guard.IfRevision != revision {
		return "", fmt.Errorf("%w: planned on %s, now %s; preview the change again", ErrRevisionChanged, guard.IfRevision, revision)
	}
	return revision, nil
}

// report completes outcome and passes it to guard.Report.
func report(paths state.ProjectPaths, guard Guard, outcome Outcome) error {
	if guard.Report == nil {
		return nil
	}
	outcome.Applied = !guard.DryRun
	if outcome.Applied {
		revision, err := Revision(paths)
		if err != nil {
			return err
		}
		outcome.NewRevision = revision
	}
	guard.Report(outcome)
	return nil
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
	return RecoverInterruptedGuarded(paths, runID, discardQueue, Guard{})
}

// RecoverInterruptedGuarded is RecoverInterrupted under guard.
func RecoverInterruptedGuarded(paths state.ProjectPaths, runID string, discardQueue bool, guard Guard) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	inspection, err := InspectConsistent(paths, !guard.DryRun)
	if err != nil {
		return err
	}
	if inspection.State != Interrupted || inspection.RunID != runID {
		return fmt.Errorf("project %q no longer has interrupted run %q", paths.ProjectName, runID)
	}
	revision, err := CheckRevision(paths, guard)
	if err != nil {
		return err
	}
	if guard.DryRun {
		return report(paths, guard, Outcome{Revision: revision})
	}
	if err := recoverInterrupted(paths, discardQueue); err != nil {
		return err
	}
	return report(paths, guard, Outcome{Revision: revision})
}

// recoverInterrupted writes the recovery; the caller holds the state lock.
func recoverInterrupted(paths state.ProjectPaths, discardQueue bool) error {
	// Queue first: a failed metadata write leaves the project interrupted and
	// recoverable instead of idle with a stale queue.
	if discardQueue {
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			return err
		}
		queue.Commands = nil
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

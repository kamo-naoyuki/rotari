package project

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ErrRunAlive reports that the run Unlock was asked to recover still has a
// live coordinator on this host, so removing its lock would let a second
// runner start.
var ErrRunAlive = errors.New("run is still running")

// ErrNoInterruptedRun reports that the project has no interrupted run with
// the requested ID.
var ErrNoInterruptedRun = errors.New("no matching interrupted run exists")

// UnlockResult is what Unlock did or, for a dry run, would do.
type UnlockResult struct {
	// RunID is the interrupted run recovered; empty when the project had none
	// and Unlock changed nothing.
	RunID string
	// Detail is what the run's jobs last reported, for a message.
	Detail string
	// JobsMayBeRunning reports that some of the run's jobs have not recorded
	// a final status, so they may still be running.
	JobsMayBeRunning bool
}

// Unlock recovers the project's interrupted run under guard: it removes the
// run's lock, unless the run's coordinator is alive on this host, and returns
// the project to collecting. runID must name that run; when it is empty, the
// run the lock or metadata names is recovered, and a project without one is
// left unchanged. A lock from another host cannot be checked, so it is
// removed on the caller's word that the run stopped. The queue is left as it
// is: the run took its jobs when it started, and they are rerun from the run.
// `rotari unlock` and the MCP unlock tools use it.
func Unlock(paths state.ProjectPaths, runID string, guard Guard) (UnlockResult, error) {
	if runID != "" && !state.IsValidPathElement(runID) {
		return UnlockResult{}, fmt.Errorf("invalid run ID %q", runID)
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return UnlockResult{}, fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	lock, err := state.LoadLock(paths.LockFile)
	lockExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return UnlockResult{}, fmt.Errorf("failed to read run lock: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return UnlockResult{}, fmt.Errorf("failed to load metadata: %w", err)
	}
	if !lockExists && runID == "" && (meta.Phase == "collecting" || meta.Phase == "finished") {
		revision, err := CheckRevision(paths, guard)
		if err != nil {
			return UnlockResult{}, err
		}
		return UnlockResult{}, report(paths, guard, Outcome{Revision: revision})
	}
	if runID == "" {
		if lockExists {
			runID = lock.RunID
		} else {
			runID = meta.LastRunID
		}
	}
	if runID == "" {
		return UnlockResult{}, ErrNoInterruptedRun
	}
	if lockExists {
		if lock.RunID != runID {
			return UnlockResult{}, fmt.Errorf("run lock belongs to %q, not %q", lock.RunID, runID)
		}
		lockState, _, err := state.InspectLock(paths.LockFile, false)
		if err != nil {
			return UnlockResult{}, fmt.Errorf("failed to inspect run lock: %w", err)
		}
		if lockState == state.LockActive {
			return UnlockResult{}, fmt.Errorf("%w: run %q of project %q", ErrRunAlive, runID, paths.ProjectName)
		}
	} else if (meta.Phase != "running" && meta.Phase != "cancelling") || meta.LastRunID != runID {
		return UnlockResult{}, ErrNoInterruptedRun
	}
	revision, err := CheckRevision(paths, guard)
	if err != nil {
		return UnlockResult{}, err
	}
	detail, stillRunning := InterruptedRunDetail(paths, runID)
	result := UnlockResult{RunID: runID, Detail: strings.TrimPrefix(detail, ": "), JobsMayBeRunning: stillRunning}
	if guard.DryRun {
		return result, report(paths, guard, Outcome{Revision: revision})
	}
	if lockExists {
		if err := os.Remove(paths.LockFile); err != nil {
			return UnlockResult{}, fmt.Errorf("failed to remove run lock: %w", err)
		}
	}
	if err := recoverInterrupted(paths); err != nil {
		return UnlockResult{}, fmt.Errorf("failed to update metadata: %w", err)
	}
	return result, report(paths, guard, Outcome{Revision: revision})
}

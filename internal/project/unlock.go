package project

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
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
	RunID string `json:"run_id,omitempty"`
	// Detail is what the run's jobs last reported, for a message.
	Detail string `json:"detail,omitempty"`
	// JobsMayBeRunning reports that some of the run's jobs have not recorded
	// a final status, so they may still be running.
	JobsMayBeRunning bool                  `json:"jobs_may_be_running"`
	ExecutionChecked bool                  `json:"execution_checked"`
	Coordinator      CoordinatorAssessment `json:"coordinator"`
	Allowed          bool                  `json:"allowed"`
	RefusalCode      string                `json:"refusal_code,omitempty"`
	AssumeStopped    bool                  `json:"assume_stopped"`
	Revision         string                `json:"revision"`
	Applied          bool                  `json:"applied"`
}

type UnlockRefusedError struct{ Result UnlockResult }

func (err *UnlockRefusedError) Error() string {
	identity := err.Result.Coordinator.Identity
	location := ""
	if identity != nil {
		location = fmt.Sprintf(" (host=%s pid=%d namespace=%s)", identity.Host, identity.PID, identity.PIDNamespace)
	}
	if err.Result.RefusalCode == "coordinator_live" {
		return fmt.Sprintf("coordinator_live: run coordinator is running: %s%s", err.Result.Coordinator.Reason, location)
	}
	return fmt.Sprintf("coordinator_unknown: %s%s; inspect on the original host in the same PID namespace if accessible, or explicitly attest that the coordinator stopped with --assume-stopped; job termination is not verified", err.Result.Coordinator.Reason, location)
}

func (err *UnlockRefusedError) Unwrap() error {
	if err.Result.RefusalCode == "coordinator_live" {
		return ErrRunAlive
	}
	return nil
}

// Unlock recovers the interrupted run after assessing coordinator ownership.
// Unknown ownership requires explicit attestation; live ownership always blocks.
// Previews return refusal evidence without mutation. Job termination remains
// the operator's responsibility. Queue contents and historical results are kept.
func Unlock(paths state.ProjectPaths, runID string, guard Guard, assumeStopped bool) (UnlockResult, error) {
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
		return UnlockResult{Allowed: true, Revision: revision}, report(paths, guard, Outcome{Revision: revision})
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
	} else if (meta.Phase != "running" && meta.Phase != "cancelling") || meta.LastRunID != runID {
		return UnlockResult{}, ErrNoInterruptedRun
	}
	revision, err := CheckRevision(paths, guard)
	if err != nil {
		return UnlockResult{}, err
	}
	detail, stillRunning := InterruptedRunDetail(paths, runID)
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return UnlockResult{}, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	assessment, err := AssessCoordinator(store, runDir)
	if err != nil {
		return UnlockResult{}, err
	}
	if assessment.Identity == nil && lockExists {
		assessment.Identity = &model.RunCoordinator{Host: lock.Host, PID: lock.PID, PIDNamespace: lock.PIDNamespace}
	}
	result := UnlockResult{RunID: runID, Detail: strings.TrimPrefix(detail, ": "), JobsMayBeRunning: stillRunning, Coordinator: assessment, AssumeStopped: assumeStopped, Revision: revision}
	switch assessment.State {
	case CoordinatorLive:
		result.RefusalCode = "coordinator_live"
	case CoordinatorUnknown:
		if !assumeStopped {
			result.RefusalCode = "coordinator_unknown"
		}
	}
	result.Allowed = result.RefusalCode == ""
	if guard.DryRun {
		// A preview describes refusal without failing, so callers can inspect
		// the same evidence and revision they will use for an explicit apply.
		if guard.Report != nil {
			guard.Report(Outcome{Revision: revision})
		}
		return result, nil
	}
	if !result.Allowed {
		return result, &UnlockRefusedError{Result: result}
	}
	context, err := state.LoadContext(store, runDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	context.Recovery = &model.RunRecovery{
		At: time.Now().UTC().Format(time.RFC3339Nano), AssumeStopped: assumeStopped,
		CoordinatorState: string(assessment.State), CoordinatorReason: assessment.Reason,
		Coordinator: assessment.Identity,
	}
	if err := state.SaveContext(store, runDir, context); err != nil {
		return result, fmt.Errorf("record recovery evidence: %w", err)
	}
	if lockExists {
		if err := os.Remove(paths.LockFile); err != nil {
			return UnlockResult{}, fmt.Errorf("failed to remove run lock: %w", err)
		}
	}
	if err := recoverInterrupted(paths); err != nil {
		return UnlockResult{}, fmt.Errorf("failed to update metadata: %w", err)
	}
	result.Applied = true
	result.Revision, err = Revision(paths)
	if err != nil {
		return result, err
	}
	return result, report(paths, guard, Outcome{Revision: revision})
}

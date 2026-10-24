package project

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
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
	Action           string                `json:"action,omitempty"`
	Repair           RepairAssessment      `json:"repair"`
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
	if err.Result.RefusalCode == "execution_unknown" {
		return "execution_unknown: recovery requires --assume-stopped after confirming all managed executions, including earlier attempts and carried origins, stopped; terminal outcome records are not termination evidence"
	}
	if err.Result.RefusalCode == "execution_active" {
		return "execution_active: a fresh probe verified active managed execution; inspect repair.executions and stop the work before recovery; --assume-stopped cannot override it"
	}
	return fmt.Sprintf("coordinator_unknown: %s%s; inspect on the original host in the same PID namespace if accessible, or explicitly attest that the coordinator stopped with --assume-stopped; job termination is not verified", err.Result.Coordinator.Reason, location)
}

func (err *UnlockRefusedError) Unwrap() error {
	if err.Result.RefusalCode == "coordinator_live" {
		return ErrRunAlive
	}
	return nil
}

// Unlock assesses coordinator and execution evidence outside the state lock,
// then revalidates the recorded state under the lock before recovery.
func Unlock(paths state.ProjectPaths, runID string, guard Guard, assumeStopped bool) (UnlockResult, error) {
	if runID != "" && !state.IsValidPathElement(runID) {
		return UnlockResult{}, fmt.Errorf("invalid run ID %q", runID)
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return UnlockResult{}, fmt.Errorf("failed to lock queue: %w", err)
	}
	snapshot, noOp, err := unlockSnapshot(paths, runID, guard)
	release()
	if err != nil {
		return UnlockResult{}, err
	}
	if noOp {
		return UnlockResult{Allowed: true, Revision: snapshot.revision}, report(paths, guard, Outcome{Revision: snapshot.revision})
	}

	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	repair, summary, err := assessRepair(paths, snapshot.runID, snapshot.meta, store)
	if err != nil {
		return UnlockResult{}, err
	}
	executions := repair.Executions
	if !repair.Complete {
		assessment, err := AssessExecutions(paths, snapshot.runID)
		if err != nil {
			return UnlockResult{}, fmt.Errorf("assess executions before recovery: %w", err)
		}
		executions = assessment.Executions
	}
	expireStaleExecutionEvidence(executions, time.Now().UTC())
	repair.Executions = executions
	result := UnlockResult{
		RunID: snapshot.runID, Detail: strings.TrimPrefix(snapshot.detail, ": "),
		Coordinator: CoordinatorAssessment{State: CoordinatorUnknown, Reason: "coordinator assessment is pending state revalidation"},
		Revision:    snapshot.revision, AssumeStopped: assumeStopped, Repair: repair,
	}
	if repair.Complete {
		result.Action = "finalize"
	} else {
		result.Action = "release"
	}
	result.ExecutionChecked = len(executions) > 0
	for _, execution := range executions {
		if execution.State != "stopped" {
			result.JobsMayBeRunning = true
		}
		if execution.State == "unknown" {
			result.ExecutionChecked = false
		}
	}

	release, err = state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return result, fmt.Errorf("failed to re-lock queue for recovery: %w", err)
	}
	defer release()
	current, noOp, err := unlockSnapshot(paths, snapshot.runID, Guard{})
	if err != nil {
		return result, fmt.Errorf("revalidate interrupted run: %w", err)
	}
	if noOp || current.revision != snapshot.revision {
		return result, fmt.Errorf("%w: execution inventory or project state changed during assessment; preview the recovery again", ErrRevisionChanged)
	}
	lock, lockExists, meta, err := loadUnlockState(paths)
	if err != nil {
		return result, err
	}
	assessment, err := AssessCoordinator(store, snapshot.runDir)
	if err != nil {
		return result, err
	}
	if assessment.Identity == nil && lockExists {
		assessment.Identity = &model.RunCoordinator{Host: lock.Host, PID: lock.PID, PIDNamespace: lock.PIDNamespace}
	}
	result.Coordinator = assessment
	expireStaleExecutionEvidence(result.Repair.Executions, time.Now().UTC())
	evaluateUnlockPolicy(&result)
	if guard.DryRun {
		if guard.Report != nil {
			guard.Report(Outcome{Revision: snapshot.revision})
		}
		return result, nil
	}
	if !result.Allowed {
		return result, &UnlockRefusedError{Result: result}
	}
	context, err := state.LoadContext(store, snapshot.runDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	context.Recovery = &model.RunRecovery{
		At: time.Now().UTC().Format(time.RFC3339Nano), AssumeStopped: assumeStopped,
		CoordinatorState: string(assessment.State), CoordinatorReason: assessment.Reason,
		Coordinator: assessment.Identity,
		Action:      result.Action, Executions: result.Repair.Executions,
	}
	if err := state.SaveContext(store, snapshot.runDir, context); err != nil {
		return result, fmt.Errorf("record recovery evidence: %w", err)
	}
	if lockExists {
		if err := os.Remove(paths.LockFile); err != nil {
			return UnlockResult{}, fmt.Errorf("failed to remove run lock: %w", err)
		}
	}
	if result.Action == "finalize" {
		err = finalizeRepair(paths, snapshot.runDir, meta, summary, store)
	} else {
		err = recoverInterrupted(paths)
	}
	if err != nil {
		return UnlockResult{}, fmt.Errorf("failed to update metadata: %w", err)
	}
	result.Applied = true
	result.Revision, err = Revision(paths)
	if err != nil {
		return result, err
	}
	return result, report(paths, guard, Outcome{Revision: snapshot.revision})
}

type unlockStateSnapshot struct {
	runID, runDir, revision, detail string
	meta                            model.Meta
}

// unlockSnapshot must be called with the exclusive project state lock held.
func unlockSnapshot(paths state.ProjectPaths, requestedRunID string, guard Guard) (unlockStateSnapshot, bool, error) {
	lock, lockExists, meta, err := loadUnlockState(paths)
	if err != nil {
		return unlockStateSnapshot{}, false, err
	}
	if !lockExists && requestedRunID == "" && (meta.Phase == "collecting" || meta.Phase == "finished") {
		revision, err := CheckRevision(paths, guard)
		return unlockStateSnapshot{revision: revision}, true, err
	}
	runID := requestedRunID
	if runID == "" {
		if lockExists {
			runID = lock.RunID
		} else {
			runID = meta.LastRunID
		}
	}
	if runID == "" {
		return unlockStateSnapshot{}, false, ErrNoInterruptedRun
	}
	if lockExists {
		if lock.RunID != runID {
			return unlockStateSnapshot{}, false, fmt.Errorf("run lock belongs to %q, not %q", lock.RunID, runID)
		}
	} else if (meta.Phase != "running" && meta.Phase != "cancelling") || meta.LastRunID != runID {
		return unlockStateSnapshot{}, false, ErrNoInterruptedRun
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return unlockStateSnapshot{}, false, err
	}
	revision, err := recoveryRevision(paths, runID)
	if err != nil {
		return unlockStateSnapshot{}, false, err
	}
	if guard.IfRevision != "" && guard.IfRevision != revision {
		return unlockStateSnapshot{}, false, fmt.Errorf("%w: planned on %s, now %s; preview the recovery again", ErrRevisionChanged, guard.IfRevision, revision)
	}
	detail, _ := InterruptedRunDetail(paths, runID)
	return unlockStateSnapshot{runID: runID, runDir: runDir, revision: revision, detail: detail, meta: meta}, false, nil
}

func loadUnlockState(paths state.ProjectPaths) (model.LockInfo, bool, model.Meta, error) {
	lock, err := state.LoadLock(paths.LockFile)
	lockExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return model.LockInfo{}, false, model.Meta{}, fmt.Errorf("failed to read run lock: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return model.LockInfo{}, false, model.Meta{}, fmt.Errorf("failed to load metadata: %w", err)
	}
	return lock, lockExists, meta, nil
}

func evaluateUnlockPolicy(result *UnlockResult) {
	result.RefusalCode = ""
	active, unknown := false, false
	result.ExecutionChecked = len(result.Repair.Executions) > 0
	result.JobsMayBeRunning = false
	for _, execution := range result.Repair.Executions {
		switch execution.State {
		case "stopped":
		case "active":
			active = true
			result.JobsMayBeRunning = true
		default:
			unknown = true
			result.ExecutionChecked = false
			result.JobsMayBeRunning = true
		}
	}
	if active {
		result.RefusalCode = "execution_active"
	} else if unknown && !result.AssumeStopped {
		result.RefusalCode = "execution_unknown"
	}
	switch result.Coordinator.State {
	case CoordinatorLive:
		result.RefusalCode = "coordinator_live"
	case CoordinatorUnknown:
		if !result.AssumeStopped && result.RefusalCode != "execution_active" {
			result.RefusalCode = "coordinator_unknown"
		}
	}
	result.Allowed = result.RefusalCode == ""
}

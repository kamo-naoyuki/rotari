// Package jobcontrol cancels, suspends, and resumes a project's running jobs.
//
// It finds the running run through the project's run lock, resolves job and
// attempt selections against it, and signals each job through the executor
// that owns it. It refuses to signal local processes or runner process groups
// from a host other than the one that started them. It should not decide run
// planning or finalization; a cancelled run finalizes through the normal run
// path.
package jobcontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Controller signals jobs through a registry of executors.
type Controller struct {
	Store     state.Store
	Executors executor.Registry
}

// Control suspends or resumes, as named by operation, the selected running
// jobs of project, or every running job when jobIDs is empty. Attempt IDs
// must name the latest attempt of a running job.
func (controller Controller) Control(paths state.ProjectPaths, project string, jobIDs []string, operation string) (string, error) {
	if operation != "suspend" && operation != "resume" {
		return "", fmt.Errorf("unsupported job operation: %s", operation)
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project %q is not running", project)
		}
		return "", fmt.Errorf("invalid running lock: %w", err)
	}
	runDir, err := runDirectory(paths, lock)
	if err != nil {
		return "", err
	}
	jobIDs, err = controller.normalizeRunningAttemptIDs(runDir, lock.RunID, jobIDs)
	if err != nil {
		return "", err
	}
	allJobs := len(jobIDs) == 0
	targets := append([]string(nil), jobIDs...)
	if allJobs {
		// codeql[go/path-injection]: runDir is produced by the validated run path helper.
		entries, err := os.ReadDir(runDir)
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				targets = append(targets, entry.Name())
			}
		}
	}
	controlled := 0
	for _, jobID := range targets {
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		if err != nil {
			return "", err
		}
		if controller.jobFinished(jobDir) {
			if allJobs {
				continue
			}
			return "", fmt.Errorf("job %q is not running", jobID)
		}
		owner, err := controller.Executors.Owner(controller.Store, jobDir)
		if err != nil {
			if allJobs {
				continue
			}
			return "", fmt.Errorf("job %q is not running", jobID)
		}
		if host, mismatch := executor.LocalHostMismatch(controller.Store, owner, runDir); mismatch {
			return "", fmt.Errorf("job %q runs on host %q; run %s from that host", jobID, host, operation)
		}
		suspender, ok := owner.(executor.Suspender)
		if !ok {
			return "", fmt.Errorf("executor %q does not support %s", owner.Name(), operation)
		}
		schedulerState := "suspended"
		if operation == "resume" {
			err = suspender.Resume(jobDir)
			schedulerState = "running"
		} else {
			err = suspender.Suspend(jobDir)
		}
		if err != nil {
			return "", fmt.Errorf("%s job %s: %w", operation, jobID, err)
		}
		executor.WriteSchedulerStatus(controller.Store, jobDir, schedulerState, time.Now())
		controlled++
	}
	if controlled == 0 {
		return "", fmt.Errorf("no running jobs found in queue %q", project)
	}
	return fmt.Sprintf("%s requested\n  Project: %s\n  Run: %s\n  Jobs: %d", strings.ToUpper(operation[:1])+operation[1:], project, lock.RunID, controlled), nil
}

// Cancel cancels the selected running jobs of project, or its whole run when
// jobIDs is empty. A whole-run cancel marks the queue as cancelling; when this
// process is the runner it cancels each unfinished job directly, and otherwise
// it signals the runner's process group. With wait, it returns once the run
// lock is released.
func (controller Controller) Cancel(paths state.ProjectPaths, project string, jobIDs []string, wait bool) (string, error) {
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project %q is not running", project)
		}
		return "", err
	}
	runDir, err := runDirectory(paths, lock)
	if err != nil {
		return "", err
	}
	jobIDs, err = controller.normalizeRunningAttemptIDs(runDir, lock.RunID, jobIDs)
	if err != nil {
		return "", err
	}
	if len(jobIDs) > 0 {
		return controller.CancelJobs(runDir, project, lock.RunID, jobIDs)
	}
	if err := markCancelling(paths); err != nil {
		return "", err
	}
	if lock.PID == os.Getpid() {
		// Cancel every still-running job directly through its owning
		// executor (job.json for schedulers, pid file for local), instead of
		// relying on an aggregate metadata file that schedulers only write
		// once the whole run finishes -- otherwise a cancel issued mid-run
		// never reaches an already-submitted Slurm/PBS/LSF job.
		snapshot, err := loadCommandSnapshot(runDir)
		if err != nil {
			return "", err
		}
		targets := make([]string, 0, len(snapshot.Commands))
		for _, job := range model.QueueToJobs(snapshot.Commands) {
			jobDir, err := state.LatestAttemptJobDir(runDir, job.ID)
			if err != nil {
				return "", err
			}
			if controller.jobFinished(jobDir) {
				continue
			}
			targets = append(targets, job.ID)
		}
		message, err := controller.CancelJobs(runDir, project, lock.RunID, targets)
		if err != nil {
			return "", err
		}
		return finishCancelMessage(message, paths, lock.RunID, wait)
	}
	if host, mismatch := runnerHostMismatch(lock); mismatch {
		return "", fmt.Errorf("run %q is owned by host %q; run cancel from that host", lock.RunID, host)
	}
	if err := syscall.Kill(-lock.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return "", fmt.Errorf("cancel local worker: %w", err)
	}
	return finishCancelMessage(fmt.Sprintf("Cancel requested\n  Project: %s\n  Run: %s\n  Worker PID: %d", project, lock.RunID, lock.PID), paths, lock.RunID, wait)
}

// CancelJobs cancels jobIDs in runDir through their owning executors. A job
// that was never submitted is marked cancelled so the run skips it.
func (controller Controller) CancelJobs(runDir, project, runID string, jobIDs []string) (string, error) {
	requested := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		requested[jobID] = true
	}
	snapshot, err := loadCommandSnapshot(runDir)
	if err != nil {
		return "", err
	}
	knownJobs := make(map[string]bool)
	for _, job := range model.QueueToJobs(snapshot.Commands) {
		knownJobs[job.ID] = true
	}
	cancelled := 0
	for jobID := range requested {
		if !state.IsValidPathElement(jobID) {
			return "", fmt.Errorf("invalid job ID %q", jobID)
		}
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		if err != nil {
			return "", err
		}
		if owner, err := controller.Executors.Owner(controller.Store, jobDir); err == nil {
			if host, mismatch := executor.LocalHostMismatch(controller.Store, owner, runDir); mismatch {
				return "", fmt.Errorf("job %q runs on host %q; run cancel from that host", jobID, host)
			}
			canceller, ok := owner.(executor.Canceller)
			if !ok {
				return "", fmt.Errorf("executor %q does not support cancel", owner.Name())
			}
			if err := canceller.Cancel(jobDir); err != nil {
				return "", fmt.Errorf("cancel job %s: %w", jobID, err)
			}
			cancelled++
			continue
		}
		// Job has no executor yet (Submit was never called): record the
		// cancellation so the scheduler skips it once it would be submitted.
		if !knownJobs[jobID] {
			return "", fmt.Errorf("job %q is not found", jobID)
		}
		// codeql[go/path-injection]: jobDir comes from validated job path helpers.
		if err := os.MkdirAll(jobDir, state.DirectoryMode()); err != nil { // NOSONAR: jobDir comes from validated job path helpers.
			return "", fmt.Errorf("prepare cancellation for job %s: %w", jobID, err)
		}
		// codeql[go/path-injection]: jobDir is validated and cancelled is a fixed file name.
		if err := os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte(now()+"\n"), state.FileMode()); err != nil { // NOSONAR: jobDir comes from validated job path helpers.
			return "", fmt.Errorf("record cancellation for job %s: %w", jobID, err)
		}
		cancelled++
	}
	return fmt.Sprintf("Cancel requested\n  Project: %s\n  Run: %s\n  Jobs: %d", project, runID, cancelled), nil
}

func runDirectory(paths state.ProjectPaths, lock model.LockInfo) (string, error) {
	if !state.IsValidPathElement(lock.RunID) {
		return "", fmt.Errorf("invalid run ID %q", lock.RunID)
	}
	return state.SafeJoin(paths.RunsDir, lock.RunID)
}

func (controller Controller) jobFinished(jobDir string) bool {
	if path, err := state.ValidatedStateFile(jobDir, "finished_at"); err == nil {
		// codeql[go/path-injection]: path is returned by ValidatedStateFile for a fixed state file.
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	// NOSONAR: jobDir is produced by validated run/job path helpers before reading the status file.
	path, err := state.ValidatedStateFile(jobDir, "status.json")
	if err != nil {
		return false
	}
	var status executor.WrapperStatus
	if err := controller.Store.ReadJSON(path, &status); err != nil {
		return false
	}
	return status.Phase != "" && status.Phase != "running"
}

// normalizeRunningAttemptIDs replaces attempt IDs with their job IDs after
// checking that each names the latest, running attempt of its job.
func (controller Controller) normalizeRunningAttemptIDs(runDir, runID string, jobIDs []string) ([]string, error) {
	normalized := make([]string, 0, len(jobIDs))
	for _, jobID := range jobIDs {
		if !strings.HasPrefix(jobID, "att_") {
			normalized = append(normalized, jobID)
			continue
		}
		payload, err := state.DecodeAttemptID(jobID)
		if err != nil {
			return nil, err
		}
		if payload.RunID != runID {
			return nil, fmt.Errorf("attempt %q belongs to run %q, not %q", jobID, payload.RunID, runID)
		}
		attemptDir, err := state.SpecificAttemptJobDir(runDir, payload.JobID, jobID)
		if err != nil {
			return nil, err
		}
		latestDir, err := state.LatestAttemptJobDir(runDir, payload.JobID)
		if err != nil || filepath.Clean(attemptDir) != filepath.Clean(latestDir) {
			latestAttemptID, _ := state.LatestAttemptID(runDir, payload.JobID)
			if latestAttemptID != "" {
				return nil, fmt.Errorf("attempt %q is not the latest attempt for job %q\nLatest attempt: %q (%s)", jobID, payload.JobID, latestAttemptID, controller.attemptState(latestDir))
			}
			return nil, fmt.Errorf("attempt %q is not the latest attempt for job %q", jobID, payload.JobID)
		}
		if controller.jobFinished(attemptDir) {
			return nil, fmt.Errorf("attempt %q is finished", jobID)
		}
		if _, err := controller.Executors.Owner(controller.Store, attemptDir); err != nil {
			return nil, fmt.Errorf("attempt %q is pending", jobID)
		}
		normalized = append(normalized, payload.JobID)
	}
	return normalized, nil
}

func (controller Controller) attemptState(jobDir string) string {
	if schedulerState := strings.ToLower(strings.TrimSpace(executor.LoadSchedulerStatus(controller.Store, jobDir))); schedulerState != "" {
		return schedulerState
	}
	if path, err := state.ValidatedStateFile(jobDir, "status.json"); err == nil { // NOSONAR: jobDir is validated run/job path
		var status executor.WrapperStatus
		if err := controller.Store.ReadJSON(path, &status); err == nil && status.Phase != "" {
			return strings.ToLower(status.Phase)
		}
	}
	if controller.jobFinished(jobDir) {
		return "finished"
	}
	if _, err := controller.Executors.Owner(controller.Store, jobDir); err == nil {
		return "running"
	}
	return "pending"
}

func loadCommandSnapshot(runDir string) (model.Queue, error) {
	var snapshot model.Queue
	// codeql[go/path-injection]: runDir is produced by validated run path helpers and commands.json is fixed.
	if data, err := os.ReadFile(filepath.Join(runDir, "commands.json")); err == nil { // NOSONAR: runDir is produced by validated run path helpers.
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return model.Queue{}, fmt.Errorf("invalid command snapshot: %w", err)
		}
	}
	return snapshot, nil
}

func markCancelling(paths state.ProjectPaths) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.Phase = "cancelling"
	meta.UpdatedAt = now()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return fmt.Errorf("failed to mark queue as cancelling: %w", err)
	}
	return nil
}

// finishCancelMessage adds an inspection hint to a cancel message and, with
// wait, waits up to five minutes for the run lock to be released.
func finishCancelMessage(message string, paths state.ProjectPaths, runID string, wait bool) (string, error) {
	if wait {
		deadline := time.Now().Add(5 * time.Minute)
		for {
			lockState, _, err := state.InspectLock(paths.LockFile, true)
			if err != nil {
				return "", err
			}
			if lockState != state.LockActive && lockState != state.LockRemote {
				message += "\n\nCancellation complete"
				break
			}
			if time.Now().After(deadline) {
				return "", errors.New("timed out waiting for cancellation")
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	message += fmt.Sprintf("\n\nInspect status:\n  rotari show --run-id %s", runID)
	return message, nil
}

// runnerHostMismatch reports the recorded host when a run's lock belongs to a
// different host than this process. Whole-run cancel signals the runner's
// process group by the PID stored in that lock; on the wrong host that PID
// belongs to (at best) nothing, so the signal harmlessly returns ESRCH and
// would silently report success without cancelling anything.
func runnerHostMismatch(lock model.LockInfo) (recordedHost string, mismatch bool) {
	if lock.Host == "" {
		return "", false
	}
	host, err := os.Hostname()
	if err != nil || strings.EqualFold(host, lock.Host) {
		return "", false
	}
	return lock.Host, true
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdCancel cancels the active run or selected running jobs through the
// background server.
func cmdCancel(args []string) int {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	wait := cliBool(fs, "wait", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 && len(jobIDs) > 0 {
		printError("usage: " + cliUsage("cancel"))
		return 1
	}
	if len(fs.Args()) > 0 {
		jobIDs = append(jobIDs, fs.Args()...)
	}
	if len(jobIDs) > 0 && *wait {
		printError("--wait may not be used with a job selection")
		return 1
	}
	baseDir, queueName, selection, err := resolveJobSelectionTarget(*basedir, *queueNameOption, jobIDs)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureServer(baseDir); err != nil {
		printError(err)
		return 1
	}
	response, err := sendServerRequest(baseDir, serverRequest{Op: serverinternal.OpCancel, QueueName: queueName, JobIDs: selection, Wait: *wait})
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Println(response.Message)
	return 0
}

// cmdJobSignal sends suspend or resume requests for selected running jobs.
func cmdJobSignal(args []string, operation string) int {
	fs := flag.NewFlagSet(operation, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 && len(jobIDs) > 0 {
		printError("usage: " + cliUsage(operation))
		return 1
	}
	if len(fs.Args()) > 0 {
		jobIDs = append(jobIDs, fs.Args()...)
	}
	baseDir, queueName, selection, err := resolveJobSelectionTarget(*basedir, *queueNameOption, jobIDs)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureServer(baseDir); err != nil {
		printError(err)
		return 1
	}
	response, err := sendServerRequest(baseDir, serverRequest{Op: operation, QueueName: queueName, JobIDs: selection})
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	fmt.Println(response.Message)
	return 0
}

func cancelQueue(baseDir, queueName string, wait bool) (string, error) {
	return cancelQueueJobs(baseDir, queueName, nil, wait)
}

func controlQueueJobs(baseDir, queueName string, jobIDs []string, operation string) (string, error) {
	if operation != "suspend" && operation != "resume" {
		return "", fmt.Errorf("unsupported job operation: %s", operation)
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project %q is not running", queueName)
		}
		return "", fmt.Errorf("invalid running lock: %w", err)
	}
	if !validWebID(lock.RunID) {
		return "", fmt.Errorf("invalid run ID %q", lock.RunID)
	}
	runDir, err := state.SafeJoin(paths.RunsDir, lock.RunID)
	if err != nil {
		return "", err
	}
	jobIDs, err = normalizeRunningAttemptIDs(runDir, lock.RunID, jobIDs)
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
		if jobFinished(jobDir) {
			if allJobs {
				continue
			}
			return "", fmt.Errorf("job %q is not running", jobID)
		}
		jobExecutor, err := jobOwnerExecutor(jobDir)
		if err != nil {
			if allJobs {
				continue
			}
			return "", fmt.Errorf("job %q is not running", jobID)
		}
		if host, mismatch := localExecutorHostMismatch(jobExecutor, runDir); mismatch {
			return "", fmt.Errorf("job %q runs on host %q; run %s from that host", jobID, host, operation)
		}
		suspender, ok := jobExecutor.(Suspender)
		if !ok {
			return "", fmt.Errorf("executor %q does not support %s", jobExecutor.Name(), operation)
		}
		if operation == "resume" {
			err = suspender.Resume(jobDir)
		} else {
			err = suspender.Suspend(jobDir)
		}
		if err != nil {
			return "", fmt.Errorf("%s job %s: %w", operation, jobID, err)
		}
		if operation == "resume" {
			executor.WriteSchedulerStatus(jsonStore(), jobDir, "running", time.Now())
		} else {
			executor.WriteSchedulerStatus(jsonStore(), jobDir, "suspended", time.Now())
		}
		controlled++
	}
	if controlled == 0 {
		return "", fmt.Errorf("no running jobs found in queue %q", queueName)
	}
	return fmt.Sprintf("%s requested\n  Project: %s\n  Run: %s\n  Jobs: %d", strings.Title(operation), queueName, lock.RunID, controlled), nil
}

func jobFinished(jobDir string) bool {
	if path, err := state.ValidatedStateFile(jobDir, stateFileFinishedAt); err == nil {
		// codeql[go/path-injection]: path is returned by ValidatedStateFile for a fixed state file.
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	// NOSONAR: jobDir is produced by validated run/job path helpers before reading the status file.
	path, err := state.ValidatedStateFile(jobDir, stateFileStatusJSON)
	if err != nil {
		return false
	}
	// NOSONAR: jobDir is produced by validated job and run path helpers.
	var status slurmStatus
	if err := jsonStore().ReadJSON(path, &status); err != nil {
		return false
	}
	return status.Phase != "" && status.Phase != "running"
}

func normalizeRunningAttemptIDs(runDir, runID string, jobIDs []string) ([]string, error) {
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
				return nil, fmt.Errorf("attempt %q is not the latest attempt for job %q\nLatest attempt: %q (%s)", jobID, payload.JobID, latestAttemptID, attemptState(latestDir))
			}
			return nil, fmt.Errorf("attempt %q is not the latest attempt for job %q", jobID, payload.JobID)
		}
		if jobFinished(attemptDir) {
			return nil, fmt.Errorf("attempt %q is finished", jobID)
		}
		if _, err := jobOwnerExecutor(attemptDir); err != nil {
			return nil, fmt.Errorf("attempt %q is pending", jobID)
		}
		normalized = append(normalized, payload.JobID)
	}
	return normalized, nil
}

func attemptState(jobDir string) string {
	if state := strings.ToLower(strings.TrimSpace(executor.LoadSchedulerStatus(jsonStore(), jobDir))); state != "" {
		return state
	}
	if path, err := state.ValidatedStateFile(jobDir, stateFileStatusJSON); err == nil { // NOSONAR: jobDir is validated run/job path
		var status slurmStatus
		if err := jsonStore().ReadJSON(path, &status); err == nil && status.Phase != "" {
			return strings.ToLower(status.Phase)
		}
	}
	if jobFinished(jobDir) {
		return "finished"
	}
	if _, err := jobOwnerExecutor(jobDir); err == nil {
		return "running"
	}
	return "pending"
}

func cancelQueueJobs(baseDir, queueName string, jobIDs []string, wait bool) (string, error) {
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project %q is not running", queueName)
		}
		return "", err
	}
	if !validWebID(lock.RunID) {
		return "", fmt.Errorf("invalid run ID %q", lock.RunID)
	}
	runDir, err := state.SafeJoin(paths.RunsDir, lock.RunID)
	if err != nil {
		return "", err
	}
	jobIDs, err = normalizeRunningAttemptIDs(runDir, lock.RunID, jobIDs)
	if err != nil {
		return "", err
	}
	if len(jobIDs) > 0 {
		return cancelJobs(runDir, queueName, lock.RunID, jobIDs)
	}
	if err := markQueueCancelling(paths); err != nil {
		return "", err
	}
	if lock.PID == os.Getpid() {
		// Cancel every still-running job directly through its owning
		// executor (job.json for schedulers, pid file for local), instead of
		// relying on an aggregate metadata file that schedulers only write
		// once the whole run finishes -- otherwise a cancel issued mid-run
		// never reaches an already-submitted Slurm/PBS/LSF job.
		commandSnapshot, err := loadCommandSnapshot(runDir)
		if err != nil {
			return "", err
		}
		targets := make([]string, 0, len(commandSnapshot.Commands))
		for _, job := range model.QueueToJobs(commandSnapshot.Commands) {
			jobDir, err := state.LatestAttemptJobDir(runDir, job.ID)
			if err != nil {
				return "", err
			}
			if jobFinished(jobDir) {
				continue
			}
			targets = append(targets, job.ID)
		}
		message, err := cancelJobs(runDir, queueName, lock.RunID, targets)
		if err != nil {
			return "", err
		}
		return finishCancelMessage(message, paths, queueName, lock.RunID, wait)
	}
	if host, mismatch := runningWorkerHostMismatch(lock); mismatch {
		return "", fmt.Errorf("run %q is owned by host %q; run cancel from that host", lock.RunID, host)
	}
	if err := syscall.Kill(-lock.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return "", fmt.Errorf("cancel local worker: %w", err)
	}
	return finishCancelMessage(fmt.Sprintf("Cancel requested\n  Project: %s\n  Run: %s\n  Worker PID: %d", queueName, lock.RunID, lock.PID), paths, queueName, lock.RunID, wait)
}

func cancelJobs(runDir, queueName, runID string, jobIDs []string) (string, error) {
	requested := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		requested[jobID] = true
	}
	commandSnapshot, err := loadCommandSnapshot(runDir)
	if err != nil {
		return "", err
	}
	knownJobs := make(map[string]JobSpec)
	for _, job := range model.QueueToJobs(commandSnapshot.Commands) {
		knownJobs[job.ID] = job
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
		if executor, err := jobOwnerExecutor(jobDir); err == nil {
			if host, mismatch := localExecutorHostMismatch(executor, runDir); mismatch {
				return "", fmt.Errorf("job %q runs on host %q; run cancel from that host", jobID, host)
			}
			canceller, ok := executor.(Canceller)
			if !ok {
				return "", fmt.Errorf("executor %q does not support cancel", executor.Name())
			}
			if err := canceller.Cancel(jobDir); err != nil {
				return "", fmt.Errorf("cancel job %s: %w", jobID, err)
			}
			cancelled++
			continue
		}
		// Job has no executor yet (Submit was never called): record the
		// cancellation so the scheduler skips it once it would be submitted.
		if _, ok := knownJobs[jobID]; !ok {
			return "", fmt.Errorf("job %q is not found", jobID)
		}
		// codeql[go/path-injection]: jobDir comes from validatedJobDir.
		if err := os.MkdirAll(jobDir, stateDirMode()); err != nil { // NOSONAR: jobDir comes from validatedJobDir.
			return "", fmt.Errorf("prepare cancellation for job %s: %w", jobID, err)
		}
		// codeql[go/path-injection]: jobDir is validated and cancelled is a fixed file name.
		if err := os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte(nowRFC3339()+"\n"), stateFileMode()); err != nil { // NOSONAR: jobDir comes from validatedJobDir.
			return "", fmt.Errorf("record cancellation for job %s: %w", jobID, err)
		}
		cancelled++
	}
	return fmt.Sprintf("Cancel requested\n  Project: %s\n  Run: %s\n  Jobs: %d", queueName, runID, cancelled), nil
}

func loadCommandSnapshot(runDir string) (Queue, error) {
	var snapshot Queue
	// codeql[go/path-injection]: runDir is produced by validatedRunDir and commands.json is fixed.
	if data, err := os.ReadFile(filepath.Join(runDir, "commands.json")); err == nil { // NOSONAR: runDir is produced by validatedRunDir.
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return Queue{}, fmt.Errorf("invalid command snapshot: %w", err)
		}
	}
	return snapshot, nil
}

func markQueueCancelling(paths pathSet) error {
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.Phase = "cancelling"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return fmt.Errorf("failed to mark queue as cancelling: %w", err)
	}
	return nil
}

func finishCancelMessage(message string, paths pathSet, queueName, runID string, wait bool) (string, error) {
	if wait {
		deadline := time.Now().Add(5 * time.Minute)
		for {
			running, err := isRunning(paths.LockFile)
			if err != nil {
				return "", err
			}
			if !running {
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

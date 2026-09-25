package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type projectRunState int

const (
	projectIdle projectRunState = iota
	projectRunning
	projectInterrupted
)

// inspectProjectRunState classifies a project as idle, running, or interrupted
// from its lock and meta files without mutating state.
func inspectProjectRunState(paths state.ProjectPaths) (projectRunState, string, error) {
	inspection, err := inspectProjectState(paths, true)
	return inspection.State, inspection.RunID, err
}

type projectStateInspection struct {
	State     projectRunState
	RunID     string
	Lock      state.LockState
	LockRunID string
}

func inspectProjectState(paths state.ProjectPaths, cleanupStale bool) (projectStateInspection, error) {
	lockState, lock, err := state.InspectLock(paths.LockFile, cleanupStale)
	if err != nil {
		return projectStateInspection{}, err
	}
	if lockState == state.LockActive || lockState == state.LockRemote {
		return projectStateInspection{State: projectRunning, RunID: lock.RunID, Lock: lockState, LockRunID: lock.RunID}, nil
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return projectStateInspection{}, err
	}
	switch meta.Phase {
	case "collecting", "running", "cancelling", "finished":
	default:
		return projectStateInspection{}, fmt.Errorf("unknown project phase %q", meta.Phase)
	}
	if (meta.Phase == "running" || meta.Phase == "cancelling") && meta.LastRunID != "" {
		return projectStateInspection{State: projectInterrupted, RunID: meta.LastRunID, Lock: lockState, LockRunID: lock.RunID}, nil
	}
	return projectStateInspection{State: projectIdle, Lock: lockState, LockRunID: lock.RunID}, nil
}

func validateProjectStateConsistency(paths state.ProjectPaths, inspection projectStateInspection) error {
	if inspection.Lock != state.LockNone && !state.IsValidPathElement(inspection.LockRunID) {
		return fmt.Errorf("invalid run ID %q in run lock", inspection.LockRunID)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	if inspection.State == projectIdle {
		if meta.Phase == "running" || meta.Phase == "cancelling" {
			return fmt.Errorf("metadata phase is %q but last_run_id is empty", meta.Phase)
		}
		return nil
	}
	if !state.IsValidPathElement(inspection.RunID) {
		return fmt.Errorf("invalid run ID %q", inspection.RunID)
	}
	if meta.LastRunID != inspection.RunID {
		return fmt.Errorf("run lock/state identifies %q but metadata identifies %q", inspection.RunID, meta.LastRunID)
	}
	if meta.Phase != "running" && meta.Phase != "cancelling" && !(inspection.State == projectRunning && meta.Phase == "finished") {
		return fmt.Errorf("run %q is active or interrupted but metadata phase is %q", inspection.RunID, meta.Phase)
	}
	if inspection.Lock == state.LockStale && inspection.LockRunID != "" && inspection.LockRunID != inspection.RunID {
		return fmt.Errorf("run lock identifies %q but metadata identifies %q", inspection.LockRunID, inspection.RunID)
	}
	if _, err := state.ValidateRunDirectory(paths.RunsDir, inspection.RunID, inspection.State == projectInterrupted); err != nil {
		return err
	}
	return nil
}

// writeIdleQueue saves a queue edited while the project is idle and marks the
// project as collecting. The two files cannot be replaced atomically together,
// so the metadata is written first: marking an idle project as collecting is
// harmless on its own, and a failed queue write then leaves the previous queue
// in place. Callers must hold the state lock and have checked that the project
// is idle.
func writeIdleQueue(paths state.ProjectPaths, queue model.Queue) error {
	return writeIdleQueueWith(state.WriteJSON, paths, &queue)
}

// markProjectCollecting updates only the metadata, for idle operations that
// leave the queue file untouched.
func markProjectCollecting(paths state.ProjectPaths) error {
	return writeIdleQueueWith(state.WriteJSON, paths, nil)
}

func writeIdleQueueWith(writeJSON func(string, any) error, paths state.ProjectPaths, queue *model.Queue) error {
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
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

func ensureProjectIdleForPaths(paths state.ProjectPaths, operation string) error {
	inspection, err := inspectConsistentProjectState(paths, true)
	if err != nil {
		return fmt.Errorf("failed to check project state: %w", err)
	}
	state, runID := inspection.State, inspection.RunID
	switch state {
	case projectRunning:
		return fmt.Errorf("project %q is running; %s is not allowed", paths.ProjectName, operation)
	case projectInterrupted:
		detail, stillRunning := interruptedRunStatusDetail(paths, runID)
		message := fmt.Sprintf("project %q has interrupted run %q%s; %s is not allowed\nInspect before deciding: rotari show --basedir %s --project-name %s --run-id %s\n",
			paths.ProjectName, runID, detail, operation, executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
		if stillRunning {
			message += "Do not recover until you have independently confirmed those jobs have actually stopped.\n"
		}
		message += fmt.Sprintf("Recover with: rotari unlock --basedir %s --project-name %s --run-id %s",
			executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
		return errors.New(message)
	default:
		return nil
	}
}

func inspectConsistentProjectRunState(paths state.ProjectPaths, cleanupStale bool) (projectRunState, string, error) {
	inspection, err := inspectConsistentProjectState(paths, cleanupStale)
	if err != nil {
		return projectIdle, "", err
	}
	return inspection.State, inspection.RunID, nil
}

func inspectConsistentProjectState(paths state.ProjectPaths, cleanupStale bool) (projectStateInspection, error) {
	inspection, err := inspectProjectState(paths, false)
	if err != nil {
		return projectStateInspection{}, err
	}
	if err := validateProjectStateConsistency(paths, inspection); err != nil {
		return projectStateInspection{}, err
	}
	if cleanupStale && inspection.Lock == state.LockStale {
		// codeql[go/path-injection]: LockFile is rooted in the resolved project directory.
		if err := os.Remove(paths.LockFile); err != nil && !errors.Is(err, os.ErrNotExist) { // NOSONAR: lockFile is rooted in the resolved project directory.
			return projectStateInspection{}, err
		}
	}
	return inspection, nil
}

// interruptedRunJobStatus summarizes what a run's own job directories report
// (command.json/status/status.json), independent of running.lock/meta.json.
type interruptedRunJobStatus struct {
	Total        int
	StillRunning int
}

// scanInterruptedRunJobStatus counts jobs whose own status/status.json is
// missing or non-terminal as "still running" -- this also covers a job
// directory that was cut off mid-write by the same crash, since there is no
// way to tell that apart from a job that is genuinely still executing, and
// treating "unknown" as "running" is the safer default here.
func scanInterruptedRunJobStatus(runDir string) (interruptedRunJobStatus, error) {
	jobDirs, err := state.ListRunJobDirs(runDir)
	if err != nil {
		return interruptedRunJobStatus{}, err
	}
	var status interruptedRunJobStatus
	for _, jobDir := range jobDirs {
		status.Total++
		if _, ok := jobstatus.ReadStatusFile(jobDir); ok {
			continue
		}
		if slurm, ok := loadWrapperStatus(filepath.Join(jobDir, "status.json")); ok && jobstatus.WrapperTerminal(slurm) {
			continue
		}
		status.StillRunning++
	}
	return status, nil
}

// interruptedRunStatusDetail renders the job-count and phase/timestamp clause
// shared by ensureProjectIdleForPaths and reset's interrupted-run messages.
// The returned string is empty when the run has no recorded job directories
// yet. stillRunning reports whether any job appears non-terminal, so callers
// can add a stronger warning before offering to recover.
func interruptedRunStatusDetail(paths state.ProjectPaths, runID string) (detail string, stillRunning bool) {
	status, err := scanInterruptedRunJobStatus(filepath.Join(paths.RunsDir, runID))
	if err != nil || status.Total == 0 {
		return "", false
	}
	var jobsClause string
	if status.StillRunning > 0 {
		jobsClause = fmt.Sprintf("%d of %d job(s) appear to still be running", status.StillRunning, status.Total)
	} else {
		jobsClause = fmt.Sprintf("all %d job(s) report having finished", status.Total)
	}
	meta, metaErr := state.LoadMeta(paths.MetaFile)
	if metaErr != nil || meta.UpdatedAt == "" {
		return ": " + jobsClause, status.StillRunning > 0
	}
	var phaseClause string
	if meta.Phase == "cancelling" {
		phaseClause = fmt.Sprintf("a cancellation had already been requested for this run, but had not finished, as of its last recorded update at %s", meta.UpdatedAt)
	} else {
		phaseClause = fmt.Sprintf("this run was still executing, with no cancellation requested, as of its last recorded update at %s", meta.UpdatedAt)
	}
	return fmt.Sprintf(": %s (%s)", jobsClause, phaseClause), status.StillRunning > 0
}

func ensureProjectIdle(baseDir, queueName, operation string) error {
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return err
	}
	return ensureProjectIdleForPaths(paths, operation)
}

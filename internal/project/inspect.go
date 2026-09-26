package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// RunState is a project's run state, derived only from running.lock and
// meta.json.
type RunState int

const (
	// Idle means no run is active or interrupted; the queue may be edited.
	Idle RunState = iota
	// Running means a live coordinator holds the run lock.
	Running
	// Interrupted means metadata still records an active run but no live
	// coordinator holds the lock.
	Interrupted
)

// Inspection is what Inspect found.
type Inspection struct {
	State RunState
	// RunID is the active or interrupted run; empty when idle.
	RunID     string
	Lock      state.LockState
	LockRunID string
}

// Inspect classifies a project from its lock and metadata. With cleanupStale,
// a stale local lock is removed while inspecting.
func Inspect(paths state.ProjectPaths, cleanupStale bool) (Inspection, error) {
	lockState, lock, err := state.InspectLock(paths.LockFile, cleanupStale)
	if err != nil {
		return Inspection{}, err
	}
	if lockState == state.LockActive || lockState == state.LockRemote {
		return Inspection{State: Running, RunID: lock.RunID, Lock: lockState, LockRunID: lock.RunID}, nil
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return Inspection{}, err
	}
	switch meta.Phase {
	case "collecting", "running", "cancelling", "finished":
	default:
		return Inspection{}, fmt.Errorf("unknown project phase %q", meta.Phase)
	}
	if (meta.Phase == "running" || meta.Phase == "cancelling") && meta.LastRunID != "" {
		return Inspection{State: Interrupted, RunID: meta.LastRunID, Lock: lockState, LockRunID: lock.RunID}, nil
	}
	return Inspection{State: Idle, Lock: lockState, LockRunID: lock.RunID}, nil
}

// RunActive reports whether runID holds the project's run lock. A stale local
// lock is removed while checking.
func RunActive(paths state.ProjectPaths, runID string) bool {
	lockState, lock, err := state.InspectLock(paths.LockFile, true)
	return err == nil && (lockState == state.LockActive || lockState == state.LockRemote) && lock.RunID == runID
}

// InspectConsistent inspects a project and rejects lock, metadata, and run
// directory combinations that disagree. A stale lock is removed only after
// the check passes.
func InspectConsistent(paths state.ProjectPaths, cleanupStale bool) (Inspection, error) {
	inspection, err := Inspect(paths, false)
	if err != nil {
		return Inspection{}, err
	}
	if err := ValidateConsistency(paths, inspection); err != nil {
		return Inspection{}, err
	}
	if cleanupStale && inspection.Lock == state.LockStale {
		// codeql[go/path-injection]: LockFile is rooted in the resolved project directory.
		if err := os.Remove(paths.LockFile); err != nil && !errors.Is(err, os.ErrNotExist) { // NOSONAR: lockFile is rooted in the resolved project directory.
			return Inspection{}, err
		}
	}
	return inspection, nil
}

// ValidateConsistency checks that the run lock, metadata, and run directory
// describe the same run.
func ValidateConsistency(paths state.ProjectPaths, inspection Inspection) error {
	if inspection.Lock != state.LockNone && !state.IsValidPathElement(inspection.LockRunID) {
		return fmt.Errorf("invalid run ID %q in run lock", inspection.LockRunID)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	if inspection.State == Idle {
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
	if meta.Phase != "running" && meta.Phase != "cancelling" && !(inspection.State == Running && meta.Phase == "finished") {
		return fmt.Errorf("run %q is active or interrupted but metadata phase is %q", inspection.RunID, meta.Phase)
	}
	if inspection.Lock == state.LockStale && inspection.LockRunID != "" && inspection.LockRunID != inspection.RunID {
		return fmt.Errorf("run lock identifies %q but metadata identifies %q", inspection.LockRunID, inspection.RunID)
	}
	if _, err := state.ValidateRunDirectory(paths.RunsDir, inspection.RunID, inspection.State == Interrupted); err != nil {
		return err
	}
	return nil
}

// EnsureIdle rejects operation unless the project is idle. The error for an
// interrupted run tells the user how to inspect and recover it.
func EnsureIdle(paths state.ProjectPaths, operation string) error {
	inspection, err := InspectConsistent(paths, true)
	if err != nil {
		return fmt.Errorf("failed to check project state: %w", err)
	}
	runID := inspection.RunID
	switch inspection.State {
	case Running:
		return fmt.Errorf("project %q is running; %s is not allowed", paths.ProjectName, operation)
	case Interrupted:
		detail, stillRunning := InterruptedRunDetail(paths, runID)
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

// interruptedRunJobs summarizes what a run's own job directories report
// (command.json/status/status.json), independent of running.lock/meta.json.
type interruptedRunJobs struct {
	Total        int
	StillRunning int
}

// scanInterruptedRunJobs counts jobs whose own status/status.json is missing
// or non-terminal as "still running" -- this also covers a job directory that
// was cut off mid-write by the same crash, since there is no way to tell that
// apart from a job that is genuinely still executing, and treating "unknown"
// as "running" is the safer default here.
func scanInterruptedRunJobs(runDir string) (interruptedRunJobs, error) {
	jobDirs, err := state.ListRunJobDirs(runDir)
	if err != nil {
		return interruptedRunJobs{}, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	var jobs interruptedRunJobs
	for _, jobDir := range jobDirs {
		jobs.Total++
		if _, ok := jobstatus.ReadStatusFile(jobDir); ok {
			continue
		}
		if status, ok := executor.LoadWrapperStatus(store, filepath.Join(jobDir, "status.json")); ok && jobstatus.WrapperTerminal(status) {
			continue
		}
		jobs.StillRunning++
	}
	return jobs, nil
}

// InterruptedRunDetail renders the job-count and phase/timestamp clause shared
// by EnsureIdle and reset's interrupted-run messages. The returned string is
// empty when the run has no recorded job directories yet. stillRunning
// reports whether any job appears non-terminal, so callers can add a stronger
// warning before offering to recover.
func InterruptedRunDetail(paths state.ProjectPaths, runID string) (detail string, stillRunning bool) {
	jobs, err := scanInterruptedRunJobs(filepath.Join(paths.RunsDir, runID))
	if err != nil || jobs.Total == 0 {
		return "", false
	}
	var jobsClause string
	if jobs.StillRunning > 0 {
		jobsClause = fmt.Sprintf("%d of %d job(s) appear to still be running", jobs.StillRunning, jobs.Total)
	} else {
		jobsClause = fmt.Sprintf("all %d job(s) report having finished", jobs.Total)
	}
	meta, metaErr := state.LoadMeta(paths.MetaFile)
	if metaErr != nil || meta.UpdatedAt == "" {
		return ": " + jobsClause, jobs.StillRunning > 0
	}
	var phaseClause string
	if meta.Phase == "cancelling" {
		phaseClause = fmt.Sprintf("a cancellation had already been requested for this run, but had not finished, as of its last recorded update at %s", meta.UpdatedAt)
	} else {
		phaseClause = fmt.Sprintf("this run was still executing, with no cancellation requested, as of its last recorded update at %s", meta.UpdatedAt)
	}
	return fmt.Sprintf(": %s (%s)", jobsClause, phaseClause), jobs.StillRunning > 0
}

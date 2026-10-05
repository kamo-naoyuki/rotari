package projectrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Start identifies a run that Begin records.
type Start struct {
	RunID   string
	RunName string
	// CWD is the working directory the run was requested from.
	CWD string
	// ConfigPath is the command config file that the client loaded.
	ConfigPath string
}

// Begin records a new run, moves the queue into it, and marks the project
// running. The caller must hold the project's state lock and have checked
// that the project is idle.
//
// The run context and the queue's command snapshot are written before the
// run lock and metadata, so a project that looks running always has its
// context.json and commands.json. The queue is emptied last, keeping its
// defaults: a crash before that leaves the jobs queued beside an interrupted
// run instead of losing them. Begin takes the run lock for the current
// process; a caller that hands the run to another process rewrites the lock
// with that process's PID.
func (runner Runner) Begin(paths state.ProjectPaths, start Start) error {
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}
	if err := runner.WriteContext(paths, start.RunID, start.CWD, start.ConfigPath); err != nil {
		return fmt.Errorf("failed to save run context: %w", err)
	}
	runDir, err := state.SafeJoin(paths.RunsDir, start.RunID)
	if err != nil {
		return err
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		return fmt.Errorf("failed to save run commands: %w", err)
	}
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: start.RunID, RunName: start.RunName, StartedAt: runner.timestamp()}); err != nil {
		return fmt.Errorf("project %q is already running: %w", paths.ProjectName, err)
	}
	if runner.RegisterRun != nil {
		if err := runner.RegisterRun(paths, start.RunID); err != nil {
			_ = removeLock(paths)
			if runDir, pathErr := state.SafeJoin(paths.RunsDir, start.RunID); pathErr == nil {
				_ = os.RemoveAll(runDir)
			}
			return fmt.Errorf("failed to register run: %w", err)
		}
	}
	previous, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		_ = removeLock(paths)
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	meta := previous
	meta.Phase = "running"
	meta.LastRunID = start.RunID
	meta.UpdatedAt = runner.timestamp()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		_ = removeLock(paths)
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	queue.Commands = nil
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		// The jobs are still queued; undo the start so they are not run twice.
		_ = state.WriteJSON(paths.MetaFile, previous)
		_ = removeLock(paths)
		return fmt.Errorf("failed to take the queue: %w", err)
	}
	return nil
}

// Run executes a run that Begin recorded and then finishes it. It samples
// load while the jobs run.
func (runner Runner) Run(paths state.ProjectPaths, options Options, observer Observer) (int, error) {
	stopSampling := runner.startLoadSampling(paths, options.RunID)
	exitCode, err := runner.Execute(paths, options, observer)
	stopSampling()
	if err != nil {
		runner.errorf("%v", err)
	}
	if err := runner.Finish(paths, options.RunID, exitCode); err != nil {
		return 1, err
	}
	return exitCode, nil
}

// Finish records the run's final context, finalizes the project, and removes
// the run lock. When the project cannot be finalized, the lock is still
// removed so the project reads as interrupted rather than running.
func (runner Runner) Finish(paths state.ProjectPaths, runID string, exitCode int) error {
	err := runner.FinishContext(paths, runID)
	if err == nil {
		err = runner.Finalize(paths, runID, exitCode)
	}
	if removeErr := removeOwnLock(paths, runID); removeErr != nil && err == nil {
		err = removeErr
	}
	return err
}

// Finalize marks the project finished with the run's exit code. It verifies,
// under the state lock, that the run lock still belongs to runID. It leaves
// the queue alone, since Begin already took the run's jobs from it and later
// edits belong to the next run, and it does not remove the run lock.
func (runner Runner) Finalize(paths state.ProjectPaths, runID string, exitCode int) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()

	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		return fmt.Errorf("failed to verify run lock: %w", err)
	}
	if lock.RunID != runID {
		return fmt.Errorf("run lock belongs to %q, not %q", lock.RunID, runID)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	meta, err = state.FinalizeRun(meta, runID, exitCode, runner.now())
	if err != nil {
		return err
	}
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return fmt.Errorf("failed to finalize metadata: %w", err)
	}
	if runner.RunFinished != nil {
		runner.RunFinished(paths, runID, exitCode)
	}
	return nil
}

func removeLock(paths state.ProjectPaths) error {
	if err := os.Remove(paths.LockFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// removeOwnLock removes the run lock unless it belongs to another run.
func removeOwnLock(paths state.ProjectPaths, runID string) error {
	lock, err := state.LoadLock(paths.LockFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil && lock.RunID != runID {
		return nil
	}
	return removeLock(paths)
}

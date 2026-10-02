package projectrun

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Check is what `rotari check` reports about a project.
type Check struct {
	// State is ready, empty, running, locked (running on another host), or
	// interrupted.
	State    string
	Runnable bool
	// RunID is the active or interrupted run, if any.
	RunID string
	// Queued counts the queued jobs when QueuedKnown is set: the queue of an
	// active or interrupted project may be unreadable.
	Queued      int
	QueuedKnown bool
	Lock        string
}

// Check reports whether the project's queued run can start, without changing
// state. A ready queue must pass ValidateQueue; deep, when set, adds checks
// that need the current host, such as finding the jobs' executables.
func (runner Runner) Check(paths state.ProjectPaths, deep func(model.Queue) error) (Check, error) {
	release, err := state.AcquireStateReadLock(paths.StateLockFile)
	if err != nil {
		return Check{}, fmt.Errorf("lock project state: %w", err)
	}
	defer release()

	inspection, err := project.InspectConsistent(paths, false)
	if err != nil {
		return Check{}, fmt.Errorf("inspect project state: %w", err)
	}
	result := Check{Lock: string(inspection.Lock), RunID: inspection.RunID}
	switch inspection.State {
	case project.Running:
		if inspection.Lock == state.LockRemote {
			result.State = "locked"
		} else {
			result.State = "running"
		}
		if queue, err := state.LoadQueue(paths.QueueFile); err == nil {
			result.Queued = len(queue.Commands)
			result.QueuedKnown = true
		}
		return result, nil
	case project.Interrupted:
		result.State = "interrupted"
		if queue, err := state.LoadQueue(paths.QueueFile); err == nil {
			result.Queued = len(queue.Commands)
			result.QueuedKnown = true
		}
		return result, nil
	}

	queue, err := runner.LoadQueue(paths, "", nil, nil)
	if err != nil {
		return Check{}, fmt.Errorf("validate queue: %w", err)
	}
	result.Queued = len(queue.Commands)
	result.QueuedKnown = true
	if result.Queued == 0 {
		result.State = "empty"
		return result, nil
	}
	if deep != nil {
		if err := deep(queue); err != nil {
			return Check{}, fmt.Errorf("validate execution environment: %w", err)
		}
	}
	result.State = "ready"
	result.Runnable = true
	return result, nil
}

package main

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func jsonStore() state.Store {
	return state.NewStore(stateDirMode(), stateFileMode())
}

// slurmStatus is the status.json contents written by every executor's
// wrapper script (not just Slurm's), kept under its historical name since it
// is read from many cmd files (jobs.go, mixed_run.go, show.go, web.go).
type slurmStatus = executor.WrapperStatus

func loadSlurmStatus(path string) (slurmStatus, bool) {
	return executor.LoadWrapperStatus(jsonStore(), path)
}

func loadRunQueue(paths pathSet, requestedExecutor string, executorOptions []string, settings executorRunSettingsMap) (Queue, error) {
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return Queue{}, err
	}
	if err := validateQueueForRun(queue, requestedExecutor, executorOptions, settings); err != nil {
		return Queue{}, err
	}
	return queue, nil
}

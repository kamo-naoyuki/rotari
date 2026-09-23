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

func defaultMeta() Meta {
	return Meta{Phase: "collecting", UpdatedAt: nowRFC3339()}
}

func loadMeta(path string) (Meta, error) {
	meta, err := state.LoadMeta(path)
	if err != nil {
		return Meta{}, err
	}
	if meta.Phase == "" {
		meta = defaultMeta()
	}
	if meta.UpdatedAt == "" {
		meta.UpdatedAt = nowRFC3339()
	}
	return Meta(meta), nil
}

func loadQueue(path string) (Queue, error) {
	queue, err := state.LoadQueue(path)
	if err != nil {
		return Queue{}, err
	}
	return Queue(queue), nil
}

func loadRunQueue(paths pathSet, requestedExecutor string, executorOptions []string, settings executorRunSettingsMap) (Queue, error) {
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		return Queue{}, err
	}
	if err := validateQueueForRun(queue, requestedExecutor, executorOptions, settings); err != nil {
		return Queue{}, err
	}
	return queue, nil
}

func writeJSON(path string, v any) error {
	return state.WriteJSON(path, v)
}

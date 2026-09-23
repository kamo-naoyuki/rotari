package main

import (
	"errors"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func jsonStore() state.Store {
	return state.NewStore(stateDirMode(), stateFileMode())
}

func defaultMeta() Meta {
	return Meta{Phase: "collecting", UpdatedAt: nowRFC3339()}
}

func loadMeta(path string) (Meta, error) {
	var meta Meta
	err := jsonStore().ReadJSON(path, &meta) // NOSONAR: callers pass paths rooted in the resolved state directory.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultMeta(), nil
		}
		return Meta{}, err
	}
	if meta.Phase == "" {
		meta = defaultMeta()
	}
	return meta, nil
}

func loadQueue(path string) (Queue, error) {
	var queue Queue
	err := jsonStore().ReadJSON(path, &queue) // NOSONAR: callers pass queue paths rooted in the resolved state directory.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Queue{}, nil
		}
		return Queue{}, err
	}
	return queue, nil
}

func loadRunQueue(paths pathSet, requestedExecutor string, executorOptions []string, settings executorRunSettingsMap) (Queue, error) {
	queue, err := loadQueue(paths.queueFile)
	if err != nil {
		return Queue{}, err
	}
	if err := validateQueueForRun(queue, requestedExecutor, executorOptions, settings); err != nil {
		return Queue{}, err
	}
	return queue, nil
}

func writeJSON(path string, v any) error {
	return jsonStore().WriteJSON(path, v)
}

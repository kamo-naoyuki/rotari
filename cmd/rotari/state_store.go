package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func defaultMeta() Meta {
	return Meta{Phase: "collecting", UpdatedAt: nowRFC3339()}
}

func loadMeta(path string) (Meta, error) {
	b, err := os.ReadFile(path) // NOSONAR: callers pass paths rooted in the resolved state directory.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultMeta(), nil
		}
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, err
	}
	if m.Phase == "" {
		m = defaultMeta()
	}
	return m, nil
}

func loadQueue(path string) (Queue, error) {
	b, err := os.ReadFile(path) // NOSONAR: callers pass queue paths rooted in the resolved state directory.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Queue{}, nil
		}
		return Queue{}, err
	}
	var q Queue
	if err := json.Unmarshal(b, &q); err != nil {
		return Queue{}, err
	}
	return q, nil
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
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), stateDirMode()); err != nil { // NOSONAR: path is an internal state path.
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".rotari-tmp-") // NOSONAR: path is an internal state path.
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(stateFileMode()); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path) // NOSONAR: path is an internal state path.
}

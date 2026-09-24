package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type LockState string

const (
	LockNone   LockState = "none"
	LockActive LockState = "active"
	LockStale  LockState = "stale"
	LockRemote LockState = "remote"
)

func ProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func LoadLock(path string) (model.LockInfo, error) {
	if err := ValidateStatePath(path); err != nil {
		return model.LockInfo{}, err
	}
	// codeql[go/path-injection]: callers pass the project lock path from the resolved state root.
	data, err := os.ReadFile(path)
	if err != nil {
		return model.LockInfo{}, err
	}
	var lock model.LockInfo
	if err := json.Unmarshal(data, &lock); err != nil {
		return model.LockInfo{}, err
	}
	return lock, nil
}

func InspectLock(path string, cleanupStale bool) (LockState, model.LockInfo, error) {
	lock, err := LoadLock(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LockNone, model.LockInfo{}, nil
		}
		return LockNone, model.LockInfo{}, fmt.Errorf("read run lock: %w", err)
	}

	localHost, err := os.Hostname()
	if err != nil {
		return LockNone, model.LockInfo{}, fmt.Errorf("determine local host: %w", err)
	}
	if lock.Host == "" || lock.Host != localHost {
		return LockRemote, lock, nil
	}
	if lock.PID > 0 && ProcessAlive(lock.PID) {
		return LockActive, lock, nil
	}
	if cleanupStale {
		// codeql[go/path-injection]: path is the resolved project lock file.
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return LockStale, lock, err
		}
	}
	return LockStale, lock, nil
}

package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

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

// stateLockTimeout bounds how long a caller waits for a project state lock.
const stateLockTimeout = 30 * time.Second

// AcquireStateLock takes the exclusive project state lock at lockPath and
// returns its release function.
func AcquireStateLock(lockPath string) (func(), error) {
	// codeql[go/path-injection]: lockPath is resolved from the trusted state root.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, FileMode()) // NOSONAR: lockPath is resolved from the trusted state root.
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(stateLockTimeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("timed out waiting %s for state lock", stateLockTimeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// AcquireStateReadLock takes the shared project state lock at lockPath. A
// missing lock file needs no lock.
func AcquireStateReadLock(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_RDONLY, FileMode()) // NOSONAR: lockPath is resolved from the trusted state root.
	if errors.Is(err, os.ErrNotExist) {
		return func() {}, nil
	}
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(stateLockTimeout)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			f.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("timed out waiting %s for state lock", stateLockTimeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// AcquireRunLock publishes info as the run lock at lockPath. It fails while
// another run holds an active or remote lock, and never replaces an existing
// lock file.
func AcquireRunLock(lockPath string, info model.LockInfo) error {
	if info.Host == "" {
		host, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("determine lock host: %w", err)
		}
		info.Host = host
	}
	lockState, _, err := InspectLock(lockPath, true)
	if err != nil {
		return err
	}
	if lockState == LockActive || lockState == LockRemote {
		return errors.New("active lock exists")
	}

	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(lockPath), ".rotari-lock-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(FileMode()); err != nil {
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
	if err := os.Link(tmpName, lockPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("active lock exists")
		}
		return err
	}
	return nil
}

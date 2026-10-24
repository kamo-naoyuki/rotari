package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
)

type LockState string

const (
	LockNone   LockState = "none"
	LockActive LockState = "active"
	LockStale  LockState = "stale"
	LockRemote LockState = "remote"
)

// StaleLockRemovalFileName is the record, in a run's directory, of the reader
// that removed the run's stale lock.
const StaleLockRemovalFileName = "stale_lock_removed.json"

// ProcessStart is Linux's per-process start-time identity from field 22 of
// /proc/PID/stat, which tells a process from a later one reusing its PID. An
// empty value means the platform cannot verify PID reuse.
func ProcessStart(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return ""
	}
	fields := strings.Fields(string(data)[end+1:])
	if len(fields) <= 19 {
		return ""
	}
	return fields[19]
}

func ProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// PIDNamespace names this process's PID namespace, within which alone a PID
// identifies a process. It is empty when the platform does not expose it.
func PIDNamespace() string {
	link, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return ""
	}
	return link
}

// OtherPIDNamespace reports whether lock records a PID namespace other than
// this process's, where its PID names another process or none.
func OtherPIDNamespace(lock model.LockInfo) bool {
	return lock.PIDNamespace != "" && lock.PIDNamespace != PIDNamespace()
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

// InspectLock classifies the run lock at path without changing it. A lock
// whose coordinator PID cannot be checked here, because it was written on
// another host or in another PID namespace such as a sandbox or container, is
// remote. A stale lock is removed by internal/project, which checks the
// project first.
func InspectLock(path string) (LockState, model.LockInfo, error) {
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
	if OtherPIDNamespace(lock) {
		return LockRemote, lock, nil
	}
	if lock.PID > 0 && ProcessAlive(lock.PID) {
		return LockActive, lock, nil
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

// TryAcquireStateReadLock takes the shared project state lock at lockPath
// without waiting, creating the lock file as AcquireStateLock does. ok is
// false while another holder, possibly in this process, has it exclusively.
func TryAcquireStateReadLock(lockPath string) (release func(), ok bool, err error) {
	// codeql[go/path-injection]: lockPath is resolved from the trusted state root.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, FileMode()) // NOSONAR: lockPath is resolved from the trusted state root.
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, true, nil
}

// AcquireRunLock publishes info as the run lock at lockPath. It fails while
// another run holds an active or remote lock, and never replaces an existing
// lock file. The caller holds the state lock exclusively and has checked the
// project, so a stale lock left at lockPath is removed first.
func AcquireRunLock(lockPath string, info model.LockInfo) error {
	if info.Host == "" {
		host, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("determine lock host: %w", err)
		}
		info.Host = host
	}
	if info.PIDNamespace == "" {
		info.PIDNamespace = PIDNamespace()
	}
	lockState, _, err := InspectLock(lockPath)
	if err != nil {
		return err
	}
	if lockState == LockActive || lockState == LockRemote {
		return errors.New("active lock exists")
	}
	if lockState == LockStale {
		// codeql[go/path-injection]: lockPath is the resolved project lock file.
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
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

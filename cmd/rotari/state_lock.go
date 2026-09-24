package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const stateLockTimeout = 30 * time.Second

func acquireStateLock(lockPath string) (func(), error) {
	// codeql[go/path-injection]: lockPath is resolved from the trusted state root.
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, stateFileMode()) // NOSONAR: lockPath is resolved from the trusted state root.
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

func acquireStateReadLock(lockPath string) (func(), error) {
	f, err := os.OpenFile(lockPath, os.O_RDONLY, stateFileMode()) // NOSONAR: lockPath is resolved from the trusted state root.
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

func acquireLock(lockPath string, info LockInfo) error {
	if info.Host == "" {
		host, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("determine lock host: %w", err)
		}
		info.Host = host
	}
	running, err := isRunning(lockPath)
	if err != nil {
		return err
	}
	if running {
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
	if err := os.Link(tmpName, lockPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("active lock exists")
		}
		return err
	}
	return nil
}

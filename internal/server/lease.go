package server

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// leaseAttempts and leaseRetryDelay bound how long Acquire waits for the
// lease. Running holds a shared lock on it for an instant, which must not
// make a starting supervisor report that another one is running.
const (
	leaseAttempts   = 20
	leaseRetryDelay = 10 * time.Millisecond
)

// Acquire takes the lease of dir, a project directory, and records the
// supervisor PID. The returned function releases them. It fails with
// ErrAlreadyRunning while another supervisor holds the lease.
func Acquire(dir string, fileMode os.FileMode) (func(), error) {
	lease, err := os.OpenFile(LockPath(dir), os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return nil, fmt.Errorf("failed to open server lock: %w", err)
	}
	for attempt := 1; ; attempt++ {
		err = syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || attempt == leaseAttempts {
			_ = lease.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, ErrAlreadyRunning
			}
			return nil, fmt.Errorf("failed to lock server lease: %w", err)
		}
		time.Sleep(leaseRetryDelay)
	}
	if err := os.WriteFile(PIDPath(dir), []byte(strconv.Itoa(os.Getpid())+"\n"), fileMode); err != nil {
		_ = lease.Close()
		return nil, fmt.Errorf("failed to write server pid: %w", err)
	}
	return func() {
		_ = os.Remove(PIDPath(dir))
		_ = lease.Close()
	}, nil
}

// Running reports whether a supervisor holds the lease of dir, a project
// directory, and its recorded PID, which is 0 while it is being written.
// It never creates the lease file.
func Running(dir string) (int, bool) {
	lease, err := os.Open(LockPath(dir))
	if err != nil {
		return 0, false
	}
	defer lease.Close()
	if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
		if err == nil {
			_ = syscall.Flock(int(lease.Fd()), syscall.LOCK_UN)
		}
		return 0, false
	}
	data, err := os.ReadFile(PIDPath(dir))
	if err != nil {
		return 0, true
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, true
	}
	return pid, true
}

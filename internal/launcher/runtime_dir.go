package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// runtimeDirPattern names a manager's private runtime directory in the
// system temporary directory.
const runtimeDirPattern = "rotari-web-launches-"

// runtimeLockName is the file a manager holds an exclusive flock on for its
// whole life. The kernel releases the lock when the process exits, however
// it exits, so an unlocked directory belongs to a manager that is gone.
const runtimeLockName = "owner.lock"

// staleRuntimeAge keeps the sweep away from a directory a starting manager
// has created but not locked yet.
const staleRuntimeAge = time.Minute

// createRuntimeDir creates and locks a private runtime directory, after
// removing those left by managers that died without Close.
func createRuntimeDir(tempDir string) (string, *os.File, error) {
	sweepRuntimeDirs(tempDir, time.Now())
	dir, err := os.MkdirTemp(tempDir, runtimeDirPattern)
	if err != nil {
		return "", nil, fmt.Errorf("create launcher runtime directory: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, runtimeLockName), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err == nil {
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	}
	if err != nil {
		if lock != nil {
			lock.Close()
		}
		os.RemoveAll(dir)
		return "", nil, fmt.Errorf("lock launcher runtime directory: %w", err)
	}
	return dir, lock, nil
}

// sweepRuntimeDirs removes this user's runtime directories that no live
// manager holds. Failures are ignored: the sweep only reclaims space.
func sweepRuntimeDirs(tempDir string, now time.Time) {
	matches, err := filepath.Glob(filepath.Join(tempDir, runtimeDirPattern+"*"))
	if err != nil {
		return
	}
	for _, dir := range matches {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || now.Sub(info.ModTime()) < staleRuntimeAge {
			continue
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Getuid() {
			continue
		}
		if runtimeDirHeld(dir) {
			continue
		}
		_ = os.RemoveAll(dir)
	}
}

// runtimeDirHeld reports whether a live manager holds dir's lock. A
// directory without a lock file was never locked, so no manager holds it.
func runtimeDirHeld(dir string) bool {
	lock, err := os.OpenFile(filepath.Join(dir, runtimeLockName), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return true
	}
	_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return false
}

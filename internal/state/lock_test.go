package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
)

func TestLoadLockPreservesMissingAndInvalidErrors(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "running.lock")
	_, err := LoadLock(missingPath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing lock error = %v, want os.ErrNotExist", err)
	}

	unsafePath := filepath.Join(t.TempDir(), "safe") + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape.lock"
	if _, err := LoadLock(unsafePath); err == nil {
		t.Fatalf("LoadLock accepted traversal path %q", unsafePath)
	}

	invalidPath := filepath.Join(t.TempDir(), "running.lock")
	if err := os.WriteFile(invalidPath, []byte("{invalid}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLock(invalidPath); err == nil {
		t.Fatal("invalid lock JSON returned nil error")
	}
}

func TestLoadLockReadsModelContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "running.lock")
	if err := os.WriteFile(path, []byte(`{"pid":42,"run_id":"run-1","host":"worker-1"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if lock.PID != 42 || lock.RunID != "run-1" || lock.Host != "worker-1" {
		t.Fatalf("lock = %#v, want persisted lock fields", lock)
	}
}

func TestInspectLockClassifiesMissingRemoteAndStaleLocks(t *testing.T) {
	missing, _, err := InspectLock(filepath.Join(t.TempDir(), "running.lock"))
	if err != nil || missing != LockNone {
		t.Fatalf("missing lock = %q, error = %v, want none", missing, err)
	}

	remotePath := filepath.Join(t.TempDir(), "running.lock")
	if err := os.WriteFile(remotePath, []byte(`{"run_id":"remote-run","host":"other-host"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	remote, _, err := InspectLock(remotePath)
	if err != nil || remote != LockRemote {
		t.Fatalf("remote lock = %q, error = %v, want remote", remote, err)
	}

	stalePath := filepath.Join(t.TempDir(), "running.lock")
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte(fmt.Sprintf(`{"run_id":"stale-run","pid":-1,"host":%q}`, host)), 0o600); err != nil {
		t.Fatal(err)
	}
	stale, _, err := InspectLock(stalePath)
	if err != nil || stale != LockStale {
		t.Fatalf("stale lock = %q, error = %v, want stale", stale, err)
	}
	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("stale lock stat error = %v, want the lock left for internal/project", err)
	}

	// A sandbox or container on this host cannot see the coordinator's PID.
	otherNamespacePath := filepath.Join(t.TempDir(), "running.lock")
	if err := os.WriteFile(otherNamespacePath, []byte(fmt.Sprintf(`{"run_id":"sandboxed-run","pid":-1,"host":%q,"pid_namespace":"pid:[1]"}`, host)), 0o600); err != nil {
		t.Fatal(err)
	}
	if PIDNamespace() == "" {
		t.Skip("PID namespace is not available")
	}
	otherNamespace, _, err := InspectLock(otherNamespacePath)
	if err != nil || otherNamespace != LockRemote {
		t.Fatalf("lock from another PID namespace = %q, error = %v, want remote", otherNamespace, err)
	}

	sameNamespacePath := filepath.Join(t.TempDir(), "running.lock")
	if err := os.WriteFile(sameNamespacePath, []byte(fmt.Sprintf(`{"run_id":"stale-run","pid":-1,"host":%q,"pid_namespace":%q}`, host, PIDNamespace())), 0o600); err != nil {
		t.Fatal(err)
	}
	if sameNamespace, _, err := InspectLock(sameNamespacePath); err != nil || sameNamespace != LockStale {
		t.Fatalf("dead lock from this PID namespace = %q, error = %v, want stale", sameNamespace, err)
	}
}

func TestAcquireRunLockRecordsPIDNamespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "running.lock")
	if err := AcquireRunLock(path, model.LockInfo{PID: os.Getpid(), RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if lock.PIDNamespace != PIDNamespace() {
		t.Fatalf("lock PID namespace = %q, want %q", lock.PIDNamespace, PIDNamespace())
	}
}

func TestTryAcquireStateReadLockYieldsToExclusiveHolder(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "state.lock")
	release, ok, err := TryAcquireStateReadLock(lockPath)
	if err != nil || !ok {
		t.Fatalf("first shared lock ok = %v, error = %v", ok, err)
	}
	second, ok, err := TryAcquireStateReadLock(lockPath)
	if err != nil || !ok {
		t.Fatalf("second shared lock ok = %v, error = %v, want shared holders together", ok, err)
	}
	second()
	release()

	exclusive, err := AcquireStateLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := TryAcquireStateReadLock(lockPath); err != nil || ok {
		t.Fatalf("shared lock under an exclusive holder ok = %v, error = %v, want not acquired", ok, err)
	}
	exclusive()
}

func TestAcquireRunLockRefusesActiveLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "running.lock")
	if err := AcquireRunLock(lockPath, model.LockInfo{PID: os.Getpid(), RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(lockPath)
	if err != nil || lock.RunID != "run-1" || lock.Host == "" {
		t.Fatalf("lock = %#v, %v, want run-1 with host", lock, err)
	}
	if err := AcquireRunLock(lockPath, model.LockInfo{PID: os.Getpid(), RunID: "run-2"}); err == nil || err.Error() != "active lock exists" {
		t.Fatalf("second lock error = %v", err)
	}
}

func TestAcquireStateLockExcludesOtherHolders(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "state.lock")
	release, err := AcquireStateLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func(), 1)
	go func() {
		next, err := AcquireStateLock(lockPath)
		if err == nil {
			acquired <- next
		}
	}()
	select {
	case <-acquired:
		t.Fatal("second holder acquired a held lock")
	case <-time.After(200 * time.Millisecond):
	}
	release()
	select {
	case next := <-acquired:
		next()
	case <-time.After(2 * time.Second):
		t.Fatal("lock was not handed over after release")
	}
}

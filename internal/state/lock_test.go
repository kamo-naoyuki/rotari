package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLockPreservesMissingAndInvalidErrors(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "running.lock")
	_, err := LoadLock(missingPath)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing lock error = %v, want os.ErrNotExist", err)
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
	missing, _, err := InspectLock(filepath.Join(t.TempDir(), "running.lock"), false)
	if err != nil || missing != LockNone {
		t.Fatalf("missing lock = %q, error = %v, want none", missing, err)
	}

	remotePath := filepath.Join(t.TempDir(), "running.lock")
	if err := os.WriteFile(remotePath, []byte(`{"run_id":"remote-run","host":"other-host"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	remote, _, err := InspectLock(remotePath, false)
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
	stale, _, err := InspectLock(stalePath, true)
	if err != nil || stale != LockStale {
		t.Fatalf("stale lock = %q, error = %v, want stale", stale, err)
	}
	if _, err := os.Stat(stalePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale lock stat error = %v, want removed lock", err)
	}
}

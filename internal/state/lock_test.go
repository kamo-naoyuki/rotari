package state

import (
	"errors"
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

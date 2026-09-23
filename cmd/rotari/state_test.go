package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeTestRunStateFiles(t *testing.T, paths pathSet, runID string) {
	t.Helper()
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "context.json"), RunContext{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{}); err != nil {
		t.Fatal(err)
	}
}

func TestStateLoadMetaDefaultsToCollectingAndTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")

	meta, err := state.LoadMeta(path)
	if err != nil {
		t.Fatalf("state.LoadMeta returned error for missing file: %v", err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("state.LoadMeta phase = %q, want %q", meta.Phase, "collecting")
	}
	if meta.UpdatedAt == "" {
		t.Fatal("state.LoadMeta set an empty UpdatedAt for a missing meta file")
	}
}

func TestValidateProjectStateConsistencyRejectsCorruptState(t *testing.T) {
	t.Run("running metadata without run ID", func(t *testing.T) {
		paths, err := resolvePaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.MetaFile, Meta{Phase: "running"}); err != nil {
			t.Fatal(err)
		}
		err = validateProjectStateConsistency(paths, projectStateInspection{State: projectIdle, Lock: projectLockNone})
		if err == nil || !strings.Contains(err.Error(), "last_run_id is empty") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid stale lock run ID", func(t *testing.T) {
		paths, err := resolvePaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		err = validateProjectStateConsistency(paths, projectStateInspection{State: projectIdle, Lock: projectLockStale, LockRunID: "../run"})
		if err == nil || !strings.Contains(err.Error(), "invalid run ID") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid run ID", func(t *testing.T) {
		paths, err := resolvePaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		err = validateProjectStateConsistency(paths, projectStateInspection{State: projectRunning, RunID: "../run", Lock: projectLockActive, LockRunID: "../run"})
		if err == nil || !strings.Contains(err.Error(), "invalid run ID") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("lock metadata mismatch", func(t *testing.T) {
		paths, err := resolvePaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-2"}); err != nil {
			t.Fatal(err)
		}
		err = validateProjectStateConsistency(paths, projectStateInspection{State: projectRunning, RunID: "run-1", Lock: projectLockActive, LockRunID: "run-1"})
		if err == nil || !strings.Contains(err.Error(), `metadata identifies "run-2"`) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing run directory", func(t *testing.T) {
		paths, err := resolvePaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		err = validateProjectStateConsistency(paths, projectStateInspection{State: projectInterrupted, RunID: "run-1", Lock: projectLockNone})
		if err == nil || !strings.Contains(err.Error(), "directory is missing") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing context", func(t *testing.T) {
		paths, err := resolvePaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
			t.Fatal(err)
		}
		err = validateProjectStateConsistency(paths, projectStateInspection{State: projectRunning, RunID: "run-1", Lock: projectLockActive, LockRunID: "run-1"})
		if err == nil || !strings.Contains(err.Error(), "missing context.json") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing interrupted commands", func(t *testing.T) {
		paths, err := resolvePaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		runDir := filepath.Join(paths.RunsDir, "run-1")
		if err := writeJSON(filepath.Join(runDir, "context.json"), RunContext{}); err != nil {
			t.Fatal(err)
		}
		err = validateProjectStateConsistency(paths, projectStateInspection{State: projectInterrupted, RunID: "run-1", Lock: projectLockNone})
		if err == nil || !strings.Contains(err.Error(), "missing commands.json") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestValidateProjectStateConsistencyAllowsActiveRunBeforeCommandSnapshot(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := writeJSON(filepath.Join(runDir, "context.json"), RunContext{}); err != nil {
		t.Fatal(err)
	}
	inspection := projectStateInspection{State: projectRunning, RunID: "run-1", Lock: projectLockActive, LockRunID: "run-1"}
	if err := validateProjectStateConsistency(paths, inspection); err != nil {
		t.Fatalf("active run before commands snapshot was rejected: %v", err)
	}
}

func TestInspectConsistentProjectStateKeepsStaleLockWhenValidationFails(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: -1, RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-2"}); err != nil {
		t.Fatal(err)
	}

	if _, err := inspectConsistentProjectState(paths, true); err == nil {
		t.Fatal("inspectConsistentProjectState accepted mismatched state")
	}
	if _, err := os.Stat(paths.LockFile); err != nil {
		t.Fatalf("stale lock was removed before validation: %v", err)
	}
}

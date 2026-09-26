package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestValidateConsistencyRejectsCorruptState(t *testing.T) {
	t.Run("running metadata without run ID", func(t *testing.T) {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running"}); err != nil {
			t.Fatal(err)
		}
		err = ValidateConsistency(paths, Inspection{State: Idle, Lock: state.LockNone})
		if err == nil || !strings.Contains(err.Error(), "last_run_id is empty") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid stale lock run ID", func(t *testing.T) {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		err = ValidateConsistency(paths, Inspection{State: Idle, Lock: state.LockStale, LockRunID: "../run"})
		if err == nil || !strings.Contains(err.Error(), "invalid run ID") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid run ID", func(t *testing.T) {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		err = ValidateConsistency(paths, Inspection{State: Running, RunID: "../run", Lock: state.LockActive, LockRunID: "../run"})
		if err == nil || !strings.Contains(err.Error(), "invalid run ID") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("lock metadata mismatch", func(t *testing.T) {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-2"}); err != nil {
			t.Fatal(err)
		}
		err = ValidateConsistency(paths, Inspection{State: Running, RunID: "run-1", Lock: state.LockActive, LockRunID: "run-1"})
		if err == nil || !strings.Contains(err.Error(), `metadata identifies "run-2"`) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing run directory", func(t *testing.T) {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		err = ValidateConsistency(paths, Inspection{State: Interrupted, RunID: "run-1", Lock: state.LockNone})
		if err == nil || !strings.Contains(err.Error(), "directory is missing") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing context", func(t *testing.T) {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
			t.Fatal(err)
		}
		err = ValidateConsistency(paths, Inspection{State: Running, RunID: "run-1", Lock: state.LockActive, LockRunID: "run-1"})
		if err == nil || !strings.Contains(err.Error(), "missing context.json") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("missing interrupted commands", func(t *testing.T) {
		paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		runDir := filepath.Join(paths.RunsDir, "run-1")
		if err := state.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{}); err != nil {
			t.Fatal(err)
		}
		err = ValidateConsistency(paths, Inspection{State: Interrupted, RunID: "run-1", Lock: state.LockNone})
		if err == nil || !strings.Contains(err.Error(), "missing commands.json") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestValidateConsistencyAllowsActiveRunBeforeCommandSnapshot(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := state.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{}); err != nil {
		t.Fatal(err)
	}
	inspection := Inspection{State: Running, RunID: "run-1", Lock: state.LockActive, LockRunID: "run-1"}
	if err := ValidateConsistency(paths, inspection); err != nil {
		t.Fatalf("active run before commands snapshot was rejected: %v", err)
	}
}

func TestInspectConsistentKeepsStaleLockWhenValidationFails(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: -1, RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-2"}); err != nil {
		t.Fatal(err)
	}

	if _, err := InspectConsistent(paths, true); err == nil {
		t.Fatal("InspectConsistent accepted mismatched state")
	}
	if _, err := os.Stat(paths.LockFile); err != nil {
		t.Fatalf("stale lock was removed before validation: %v", err)
	}
}

func TestScanInterruptedRunJobsSkipsNonJobEntries(t *testing.T) {
	runDir := t.TempDir()
	// A stray file alongside job directories (e.g. commands.json) must not
	// be counted as a job.
	if err := os.WriteFile(filepath.Join(runDir, "commands.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory without command.json isn't a job directory either.
	if err := os.MkdirAll(filepath.Join(runDir, "not-a-job"), 0o755); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "command.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "status.json"), []byte(`{"phase":"finished","exit_code":0,"finished_at":"2026-09-19T10:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := scanInterruptedRunJobs(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Total != 1 || status.StillRunning != 0 {
		t.Fatalf("status = %+v, want Total=1 StillRunning=0", status)
	}
}

func TestScanInterruptedRunJobsTreatsNonTerminalStatusJSONAsRunning(t *testing.T) {
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "command.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A wrapper-written status.json whose phase is still "running" (no
	// finished_at yet) must count as still running, same as a missing file.
	if err := os.WriteFile(filepath.Join(jobDir, "status.json"), []byte(`{"phase":"running","started_at":"2026-09-19T10:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := scanInterruptedRunJobs(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Total != 1 || status.StillRunning != 1 {
		t.Fatalf("status = %+v, want Total=1 StillRunning=1", status)
	}
}

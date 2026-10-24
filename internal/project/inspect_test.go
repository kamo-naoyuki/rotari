package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
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

	if _, err := InspectConsistent(paths); err == nil {
		t.Fatal("InspectConsistent accepted mismatched state")
	}
	if _, err := os.Stat(paths.LockFile); err != nil {
		t.Fatalf("stale lock was removed before validation: %v", err)
	}
}

func TestReadersRemoveStaleLockOnlyWhenConsistentAndUnlocked(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	readers := map[string]func(state.ProjectPaths) error{
		"Inspect": func(paths state.ProjectPaths) error {
			_, err := Inspect(paths)
			return err
		},
		"InspectRunLock": func(paths state.ProjectPaths) error {
			_, _, err := InspectRunLock(paths)
			return err
		},
		"RunActive": func(paths state.ProjectPaths) error {
			RunActive(paths, "run-1")
			return nil
		},
		"RunPhaseOf": func(paths state.ProjectPaths) error {
			_, err := RunPhaseOf(paths, "run-1")
			return err
		},
	}
	cases := []struct {
		name        string
		lock        model.LockInfo
		metaRunID   string
		holdState   bool
		wantRemoved bool
	}{
		{name: "stale and consistent", lock: model.LockInfo{PID: -1, RunID: "run-1", Host: host}, metaRunID: "run-1", wantRemoved: true},
		{name: "stale but metadata names another run", lock: model.LockInfo{PID: -1, RunID: "run-1", Host: host}, metaRunID: "run-2"},
		{name: "stale while the state lock is held exclusively", lock: model.LockInfo{PID: -1, RunID: "run-1", Host: host}, metaRunID: "run-1", holdState: true},
		{name: "active", lock: model.LockInfo{PID: os.Getpid(), RunID: "run-1", Host: host}, metaRunID: "run-1"},
		{name: "remote", lock: model.LockInfo{PID: -1, RunID: "run-1", Host: "other-host"}, metaRunID: "run-1"},
		{name: "other PID namespace", lock: model.LockInfo{PID: -1, RunID: "run-1", Host: host, PIDNamespace: "pid:[1]"}, metaRunID: "run-1"},
	}
	for readerName, read := range readers {
		for _, test := range cases {
			t.Run(readerName+"/"+test.name, func(t *testing.T) {
				paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
				if err != nil {
					t.Fatal(err)
				}
				if err := state.WriteJSON(paths.LockFile, test.lock); err != nil {
					t.Fatal(err)
				}
				if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: test.metaRunID}); err != nil {
					t.Fatal(err)
				}
				writeTestRunStateFiles(t, paths, test.metaRunID)
				if test.holdState {
					release, err := state.AcquireStateLock(paths.StateLockFile)
					if err != nil {
						t.Fatal(err)
					}
					defer release()
				}
				if err := read(paths); err != nil {
					t.Fatal(err)
				}
				_, statErr := os.Stat(paths.LockFile)
				if removed := os.IsNotExist(statErr); removed != test.wantRemoved {
					t.Fatalf("lock removed = %v (stat error %v), want %v", removed, statErr, test.wantRemoved)
				}
				var record model.StaleLockRemoval
				data, recordErr := os.ReadFile(filepath.Join(paths.RunsDir, test.metaRunID, state.StaleLockRemovalFileName))
				if recordErr == nil {
					recordErr = json.Unmarshal(data, &record)
				}
				if test.wantRemoved {
					if recordErr != nil || record.Lock != test.lock || record.ReaderPID != os.Getpid() || record.ReaderHost != host || record.ReaderPIDNamespace != state.PIDNamespace() || record.RemovedAt == "" {
						t.Fatalf("removal record = %#v, %v, want the lock and this reader", record, recordErr)
					}
				} else if !os.IsNotExist(recordErr) {
					t.Fatalf("removal record without removal: %#v, %v", record, recordErr)
				}
				meta, err := state.LoadMeta(paths.MetaFile)
				if err != nil || meta.Phase != "running" || meta.LastRunID != test.metaRunID {
					t.Fatalf("metadata = %#v, %v, want it unchanged", meta, err)
				}
			})
		}
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

// TestScanInterruptedRunJobsReadsLatestAttempts scans a run whose jobs keep
// their files per attempt, as runs do: a job whose latest attempt finished
// and one whose latest attempt has not.
func TestScanInterruptedRunJobsReadsLatestAttempts(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "20260101-000000-aaaaaaaa")
	finished := filepath.Join(runDir, "done", "attempts", "att_20260101-000000-aaaaaaaa-done-0")
	running := filepath.Join(runDir, "busy", "attempts", "att_20260101-000000-aaaaaaaa-busy-0")
	for _, dir := range []string{finished, running} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "command.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(finished, "status.json"), []byte(`{"phase":"finished","exit_code":0,"finished_at":"2026-09-19T10:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := scanInterruptedRunJobs(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if status.Total != 2 || status.StillRunning != 1 {
		t.Fatalf("status = %+v, want Total=2 StillRunning=1", status)
	}
}

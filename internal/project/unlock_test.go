package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestUnlockCoordinatorPolicy(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	identity := model.RunCoordinator{Host: host, PID: os.Getpid(), PIDNamespace: state.PIDNamespace(), ProcessStart: state.ProcessStart(os.Getpid())}
	for _, test := range []struct {
		name     string
		identity *model.RunCoordinator
		state    CoordinatorState
	}{
		{"legacy", nil, CoordinatorUnknown},
		{"live", &identity, CoordinatorLive},
		{"live with management marker", &model.RunCoordinator{Host: host, PID: identity.PID, PIDNamespace: identity.PIDNamespace, ProcessStart: identity.ProcessStart, ManagementFinishedAt: "2026-10-20T00:00:00Z"}, CoordinatorLive},
		{"dead", &model.RunCoordinator{Host: host, PID: 1 << 30, PIDNamespace: identity.PIDNamespace, ProcessStart: "1"}, CoordinatorStopped},
		{"reused PID", &model.RunCoordinator{Host: host, PID: identity.PID, PIDNamespace: identity.PIDNamespace, ProcessStart: identity.ProcessStart + "-old"}, CoordinatorStopped},
		{"missing start", &model.RunCoordinator{Host: host, PID: identity.PID, PIDNamespace: identity.PIDNamespace}, CoordinatorUnknown},
		{"remote host", &model.RunCoordinator{Host: host + "-remote", PID: identity.PID, PIDNamespace: identity.PIDNamespace, ProcessStart: identity.ProcessStart}, CoordinatorUnknown},
		{"remote namespace", &model.RunCoordinator{Host: host, PID: identity.PID, PIDNamespace: identity.PIDNamespace + "-remote", ProcessStart: identity.ProcessStart}, CoordinatorUnknown},
	} {
		for _, locked := range []bool{false, true} {
			for _, attest := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/lock=%t/attest=%t", test.name, locked, attest), func(t *testing.T) {
					paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
					if err != nil {
						t.Fatal(err)
					}
					if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
						t.Fatal(err)
					}
					runDir := filepath.Join(paths.RunsDir, "run-1")
					store := state.NewStore(state.DirectoryMode(), state.FileMode())
					context := model.RunContext{Coordinator: test.identity}
					if err := state.SaveContext(store, runDir, context); err != nil {
						t.Fatal(err)
					}
					if locked {
						if err := state.WriteJSON(paths.LockFile, model.LockInfo{Host: host, PID: os.Getpid(), PIDNamespace: identity.PIDNamespace, RunID: "run-1"}); err != nil {
							t.Fatal(err)
						}
					}
					before := unlockFiles(t, paths)
					preview, err := Unlock(paths, "", Guard{DryRun: true}, attest)
					allowed := test.state == CoordinatorStopped || (test.state == CoordinatorUnknown && attest)
					if err != nil || preview.Allowed != allowed || preview.Coordinator.State != test.state || preview.Applied || preview.ExecutionChecked {
						t.Fatalf("preview = %+v, %v; want allowed=%t state=%s", preview, err, allowed, test.state)
					}
					if !reflect.DeepEqual(before, unlockFiles(t, paths)) {
						t.Fatal("preview mutated state")
					}
					result, err := Unlock(paths, "", Guard{IfRevision: preview.Revision}, attest)
					if !allowed {
						var refusal *UnlockRefusedError
						if !errors.As(err, &refusal) || result.RefusalCode == "" || result.Applied {
							t.Fatalf("apply = %+v, %v", result, err)
						}
						if !reflect.DeepEqual(before, unlockFiles(t, paths)) {
							t.Fatal("refusal mutated state")
						}
						return
					}
					if err != nil || !result.Applied {
						t.Fatalf("apply = %+v, %v", result, err)
					}
					saved, err := state.LoadContext(store, runDir)
					if err != nil || saved.Recovery == nil {
						t.Fatalf("recovery context = %+v, %v", saved, err)
					}
					if saved.Recovery.AssumeStopped != attest || saved.Recovery.CoordinatorState != string(test.state) || saved.Recovery.At == "" || !reflect.DeepEqual(saved.Coordinator, test.identity) {
						t.Fatalf("recovery evidence = %+v", saved)
					}
				})
			}
		}
	}
}

func unlockFiles(t *testing.T, paths state.ProjectPaths) map[string]string {
	t.Helper()
	files := make(map[string]string)
	if err := filepath.WalkDir(paths.ProjectDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path == paths.StateLockFile {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestUnlockReassessesCoordinatorAtApply(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	preview, err := Unlock(paths, "", Guard{DryRun: true}, true)
	if err != nil || !preview.Allowed {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	if err := state.SaveContext(store, runDir, model.RunContext{Coordinator: &model.RunCoordinator{
		Host: host, PID: os.Getpid(), PIDNamespace: state.PIDNamespace(), ProcessStart: state.ProcessStart(os.Getpid()),
	}}); err != nil {
		t.Fatal(err)
	}
	result, err := Unlock(paths, "", Guard{IfRevision: preview.Revision}, true)
	if !errors.Is(err, ErrRunAlive) || result.Allowed {
		t.Fatalf("apply = %+v, %v", result, err)
	}
}

func TestUnlockRejectsMalformedContext(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := state.ContextPath(runDir)
	data := []byte("{broken")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dryRun := range []bool{false, true} {
		if _, err := Unlock(paths, "", Guard{DryRun: dryRun}, true); err == nil {
			t.Fatal("accepted malformed context")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatalf("context changed: %q, %v", after, err)
	}
}

// TestUnlockByProjectState checks recovery, retained queues, and live refusal.
func TestUnlockByProjectState(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		phase   string
		lockPID int
		runID   string
		want    string
		wantErr error
		errText string
	}{
		{name: "idle", phase: "finished", want: ""},
		{name: "interrupted without lock", phase: "running", want: "run-1"},
		{name: "interrupted with stale lock", phase: "running", lockPID: -1, want: "run-1"},
		{name: "cancelling", phase: "cancelling", runID: "run-1", want: "run-1"},
		{name: "live run", phase: "running", lockPID: os.Getpid(), wantErr: ErrRunAlive},
		{name: "other run", phase: "running", runID: "run-0", wantErr: ErrNoInterruptedRun},
		{name: "other lock", phase: "running", lockPID: -1, runID: "run-0", errText: `run lock belongs to "run-1", not "run-0"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
			if err != nil {
				t.Fatal(err)
			}
			queue := model.Queue{Commands: []model.QueuedCommand{{ID: "next", Command: []string{"true"}}}}
			if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
				t.Fatal(err)
			}
			if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: test.phase, LastRunID: "run-1"}); err != nil {
				t.Fatal(err)
			}
			writeTestRunStateFiles(t, paths, "run-1")
			if test.wantErr == ErrRunAlive {
				store := state.NewStore(state.DirectoryMode(), state.FileMode())
				if err := state.SaveContext(store, paths.RunsDir+"/run-1", model.RunContext{Coordinator: &model.RunCoordinator{
					Host: host, PID: os.Getpid(), PIDNamespace: state.PIDNamespace(), ProcessStart: state.ProcessStart(os.Getpid()),
				}}); err != nil {
					t.Fatal(err)
				}
			}
			if test.lockPID != 0 {
				if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: test.lockPID, RunID: "run-1", Host: host}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := Revision(paths)
			if err != nil {
				t.Fatal(err)
			}

			preview, err := Unlock(paths, test.runID, Guard{DryRun: true}, true)
			if test.wantErr == ErrRunAlive {
				if err != nil || preview.Allowed || preview.RefusalCode != "coordinator_live" {
					t.Fatalf("live preview = %+v, %v", preview, err)
				}
				if _, err := Unlock(paths, test.runID, Guard{}, true); !errors.Is(err, ErrRunAlive) {
					t.Fatalf("live apply = %v", err)
				}
				return
			}
			if test.wantErr != nil || test.errText != "" {
				if err == nil || (test.wantErr != nil && !errors.Is(err, test.wantErr)) || (test.errText != "" && err.Error() != test.errText) {
					t.Fatalf("dry run error = %v, want %v %q", err, test.wantErr, test.errText)
				}
				return
			}
			if err != nil || preview.RunID != test.want {
				t.Fatalf("dry run = %+v, %v; want run %q", preview, err, test.want)
			}
			if after, _ := Revision(paths); after != before {
				t.Fatal("dry run changed the project")
			}

			if _, err := Unlock(paths, test.runID, Guard{IfRevision: "stale"}, true); test.want != "" && !errors.Is(err, ErrRevisionChanged) {
				t.Fatalf("stale revision error = %v", err)
			}
			result, err := Unlock(paths, test.runID, Guard{IfRevision: before}, true)
			if err != nil || result.RunID != test.want {
				t.Fatalf("Unlock = %+v, %v; want run %q", result, err, test.want)
			}
			if inspection, err := Inspect(paths); err != nil || inspection.State != Idle {
				t.Fatalf("after Unlock state = %v, %v; want idle", inspection.State, err)
			}
			if _, err := os.Stat(paths.LockFile); !os.IsNotExist(err) {
				t.Fatalf("run lock remains: %v", err)
			}
			if left := loadQueueForTest(t, paths); len(left.Commands) != 1 || left.Commands[0].ID != "next" {
				t.Fatalf("queue = %#v; want it left alone", left)
			}
		})
	}
}

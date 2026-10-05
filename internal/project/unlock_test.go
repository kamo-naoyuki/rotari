package project

import (
	"errors"
	"os"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// TestUnlockByProjectState unlocks projects with and without an interrupted
// run, as a dry run and applied, and checks that only an interrupted run is
// recovered, that the queue is left alone, and that a live run is refused.
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
			if test.lockPID != 0 {
				if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: test.lockPID, RunID: "run-1", Host: host}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := Revision(paths)
			if err != nil {
				t.Fatal(err)
			}

			preview, err := Unlock(paths, test.runID, Guard{DryRun: true})
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

			if _, err := Unlock(paths, test.runID, Guard{IfRevision: "stale"}); test.want != "" && !errors.Is(err, ErrRevisionChanged) {
				t.Fatalf("stale revision error = %v", err)
			}
			result, err := Unlock(paths, test.runID, Guard{IfRevision: before})
			if err != nil || result.RunID != test.want {
				t.Fatalf("Unlock = %+v, %v; want run %q", result, err, test.want)
			}
			if inspection, err := Inspect(paths, false); err != nil || inspection.State != Idle {
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

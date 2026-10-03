package project

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// TestResetByProjectState resets an idle, an interrupted, and a running
// project, as a dry run and applied, and checks that only a confirmed reset
// recovers an interrupted run and that a running project is refused.
func TestResetByProjectState(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		phase   string
		lockPID int
		recover bool
		want    ResetResult
		wantErr string
	}{
		{name: "idle", phase: "finished", want: ResetResult{Cleared: 2}},
		{name: "interrupted, confirmed", phase: "running", recover: true, want: ResetResult{Cleared: 2, RecoveredRunID: "run-1"}},
		{name: "interrupted, unconfirmed", phase: "running", wantErr: ErrInterruptedRun.Error()},
		{name: "running", phase: "running", lockPID: os.Getpid(), recover: true, wantErr: "is running run"},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
			if err != nil {
				t.Fatal(err)
			}
			queue := model.Queue{Commands: []model.QueuedCommand{{ID: "a", Command: []string{"true"}}, {ID: "b", Command: []string{"true"}}}}
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
			before, _ := Revision(paths)

			preview, err := Reset(paths, test.recover, Guard{DryRun: true})
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("dry run error = %v, want %q", err, test.wantErr)
				}
			} else if err != nil || preview != test.want {
				t.Fatalf("dry run = %+v, %v; want %+v", preview, err, test.want)
			}
			if after, _ := Revision(paths); after != before {
				t.Fatal("the dry run changed the project")
			}

			applied, err := Reset(paths, test.recover, Guard{})
			if test.wantErr != "" {
				if err == nil || (test.name == "interrupted, unconfirmed" && !errors.Is(err, ErrInterruptedRun)) {
					t.Fatalf("reset error = %v, want %q", err, test.wantErr)
				}
				if len(loadQueueForTest(t, paths).Commands) != 2 {
					t.Fatal("a refused reset cleared the queue")
				}
				return
			}
			if err != nil || applied != test.want || len(loadQueueForTest(t, paths).Commands) != 0 {
				t.Fatalf("reset = %+v, %v; queue %+v", applied, err, loadQueueForTest(t, paths))
			}
			if state, _ := Inspect(paths, false); state.State != Idle {
				t.Fatalf("project state after reset = %v", state.State)
			}
		})
	}
}

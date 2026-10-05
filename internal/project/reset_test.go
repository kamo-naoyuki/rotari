package project

import (
	"os"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// TestResetByProjectState clears queued work without changing an active or
// interrupted run, both in preview and when applied.
func TestResetByProjectState(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		phase   string
		lockPID int
		state   RunState
		want    ResetResult
	}{
		{name: "idle", phase: "finished", state: Idle, want: ResetResult{Cleared: 2}},
		{name: "interrupted", phase: "running", state: Interrupted, want: ResetResult{Cleared: 2}},
		{name: "running", phase: "running", lockPID: os.Getpid(), state: Running, want: ResetResult{Cleared: 2}},
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

			preview, err := Reset(paths, Guard{DryRun: true})
			if err != nil || preview != test.want {
				t.Fatalf("dry run = %+v, %v; want %+v", preview, err, test.want)
			}
			if after, _ := Revision(paths); after != before {
				t.Fatal("the dry run changed the project")
			}

			applied, err := Reset(paths, Guard{})
			if err != nil || applied != test.want || len(loadQueueForTest(t, paths).Commands) != 0 {
				t.Fatalf("reset = %+v, %v; queue %+v", applied, err, loadQueueForTest(t, paths))
			}
			if inspection, _ := Inspect(paths, false); inspection.State != test.state {
				t.Fatalf("project state after reset = %v, want %v", inspection.State, test.state)
			}
		})
	}
}

package jobcontrol

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSelectRetryUsesFinalResultsAndArrayMode(t *testing.T) {
	base := t.TempDir()
	paths, err := state.ResolveProjectPaths(base, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "failed", Name: "failed", Command: []string{"false"}},
		{ID: "success", Name: "success", Command: []string{"true"}},
		{ID: "array", Name: "array", Command: []string{"task"}, Array: &model.ArraySpec{First: 1, Last: 3}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: "run-1", Results: []model.JobResult{
		{ID: "failed", ExitCode: 3}, {ID: "success", ExitCode: 0},
		{ID: "array-1", ExitCode: 0}, {ID: "array-2", ExitCode: 4},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: 1, RunID: "run-1", Host: "remote-host"}); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o755, 0o644)
	controller := Controller{Store: store, Executors: executor.NewRegistry(store, nil)}
	selection := RetrySelection{Selection: model.ResultSelection(true, true, false), Filter: jobfilter.Filter{}, PartialArray: true}
	_, partial, err := controller.SelectRetry(base, "demo", "run-1", selection, time.Now())
	if err != nil || !reflect.DeepEqual(partial, []string{"array-2", "array-3", "failed"}) {
		t.Fatalf("partial retry selection = %v, %v", partial, err)
	}
	selection.PartialArray = false
	_, whole, err := controller.SelectRetry(base, "demo", "run-1", selection, time.Now())
	if err != nil || !reflect.DeepEqual(whole, []string{"array-1", "array-2", "array-3", "failed"}) {
		t.Fatalf("whole-array retry selection = %v, %v", whole, err)
	}
	selection = RetrySelection{JobIDs: []string{"success"}, Selection: "job-id", PartialArray: true}
	_, explicit, err := controller.SelectRetry(base, "demo", "run-1", selection, time.Now())
	if err != nil || !reflect.DeepEqual(explicit, []string{"success"}) {
		t.Fatalf("explicit retry selection = %v, %v", explicit, err)
	}
}

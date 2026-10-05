package projectrun

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writePlanQueue(t *testing.T, paths state.ProjectPaths, commands ...model.QueuedCommand) {
	t.Helper()
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: commands}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckReportsEachProjectState(t *testing.T) {
	runner, paths := testRunner(t)

	check, err := runner.Check(paths, nil)
	if err != nil || check.State != "empty" || check.Runnable || !check.QueuedKnown || check.Revision == "" {
		t.Fatalf("Check(empty) = %+v, %v", check, err)
	}

	writePlanQueue(t, paths, model.QueuedCommand{ID: "job-1", Name: "train", Command: []string{"true"}})
	var deepQueue model.Queue
	check, err = runner.Check(paths, func(queue model.Queue) error { deepQueue = queue; return nil })
	if err != nil || check.State != "ready" || !check.Runnable || check.Queued != 1 {
		t.Fatalf("Check(ready) = %+v, %v", check, err)
	}
	if len(deepQueue.Commands) != 1 {
		t.Fatalf("deep check saw %+v", deepQueue)
	}
	if _, err := runner.Check(paths, func(model.Queue) error { return errors.New("python not found") }); err == nil || err.Error() != "validate execution environment: python not found" {
		t.Fatalf("Check(deep failure) error = %v", err)
	}

	writePlanQueue(t, paths, model.QueuedCommand{ID: "job-1", Command: []string{"true"}, Executor: "nonexistent"})
	if _, err := runner.Check(paths, nil); err == nil || !strings.HasPrefix(err.Error(), "validate queue: ") {
		t.Fatalf("Check(invalid queue) error = %v", err)
	}

	// An interrupted run reports its queue even when that queue cannot run.
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := state.SaveContext(runner.Store, runDir, model.RunContext{CWD: "/work"}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{}); err != nil {
		t.Fatal(err)
	}
	check, err = runner.Check(paths, nil)
	if err != nil || check.State != "interrupted" || check.RunID != "run-1" || check.Queued != 1 || check.Runnable {
		t.Fatalf("Check(interrupted) = %+v, %v", check, err)
	}

	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ host, want string }{{host, "running"}, {host + "-other", "locked"}} {
		if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", Host: test.host}); err != nil {
			t.Fatal(err)
		}
		check, err = runner.Check(paths, nil)
		if err != nil || check.State != test.want || check.RunID != "run-1" || check.Queued != 1 {
			t.Fatalf("Check(lock on %s) = %+v, %v; want %s", test.host, check, err, test.want)
		}
	}
}

func TestPreviewRunPlansTheQueueUnderTheRevision(t *testing.T) {
	runner, paths := testRunner(t)
	writePlanQueue(t, paths,
		model.QueuedCommand{ID: "job-1", Name: "train", Stage: "fit", Command: []string{"true"}},
		model.QueuedCommand{ID: "job-2", Name: "eval", Stage: "score", Command: []string{"true"}},
	)

	planned, revision, err := runner.PreviewRun(paths, nil, PlanRequest{}, "")
	if err != nil || revision == "" {
		t.Fatalf("PreviewRun = %+v, %q, %v", planned, revision, err)
	}
	if want := map[string]bool{"job-1": true, "job-2": true}; !reflect.DeepEqual(planned.Plan.Execute, want) {
		t.Fatalf("Execute = %v, want %v", planned.Plan.Execute, want)
	}
	if len(planned.Queue.Commands) != 2 || planned.SourceRunID != "" {
		t.Fatalf("planned = %+v", planned)
	}

	// A replacement queue is planned instead of the project's.
	replacement := model.Queue{Commands: []model.QueuedCommand{{ID: "job-9", Command: []string{"true"}}}}
	planned, _, err = runner.PreviewRun(paths, &replacement, PlanRequest{}, revision)
	if err != nil || !reflect.DeepEqual(planned.Plan.Execute, map[string]bool{"job-9": true}) {
		t.Fatalf("PreviewRun(replacement) = %+v, %v", planned, err)
	}

	if _, _, err := runner.PreviewRun(paths, nil, PlanRequest{}, "stale-revision"); err == nil {
		t.Fatal("PreviewRun accepted a stale revision")
	}
	if _, _, err := runner.PreviewRun(paths, nil, PlanRequest{Scope: model.CommandSelector{Stage: "missing"}}, ""); err == nil {
		t.Fatal("PreviewRun accepted a scope that matches no command")
	}
	if _, _, err := runner.PreviewRun(paths, &model.Queue{}, PlanRequest{}, ""); err == nil || err.Error() != `queue "demo" has no queued commands` {
		t.Fatalf("PreviewRun(empty queue) error = %v", err)
	}
	if _, _, err := runner.PreviewRun(paths, nil, PlanRequest{Executor: "nonexistent"}, ""); err == nil {
		t.Fatal("PreviewRun accepted an unknown executor")
	}

	if err := os.WriteFile(paths.QueueFile, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.PreviewRun(paths, nil, PlanRequest{}, ""); err == nil {
		t.Fatal("PreviewRun accepted a malformed queue")
	}
}

func TestPreviewRunRefusesActiveProject(t *testing.T) {
	runner, paths := testRunner(t)
	writePlanQueue(t, paths, model.QueuedCommand{ID: "job-1", Command: []string{"true"}})
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runner.PreviewRun(paths, nil, PlanRequest{}, ""); err == nil {
		t.Fatal("PreviewRun planned a run of an interrupted project")
	}
}

// TestPlanRunReadsResultsOfARunWithoutSummary checks that a run that ended
// without writing summary.json, such as an interrupted run after unlock, can
// be the reference of a filtered rerun: its jobs' results resolve from their
// attempts, and jobs that never finished stay unfinished.
func TestPlanRunReadsResultsOfARunWithoutSummary(t *testing.T) {
	runner, paths := testRunner(t)
	runner.Executors = executor.NewRegistry(runner.Store, func(string, ...any) {})
	commands := []model.QueuedCommand{
		{ID: "ok", Command: []string{"true"}},
		{ID: "bad", Command: []string{"sh", "-c", "exit 3"}},
	}
	writePlanQueue(t, paths, commands...)
	if err := runner.Begin(paths, Start{RunID: "run-1", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(paths, Options{RunID: "run-1", LocalConcurrency: 1, EnvMode: model.EnvModeNone}, Observer{}); err != nil {
		t.Fatal(err)
	}
	// Leave the run as an interrupted one would: no summary, and a job that
	// never started.
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.Remove(filepath.Join(runDir, "summary.json")); err != nil {
		t.Fatal(err)
	}
	commands = append(commands, model.QueuedCommand{ID: "never", Command: []string{"true"}})
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: commands}); err != nil {
		t.Fatal(err)
	}
	writePlanQueue(t, paths, commands...)

	planned, _, err := runner.PreviewRun(paths, nil, PlanRequest{Selection: model.ResultSelection(true, true, false), SourceRunID: "run-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]bool{"bad": true, "never": true}; !reflect.DeepEqual(planned.Plan.Execute, want) {
		t.Fatalf("Execute = %v, want %v", planned.Plan.Execute, want)
	}
}

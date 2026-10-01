package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestResolveQueueExecutorUsesDefaultExecutor(t *testing.T) {
	baseDir := t.TempDir()
	queueDir := filepath.Join(baseDir, "projects", "default")
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(queueDir, "queue.json"), model.Queue{
		DefaultExecutor: "slurm",
		Commands:        []model.QueuedCommand{{ID: "hello", Command: []string{"echo", "hello"}}},
	}); err != nil {
		t.Fatal(err)
	}

	store := state.NewStore(0o755, 0o644)
	ops := Operations{BaseDir: baseDir, Runner: projectrun.Runner{Store: store, Executors: executor.NewRegistry(store, nil)}}
	got, err := ops.resolveQueueExecutor("default", "")
	if err != nil {
		t.Fatalf("resolveQueueExecutor returned error: %v", err)
	}
	if got != "slurm" {
		t.Fatalf("resolved executor = %q, want slurm", got)
	}

	if _, err := ops.resolveQueueExecutor("default", "invalid"); err == nil {
		t.Fatal("resolveQueueExecutor accepted unsupported executor")
	}
}

func TestFailedJobHintsUsesJobIDWhenAttemptIDIsMissing(t *testing.T) {
	hints := failedJobHints("run-1", []model.JobResult{{
		ID: "job-1", Command: []string{"false"}, ExitCode: 1,
	}})
	if !strings.Contains(hints, "Attempt ID: job-1") {
		t.Fatalf("hints = %q, want fallback attempt ID", hints)
	}
}

func TestPrepareRunResolvesReferenceRunBeforeBegin(t *testing.T) {
	baseDir := t.TempDir()
	store := state.NewStore(0o755, 0o644)
	ops := Operations{BaseDir: baseDir, Runner: projectrun.Runner{Store: store, Executors: executor.NewRegistry(store, nil)}}
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "previous-run"}); err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"previous-run", "chosen-run"} {
		if err := state.WriteJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		name, selection, sourceRunID, want string
	}{
		{name: "selection uses the last run", selection: "failed", want: "previous-run"},
		{name: "explicit source run wins", selection: "failed", sourceRunID: "chosen-run", want: "chosen-run"},
		{name: "no selection needs no reference", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			prepared, err := ops.prepareRun(server.Request{QueueName: "default", LocalConcurrency: 1, Selection: test.selection, SourceRunID: test.sourceRunID})
			if err != nil {
				t.Fatal(err)
			}
			prepared.release()
			if prepared.sourceRunID != test.want {
				t.Fatalf("sourceRunID = %q, want %q", prepared.sourceRunID, test.want)
			}
		})
	}
}

func TestPlanningFailureDoesNotCreateRun(t *testing.T) {
	for _, async := range []bool{false, true} {
		name := "sync"
		if async {
			name = "async"
		}
		t.Run(name, func(t *testing.T) { testPlanningFailureDoesNotCreateRun(t, async) })
	}
}

func testPlanningFailureDoesNotCreateRun(t *testing.T, async bool) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}}}}
	meta := model.Meta{Phase: "finished", LastRunID: "previous-run", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o755, 0o644)
	registered := false
	ops := Operations{
		BaseDir: baseDir,
		Runner: projectrun.Runner{
			Store: store, Executors: executor.NewRegistry(store, nil),
			RegisterRun: func(state.ProjectPaths, string) error { registered = true; return nil },
		},
		NewRunID: func() string { t.Fatal("run ID allocated before planning succeeded"); return "new-run" },
	}
	request := server.Request{QueueName: "default", LocalConcurrency: 1, Selection: "failed"}
	if async {
		_, err = ops.StartRun(request, nil)
	} else {
		_, _, err = ops.Run(request, nil)
	}
	if err == nil || !strings.Contains(err.Error(), `failed to load run summary for origin run "previous-run"`) {
		t.Fatalf("planning error = %v", err)
	}
	if registered {
		t.Fatal("failed run was registered")
	}
	assertPlanningFailureLeavesProjectIdle(t, paths, queue, meta)
}

func assertPlanningFailureLeavesProjectIdle(t *testing.T, paths state.ProjectPaths, queue model.Queue, meta model.Meta) {
	t.Helper()
	if _, err := os.Stat(paths.LockFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run lock after planning error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "new-run")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run directory after planning error: %v", err)
	}
	gotMeta, err := state.LoadMeta(paths.MetaFile)
	if err != nil || !reflect.DeepEqual(gotMeta, meta) {
		t.Fatalf("metadata after planning error = %+v, %v", gotMeta, err)
	}
	gotQueue, err := state.LoadQueue(paths.QueueFile)
	if err != nil || !reflect.DeepEqual(gotQueue.Commands, queue.Commands) {
		t.Fatalf("queue after planning error = %+v, %v", gotQueue, err)
	}
	if err := project.EnsureIdle(paths, "run"); err != nil {
		t.Fatalf("project cannot run again after planning error: %v", err)
	}
}

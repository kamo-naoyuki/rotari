package supervisor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestWaitForWorkerCallsOnDone(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	waitForWorker(command, func() { close(done) })
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("async run completion callback was not called")
	}
}

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

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

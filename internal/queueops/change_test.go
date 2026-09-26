package queueops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestChangeRestoresAndEditsPreviousRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Command: []string{"echo", "prepare"}, Name: "prepare"},
		{ID: "train-id", Command: []string{"echo", "train"}, Name: "train", DependsOn: []string{"prepare"}},
	}}
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	message, err := testEditor().Change(baseDir, "default", "run-1", model.CommandSelector{IDs: []string{"train-id"}}, Mutation{Executor: "slurm", ExecutorOptions: []string{"-p gpu"}, DependsOn: []string{"prepare"}, Command: []string{"./train-v2"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "job=train-id") {
		t.Fatalf("change message = %q, want train-id", message)
	}
	changed, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Commands[1].ID != "train-id" || changed.Commands[1].Executor != "slurm" ||
		changed.Commands[1].Command[0] != "./train-v2" || len(changed.Commands[1].ExecutorOptions) != 1 {
		t.Fatalf("changed queue = %#v", changed)
	}
	original, err := state.LoadQueue(filepath.Join(paths.RunsDir, "run-1", "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	if original.Commands[1].Command[0] != "echo" || original.Commands[1].Executor != "" {
		t.Fatalf("snapshot was modified: %#v", original.Commands[1])
	}
}

func TestRemoveRemovesJobsAndRejectsDependencies(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Command: []string{"echo", "prepare"}, Name: "prepare"},
		{ID: "train-id", Command: []string{"echo", "train"}, Name: "train", DependsOn: []string{"prepare"}},
		{ID: "other-id", Command: []string{"echo", "other"}, Name: "other"},
	}}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, state.DefaultMeta()); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"other-id"}}); err != nil {
		t.Fatal(err)
	}
	remaining, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Commands) != 2 || remaining.Commands[0].ID != "prepare-id" || remaining.Commands[1].ID != "train-id" {
		t.Fatalf("remaining queue = %#v", remaining.Commands)
	}

	if _, err := testEditor().Remove(baseDir, "default", "", model.CommandSelector{Name: "prepare"}); err == nil {
		t.Fatal("removing a job referenced by a dependency succeeded")
	}
	remaining, err = state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Commands) != 2 {
		t.Fatalf("queue changed after rejected removal: %#v", remaining.Commands)
	}
}

func TestRemoveRestoresOnlyAnExplicitRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "one-id", Command: []string{"echo", "one"}, Name: "one"},
		{ID: "two-id", Command: []string{"echo", "two"}, Name: "two"},
	}}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"one-id"}}); err == nil || !strings.Contains(err.Error(), "no queued jobs") {
		t.Fatalf("remove from an empty queue error = %v, want no queued jobs", err)
	}
	if queue, err := state.LoadQueue(paths.QueueFile); err != nil || len(queue.Commands) != 0 {
		t.Fatalf("remove from an empty queue restored %#v (err %v)", queue.Commands, err)
	}

	message, err := testEditor().Remove(baseDir, "default", "latest", model.CommandSelector{IDs: []string{"one-id"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "removed 1 job") {
		t.Fatalf("remove message = %q", message)
	}
	remaining, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Commands) != 1 || remaining.Commands[0].ID != "two-id" {
		t.Fatalf("restored queue = %#v", remaining.Commands)
	}
}

func TestChangeDoesNotRestoreSnapshotIntoEmptyQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{LastRunID: "previous-run"}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "previous-run", "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job-id", Name: "job", Command: []string{"old"}}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{Name: "job"}, Mutation{Command: []string{"new"}}); err == nil {
		t.Fatal("change restored a job from the previous run into an empty queue")
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands = %#v, want empty queue", queue.Commands)
	}
}

func TestChangeRejectsUnknownExecutor(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job-id", Name: "job", Command: []string{"old"}}}}); err != nil {
		t.Fatal(err)
	}

	_, err = testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"job-id"}}, Mutation{Executor: "nosuch"})
	if err == nil || !strings.Contains(err.Error(), "unsupported executor: nosuch") {
		t.Fatalf("change error = %v, want unsupported executor", err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if queue.Commands[0].Executor != "" {
		t.Fatalf("executor = %q, want unchanged", queue.Commands[0].Executor)
	}
}

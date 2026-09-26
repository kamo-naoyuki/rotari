package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCmdChangeUpdatesExecutorEnvironmentAndCommandByJobID(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "old"}, "", nil, nil, "job", nil); err != nil {
		t.Fatal(err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	jobID := queue.Commands[0].ID

	code := cmdChange([]string{
		"--basedir", baseDir, "--project-name", "default", "--job-id", jobID,
		"--executor", "slurm", "--executor-option", "-p short",
		"--env", "TOKEN=secret", "--set-job-name", "renamed",
		"echo", "new",
	})
	if code != 0 {
		t.Fatalf("cmdChange exit code = %d, want 0", code)
	}

	queue, err = loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("queue commands = %#v, want 1 job", queue.Commands)
	}
	changed := queue.Commands[0]
	if changed.Executor != "slurm" ||
		len(changed.ExecutorOptions) != 1 || changed.ExecutorOptions[0] != "-p short" ||
		len(changed.Environment) != 1 || changed.Environment[0] != "TOKEN=secret" ||
		changed.Name != "renamed" || strings.Join(changed.Command, " ") != "echo new" {
		t.Fatalf("changed command = %#v, want updated fields", changed)
	}

	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("meta.Phase = %q, want collecting", meta.Phase)
	}
}

func TestChangeDoesNotRestoreSnapshotIntoEmptyQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: "previous-run"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "previous-run", "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job-id", Name: "job", Command: []string{"old"}}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := changeBatch(baseDir, "default", "", "", "job", "", nil, false, nil, false, "", nil, false, []string{"new"}); err == nil {
		t.Fatal("change restored a job from the previous run into an empty queue")
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands = %#v, want empty queue", queue.Commands)
	}
}

func TestCmdChangeRejectsRunningProject(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "job"}, "", nil, nil, "job", nil); err != nil {
		t.Fatal(err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "active-run")
	defer os.Remove(paths.LockFile)

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--job-name", "job", "--executor", "local"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), `project "default" is running; change is not allowed`) {
		t.Fatalf("cmdChange exit code = %d, stderr = %q", code, output)
	}
}

func TestCmdChangeRejectsAmbiguousSelector(t *testing.T) {
	baseDir := t.TempDir()
	code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--job-id", "a", "--job-name", "b", "--executor", "local"})
	if code != 1 {
		t.Fatalf("cmdChange exit code = %d, want 1 for ambiguous selector", code)
	}
}

func TestCmdChangeRejectsUnknownJobID(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "job"}, "", nil, nil, "job", nil); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--job-id", "missing", "--executor", "local"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "job not found") {
		t.Fatalf("cmdChange exit code = %d, stderr = %q", code, output)
	}
}

func TestCmdChangeRejectsRenameReferencedByDependency(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "a"}, "", nil, nil, "a", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "b"}, "", nil, nil, "b", []string{"a"}); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--job-name", "a", "--set-job-name", "c"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), `job "a" is referenced by dependency; rename is not allowed`) {
		t.Fatalf("cmdChange exit code = %d, stderr = %q", code, output)
	}
}

func TestCmdChangeSetsAndClearsFinishedDependencies(t *testing.T) {
	baseDir := t.TempDir()
	for _, args := range [][]string{
		{"--job-name", "sweep", "--", "echo", "a"},
		{"--job-name", "collect", "--", "echo", "b"},
	} {
		if code := cmdAdd(append([]string{"--basedir", baseDir, "--project-name", "default", "--quiet"}, args...)); code != 0 {
			t.Fatalf("cmdAdd(%v) exit = %d", args, code)
		}
	}
	change := func(args ...string) model.QueuedCommand {
		t.Helper()
		if code := cmdChange(append([]string{"--basedir", baseDir, "--project-name", "default", "--job-name", "collect", "--quiet"}, args...)); code != 0 {
			t.Fatalf("cmdChange(%v) exit = %d", args, code)
		}
		queue, err := loadQueue(filepath.Join(baseDir, "projects", "default", "queue.json"))
		if err != nil {
			t.Fatal(err)
		}
		return queue.Commands[1]
	}
	if got := change("--depends-on-finished", "sweep"); len(got.DependsOnFinished) != 1 || got.DependsOnFinished[0] != "sweep" {
		t.Fatalf("collect after --depends-on-finished = %#v", got)
	}
	if code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--job-name", "sweep", "--set-job-name", "renamed"}); code != 1 {
		t.Fatalf("renaming a finished prerequisite exit = %d, want 1", code)
	}
	if code := cmdRemove([]string{"--basedir", baseDir, "--project-name", "default", "--job-name", "sweep"}); code != 1 {
		t.Fatalf("removing a finished prerequisite exit = %d, want 1", code)
	}
	if got := change("--clear-depends-on-finished"); len(got.DependsOnFinished) != 0 {
		t.Fatalf("collect after --clear-depends-on-finished = %#v", got)
	}
}

func writeChangeTestQueue(t *testing.T, commands []model.QueuedCommand) (string, state.ProjectPaths) {
	t.Helper()
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: commands}); err != nil {
		t.Fatal(err)
	}
	return baseDir, paths
}

func TestCmdChangeStageChangesEveryJobInStage(t *testing.T) {
	baseDir, paths := writeChangeTestQueue(t, []model.QueuedCommand{
		{ID: "a", Command: []string{"a"}, Stage: "sweep"},
		{ID: "b", Command: []string{"b"}, Stage: "sweep"},
		{ID: "c", Name: "evaluate", Command: []string{"c"}, DependsOn: []string{"sweep"}},
	})
	if code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--stage", "sweep", "--retry", "0", "--timeout", "1h", "--quiet"}); code != 0 {
		t.Fatalf("cmdChange exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range queue.Commands[:2] {
		if command.Retry == nil || *command.Retry != 0 || command.Timeout != "1h" {
			t.Fatalf("stage job %q = %#v, want retry 0 and timeout 1h", command.ID, command)
		}
	}
	if evaluate := queue.Commands[2]; evaluate.Retry != nil || evaluate.Timeout != "" {
		t.Fatalf("job outside the stage changed: %#v", evaluate)
	}
}

func TestCmdChangeMatrixKeepsProvenanceWhenAllMembersChange(t *testing.T) {
	baseDir, paths := writeChangeTestQueue(t, testMatrixQueueWithDependent("group"))
	if code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--matrix", "train", "--executor-option=-p gpu", "--quiet"}); code != 0 {
		t.Fatalf("cmdChange exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range queue.Commands[:2] {
		if command.Matrix == nil || strings.Join(command.ExecutorOptions, " ") != "-p gpu" {
			t.Fatalf("matrix member %q = %#v, want new options with provenance", command.ID, command)
		}
	}
	if evaluate := queue.Commands[2]; len(evaluate.ExecutorOptions) != 0 || strings.Join(evaluate.DependsOn, ",") != "train" {
		t.Fatalf("dependent = %#v, want unchanged options and base-name dependency", evaluate)
	}
}

func TestCmdChangeAllChangesEveryJob(t *testing.T) {
	baseDir, paths := writeChangeTestQueue(t, []model.QueuedCommand{
		{ID: "a", Command: []string{"a"}},
		{ID: "b", Command: []string{"b"}, Timeout: "5m"},
	})
	if code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--all", "--clear-timeout", "--env", "X=1", "--quiet"}); code != 0 {
		t.Fatalf("cmdChange exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range queue.Commands {
		if command.Timeout != "" || strings.Join(command.Environment, ",") != "X=1" {
			t.Fatalf("job %q = %#v, want no timeout and X=1", command.ID, command)
		}
	}
}

func TestCmdChangeRejectsInvalidBulkChanges(t *testing.T) {
	commands := []model.QueuedCommand{{ID: "a", Command: []string{"a"}, Stage: "sweep"}}
	for _, test := range []struct {
		name string
		args []string
	}{
		{"command", []string{"--stage", "sweep", "--", "new"}},
		{"rename", []string{"--all", "--set-job-name", "x"}},
		{"two selectors", []string{"--all", "--stage", "sweep", "--retry", "1"}},
		{"unknown stage", []string{"--stage", "other", "--retry", "1"}},
		{"unknown matrix", []string{"--matrix", "sweep", "--retry", "1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseDir, paths := writeChangeTestQueue(t, commands)
			args := append([]string{"--basedir", baseDir, "--project-name", "default"}, test.args...)
			if code := cmdChange(args); code == 0 {
				t.Fatalf("cmdChange(%v) succeeded, want an error", test.args)
			}
			queue, err := loadQueue(paths.QueueFile)
			if err != nil {
				t.Fatal(err)
			}
			if queue.Commands[0].Retry != nil || queue.Commands[0].Name != "" || strings.Join(queue.Commands[0].Command, " ") != "a" {
				t.Fatalf("rejected change modified the queue: %#v", queue.Commands[0])
			}
		})
	}
}

func TestCmdChangeSelectsQueueCommandsAfterArrayJob(t *testing.T) {
	baseDir, paths := writeChangeTestQueue(t, []model.QueuedCommand{
		{ID: "prep", Name: "prep", Command: []string{"prep"}, Array: &model.ArraySpec{First: 1, Last: 3}},
		{ID: "b", Name: "b", Command: []string{"b"}},
		{ID: "c", Name: "c", Command: []string{"c"}},
	})
	for _, name := range []string{"prep", "c"} {
		if code := cmdChange([]string{"--basedir", baseDir, "--project-name", "default", "--job-name", name, "--timeout", "1h", "--quiet"}); code != 0 {
			t.Fatalf("change --job-name %s exit code = %d, want 0", name, code)
		}
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if queue.Commands[0].Timeout != "1h" || queue.Commands[1].Timeout != "" || queue.Commands[2].Timeout != "1h" {
		t.Fatalf("timeouts = %q, %q, %q; want 1h, none, 1h", queue.Commands[0].Timeout, queue.Commands[1].Timeout, queue.Commands[2].Timeout)
	}
	if _, err := changeQueueJobs(baseDir, "default", "", changeSelector{jobID: "prep-2"}, changeMutation{timeout: "2h"}); err == nil || !strings.Contains(err.Error(), "array job prep") {
		t.Fatalf("change of one array task error = %v, want array job hint", err)
	}
}

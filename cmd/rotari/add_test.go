package main

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"io"
	"os"
	"strings"
	"testing"
)

// enqueueCommand appends one command to the project queue, as `add` does.
func enqueueCommand(baseDir, queueName string, command []string, executor string, executorOptions, environment []string, jobName string, dependsOn []string, arrays ...*model.ArraySpec) (string, error) {
	var array *model.ArraySpec
	if len(arrays) > 0 {
		array = arrays[0]
	}
	queued := model.QueuedCommand{Command: command, Executor: executor, ExecutorOptions: executorOptions, Environment: environment, Name: jobName, DependsOn: dependsOn}
	return queueEditor().Add(baseDir, queueName, []model.QueuedCommand{queued}, array)
}

func TestEnqueueCommandRejectsInterruptedRunWithoutChangingQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	original := model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}
	if err := writeJSON(paths.QueueFile, original); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "interrupted-run"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "interrupted-run")

	_, err = enqueueCommand(baseDir, "default", []string{"duplicate"}, "", nil, nil, "", nil)
	if err == nil || !strings.Contains(err.Error(), `project "default" has interrupted run "interrupted-run"; add is not allowed`) {
		t.Fatalf("enqueueCommand error = %v, want interrupted run error", err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "existing" {
		t.Fatalf("queue changed after rejected add: %#v", queue.Commands)
	}
}

func TestCmdAddEnqueuesJob(t *testing.T) {
	baseDir := t.TempDir()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdAdd([]string{
		"--basedir", baseDir, "--project-name", "demo", "--job-name", "job",
		"--stage", "prepare", "--executor", "local", "--env", "TOKEN=secret", "echo", "hello",
	})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "added project=demo") {
		t.Fatalf("cmdAdd exit code = %d, stdout = %q", code, output)
	}

	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "job" || queue.Commands[0].Stage != "prepare" || queue.Commands[0].Executor != "local" ||
		len(queue.Commands[0].Environment) != 1 || queue.Commands[0].Environment[0] != "TOKEN=secret" {
		t.Fatalf("queue commands = %#v, want persisted job with executor and env", queue.Commands)
	}
}

func TestCmdAddRejectsMissingCommand(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo"}); code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1 for missing command", code)
	}
}

func TestCmdAddRejectsInvalidArrayRange(t *testing.T) {
	baseDir := t.TempDir()
	code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--array", "not-a-range", "echo", "hello"})
	if code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1 for invalid array range", code)
	}
}

func TestCmdAddRejectsInvalidEnv(t *testing.T) {
	baseDir := t.TempDir()
	code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--env", "NOVALUE", "echo", "hello"})
	if code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1 for invalid --env", code)
	}
}

func TestCmdAddRejectsDuplicateJobNameWithoutWriting(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "prepare", "echo", "one"}); code != 0 {
		t.Fatalf("first cmdAdd exit code = %d, want 0", code)
	}
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "prepare", "echo", "two"}); code == 0 {
		t.Fatal("cmdAdd accepted a duplicate job name")
	}

	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Command[len(queue.Commands[0].Command)-1] != "one" {
		t.Fatalf("queue commands = %#v, want only the first job (rejected add must not write)", queue.Commands)
	}
}

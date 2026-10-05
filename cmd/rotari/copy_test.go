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

// TestCmdCopyRejectsTheActiveRunOnly checks that copy works while a run is
// active, except from that run, whose results are not final yet.
func TestCmdCopyRejectsTheActiveRunOnly(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "copy-finished-source"
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "active-run")
	if err := writeJSON(filepath.Join(paths.RunsDir, "active-run", "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "running", Command: []string{"running"}}}}); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(paths.LockFile)

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdCopy([]string{"--basedir", baseDir, "--project-name", "default", "--append", "active-run"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), `run "active-run" is still running`) {
		t.Fatalf("cmdCopy of the active run exit code = %d, stderr = %q", code, output)
	}

	if code := cmdCopy([]string{"--basedir", baseDir, "--project-name", "default", "--append", runID}); code != 0 {
		t.Fatalf("cmdCopy of a finished run exit code = %d", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil || len(queue.Commands) != 2 || queue.Commands[1].ID != "source" {
		t.Fatalf("queue = %#v, err = %v", queue, err)
	}
	if meta, err := loadMeta(paths.MetaFile); err != nil || meta.Phase != "running" {
		t.Fatalf("meta = %+v, err = %v; want the run's phase kept", meta, err)
	}
}

func TestConfirmQueueOverwriteSkipsPromptWhenAppendRequested(t *testing.T) {
	baseDir := t.TempDir()
	confirmed, err := confirmQueueOverwrite(baseDir, "default", true, false)
	if err != nil || confirmed {
		t.Fatalf("confirmQueueOverwrite(append) = %v, %v, want false, nil", confirmed, err)
	}
}

func TestConfirmQueueOverwriteSkipsPromptWhenOverwriteRequested(t *testing.T) {
	baseDir := t.TempDir()
	confirmed, err := confirmQueueOverwrite(baseDir, "default", false, true)
	if err != nil || !confirmed {
		t.Fatalf("confirmQueueOverwrite(overwrite) = %v, %v, want true, nil", confirmed, err)
	}
}

func TestConfirmQueueOverwriteAllowsEmptyQueueWithoutPrompt(t *testing.T) {
	baseDir := t.TempDir()
	confirmed, err := confirmQueueOverwrite(baseDir, "default", false, false)
	if err != nil || confirmed {
		t.Fatalf("confirmQueueOverwrite(empty queue) = %v, %v, want false, nil", confirmed, err)
	}
}

func TestConfirmQueueOverwriteRejectsNonEmptyQueueWithoutTerminal(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}

	stdin, err := os.CreateTemp(t.TempDir(), "not-a-tty-stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	oldStdin := os.Stdin
	os.Stdin = stdin
	defer func() { os.Stdin = oldStdin }()

	_, err = confirmQueueOverwrite(baseDir, "default", false, false)
	if err == nil || !strings.Contains(err.Error(), "queue is not empty; use --append or --overwrite") {
		t.Fatalf("confirmQueueOverwrite error = %v, want non-empty queue error", err)
	}
}

func writeCopySourceRun(t *testing.T, paths state.ProjectPaths, runID string, commands []model.QueuedCommand) {
	t.Helper()
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: commands}); err != nil {
		t.Fatal(err)
	}
	results := make([]model.JobResult, 0, len(commands))
	for _, command := range commands {
		results = append(results, model.JobResult{ID: command.ID, ExitCode: 0})
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: results}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
}

func captureCopyStderr(t *testing.T, args []string) (int, string) {
	t.Helper()
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdCopy(args)
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(output)
}

func TestCmdCopyJobNameWithRunIDSelectsJobFromThatRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	olderRunID := "copy-older-run"
	writeCopySourceRun(t, paths, olderRunID, []model.QueuedCommand{{ID: "job-old", Name: "train", Command: []string{"old"}}})
	writeCopySourceRun(t, paths, "copy-latest-run", []model.QueuedCommand{{ID: "job-new", Name: "train", Command: []string{"new"}}})

	if code := cmdCopy([]string{"--basedir", baseDir, "--project-name", "default", "--run-id", olderRunID, "--job-name", "train", "--quiet"}); code != 0 {
		t.Fatalf("cmdCopy --run-id --job-name exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "job-old" || queue.Commands[0].Origin == nil || queue.Commands[0].Origin.RunID != olderRunID {
		t.Fatalf("copied queue = %#v, want job-old from run %q", queue.Commands, olderRunID)
	}
}

func TestCmdCopyJobNameRejectsMissingAndAmbiguousNames(t *testing.T) {
	t.Setenv(envProjectName, "")
	baseDir := t.TempDir()
	projectPaths := make(map[string]state.ProjectPaths)
	for _, projectName := range []string{"first", "second"} {
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		writeCopySourceRun(t, paths, "copy-run-"+projectName, []model.QueuedCommand{{ID: "job-" + projectName, Name: "train", Command: []string{"train"}}})
		projectPaths[projectName] = paths
	}

	tests := map[string]struct {
		args []string
		want string
	}{
		"missing in run":    {[]string{"--project-name", "first", "--run-id", "copy-run-first", "--job-name", "missing"}, `job name "missing" not found in run "copy-run-first"`},
		"missing in latest": {[]string{"--project-name", "first", "--job-name", "missing"}, `job name "missing" not found`},
		"ambiguous":         {[]string{"--job-name", "train"}, `job name "train" matches more than one target; pass --project-name or --run-id`},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			code, output := captureCopyStderr(t, append([]string{"--basedir", baseDir}, test.args...))
			if code != 1 || !strings.Contains(output, test.want) {
				t.Fatalf("cmdCopy exit code = %d, stderr = %q, want %q", code, output, test.want)
			}
			for projectName, paths := range projectPaths {
				if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
					t.Fatalf("rejected copy wrote the %s queue: %v", projectName, err)
				}
			}
		})
	}
}

func TestCmdCopyRejectsInvalidOptionCombinations(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	firstRunID, secondRunID := makeRunID(), makeRunID()
	writeCopySourceRun(t, paths, firstRunID, []model.QueuedCommand{{ID: "job-1", Name: "train", Command: []string{"train"}}})

	tests := map[string]struct {
		args []string
		want string
	}{
		"append and overwrite":   {[]string{"--append", "--overwrite"}, "usage:"},
		"positional and run-id":  {[]string{"--run-id", firstRunID, firstRunID}, "usage:"},
		"two positional run IDs": {[]string{firstRunID, secondRunID}, "usage:"},
		"job-name and job-id":    {[]string{"--job-name", "train", "--job-id", "job-1"}, "--job-name cannot be combined with --job-id"},
		"attempts from two runs": {
			[]string{"--job-id", makeAttemptID(firstRunID, "job-1", 0), "--job-id", makeAttemptID(secondRunID, "job-1", 0)},
			"belongs to run",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			code, output := captureCopyStderr(t, append([]string{"--basedir", baseDir, "--project-name", "default"}, test.args...))
			if code != 1 || !strings.Contains(output, test.want) {
				t.Fatalf("cmdCopy exit code = %d, stderr = %q, want %q", code, output, test.want)
			}
			if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
				t.Fatalf("rejected copy wrote a queue: %v", err)
			}
		})
	}
}

func TestCmdCopyRejectsProjectWithoutPreviousRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{}); err != nil {
		t.Fatal(err)
	}
	code, output := captureCopyStderr(t, []string{"--basedir", baseDir, "--project-name", "default"})
	if code != 1 || !strings.Contains(output, `project "default" has no previous run`) {
		t.Fatalf("cmdCopy exit code = %d, stderr = %q", code, output)
	}
}

func TestConfirmQueueOverwritePromptsOnTerminal(t *testing.T) {
	tests := map[string]struct {
		input   string
		want    bool
		wantErr string
	}{
		"yes":   {"yes\n", true, ""},
		"y":     {"Y\n", true, ""},
		"no":    {"n\n", false, "copy cancelled"},
		"empty": {"\n", false, "copy cancelled"},
		"eof":   {"", false, "EOF"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, err := state.ResolveProjectPaths(baseDir, "default")
			if err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
				t.Fatal(err)
			}
			usePromptStdin(t, test.input)

			confirmed, err := confirmQueueOverwrite(baseDir, "default", false, false)
			if test.wantErr == "" {
				if err != nil || confirmed != test.want {
					t.Fatalf("confirmQueueOverwrite = %v, %v, want %v, nil", confirmed, err, test.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("confirmQueueOverwrite error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

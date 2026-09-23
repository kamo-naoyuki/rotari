package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdCopyRejectsRunningProjectBeforeQueueConfirmation(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "copy-running-source"
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := acquireLock(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "active-run"}); err != nil {
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
	code := cmdCopy([]string{"--basedir", baseDir, "--project-name", "default", runID})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), `project "default" is running; copy is not allowed`) {
		t.Fatalf("cmdCopy exit code = %d, stderr = %q", code, output)
	}
	if strings.Contains(string(output), "queue is not empty") {
		t.Fatalf("cmdCopy checked queue before running state: %q", output)
	}
}

func TestCmdCopyDerivesRunIDFromAttemptID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	attemptID := makeAttemptID(runID, "source", 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{{ID: "source", AttemptID: attemptID, ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}
	attemptDir, err := specificAttemptJobDir(runDir, "source", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if code := cmdCopy([]string{"--basedir", baseDir, "--project-name", "default", "--job-id", attemptID}); code != 0 {
		t.Fatalf("cmdCopy exit code = %d", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Origin == nil || queue.Commands[0].Origin.AttemptID != attemptID {
		t.Fatalf("copied queue = %#v, want attempt %q", queue.Commands, attemptID)
	}
}

func TestCmdCopyJobIDUsesLatestRunWithoutRunID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "job-1", Name: "train", Command: []string{"train"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{RunID: runID, Results: []JobResult{{ID: "job-1", ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unregisterRun(runID) })

	if code := cmdCopy([]string{"--basedir", baseDir, "--project-name", "default", "--job-id", "job-1"}); code != 0 {
		t.Fatalf("cmdCopy exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "job-1" || queue.Commands[0].Origin == nil || queue.Commands[0].Origin.RunID != runID {
		t.Fatalf("copied queue = %#v, want job from latest run %q", queue.Commands, runID)
	}
	if err := writeJSON(paths.QueueFile, Queue{}); err != nil {
		t.Fatal(err)
	}
	if code := cmdCopy([]string{"--basedir", baseDir, "--project-name", "default", "--job-name", "train"}); code != 0 {
		t.Fatalf("cmdCopy --job-name exit code = %d, want 0", code)
	}
	queue, err = loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "job-1" || queue.Commands[0].Origin == nil || queue.Commands[0].Origin.RunID != runID {
		t.Fatalf("job-name copied queue = %#v, want job from latest run %q", queue.Commands, runID)
	}
}

func TestCopyRunToQueuePreservesSourceJobIDs(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := Queue{Commands: []QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"prepare"}},
		{ID: "train-id", Name: "train", Command: []string{"train"}, DependsOn: []string{"prepare"}},
		{ID: "test-id", Name: "test", Command: []string{"test"}, DependsOn: []string{"train"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{
		{ID: "prepare-id", ExitCode: 0},
		{ID: "train-id", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	message, err := copyRunToQueue(baseDir, "default", "run-1", "failed", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if message == "" {
		t.Fatal("copy message is empty")
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("copied commands = %#v, want one command", queue.Commands)
	}
	copied := queue.Commands[0]
	if copied.ID != "train-id" || copied.Name != "train" || len(copied.DependsOn) != 0 {
		t.Fatalf("copied command = %#v, want the source ID preserved and no excluded dependency", copied)
	}
	if copied.Origin == nil || copied.Origin.RunID != "run-1" || copied.Origin.JobID != "train-id" || copied.Origin.Status != "failed" {
		t.Fatalf("copied origin = %#v, want source run/job and failed status", copied.Origin)
	}
}

func TestCopyRunToQueuePreservesCompleteStageDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := Queue{Commands: []QueuedCommand{
		{ID: "prepare-a", Stage: "prepare", Command: []string{"prepare-a"}},
		{ID: "prepare-b", Stage: "prepare", Command: []string{"prepare-b"}},
		{ID: "train", Name: "train", DependsOn: []string{"prepare"}, Command: []string{"train"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := copyRunToQueue(baseDir, "default", "run-1", "all", nil, false); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := queue.Commands[2].DependsOn; len(got) != 1 || got[0] != "prepare" {
		t.Fatalf("copied dependencies = %v, want [prepare]", got)
	}
}

func TestCopyRunToQueueRejectsExcludedFailedDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := Queue{Commands: []QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"prepare"}},
		{ID: "train-id", Name: "train", Command: []string{"train"}, DependsOn: []string{"prepare"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{
		{ID: "prepare-id", ExitCode: 1},
		{ID: "train-id", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	_, err = copyRunToQueue(baseDir, "default", "run-1", "job-id", []string{"train-id"}, false)
	if err == nil || !strings.Contains(err.Error(), `excluded dependency "prepare" did not succeed`) {
		t.Fatalf("copy error = %v, want excluded failed dependency error", err)
	}
}

func TestCopyRunToQueueRejectsExcludedUnfinishedDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := Queue{Commands: []QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"prepare"}},
		{ID: "train-id", Name: "train", Command: []string{"train"}, DependsOn: []string{"prepare"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{{ID: "train-id", ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}

	_, err = copyRunToQueue(baseDir, "default", "run-1", "job-id", []string{"train-id"}, false)
	if err == nil || !strings.Contains(err.Error(), `excluded dependency "prepare" did not succeed`) {
		t.Fatalf("copy error = %v, want excluded unfinished dependency error", err)
	}
}

func TestCopyRunToQueuePreservesExplicitAttemptID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	attemptID := makeAttemptID(runID, "train-id", 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "train-id", Name: "train", Command: []string{"train"}}}}); err != nil {
		t.Fatal(err)
	}
	attemptDir, err := specificAttemptJobDir(runDir, "train-id", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{{ID: "train-id", AttemptID: attemptID, ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := copyRunToQueue(baseDir, "default", runID, "job-id", []string{attemptID}, false); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Origin == nil || queue.Commands[0].Origin.AttemptID != attemptID {
		t.Fatalf("copied command = %#v, want explicit attempt %q", queue.Commands, attemptID)
	}
}

func TestCopyRunToQueueExplicitArrayAttemptSelectsOnlyTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	attemptID := makeAttemptID(runID, "train-2", 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	snapshot := Queue{Commands: []QueuedCommand{{ID: "train", Command: []string{"train"}, Array: &ArraySpec{First: 1, Last: 3}}}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{{ID: "train-1", AttemptID: makeAttemptID(runID, "train-1", 0), ExitCode: 0}, {ID: "train-2", AttemptID: attemptID, ExitCode: 1}, {ID: "train-3", AttemptID: makeAttemptID(runID, "train-3", 0), ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	attemptDir, err := specificAttemptJobDir(runDir, "train-2", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := copyRunToQueue(baseDir, "default", runID, "job-id", []string{attemptID}, false); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Array == nil || len(queue.Commands[0].Array.Tasks) != 1 || queue.Commands[0].Array.Tasks[0] != 2 {
		t.Fatalf("copied array = %#v, want only task 2", queue.Commands)
	}
	if origin := queue.Commands[0].TaskOrigins["train-2"]; origin == nil || origin.AttemptID != attemptID {
		t.Fatalf("task origin = %#v, want attempt %q", origin, attemptID)
	}
}

func TestCopyRunToQueueReassignsIDOnCollision(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "train-id", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := Queue{Commands: []QueuedCommand{
		{ID: "train-id", Name: "train", Command: []string{"train"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}

	if _, err := copyRunToQueue(baseDir, "default", "run-1", "all", nil, true); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 {
		t.Fatalf("commands after append = %#v, want two commands", queue.Commands)
	}
	if queue.Commands[1].ID == "train-id" {
		t.Fatal("colliding job ID was not reassigned")
	}
}

func TestCopyRunToQueueCombinesResultSelections(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := Queue{Commands: []QueuedCommand{
		{ID: "success-id", Name: "success", Command: []string{"success"}},
		{ID: "failed-id", Name: "failed", Command: []string{"failed"}},
		{ID: "unfinished-id", Name: "unfinished", Command: []string{"unfinished"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{
		{ID: "success-id", ExitCode: 0},
		{ID: "failed-id", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	if _, err := copyRunToQueue(baseDir, "default", "run-1", "failed,unfinished", []string{"success-id"}, false); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 3 || queue.Commands[0].Name != "success" || queue.Commands[1].Name != "failed" || queue.Commands[2].Name != "unfinished" {
		t.Fatalf("combined selection = %#v, want selected filters plus job ID", queue.Commands)
	}
}

func TestCopyRunToQueueRequiresAppendForNonEmptyQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := copyRunToQueue(baseDir, "default", "run-1", "all", nil, false); err == nil {
		t.Fatal("copy succeeded without --append")
	}
	if _, err := copyRunToQueue(baseDir, "default", "run-1", "all", nil, true); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 {
		t.Fatalf("commands after append = %#v, want two commands", queue.Commands)
	}
}

func TestCopyRunToQueueCanOverwriteNonEmptyQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "source", Command: []string{"source"}}}}); err != nil {
		t.Fatal(err)
	}

	if _, err := copyRunToQueue(baseDir, "default", "run-1", "all", nil, false, true); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Command[0] != "source" {
		t.Fatalf("commands after overwrite = %#v, want only copied command", queue.Commands)
	}
}
func TestConfirmQueueOverwriteSkipsPromptWhenAppendRequested(t *testing.T) {
	baseDir := t.TempDir()
	confirmed, err := confirmQueueOverwrite(baseDir, "default", true, false)
	if err != nil || confirmed {
		t.Fatalf("confirmQueueOverwrite(append) = %v, %v, want false, nil", confirmed, err)
	}
}

func TestCopyRunToQueueAggregatesArrayTaskResults(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	snapshot := Queue{Commands: []QueuedCommand{
		{ID: "success-array", Name: "success-array", Command: []string{"true"}, Array: &ArraySpec{First: 1, Last: 2}},
		{ID: "failed-array", Name: "failed-array", Command: []string{"false"}, Array: &ArraySpec{First: 1, Last: 2}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{
		{ID: "success-array-1", AttemptID: makeAttemptID("20260922-000000-00000000", "success-array-1", 0), ExitCode: 0},
		{ID: "success-array-2", AttemptID: makeAttemptID("20260922-000000-00000000", "success-array-2", 0), ExitCode: 0},
		{ID: "failed-array-1", AttemptID: makeAttemptID("20260922-000000-00000000", "failed-array-1", 0), ExitCode: 0},
		{ID: "failed-array-2", AttemptID: makeAttemptID("20260922-000000-00000000", "failed-array-2", 0), ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	if _, err := copyRunToQueue(baseDir, "default", "run-1", "failed", nil, false); err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "failed-array" {
		t.Fatalf("--failed selection = %#v, want only failed-array (finished array jobs must not be treated as unfinished)", queue.Commands)
	}
	if queue.Commands[0].Origin == nil || queue.Commands[0].Origin.Status != "failed" {
		t.Fatalf("failed-array origin = %#v, want status=failed", queue.Commands[0].Origin)
	}
	if got := queue.Commands[0].TaskOrigins["failed-array-2"].AttemptID; got == "" {
		t.Fatalf("failed-array task origin has no attempt ID")
	}

	if _, err := copyRunToQueue(baseDir, "default", "run-1", "success", nil, false, true); err != nil {
		t.Fatal(err)
	}
	queue, err = loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "success-array" || queue.Commands[0].Origin.Status != "success" {
		t.Fatalf("--success selection = %#v, want only success-array with status=success", queue.Commands)
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
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "existing", Command: []string{"existing"}}}}); err != nil {
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

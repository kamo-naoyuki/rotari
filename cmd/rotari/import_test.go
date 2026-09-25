package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

func writeWorkflowFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "workflow.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeWorkflowRunFixture(t *testing.T, baseDir string) (state.ProjectPaths, string) {
	t.Helper()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-120000-12345678"
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "success-id", Name: "prepare", Command: []string{"true"}},
		{ID: "failed-id", Name: "train", Command: []string{"false"}, DependsOn: []string{"prepare"}},
	}}
	results := []model.JobResult{
		{ID: "success-id", AttemptID: makeAttemptID(runID, "success-id", 0), ExitCode: 0},
		{ID: "failed-id", AttemptID: makeAttemptID(runID, "failed-id", 0), ExitCode: 1},
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: results}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: runID, Phase: "collecting"}); err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		attemptDir, err := specificAttemptJobDir(runDir, result.ID, result.AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(attemptDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return paths, runID
}

func TestCmdImportWritesFreshQueue(t *testing.T) {
	baseDir := t.TempDir()
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - name: train\n    command: [echo, hello]\n    array: \"1-2\"\n")
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", manifest}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Name != "train" || queue.Commands[0].Array == nil {
		t.Fatalf("queue = %#v", queue)
	}
}

func TestCmdImportAcceptsFileBeforeFlags(t *testing.T) {
	baseDir := t.TempDir()
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [true]\n")
	if code := cmdImport([]string{manifest, "--basedir", baseDir, "--project-name", "demo"}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
}

func TestCmdImportDryRunDoesNotWrite(t *testing.T) {
	baseDir := t.TempDir()
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [true]\n")
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", "--dry-run", manifest}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
		t.Fatalf("queue file exists after dry run: %v", err)
	}
}

func TestCmdImportRejectsInvalidSourceAttemptWithoutWriting(t *testing.T) {
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Jobs[0].AttemptID = "not-an-attempt"
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
	if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
		t.Fatalf("queue file exists after failed import: %v", err)
	}
}

func TestCmdImportRejectsInvalidInstanceAttemptAfterDefinitionChange(t *testing.T) {
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Jobs[1].Command = []string{"changed"}
	manifest.Jobs[1].Array = "1"
	task := 1
	manifest.Jobs[1].Instances = []workflow.Instance{{Task: &task, Status: "failed", AttemptID: "not-an-attempt"}}
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
	if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
		t.Fatalf("queue file exists after failed import: %v", err)
	}
}

func TestCmdImportRejectsUnknownExecutorWithoutWriting(t *testing.T) {
	baseDir := t.TempDir()
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [true]\n    executor: unknown\n")
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", manifest}); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
		t.Fatalf("queue file exists after failed import: %v", err)
	}
}

func TestCmdImportRequiresOverwriteForNonEmptyQueue(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "demo", []string{"existing"}, "", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [replacement]\n")
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", manifest}); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", "--overwrite", manifest}); code != 0 {
		t.Fatalf("cmdImport --overwrite exit code = %d, want 0", code)
	}
}

func TestCmdImportDryRunChecksOverwriteWithoutWriting(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "demo", []string{"existing"}, "", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [replacement]\n")
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", "--dry-run", manifest}); code != 1 {
		t.Fatalf("cmdImport --dry-run exit code = %d, want 1", code)
	}
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", "--dry-run", "--overwrite", manifest}); code != 0 {
		t.Fatalf("cmdImport --dry-run --overwrite exit code = %d, want 0", code)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].Command[0] != "existing" {
		t.Fatalf("dry run changed queue: %#v", queue)
	}
}

func TestCmdImportReusesSuccessAndExecutesChangedFailure(t *testing.T) {
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Jobs[1].Command = []string{"true"}
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if !queue.WorkflowImport || queue.Commands[0].Origin == nil || queue.Commands[0].Force {
		t.Fatalf("success import = %#v", queue.Commands[0])
	}
	if queue.Commands[1].Origin != nil || !queue.Commands[1].Force || queue.Commands[1].Command[0] != "true" || queue.Commands[1].ID == "failed-id" {
		t.Fatalf("changed import = %#v", queue.Commands[1])
	}
}

func TestImportedWorkflowRunCarriesSuccessAndExecutesChangedJob(t *testing.T) {
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Jobs[1].Command = []string{"true"}
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	if code := executeMixedRun(paths, "new-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit code = %d, want 0", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "new-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	results := model.ResultsByID(summary.Results)
	if results["success-id"].AttemptID != makeAttemptID(runID, "success-id", 0) || results["success-id"].ExitCode != 0 {
		t.Fatalf("carried result = %#v", results["success-id"])
	}
	if results["failed-id"].ExitCode != 0 || results["failed-id"].AttemptID == makeAttemptID(runID, "failed-id", 0) {
		t.Fatalf("executed result = %#v", results["failed-id"])
	}
}

func TestCmdImportAcceptsFailedJobAsSuccess(t *testing.T) {
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	manifest.Jobs[1].Status = "success"
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if !queue.Commands[1].Accepted || queue.Commands[1].Origin == nil || queue.Commands[1].Origin.Status != "failed" {
		t.Fatalf("accepted import = %#v", queue.Commands[1])
	}
}

func TestCmdImportReconcilesMatrixInstances(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-130000-12345678"
	runDir := filepath.Join(paths.RunsDir, runID)
	dimensions := []model.MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "seed-one", Name: "train-SEED1", Command: []string{"train"}, Environment: []string{"SEED=1"}, Matrix: &model.MatrixSpec{GroupID: "matrix-group", Dimensions: dimensions, Values: []model.MatrixValue{{Name: "SEED", Value: "1"}}, BaseName: "train"}},
		{ID: "seed-two", Name: "train-SEED2", Command: []string{"train"}, Environment: []string{"SEED=2"}, Matrix: &model.MatrixSpec{GroupID: "matrix-group", Dimensions: dimensions, Values: []model.MatrixValue{{Name: "SEED", Value: "2"}}, BaseName: "train"}},
	}}
	results := []model.JobResult{
		{ID: "seed-one", AttemptID: makeAttemptID(runID, "seed-one", 0), ExitCode: 0},
		{ID: "seed-two", AttemptID: makeAttemptID(runID, "seed-two", 0), ExitCode: 1},
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: results}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: runID, Phase: "collecting"}); err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		attemptDir, err := specificAttemptJobDir(runDir, result.ID, result.AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(attemptDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	imported, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported.Commands) != 2 || imported.Commands[0].Origin == nil || imported.Commands[0].Force || imported.Commands[1].Origin == nil || !imported.Commands[1].Force {
		t.Fatalf("imported matrix queue = %#v", imported.Commands)
	}
}

func TestCmdImportReconcilesArrayTasks(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-140000-12345678"
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "array", Name: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2}}}}
	results := []model.JobResult{
		{ID: "array-1", AttemptID: makeAttemptID(runID, "array-1", 0), ExitCode: 0},
		{ID: "array-2", AttemptID: makeAttemptID(runID, "array-2", 0), ExitCode: 1},
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: results}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: runID, Phase: "collecting"}); err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		attemptDir, err := specificAttemptJobDir(runDir, result.ID, result.AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(attemptDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	imported, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	command := imported.Commands[0]
	if command.TaskOrigins["array-1"] == nil || command.TaskOrigins["array-2"] == nil || !command.TaskForce["array-2"] || command.TaskForce["array-1"] {
		t.Fatalf("imported array command = %#v", command)
	}
	plan, err := planRerunSelection(paths, imported, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || !plan.Execute["array-2"] {
		t.Fatalf("array plan execute = %#v", plan.Execute)
	}
}

func TestCmdImportResolvesPositionalProject(t *testing.T) {
	baseDir := t.TempDir()
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [true]\n")
	if code := cmdImport([]string{"--basedir", baseDir, manifest, "demo"}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("queue = %#v", queue)
	}
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", manifest, "other"}); code == 0 {
		t.Fatal("cmdImport with --project-name and PROJECT succeeded, want usage error")
	}
	if code := cmdImport([]string{"--basedir", baseDir, manifest, "demo", "extra"}); code == 0 {
		t.Fatal("cmdImport with two selectors succeeded, want usage error")
	}
}

func TestCmdImportResolvesPositionalRunIDToItsProject(t *testing.T) {
	t.Setenv(envMasterDir, t.TempDir())
	t.Setenv(envProjectName, "")
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	exported, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	data, err := workflow.Encode(exported, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	manifest := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--dry-run", manifest, runID}); code != 0 {
		t.Fatalf("cmdImport RUN_ID exit code = %d, want 0", code)
	}
	if code := cmdImport([]string{"--basedir", baseDir, manifest, "20260925-120000-deadbeef"}); code == 0 {
		t.Fatal("cmdImport with unknown run ID succeeded, want failure")
	}
}

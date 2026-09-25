package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

func TestCmdExportReportsExportErrors(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	code, output := captureWorkflowStdout(t, func() int { return cmdExport([]string{"--basedir", baseDir, "--project-name", "demo"}) })
	if code != 1 || len(output) != 0 {
		t.Fatalf("cmdExport of empty queue code = %d, stdout = %q", code, output)
	}
}

func TestExportWorkflowRejectsInvalidQueueState(t *testing.T) {
	tests := map[string]func(*testing.T, state.ProjectPaths){
		"corrupt queue": func(t *testing.T, paths state.ProjectPaths) {
			if err := os.WriteFile(paths.QueueFile, []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"unknown executor": func(t *testing.T, paths state.ProjectPaths) {
			if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, Executor: "unknown"}}}); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, err := resolvePaths(baseDir, "demo")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths.ProjectDir, 0o700); err != nil {
				t.Fatal(err)
			}
			setup(t, paths)
			if _, err := exportWorkflow(baseDir, "demo", nil); err == nil {
				t.Fatal("exportWorkflow accepted invalid queue state")
			}
		})
	}
	if _, err := exportWorkflow(t.TempDir(), "bad/name", nil); err == nil {
		t.Fatal("exportWorkflow accepted a project name with a path separator")
	}
}

func TestExportWorkflowRejectsInvalidRunSnapshots(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"missing summary": func(t *testing.T, runDir string) {
			if err := os.Remove(filepath.Join(runDir, "summary.json")); err != nil {
				t.Fatal(err)
			}
		},
		"corrupt commands": func(t *testing.T, runDir string) {
			if err := os.WriteFile(filepath.Join(runDir, "commands.json"), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"unknown executor": func(t *testing.T, runDir string) {
			if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, Executor: "unknown"}}}); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, corrupt := range tests {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeWorkflowPipelineRun(t, baseDir)
			corrupt(t, filepath.Join(paths.RunsDir, workflowPipelineRunID))
			if _, err := exportWorkflow(baseDir, "demo", []string{workflowPipelineRunID}); err == nil {
				t.Fatal("exportWorkflow accepted an invalid run snapshot")
			}
		})
	}
}

func TestCmdImportRejectsInvalidInvocation(t *testing.T) {
	baseDir := t.TempDir()
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [true]\n")
	for name, args := range map[string][]string{
		"unknown flag":      {"--unknown", manifest},
		"missing file":      {"--basedir", baseDir, "--project-name", "demo"},
		"extra positional":  {manifest, "--basedir", baseDir, "--project-name", "demo", "extra"},
		"unreadable file":   {"--basedir", baseDir, "--project-name", "demo", filepath.Join(t.TempDir(), "missing.yaml")},
		"invalid project":   {"--basedir", baseDir, "--project-name", "bad/name", manifest},
		"invalid project 2": {"--basedir", baseDir, "--project-name", `bad\name`, manifest},
	} {
		t.Run(name, func(t *testing.T) {
			if code := cmdImport(args); code != 1 {
				t.Fatalf("cmdImport(%q) exit code = %d, want 1", args, code)
			}
		})
	}
}

func TestCmdImportRejectsInvalidManifestContent(t *testing.T) {
	for name, content := range map[string]string{
		"syntax":             "version: [\n",
		"unknown dependency": "version: 1\njobs:\n  - name: a\n    command: [true]\n    depends_on: [missing]\n",
	} {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			path := writeWorkflowFixture(t, content)
			if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 1 {
				t.Fatalf("cmdImport exit code = %d, want 1", code)
			}
		})
	}
}

func TestCmdImportReadsJSONManifest(t *testing.T) {
	baseDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "workflow.JSON")
	if err := os.WriteFile(path, []byte(`{"version":1,"jobs":[{"name":"job","command":["true"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "demo", path}); code != 0 {
		t.Fatalf("cmdImport exit code = %d, want 0", code)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if queue := loadCarryStateQueue(t, paths); len(queue.Commands) != 1 || queue.Commands[0].Name != "job" {
		t.Fatalf("queue = %#v", queue.Commands)
	}
}

func TestCmdImportRejectsInterruptedProject(t *testing.T) {
	for _, extra := range [][]string{nil, {"--dry-run"}} {
		baseDir := t.TempDir()
		paths, err := resolvePaths(baseDir, "demo")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"retained"}}}}); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
			t.Fatal(err)
		}
		writeTestRunStateFiles(t, paths, "run-1")
		manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [replacement]\n")
		args := append([]string{"--basedir", baseDir, "--project-name", "demo", "--overwrite"}, extra...)
		if code := cmdImport(append(args, manifest)); code != 1 {
			t.Fatalf("cmdImport(%q) exit code = %d, want 1", extra, code)
		}
		if queue := loadCarryStateQueue(t, paths); len(queue.Commands) != 1 || queue.Commands[0].ID != "retained" {
			t.Fatalf("interrupted project queue changed: %#v", queue.Commands)
		}
	}
}

func TestCmdImportRejectsCorruptDestinationState(t *testing.T) {
	tests := map[string]func(state.ProjectPaths) string{
		"queue": func(paths state.ProjectPaths) string { return paths.QueueFile },
		"meta":  func(paths state.ProjectPaths) string { return paths.MetaFile },
	}
	for name, target := range tests {
		for _, extra := range [][]string{nil, {"--dry-run"}} {
			t.Run(name, func(t *testing.T) {
				baseDir := t.TempDir()
				paths, err := resolvePaths(baseDir, "demo")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(paths.ProjectDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target(paths), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
				manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [true]\n")
				args := append([]string{"--basedir", baseDir, "--project-name", "demo", "--overwrite"}, extra...)
				if code := cmdImport(append(args, manifest)); code != 1 {
					t.Fatalf("cmdImport(%q) exit code = %d, want 1", extra, code)
				}
				if name == "meta" {
					if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
						t.Fatalf("import wrote a queue despite corrupt metadata: %v", err)
					}
				}
			})
		}
	}
}

func TestCmdImportRejectsBrokenSourceRunState(t *testing.T) {
	tests := map[string]func(*testing.T, state.ProjectPaths, *workflow.Manifest){
		"missing source summary": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			if err := os.Remove(filepath.Join(paths.RunsDir, workflowPipelineRunID, "summary.json")); err != nil {
				t.Fatal(err)
			}
		},
		"corrupt source commands": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			if err := os.WriteFile(filepath.Join(paths.RunsDir, workflowPipelineRunID, "commands.json"), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"attempt without completed status": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			attemptID := makeAttemptID(workflowPipelineRunID, "other-id", 1)
			attemptDir, err := specificAttemptJobDir(filepath.Join(paths.RunsDir, workflowPipelineRunID), "other-id", attemptID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(attemptDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(filepath.Join(attemptDir, statusJSONName), map[string]any{"phase": "running"}); err != nil {
				t.Fatal(err)
			}
			manifest.Jobs[2].AttemptID = attemptID
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeWorkflowPipelineRun(t, baseDir)
			manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
			mutate(t, paths, &manifest)
			if code := importEditedWorkflow(t, baseDir, manifest); code != 1 {
				t.Fatalf("cmdImport exit code = %d, want 1", code)
			}
			if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
				t.Fatalf("failed import wrote a queue: %v", err)
			}
		})
	}
}

func TestCmdImportUsesStatusOfNonLatestAttempt(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	olderAttempt := makeAttemptID(workflowPipelineRunID, "other-id", 1)
	attemptDir, err := specificAttemptJobDir(filepath.Join(paths.RunsDir, workflowPipelineRunID), "other-id", olderAttempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(attemptDir, statusJSONName), map[string]any{"phase": "failed", "exit_code": 4, "error": "boom"}); err != nil {
		t.Fatal(err)
	}
	manifest.Jobs[2].AttemptID = olderAttempt
	manifest.Jobs[2].Status = ""
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	other := queuedCommandByName(t, loadCarryStateQueue(t, paths), "other")
	if other.Origin == nil || other.Origin.AttemptID != olderAttempt || other.Origin.Status != "failed" || !other.Force {
		t.Fatalf("imported job = %#v, origin = %#v", other, other.Origin)
	}
}

func TestCmdImportRejectsAttemptForJobMissingFromSourceSnapshot(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	firstRun, secondRun := "20260925-120000-11111111", "20260925-130000-22222222"
	ghostAttempt := makeAttemptID(firstRun, "ghost", 0)
	writeWorkflowSourceRun(t, paths, firstRun, model.Queue{Commands: []model.QueuedCommand{{ID: "other", Command: []string{"true"}}}}, nil)
	writeWorkflowSourceRun(t, paths, secondRun, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Name: "job", Command: []string{"true"}}}},
		[]model.JobResult{{ID: "job", AttemptID: ghostAttempt, ExitCode: 0}})
	manifest := mustExportWorkflow(t, baseDir, secondRun)
	if code := importEditedWorkflow(t, baseDir, manifest); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
}

func TestCmdImportRejectsOriginToDeletedRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	firstRun, secondRun := "20260925-120000-11111111", "20260925-130000-22222222"
	firstAttempt := makeAttemptID(firstRun, "job", 0)
	writeWorkflowSourceRun(t, paths, firstRun, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Name: "job", Command: []string{"true"}}}},
		[]model.JobResult{{ID: "job", AttemptID: firstAttempt, ExitCode: 0}})
	writeWorkflowSourceRun(t, paths, secondRun, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Name: "job", Command: []string{"true"},
		Origin: &model.JobOrigin{RunID: firstRun, JobID: "job", AttemptID: firstAttempt, Status: "success"}}}},
		[]model.JobResult{{ID: "job", AttemptID: firstAttempt, ExitCode: 0}})
	manifest := mustExportWorkflow(t, baseDir, secondRun)
	if err := os.RemoveAll(filepath.Join(paths.RunsDir, firstRun)); err != nil {
		t.Fatal(err)
	}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
}

const workflowArrayRunID = "20260925-200000-12345678"

// writeWorkflowArrayRun writes an array run where task 1 succeeds, task 2
// fails, task 3 is cancelled, and task 4 never finishes.
func writeWorkflowArrayRun(t *testing.T, baseDir string) state.ProjectPaths {
	t.Helper()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	writeWorkflowSourceRun(t, paths, workflowArrayRunID, model.Queue{Commands: []model.QueuedCommand{{ID: "array", Name: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 4}}}}, []model.JobResult{
		{ID: "array-1", AttemptID: makeAttemptID(workflowArrayRunID, "array-1", 0), ExitCode: 0},
		{ID: "array-2", AttemptID: makeAttemptID(workflowArrayRunID, "array-2", 0), ExitCode: 1},
		{ID: "array-3", AttemptID: makeAttemptID(workflowArrayRunID, "array-3", 0), ExitCode: 130, Error: "cancelled"},
	})
	return paths
}

func TestCmdImportReconcilesArrayTaskStatuses(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowArrayRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowArrayRunID)
	if job := manifest.Jobs[0]; job.Status != "failed" || len(job.Instances) != 3 {
		t.Fatalf("exported array job = %#v", job)
	}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	command := loadCarryStateQueue(t, paths).Commands[0]
	want := map[string]string{"array-1": "success", "array-2": "failed", "array-3": "cancelled", "array-4": "unfinished"}
	for taskID, status := range want {
		origin := command.TaskOrigins[taskID]
		if origin == nil || origin.Status != status || command.TaskForce[taskID] != (status != "success") {
			t.Fatalf("task %s origin = %#v, force = %v", taskID, origin, command.TaskForce[taskID])
		}
	}
	if command.TaskOrigins["array-4"].AttemptID != "" {
		t.Fatalf("unfinished task has an attempt: %#v", command.TaskOrigins["array-4"])
	}
}

func TestCmdImportAnchorsArrayJobFromInstanceAttempt(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowArrayRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowArrayRunID)
	manifest.Jobs[0].AttemptID = ""
	for index := range manifest.Jobs[0].Instances {
		if *manifest.Jobs[0].Instances[index].Task != 2 {
			manifest.Jobs[0].Instances[index].AttemptID = ""
		}
	}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	command := loadCarryStateQueue(t, paths).Commands[0]
	if command.ID != "array" || command.TaskOrigins["array-1"] == nil || command.TaskForce["array-1"] || !command.TaskForce["array-2"] {
		t.Fatalf("imported array command = %#v", command)
	}
}

func TestCmdImportRejectsInvalidArrayInstances(t *testing.T) {
	tests := map[string]func(*workflow.Job){
		"instance without task": func(job *workflow.Job) { job.Instances[0].Task = nil },
		"task outside array":    func(job *workflow.Job) { job.Instances[0].Task = intPointerForTest(9) },
		"attempt of other task": func(job *workflow.Job) {
			job.Instances[0].AttemptID, job.Instances[1].AttemptID = job.Instances[1].AttemptID, job.Instances[0].AttemptID
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeWorkflowArrayRun(t, baseDir)
			manifest := mustExportWorkflow(t, baseDir, workflowArrayRunID)
			mutate(&manifest.Jobs[0])
			if code := importEditedWorkflow(t, baseDir, manifest); code != 1 {
				t.Fatalf("cmdImport exit code = %d, want 1", code)
			}
			if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
				t.Fatalf("failed import wrote a queue: %v", err)
			}
		})
	}
}

func TestWorkflowExportImportOfFilteredArrayRetryReusesRetriedTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	firstRun, retryRun := "20260925-120000-11111111", "20260925-130000-22222222"
	firstTaskAttempt := makeAttemptID(firstRun, "array-1", 0)
	retriedTaskAttempt := makeAttemptID(retryRun, "array-2", 0)
	array := &model.ArraySpec{First: 1, Last: 2}
	writeWorkflowSourceRun(t, paths, firstRun, model.Queue{Commands: []model.QueuedCommand{{ID: "array", Name: "array", Command: []string{"work"}, Array: array}}}, []model.JobResult{
		{ID: "array-1", AttemptID: firstTaskAttempt, ExitCode: 0},
		{ID: "array-2", AttemptID: makeAttemptID(firstRun, "array-2", 0), ExitCode: 1},
	})
	writeWorkflowSourceRun(t, paths, retryRun, model.Queue{Commands: []model.QueuedCommand{{ID: "array", Name: "array", Command: []string{"work"}, Array: array,
		TaskOrigins: map[string]*model.JobOrigin{"array-1": {RunID: firstRun, JobID: "array-1", AttemptID: firstTaskAttempt, Status: "success"}}}}}, []model.JobResult{
		{ID: "array-1", AttemptID: firstTaskAttempt, ExitCode: 0},
		{ID: "array-2", AttemptID: retriedTaskAttempt, ExitCode: 0},
	})
	manifest := mustExportWorkflow(t, baseDir, retryRun)
	if job := manifest.Jobs[0]; job.Status != "success" || len(job.Instances) != 0 {
		t.Fatalf("exported retried array = %#v", job)
	}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	command := loadCarryStateQueue(t, paths).Commands[0]
	if len(command.TaskForce) != 0 || len(command.TaskAccepted) != 0 {
		t.Fatalf("successful retried array was forced or manually accepted: force=%#v accepted=%#v", command.TaskForce, command.TaskAccepted)
	}
	if origin := command.TaskOrigins["array-1"]; origin == nil || origin.AttemptID != firstTaskAttempt {
		t.Fatalf("task 1 origin = %#v", origin)
	}
	if origin := command.TaskOrigins["array-2"]; origin == nil || origin.AttemptID != retriedTaskAttempt || origin.Status != "success" {
		t.Fatalf("task 2 origin = %#v, want retried attempt %q", origin, retriedTaskAttempt)
	}
}

func TestCmdImportRecordsSourceRunContext(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	if err := writeJSON(filepath.Join(paths.RunsDir, workflowPipelineRunID, "context.json"), model.RunContext{CWD: "/source/cwd"}); err != nil {
		t.Fatal(err)
	}
	if code := importEditedWorkflow(t, baseDir, mustExportWorkflow(t, baseDir, workflowPipelineRunID)); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	for _, command := range loadCarryStateQueue(t, paths).Commands {
		if command.Origin == nil || command.Origin.CWD != "/source/cwd" {
			t.Fatalf("origin = %#v, want source CWD", command.Origin)
		}
	}
}

func TestExportWorkflowRejectsPathLikeRunIDs(t *testing.T) {
	baseDir := t.TempDir()
	writeWorkflowPipelineRun(t, baseDir)
	for _, runID := range []string{"../" + workflowPipelineRunID, `bad\run`, "bad/run"} {
		if _, err := exportWorkflow(baseDir, "demo", []string{runID}); err == nil {
			t.Fatalf("exportWorkflow accepted run ID %q", runID)
		}
	}
}

func TestExportWorkflowRejectsRegisteredRunsFromDifferentProjects(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	firstRun, secondRun := "20260925-120000-11111111", "20260925-130000-22222222"
	for project, runID := range map[string]string{"first": firstRun, "second": secondRun} {
		paths, err := resolvePaths(baseDir, project)
		if err != nil {
			t.Fatal(err)
		}
		writeWorkflowSourceRun(t, paths, runID, model.Queue{Commands: []model.QueuedCommand{{ID: project + "-job", Command: []string{"true"}}}}, nil)
		if err := registerRun(paths, runID); err != nil {
			t.Fatal(err)
		}
	}
	// Without --project-name, each run ID resolves to the project it is
	// registered under, so the merge must reject the mixed projects.
	manifest, err := exportWorkflow("", "", []string{firstRun})
	if err != nil || manifest.Source.Project != "first" {
		t.Fatalf("single registered run export = %#v, %v", manifest.Source, err)
	}
	_, err = exportWorkflow("", "", []string{firstRun, secondRun})
	if err == nil || !strings.Contains(err.Error(), "same project") {
		t.Fatalf("exportWorkflow error = %v, want same-project rejection", err)
	}
}

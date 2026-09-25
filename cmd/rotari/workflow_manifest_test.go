package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

func captureWorkflowStdout(t *testing.T, run func() int) (int, []byte) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	code := run()
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return code, output
}

// writeWorkflowSourceRun persists a run snapshot and creates an attempt
// directory for every result that has an attempt ID.
func writeWorkflowSourceRun(t *testing.T, paths state.ProjectPaths, runID string, queue model.Queue, results []model.JobResult) {
	t.Helper()
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "finished", Results: results}); err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.AttemptID == "" {
			continue
		}
		payload, err := decodeAttemptID(result.AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		attemptDir, err := specificAttemptJobDir(filepath.Join(paths.RunsDir, payload.RunID), payload.JobID, result.AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(attemptDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: runID, Phase: "collecting"}); err != nil {
		t.Fatal(err)
	}
}

const workflowPipelineRunID = "20260925-150000-abcdef12"

// writeWorkflowPipelineRun writes a successful run where train depends on
// prepare and other is independent.
func writeWorkflowPipelineRun(t *testing.T, baseDir string) state.ProjectPaths {
	t.Helper()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"true"}},
		{ID: "train-id", Name: "train", Command: []string{"true"}, DependsOn: []string{"prepare"}},
		{ID: "other-id", Name: "other", Command: []string{"true"}},
	}}
	results := make([]model.JobResult, 0, len(queue.Commands))
	for _, command := range queue.Commands {
		results = append(results, model.JobResult{ID: command.ID, AttemptID: makeAttemptID(workflowPipelineRunID, command.ID, 0), ExitCode: 0})
	}
	writeWorkflowSourceRun(t, paths, workflowPipelineRunID, queue, results)
	return paths
}

func importEditedWorkflow(t *testing.T, baseDir string, manifest workflow.Manifest, extraArgs ...string) int {
	t.Helper()
	data, err := workflow.Encode(manifest, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	args := append([]string{"--basedir", baseDir, "--project-name", "demo"}, extraArgs...)
	return cmdImport(append(args, path))
}

func mustExportWorkflow(t *testing.T, baseDir string, runIDs ...string) workflow.Manifest {
	t.Helper()
	manifest, err := exportWorkflow(baseDir, "demo", runIDs)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func workflowJobByName(t *testing.T, manifest *workflow.Manifest, name string) *workflow.Job {
	t.Helper()
	for index := range manifest.Jobs {
		if manifest.Jobs[index].Name == name {
			return &manifest.Jobs[index]
		}
	}
	t.Fatalf("manifest has no job %q", name)
	return nil
}

func queuedCommandByName(t *testing.T, queue model.Queue, name string) model.QueuedCommand {
	t.Helper()
	for _, command := range queue.Commands {
		if command.Name == name {
			return command
		}
	}
	t.Fatalf("queue has no job %q: %#v", name, queue.Commands)
	return model.QueuedCommand{}
}

func snapshotStateTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[path] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCmdExportTemplateFormatsAreImportable(t *testing.T) {
	for _, format := range []string{"yaml", "toml", "json"} {
		code, output := captureWorkflowStdout(t, func() int { return cmdExport([]string{"--template", "--format", format}) })
		if code != 0 {
			t.Fatalf("export --template --format %s exit code = %d", format, code)
		}
		manifest, err := workflow.Decode(bytes.NewReader(output), format)
		if err != nil {
			t.Fatalf("%s template is not importable: %v\n%s", format, err, output)
		}
		if manifest.Source != nil || len(manifest.Jobs) != 1 || manifest.Jobs[0].Status != "" || manifest.Jobs[0].AttemptID != "" {
			t.Fatalf("%s template = %#v", format, manifest)
		}
		if format == "toml" && !strings.Contains(string(output), "# matrix") {
			t.Fatalf("TOML template has no field comments:\n%s", output)
		}
	}
}

func TestCmdExportRejectsInvalidOptionCombinations(t *testing.T) {
	baseDir := t.TempDir()
	for _, args := range [][]string{
		{"--template", "--run-id", "run"},
		{"--template", "--project-name", "demo"},
		{"--template", "--basedir", baseDir},
		{"--format", "xml", "--template"},
		{"--basedir", baseDir, "--project-name", "demo", "extra"},
	} {
		code, output := captureWorkflowStdout(t, func() int { return cmdExport(args) })
		if code != 1 || len(output) != 0 {
			t.Fatalf("cmdExport(%q) code = %d, stdout = %q", args, code, output)
		}
	}
}

func TestCmdExportRunWritesManifestWithoutChangingState(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"queued"}}}}); err != nil {
		t.Fatal(err)
	}
	before := snapshotStateTree(t, baseDir)
	for _, args := range [][]string{
		{"--basedir", baseDir, "--project-name", "demo", "--run-id", workflowPipelineRunID, "--format", "json"},
		{"--basedir", baseDir, "--project-name", "demo"},
	} {
		code, output := captureWorkflowStdout(t, func() int { return cmdExport(args) })
		if code != 0 {
			t.Fatalf("cmdExport(%q) exit code = %d", args, code)
		}
		format := "yaml"
		if strings.Contains(strings.Join(args, " "), "json") {
			format = "json"
		}
		if _, err := workflow.Decode(bytes.NewReader(output), format); err != nil {
			t.Fatalf("exported %s is not importable: %v\n%s", format, err, output)
		}
	}
	if after := snapshotStateTree(t, baseDir); !reflect.DeepEqual(after, before) {
		t.Fatal("export changed project state")
	}
}

func TestExportWorkflowRejectsEmptyQueueAndDuplicateRunIDs(t *testing.T) {
	baseDir := t.TempDir()
	writeWorkflowPipelineRun(t, baseDir)
	if _, err := exportWorkflow(baseDir, "demo", nil); err == nil {
		t.Fatal("exportWorkflow accepted an empty queue")
	}
	if _, err := exportWorkflow(baseDir, "demo", []string{workflowPipelineRunID, workflowPipelineRunID}); err == nil {
		t.Fatal("exportWorkflow accepted duplicate run IDs")
	}
	if _, err := exportWorkflow(baseDir, "demo", []string{"20260925-000000-00000000"}); err == nil {
		t.Fatal("exportWorkflow accepted a missing run")
	}
}

func TestExportWorkflowFlattensQueueDefaults(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{DefaultExecutor: "local", DefaultExecutorOptions: []string{"--default"}, Commands: []model.QueuedCommand{
		{ID: "inherits", Command: []string{"true"}},
		{ID: "overrides", Command: []string{"true"}, ExecutorOptions: []string{"--own"}},
	}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	manifest := mustExportWorkflow(t, baseDir)
	if manifest.Jobs[0].Executor != "local" || !reflect.DeepEqual(manifest.Jobs[0].ExecutorOptions, []string{"--default"}) {
		t.Fatalf("inherited job = %#v", manifest.Jobs[0])
	}
	if !reflect.DeepEqual(manifest.Jobs[1].ExecutorOptions, []string{"--own"}) {
		t.Fatalf("overriding job = %#v", manifest.Jobs[1])
	}
}

func TestExportWorkflowIncludesDistinctJobIDsFromMultipleRuns(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runA, runB := "20260925-120000-11111111", "20260925-130000-22222222"
	writeWorkflowSourceRun(t, paths, runA, model.Queue{Commands: []model.QueuedCommand{{ID: "job-a", Name: "a", Command: []string{"same"}}}},
		[]model.JobResult{{ID: "job-a", AttemptID: makeAttemptID(runA, "job-a", 0)}})
	writeWorkflowSourceRun(t, paths, runB, model.Queue{Commands: []model.QueuedCommand{{ID: "job-b", Command: []string{"same"}}, {ID: "job-c", Command: []string{"same"}}}},
		[]model.JobResult{{ID: "job-b", AttemptID: makeAttemptID(runB, "job-b", 0)}})
	manifest := mustExportWorkflow(t, baseDir, runA, runB)
	if !reflect.DeepEqual(manifest.Source.RunIDs, []string{runA, runB}) || len(manifest.Jobs) != 3 {
		t.Fatalf("manifest = %#v", manifest)
	}
	if manifest.Jobs[2].Status != "unfinished" || manifest.Jobs[2].AttemptID != "" {
		t.Fatalf("job without attempt = %#v", manifest.Jobs[2])
	}
}

func TestExportWorkflowSelectsLatestAttemptByJobTimestamp(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	older, newer := "20260925-120000-11111111", "20260925-130000-22222222"
	finishedAt := map[string]string{older: "2026-09-25T14:00:00Z", newer: "2026-09-25T13:30:00Z"}
	for _, runID := range []string{older, newer} {
		attemptID := makeAttemptID(runID, "job", 0)
		writeWorkflowSourceRun(t, paths, runID, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Name: "job", Command: []string{"work"}}}},
			[]model.JobResult{{ID: "job", AttemptID: attemptID}})
		attemptDir, err := specificAttemptJobDir(filepath.Join(paths.RunsDir, runID), "job", attemptID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(attemptDir, stateFileFinishedAt), []byte(finishedAt[runID]+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := mustExportWorkflow(t, baseDir, newer, older)
	if want := makeAttemptID(older, "job", 0); len(manifest.Jobs) != 1 || manifest.Jobs[0].AttemptID != want {
		t.Fatalf("manifest = %#v, want attempt %q", manifest, want)
	}
}

func TestWorkflowUnchangedRunImportReusesEveryJob(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	code, output := captureWorkflowStdout(t, func() int {
		return importEditedWorkflow(t, baseDir, mustExportWorkflow(t, baseDir, workflowPipelineRunID), "--json")
	})
	if code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	var plan importPlan
	if err := json.Unmarshal(output, &plan); err != nil {
		t.Fatalf("decode import plan: %v\n%s", err, output)
	}
	if plan.Version != 1 || plan.Project != "demo" || len(plan.Jobs) != 3 {
		t.Fatalf("plan = %#v", plan)
	}
	for _, job := range plan.Jobs {
		if job.Action != "reuse" {
			t.Fatalf("plan job = %#v, want reuse", job)
		}
	}
	queue := loadCarryStateQueue(t, paths)
	for index, want := range []string{"prepare-id", "train-id", "other-id"} {
		command := queue.Commands[index]
		if command.ID != want || command.Force || command.Accepted || command.Origin == nil || command.Origin.RunID != workflowPipelineRunID || command.Origin.Status != "success" {
			t.Fatalf("command %d = %#v", index, command)
		}
	}
	rerun, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rerun.Execute) != 0 || len(rerun.CarriedResults) != 3 {
		t.Fatalf("run plan = %#v", rerun)
	}
}

func TestWorkflowChangedJobExecutesDownstreamOnly(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	workflowJobByName(t, &manifest, "prepare").Environment = []string{"CHANGED=1"}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	queue := loadCarryStateQueue(t, paths)
	prepare := queuedCommandByName(t, queue, "prepare")
	if prepare.ID == "prepare-id" || prepare.Origin != nil || !prepare.Force {
		t.Fatalf("changed job = %#v", prepare)
	}
	rerun, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !rerun.Execute[prepare.ID] || !rerun.Execute["train-id"] || rerun.Execute["other-id"] {
		t.Fatalf("execute = %#v", rerun.Execute)
	}
}

func TestCmdImportDetectsDefinitionChanges(t *testing.T) {
	tests := map[string]func(*workflow.Manifest){
		"command":           func(m *workflow.Manifest) { m.Jobs[2].Command = []string{"true", "changed"} },
		"environment":       func(m *workflow.Manifest) { m.Jobs[2].Environment = []string{"A=1"} },
		"working directory": func(m *workflow.Manifest) { m.Jobs[2].WorkingDirectory = "work" },
		"executor":          func(m *workflow.Manifest) { m.Jobs[2].Executor = "local" },
		"executor options":  func(m *workflow.Manifest) { m.Jobs[2].ExecutorOptions = []string{"--option"} },
		"stage":             func(m *workflow.Manifest) { m.Jobs[2].Stage = "extra" },
		"dependencies":      func(m *workflow.Manifest) { m.Jobs[2].DependsOn = []string{"prepare"} },
		"array":             func(m *workflow.Manifest) { m.Jobs[2].Array = "1-2" },
		"matrix":            func(m *workflow.Manifest) { m.Jobs[2].Matrix = []string{"SEED=1"} },
		"name":              func(m *workflow.Manifest) { m.Jobs[2].Name = "renamed" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeWorkflowPipelineRun(t, baseDir)
			manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
			mutate(&manifest)
			if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
				t.Fatalf("cmdImport exit code = %d", code)
			}
			queue := loadCarryStateQueue(t, paths)
			changed := queue.Commands[len(queue.Commands)-1]
			if changed.Origin != nil || len(changed.TaskOrigins) != 0 || !changed.Force || changed.ID == "other-id" {
				t.Fatalf("changed job was reused: %#v", changed)
			}
			if prepare := queuedCommandByName(t, queue, "prepare"); prepare.Origin == nil || prepare.Force {
				t.Fatalf("unrelated job was not reused: %#v", prepare)
			}
		})
	}
}

func TestCmdImportSuccessStatusDoesNotSuppressChangedDefinition(t *testing.T) {
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, runID)
	train := workflowJobByName(t, &manifest, "train")
	train.Status = "success"
	train.Command = []string{"true"}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	command := queuedCommandByName(t, loadCarryStateQueue(t, paths), "train")
	if command.Accepted || !command.Force || command.Origin != nil {
		t.Fatalf("changed job marked success was accepted: %#v", command)
	}
}

func TestCmdImportDowngradedSuccessStatusForcesExecution(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	workflowJobByName(t, &manifest, "other").Status = "failed"
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	queue := loadCarryStateQueue(t, paths)
	if other := queuedCommandByName(t, queue, "other"); !other.Force || other.Origin == nil || other.ID != "other-id" {
		t.Fatalf("downgraded job = %#v", other)
	}
	rerun, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !rerun.Execute["other-id"] || rerun.Execute["prepare-id"] {
		t.Fatalf("execute = %#v", rerun.Execute)
	}
}

func TestCmdImportOmitsJobsRemovedFromManifest(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	manifest.Jobs = manifest.Jobs[:2]
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 2 || queue.Commands[0].ID != "prepare-id" || queue.Commands[1].ID != "train-id" {
		t.Fatalf("queue = %#v", queue.Commands)
	}
}

func TestCmdImportFailureKeepsExistingQueueAndMeta(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	if _, err := enqueueCommand(baseDir, "demo", []string{"existing"}, "", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	queueBefore, err := os.ReadFile(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	metaBefore, err := os.ReadFile(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	manifest.Jobs[2].AttemptID = makeAttemptID(workflowPipelineRunID, "missing-job", 0)
	if code := importEditedWorkflow(t, baseDir, manifest, "--overwrite"); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
	if queueAfter, _ := os.ReadFile(paths.QueueFile); !bytes.Equal(queueAfter, queueBefore) {
		t.Fatalf("failed import changed queue:\n%s", queueAfter)
	}
	if metaAfter, _ := os.ReadFile(paths.MetaFile); !bytes.Equal(metaAfter, metaBefore) {
		t.Fatalf("failed import changed meta:\n%s", metaAfter)
	}
}

func TestCmdImportRejectsRunningProject(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(paths.LockFile)
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "active-run"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "active-run")
	manifest := writeWorkflowFixture(t, "version: 1\njobs:\n  - command: [true]\n")
	for _, extra := range [][]string{nil, {"--dry-run"}, {"--overwrite"}} {
		args := append([]string{"--basedir", baseDir, "--project-name", "demo"}, extra...)
		if code := cmdImport(append(args, manifest)); code != 1 {
			t.Fatalf("cmdImport(%q) exit code = %d, want 1", extra, code)
		}
	}
	if _, err := os.Stat(paths.QueueFile); !os.IsNotExist(err) {
		t.Fatalf("import wrote a queue for a running project: %v", err)
	}
}

func TestCmdImportRejectsSourceProjectMismatch(t *testing.T) {
	baseDir := t.TempDir()
	writeWorkflowPipelineRun(t, baseDir)
	data, err := workflow.Encode(mustExportWorkflow(t, baseDir, workflowPipelineRunID), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	path := writeWorkflowFixture(t, string(data))
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "other", path}); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
	otherPaths, err := resolvePaths(baseDir, "other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(otherPaths.QueueFile); !os.IsNotExist(err) {
		t.Fatalf("mismatched import wrote a queue: %v", err)
	}
}

func TestCmdImportRejectsUnsupportedExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workflow.txt")
	if err := os.WriteFile(path, []byte("version: 1\njobs:\n  - command: [true]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cmdImport([]string{"--basedir", t.TempDir(), "--project-name", "demo", path}); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
}

func TestCmdImportRejectsInvalidSourceReferences(t *testing.T) {
	tests := map[string]func(*testing.T, state.ProjectPaths, *workflow.Manifest){
		"unlisted source run": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			otherRun := "20260925-160000-99999999"
			attemptID := makeAttemptID(otherRun, "other-id", 0)
			writeWorkflowSourceRun(t, paths, otherRun, model.Queue{Commands: []model.QueuedCommand{{ID: "other-id", Name: "other", Command: []string{"true"}}}},
				[]model.JobResult{{ID: "other-id", AttemptID: attemptID}})
			manifest.Jobs[2].AttemptID = attemptID
		},
		"missing attempt directory": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			manifest.Jobs[2].AttemptID = makeAttemptID(workflowPipelineRunID, "other-id", 5)
		},
		"missing source run": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			manifest.Source.RunIDs = []string{"20260925-000000-00000000"}
		},
		"duplicate source run": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			manifest.Source.RunIDs = []string{workflowPipelineRunID, workflowPipelineRunID}
		},
		"path-like source run": func(t *testing.T, paths state.ProjectPaths, manifest *workflow.Manifest) {
			manifest.Source.RunIDs = []string{"../" + workflowPipelineRunID}
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

func TestCmdImportSwappedAttemptIDsDoNotReuseOtherJobResults(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	manifest.Jobs[0].AttemptID, manifest.Jobs[2].AttemptID = manifest.Jobs[2].AttemptID, manifest.Jobs[0].AttemptID
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	queue := loadCarryStateQueue(t, paths)
	for _, name := range []string{"prepare", "other"} {
		if command := queuedCommandByName(t, queue, name); command.Origin != nil || !command.Force {
			t.Fatalf("job %q reused another job's attempt: %#v", name, command)
		}
	}
}

func writeWorkflowMatrixRun(t *testing.T, baseDir string) (state.ProjectPaths, string) {
	t.Helper()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-170000-12345678"
	commands := testMatrixQueueCommands("matrix-group")
	results := []model.JobResult{
		{ID: "seed-1", AttemptID: makeAttemptID(runID, "seed-1", 0), ExitCode: 0},
		{ID: "seed-2", AttemptID: makeAttemptID(runID, "seed-2", 0), ExitCode: 1},
	}
	writeWorkflowSourceRun(t, paths, runID, model.Queue{Commands: commands}, results)
	return paths, runID
}

func TestCmdImportRejectsInvalidInstanceCoordinates(t *testing.T) {
	tests := map[string]func(*workflow.Job){
		"unknown matrix value": func(job *workflow.Job) { job.Instances[0].Matrix = map[string]string{"SEED": "9"} },
		"missing matrix value": func(job *workflow.Job) { job.Instances[0].Matrix = nil },
		"task on non-array":    func(job *workflow.Job) { job.Instances[0].Task = intPointerForTest(1) },
		"duplicate instance":   func(job *workflow.Job) { job.Instances = append(job.Instances, job.Instances[0]) },
		"attempt of other leaf": func(job *workflow.Job) {
			job.Instances[0].AttemptID = job.AttemptID
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, runID := writeWorkflowMatrixRun(t, baseDir)
			manifest := mustExportWorkflow(t, baseDir, runID)
			if len(manifest.Jobs) != 1 || len(manifest.Jobs[0].Instances) != 1 {
				t.Fatalf("exported matrix manifest = %#v", manifest)
			}
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

func intPointerForTest(value int) *int {
	return &value
}

func TestCmdImportGroupSuccessAcceptsEveryFailedInstance(t *testing.T) {
	baseDir := t.TempDir()
	paths, runID := writeWorkflowMatrixRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, runID)
	manifest.Jobs[0].Status = "success"
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 2 {
		t.Fatalf("queue = %#v", queue.Commands)
	}
	if first := queue.Commands[0]; first.ID != "seed-1" || first.Accepted || first.Force || first.Origin == nil {
		t.Fatalf("successful member = %#v", first)
	}
	if second := queue.Commands[1]; second.ID != "seed-2" || !second.Accepted || second.Force || second.Origin == nil || second.Origin.Status != "failed" {
		t.Fatalf("accepted member = %#v", second)
	}
}

func TestCmdImportInstanceSuccessAcceptsOnlyThatArrayTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-180000-12345678"
	var results []model.JobResult
	for task, exitCode := range map[int]int{1: 0, 2: 1, 3: 1} {
		jobID := "array-" + string(rune('0'+task))
		results = append(results, model.JobResult{ID: jobID, AttemptID: makeAttemptID(runID, jobID, 0), ExitCode: exitCode})
	}
	writeWorkflowSourceRun(t, paths, runID, model.Queue{Commands: []model.QueuedCommand{{ID: "array", Name: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 3}}}}, results)
	manifest := mustExportWorkflow(t, baseDir, runID)
	job := &manifest.Jobs[0]
	if job.Status != "failed" || len(job.Instances) != 2 {
		t.Fatalf("exported array job = %#v", job)
	}
	for index := range job.Instances {
		if *job.Instances[index].Task == 2 {
			job.Instances[index].Status = "success"
		}
	}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	command := loadCarryStateQueue(t, paths).Commands[0]
	if !command.TaskAccepted["array-2"] || command.TaskForce["array-2"] || command.TaskAccepted["array-3"] || !command.TaskForce["array-3"] || command.TaskForce["array-1"] {
		t.Fatalf("imported array command = %#v", command)
	}
}

func TestImportedWorkflowRunAcceptsFailureAndUnblocksDependent(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-190000-12345678"
	prepareAttempt := makeAttemptID(runID, "prepare-id", 0)
	writeWorkflowSourceRun(t, paths, runID, model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"false"}},
		{ID: "train-id", Name: "train", Command: []string{"true"}, DependsOn: []string{"prepare"}},
	}}, []model.JobResult{
		{ID: "prepare-id", AttemptID: prepareAttempt, ExitCode: 3, Error: "exit status 3"},
		{ID: "train-id", ExitCode: 1, Error: "blocked by failed dependency"},
	})
	manifest := mustExportWorkflow(t, baseDir, runID)
	workflowJobByName(t, &manifest, "prepare").Status = "success"
	code, output := captureWorkflowStdout(t, func() int { return importEditedWorkflow(t, baseDir, manifest) })
	if code != 0 || !strings.Contains(string(output), "accept job_id=prepare-id job_name=prepare") || !strings.Contains(string(output), "job_name=train") {
		t.Fatalf("cmdImport code = %d, output = %q", code, output)
	}
	// The blocked dependent has no source attempt, so it is imported as fresh work.
	trainID := queuedCommandByName(t, loadCarryStateQueue(t, paths), "train").ID
	if !strings.Contains(string(output), "execute job_id="+trainID+" job_name=train") {
		t.Fatalf("dependent is not planned for execution: %q", output)
	}
	if code := executeMixedRun(paths, "accepted-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit code = %d, want 0", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "accepted-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	results := model.ResultsByID(summary.Results)
	prepare, train := results["prepare-id"], results[trainID]
	if !prepare.Accepted || prepare.ExitCode != 0 || prepare.AttemptID != prepareAttempt {
		t.Fatalf("accepted result = %#v", prepare)
	}
	if train.Accepted || train.ExitCode != 0 || train.AttemptID == "" {
		t.Fatalf("dependent result = %#v", train)
	}
	snapshot, err := loadQueue(filepath.Join(paths.RunsDir, "accepted-run", "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	if origin := queuedCommandByName(t, snapshot, "prepare").Origin; origin == nil || origin.Status != "failed" || origin.AttemptID != prepareAttempt {
		t.Fatalf("accepted origin = %#v", origin)
	}
}

func TestWorkflowQueueExportImportRoundTripIsFreshWork(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "train", "--env", "BASE=1",
		"--matrix", "SEED=1,2", "--array", "1-2", "echo", "hello"}); code != 0 {
		t.Fatalf("cmdAdd exit code = %d", code)
	}
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "evaluate", "--depends-on", "train", "echo", "done"}); code != 0 {
		t.Fatalf("cmdAdd exit code = %d", code)
	}
	exported := mustExportWorkflow(t, baseDir)
	if len(exported.Jobs) != 2 || !reflect.DeepEqual(exported.Jobs[0].Matrix, []string{"SEED=1,2"}) || exported.Jobs[0].Array != "1-2" || exported.Source != nil {
		t.Fatalf("exported queue manifest = %#v", exported)
	}
	data, err := workflow.Encode(exported, "toml")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "workflow.toml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := cmdImport([]string{"--basedir", baseDir, "--project-name", "copy", path}); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	copyPaths, err := resolvePaths(baseDir, "copy")
	if err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, copyPaths)
	if queue.WorkflowImport || len(queue.Commands) != 3 {
		t.Fatalf("imported queue = %#v", queue)
	}
	for _, command := range queue.Commands {
		if command.Origin != nil || command.Force || command.Accepted {
			t.Fatalf("fresh import carried state: %#v", command)
		}
	}
	reexported, err := exportWorkflow(baseDir, "copy", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reexported, exported) {
		t.Fatalf("re-exported manifest = %#v, want %#v", reexported, exported)
	}
}

func TestWorkflowExportImportFollowsCarriedOrigin(t *testing.T) {
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
	if manifest.Jobs[0].AttemptID != firstAttempt || !reflect.DeepEqual(manifest.Source.RunIDs, []string{secondRun}) {
		t.Fatalf("manifest = %#v", manifest)
	}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	command := loadCarryStateQueue(t, paths).Commands[0]
	if command.Force || command.Origin == nil || command.Origin.RunID != firstRun || command.Origin.AttemptID != firstAttempt {
		t.Fatalf("imported command = %#v", command)
	}
}

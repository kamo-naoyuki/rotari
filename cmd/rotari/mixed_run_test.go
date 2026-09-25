package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteMixedRunPersistsAcceptedImportedResult(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	sourceRunID := "source-run"
	if err := writeJSON(filepath.Join(paths.RunsDir, sourceRunID, "summary.json"), model.RunSummary{RunID: sourceRunID, Results: []model.JobResult{{ID: "source", AttemptID: "source-attempt", ExitCode: 7, Error: "failed"}}}); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{
		ID: "accepted", Command: []string{"must-not-run"}, Accepted: true,
		Origin: &model.JobOrigin{RunID: sourceRunID, JobID: "source", AttemptID: "source-attempt", Status: "failed"},
	}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "accepted-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit code = %d, want 0", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "accepted-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || !summary.Results[0].Accepted || summary.Results[0].ExitCode != 0 || summary.Results[0].Error != "" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestExecuteMixedRunRetriesFailedJob(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(baseDir, "retry-marker")
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{
		ID:      "retry",
		Command: []string{"/bin/sh", "-c", fmt.Sprintf("if [ -f %q ]; then exit 0; else touch %q; exit 1; fi", marker, marker)},
	}}}); err != nil {
		t.Fatal(err)
	}

	if code := executeMixedRun(paths, "retry-run", "", 1, 1, 1, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("retry marker was not created: %v", err)
	}

	summaryPath := filepath.Join(paths.RunsDir, "retry-run", "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var summary model.RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || summary.Results[0].ExitCode != 0 {
		t.Fatalf("summary = %#v, want single successful result", summary.Results)
	}
}

type recordingExecutor struct {
	name      string
	submitted []string
}

func (recorder *recordingExecutor) Name() string { return recorder.name }

func (recorder *recordingExecutor) Submit(_ string, job model.JobSpec, _ []string) (executor.JobHandle, error) {
	recorder.submitted = append(recorder.submitted, job.ID)
	return executor.JobHandle{Job: job}, nil
}

func (recorder *recordingExecutor) Wait(_ string, handle executor.JobHandle) model.JobResult {
	return model.JobResult{ID: handle.Job.ID, Command: handle.Job.Command, ExitCode: 0}
}

func TestExecuteMixedRunKeepsPerJobExecutorOverrides(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scheduler := &recordingExecutor{name: "slurm"}
	previous, existed := executorRegistry["slurm"]
	executorRegistry["slurm"] = scheduler
	t.Cleanup(func() {
		if existed {
			executorRegistry["slurm"] = previous
		} else {
			delete(executorRegistry, "slurm")
		}
	})
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "local-job", Command: []string{"sh", "-c", "exit 0"}},
		{ID: "slurm-job", Command: []string{"echo", "scheduler"}, Executor: "slurm"},
	}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "mixed-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	if len(scheduler.submitted) != 1 || scheduler.submitted[0] != "slurm-job" {
		t.Fatalf("scheduler submissions = %#v, want only slurm-job", scheduler.submitted)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "mixed-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 2 {
		t.Fatalf("summary results = %#v, want local and slurm jobs", summary.Results)
	}
}

func TestExecuteMixedRunWaitsForEveryJobInDependentStage(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scheduler := &recordingExecutor{name: "slurm"}
	previous := executorRegistry["slurm"]
	executorRegistry["slurm"] = scheduler
	t.Cleanup(func() { executorRegistry["slurm"] = previous })
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-a", Stage: "prepare", Executor: "slurm", Command: []string{"prepare-a"}},
		{ID: "prepare-b", Stage: "prepare", Executor: "slurm", Command: []string{"prepare-b"}},
		{ID: "train", Name: "train", DependsOn: []string{"prepare"}, Executor: "slurm", Command: []string{"train"}},
	}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "stage-run", "", 1, 3, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	if len(scheduler.submitted) != 3 || scheduler.submitted[2] != "train" {
		t.Fatalf("scheduler submissions = %#v, want train after both prepare jobs", scheduler.submitted)
	}
}

func TestExecuteMixedRunPersistsRuleDiagnoses(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{
		ID: "failed", Command: []string{"sh", "-c", "echo 'CUDA out of memory' >&2; exit 1"},
	}}}); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "diagnosed-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 1 {
		t.Fatalf("executeMixedRun exit = %d, want 1", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "diagnosed-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || len(summary.Results[0].Diagnoses) != 1 || summary.Results[0].Diagnoses[0].Name != "CUDA/GPU memory exhausted" {
		t.Fatalf("summary results = %#v, want persisted CUDA diagnosis", summary.Results)
	}
}

func TestExecuteMixedRunPersistsNoMatchDiagnosis(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{
		ID: "failed", Command: []string{"sh", "-c", "echo ordinary failure >&2; exit 1"},
	}}}); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "no-match-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 1 {
		t.Fatalf("executeMixedRun exit = %d, want 1", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "no-match-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 || len(summary.Results[0].Diagnoses) != 0 || summary.Results[0].DiagnosisStatus != model.DiagnosisNoMatch {
		t.Fatalf("summary results = %#v, want persisted no-match diagnosis", summary.Results)
	}
}

func TestExecuteMixedRunExecutesAllArrayTasks(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"sh", "-c", "exit 0"}, Array: &model.ArraySpec{First: 1, Last: 2},
	}}}); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "array-run", "", 2, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "array-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 2 || summary.Results[0].ID == "array" || summary.Results[1].ID == "array" {
		t.Fatalf("summary results = %#v, want two array tasks", summary.Results)
	}
}

func TestExecuteMixedRunPartialArrayReexecutesOnlyFailedTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"sh", "-c", "exit 0"}, Array: &model.ArraySpec{First: 1, Last: 2},
	}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{
		RunID: "run-1",
		Results: []model.JobResult{
			{ID: "array-1", ExitCode: 1},
			{ID: "array-2", ExitCode: 0},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if code := executeMixedRun(paths, "run-2", "", 1, 1, 0, "", nil, "failed", nil, "run-1", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-2", "array-1")); err != nil {
		t.Fatalf("array-1 was not re-executed in run-2: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-2", "array-2")); !os.IsNotExist(err) {
		t.Fatalf("array-2 should not have been re-executed, stat error = %v", err)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "run-2", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	results := make(map[string]model.JobResult, len(summary.Results))
	for _, result := range summary.Results {
		results[result.ID] = result
	}
	if result, ok := results["array-1"]; !ok || result.ExitCode != 0 {
		t.Fatalf("array-1 result = %#v, want re-executed with exit 0", result)
	}
	if result, ok := results["array-2"]; !ok || result.ExitCode != 0 {
		t.Fatalf("array-2 result = %#v, want carried forward with exit 0", result)
	}

	newQueue, err := loadQueue(filepath.Join(paths.RunsDir, "run-2", "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	origin := newQueue.Commands[0].TaskOrigins["array-2"]
	if origin == nil || origin.RunID != "run-1" || origin.JobID != "array-2" || origin.Status != "success" {
		t.Fatalf("array-2 TaskOrigins = %#v, want run-1/array-2 success", origin)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1", "array-2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.RunsDir, "run-1", "array-2", "output"), []byte("carried output\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if code := showJob(&buffer, paths, "run-2", "array-2"); code != 0 {
		t.Fatalf("showJob exit = %d, want 0", code)
	}
	if !strings.Contains(buffer.String(), "carried forward from run run-1") {
		t.Fatalf("showJob output = %q, want carried-forward note", buffer.String())
	}
}

func TestExecuteMixedRunPersistsRunName(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{
		ID: "named-job", Command: []string{"sh", "-c", "exit 0"}, Name: "named-job",
	}}}); err != nil {
		t.Fatal(err)
	}

	if code := executeMixedRun(paths, "named-run", "nightly-build", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "named-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunName != "nightly-build" {
		t.Fatalf("summary run name = %q, want nightly-build", summary.RunName)
	}
	if got := formatRunLabel(summary.RunID, summary.RunName); got != "nightly-build (named-run)" {
		t.Fatalf("run label = %q, want named-run with display name", got)
	}
}

func TestFormatRunCompletionIncludesRunNameAndFailedJobHint(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "build")
	if err != nil {
		t.Fatal(err)
	}
	message := formatRunCompletion(paths, "run-1", model.RunSummary{
		RunID: "run-1", RunName: "nightly", Status: "failed", ExitCode: 1,
		Results: []model.JobResult{{ID: "job-1", ExitCode: 1, Hosts: []string{"compute-01"}}},
	})
	for _, want := range []string{"nightly (run-1)", "Failed: 1", "Hosts: compute-01", "rotari show", "rotari retry"} {
		if !strings.Contains(message, want) {
			t.Errorf("completion message missing %q: %s", want, message)
		}
	}
}

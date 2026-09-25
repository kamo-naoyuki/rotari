package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

func TestExportWorkflowCurrentQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{DefaultExecutor: "slurm", Commands: []model.QueuedCommand{{ID: "job-id", Name: "job", Command: []string{"echo", "hello"}, Array: &model.ArraySpec{First: 1, Last: 2}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	manifest, err := exportWorkflow(baseDir, "demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Source != nil || len(manifest.Jobs) != 1 || manifest.Jobs[0].Executor != "slurm" || manifest.Jobs[0].Array != "1-2" || manifest.Jobs[0].AttemptID != "" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestExportWorkflowRunIncludesStatusAndAttempt(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-120000-12345678"
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-id", Name: "job", Command: []string{"false"}}}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	attemptID := makeAttemptID(runID, "job-id", 0)
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: []model.JobResult{{ID: "job-id", AttemptID: attemptID, ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}
	manifest, err := exportWorkflow(baseDir, "demo", []string{runID})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Source == nil || len(manifest.Source.RunIDs) != 1 || len(manifest.Jobs) != 1 || manifest.Jobs[0].Status != "failed" || manifest.Jobs[0].AttemptID != attemptID {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestCmdExportTemplateWritesCommentedYAML(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	code := cmdExport([]string{"--template"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "# matrix:") {
		t.Fatalf("code = %d, output = %q", code, output)
	}
	if _, err := workflow.Decode(strings.NewReader(string(output)), "yaml"); err != nil {
		t.Fatalf("template is not importable: %v", err)
	}
}

func TestExportWorkflowMergesSameJobIDUsingLatestRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runIDs := []string{"20260925-120000-11111111", "20260925-130000-22222222"}
	for index, runID := range runIDs {
		runDir := filepath.Join(paths.RunsDir, runID)
		if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "same-job", Name: "job", Command: []string{"work"}}}}); err != nil {
			t.Fatal(err)
		}
		attemptID := makeAttemptID(runID, "same-job", 0)
		if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, FinishedAt: runID, Results: []model.JobResult{{ID: "same-job", AttemptID: attemptID, ExitCode: index}}}); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := exportWorkflow(baseDir, "demo", runIDs)
	if err != nil {
		t.Fatal(err)
	}
	wantAttempt := makeAttemptID(runIDs[1], "same-job", 0)
	if len(manifest.Jobs) != 1 || manifest.Jobs[0].AttemptID != wantAttempt || manifest.Jobs[0].Status != "failed" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestExportWorkflowRejectsDifferentJobIDsWithSameName(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runIDs := []string{"20260925-120000-11111111", "20260925-130000-22222222"}
	for index, runID := range runIDs {
		jobID := []string{"first-job", "second-job"}[index]
		if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: jobID, Name: "same-name", Command: []string{"work"}}}}); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{RunID: runID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := exportWorkflow(baseDir, "demo", runIDs); err == nil {
		t.Fatal("exportWorkflow accepted different job IDs with the same name")
	}
}

func TestCmdExportResolvesPositionalRunIDAndProject(t *testing.T) {
	t.Setenv(envMasterDir, t.TempDir())
	t.Setenv(envProjectName, "")
	baseDir := t.TempDir()
	paths, runID := writeWorkflowRunFixture(t, baseDir)
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "queued-id", Name: "queued", Command: []string{"echo"}}}}); err != nil {
		t.Fatal(err)
	}

	code, output := captureWorkflowStdout(t, func() int { return cmdExport([]string{runID}) })
	if code != 0 {
		t.Fatalf("cmdExport RUN_ID exit code = %d", code)
	}
	manifest, err := workflow.Decode(strings.NewReader(string(output)), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Source == nil || manifest.Source.Project != "demo" || len(manifest.Source.RunIDs) != 1 || manifest.Source.RunIDs[0] != runID {
		t.Fatalf("run export source = %#v", manifest.Source)
	}

	code, output = captureWorkflowStdout(t, func() int { return cmdExport([]string{"--basedir", baseDir, "demo"}) })
	if code != 0 {
		t.Fatalf("cmdExport PROJECT exit code = %d", code)
	}
	manifest, err = workflow.Decode(strings.NewReader(string(output)), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Source != nil || len(manifest.Jobs) != 1 || manifest.Jobs[0].Name != "queued" {
		t.Fatalf("queue export = %#v", manifest)
	}

	code, output = captureWorkflowStdout(t, func() int { return cmdExport([]string{"--basedir", baseDir, "demo", runID}) })
	if code != 0 || !strings.Contains(string(output), runID) {
		t.Fatalf("cmdExport PROJECT RUN_ID code = %d, output = %q", code, output)
	}
}

func TestCmdExportRejectsInvalidPositionalSelectors(t *testing.T) {
	baseDir := t.TempDir()
	for _, args := range [][]string{
		{"--basedir", baseDir, "--project-name", "demo", "other"},
		{"--basedir", baseDir, "demo", "other"},
		{"--template", "demo"},
	} {
		if code, _ := captureWorkflowStdout(t, func() int { return cmdExport(args) }); code == 0 {
			t.Fatalf("cmdExport(%q) exit code = 0, want failure", args)
		}
	}
}

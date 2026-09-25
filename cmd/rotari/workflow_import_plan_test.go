package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

func importPlanFor(t *testing.T, baseDir string, manifest workflow.Manifest) importPlan {
	t.Helper()
	code, output := captureWorkflowStdout(t, func() int { return importEditedWorkflow(t, baseDir, manifest, "--dry-run", "--json") })
	if code != 0 {
		t.Fatalf("cmdImport --dry-run --json exit code = %d", code)
	}
	var plan importPlan
	if err := json.Unmarshal(output, &plan); err != nil {
		t.Fatalf("decode import plan: %v\n%s", err, output)
	}
	return plan
}

func importPlanJobByName(t *testing.T, plan importPlan, name string) importPlanJob {
	t.Helper()
	for _, job := range plan.Jobs {
		if job.Name == name {
			return job
		}
	}
	t.Fatalf("import plan has no job %q: %#v", name, plan.Jobs)
	return importPlanJob{}
}

func TestCmdImportPlanReportsSourcesAndRemovedJobs(t *testing.T) {
	baseDir := t.TempDir()
	writeWorkflowPipelineRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	workflowJobByName(t, &manifest, "train").Command = []string{"true", "changed"}
	manifest.Jobs = manifest.Jobs[:2]

	plan := importPlanFor(t, baseDir, manifest)
	prepare := importPlanJobByName(t, plan, "prepare")
	wantSource := &importPlanSource{RunID: workflowPipelineRunID, JobID: "prepare-id", AttemptID: makeAttemptID(workflowPipelineRunID, "prepare-id", 0), Status: "success"}
	if prepare.Action != "reuse" || !reflect.DeepEqual(prepare.Source, wantSource) {
		t.Fatalf("prepare plan = %#v, source = %#v", prepare, prepare.Source)
	}
	if train := importPlanJobByName(t, plan, "train"); train.Action != "execute" || train.Source != nil || train.ID == "train-id" {
		t.Fatalf("changed train plan = %#v", train)
	}
	wantRemoved := []workflowRemovedJob{{RunID: workflowPipelineRunID, JobID: "other-id", Name: "other"}}
	if !reflect.DeepEqual(plan.Removed, wantRemoved) {
		t.Fatalf("removed = %#v, want %#v", plan.Removed, wantRemoved)
	}

	code, output := captureWorkflowStdout(t, func() int { return importEditedWorkflow(t, baseDir, manifest, "--dry-run") })
	text := string(output)
	wantLines := []string{
		"reuse job_id=prepare-id job_name=prepare source_run_id=" + workflowPipelineRunID + " source_job_id=prepare-id source_attempt_id=" + wantSource.AttemptID + " source_status=success",
		"remove job_id=other-id job_name=other source_run_id=" + workflowPipelineRunID,
	}
	for _, want := range wantLines {
		if code != 0 || !strings.Contains(text, want+"\n") {
			t.Fatalf("human plan missing %q (code %d):\n%s", want, code, text)
		}
	}
	if strings.Index(text, "remove ") < strings.Index(text, "execute ") {
		t.Fatalf("removed jobs are not listed after queued jobs:\n%s", text)
	}
}

func TestCmdImportPlanReportsAcceptedSourceStatus(t *testing.T) {
	baseDir := t.TempDir()
	_, runID := writeWorkflowRunFixture(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, runID)
	workflowJobByName(t, &manifest, "train").Status = "success"
	plan := importPlanFor(t, baseDir, manifest)
	train := importPlanJobByName(t, plan, "train")
	if train.Action != "accept" || train.Source == nil || train.Source.Status != "failed" || train.Source.AttemptID != makeAttemptID(runID, "failed-id", 0) {
		t.Fatalf("accepted plan = %#v, source = %#v", train, train.Source)
	}
	if len(plan.Removed) != 0 {
		t.Fatalf("removed = %#v", plan.Removed)
	}
}

func TestCmdImportPlanReportsArrayTasks(t *testing.T) {
	baseDir := t.TempDir()
	writeWorkflowArrayRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowArrayRunID)
	for index := range manifest.Jobs[0].Instances {
		if *manifest.Jobs[0].Instances[index].Task == 2 {
			manifest.Jobs[0].Instances[index].Status = "success"
		}
	}
	plan := importPlanFor(t, baseDir, manifest)
	job := plan.Jobs[0]
	if job.Action != "execute" || job.Source != nil || len(job.Tasks) != 4 {
		t.Fatalf("array plan = %#v", job)
	}
	want := []struct{ action, status, attempt string }{
		{"reuse", "success", makeAttemptID(workflowArrayRunID, "array-1", 0)},
		{"accept", "failed", makeAttemptID(workflowArrayRunID, "array-2", 0)},
		{"execute", "cancelled", makeAttemptID(workflowArrayRunID, "array-3", 0)},
		{"execute", "unfinished", ""},
	}
	for index, expected := range want {
		task := job.Tasks[index]
		if task.Action != expected.action || task.Source == nil || task.Source.Status != expected.status || task.Source.AttemptID != expected.attempt {
			t.Fatalf("task %d = %#v, source = %#v, want %+v", index, task, task.Source, expected)
		}
	}

	_, output := captureWorkflowStdout(t, func() int { return importEditedWorkflow(t, baseDir, manifest, "--dry-run") })
	wantLine := "  accept task_id=array-2 source_run_id=" + workflowArrayRunID + " source_job_id=array-2 source_attempt_id=" + want[1].attempt + " source_status=failed\n"
	if !strings.Contains(string(output), "execute job_id=array job_name=array\n"+"  reuse task_id=array-1") || !strings.Contains(string(output), wantLine) {
		t.Fatalf("human array plan:\n%s", output)
	}
}

func TestCmdImportPlanKeepsMatrixGroupsAndReportsRemovedGroup(t *testing.T) {
	baseDir := t.TempDir()
	_, runID := writeWorkflowMatrixRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, runID)
	manifest.Jobs[0].Name = "renamed"
	if plan := importPlanFor(t, baseDir, manifest); len(plan.Removed) != 0 {
		t.Fatalf("renamed matrix group was reported removed: %#v", plan.Removed)
	}

	manifest = mustExportWorkflow(t, baseDir, runID)
	manifest.Jobs = append(manifest.Jobs, workflow.Job{Name: "replacement", Command: []string{"true"}})
	manifest.Jobs = manifest.Jobs[1:]
	plan := importPlanFor(t, baseDir, manifest)
	wantRemoved := []workflowRemovedJob{
		{RunID: runID, JobID: "seed-1", Name: "train-SEED1"},
		{RunID: runID, JobID: "seed-2", Name: "train-SEED2"},
	}
	if !reflect.DeepEqual(plan.Removed, wantRemoved) {
		t.Fatalf("removed = %#v, want %#v", plan.Removed, wantRemoved)
	}
}

func TestCmdImportPlanMatchesJobsWithoutAttempts(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-230000-12345678"
	writeWorkflowSourceRun(t, paths, runID, Queue{Commands: []QueuedCommand{
		{ID: "prepare-id", Name: "prepare", Command: []string{"false"}},
		{ID: "blocked-id", Name: "blocked", Command: []string{"true"}, DependsOn: []string{"prepare"}},
		{ID: "unnamed-id", Command: []string{"never", "ran"}},
	}}, []JobResult{
		{ID: "prepare-id", AttemptID: makeAttemptID(runID, "prepare-id", 0), ExitCode: 1},
		{ID: "blocked-id", ExitCode: 1, Error: "blocked by failed dependency"},
	})
	manifest := mustExportWorkflow(t, baseDir, runID)
	if plan := importPlanFor(t, baseDir, manifest); len(plan.Removed) != 0 {
		t.Fatalf("jobs without attempts were reported removed: %#v", plan.Removed)
	}
	manifest.Jobs = manifest.Jobs[:2]
	plan := importPlanFor(t, baseDir, manifest)
	wantRemoved := []workflowRemovedJob{{RunID: runID, JobID: "unnamed-id"}}
	if !reflect.DeepEqual(plan.Removed, wantRemoved) {
		t.Fatalf("removed = %#v, want %#v", plan.Removed, wantRemoved)
	}
}

func TestCmdImportPlanForFreshManifestHasNoSourceFields(t *testing.T) {
	baseDir := t.TempDir()
	plan := importPlanFor(t, baseDir, workflow.Manifest{Version: 1, Jobs: []workflow.Job{{Name: "job", Command: []string{"true"}}}})
	if plan.Removed == nil || len(plan.Removed) != 0 || plan.Jobs[0].Source != nil || plan.Jobs[0].Tasks != nil || plan.Jobs[0].Action != "execute" {
		t.Fatalf("fresh plan = %#v", plan)
	}
	_, output := captureWorkflowStdout(t, func() int {
		return importEditedWorkflow(t, baseDir, workflow.Manifest{Version: 1, Jobs: []workflow.Job{{Command: []string{"true"}}}}, "--dry-run")
	})
	if text := string(output); !strings.HasPrefix(text, "execute job_id=") || strings.Contains(text, "source_") || strings.Contains(text, "job_name=") {
		t.Fatalf("fresh human plan = %q", text)
	}
}

func TestImportedWorkflowPlansNewJobsWithoutPreviousRunLookup(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowPipelineRunID)
	workflowJobByName(t, &manifest, "other").Command = []string{"true", "changed"}
	manifest.Jobs = append(manifest.Jobs, workflow.Job{Name: "added", Command: []string{"true"}, Array: "1-2"})
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	// The run server registers the new run as LastRunID before planning, while
	// that run has no summary yet.
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "starting-run"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{LastRunID: "starting-run", Phase: "running"}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	plan, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatalf("planRerunSelection: %v", err)
	}
	other := queuedCommandByName(t, queue, "other")
	added := queuedCommandByName(t, queue, "added")
	if !plan.Execute[other.ID] || !plan.Execute[added.ID+"-1"] || !plan.Execute[added.ID+"-2"] || plan.Execute["prepare-id"] {
		t.Fatalf("execute = %#v", plan.Execute)
	}
	if _, carried := plan.CarriedOrigins[other.ID]; carried {
		t.Fatalf("new job has a carried origin: %#v", plan.CarriedOrigins[other.ID])
	}
}

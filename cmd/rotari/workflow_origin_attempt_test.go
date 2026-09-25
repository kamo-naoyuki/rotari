package main

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const originAttemptRunID = "20260925-210000-12345678"

// writeOriginAttempt creates an attempt directory under the origin run with a
// command snapshot and optional local status or wrapper status.
func writeOriginAttempt(t *testing.T, paths state.ProjectPaths, jobID, attemptID, localStatus string, wrapperStatus map[string]any) {
	t.Helper()
	attemptDir, err := specificAttemptJobDir(filepath.Join(paths.RunsDir, originAttemptRunID), jobID, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(attemptDir, commandJSONName), model.JobSpec{ID: jobID, Command: []string{"work"}}); err != nil {
		t.Fatal(err)
	}
	if localStatus != "" {
		if err := os.WriteFile(filepath.Join(attemptDir, "status"), []byte(localStatus+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(attemptDir, stateFileFinishedAt), []byte("2026-09-25T21:00:00Z\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if wrapperStatus != nil {
		if err := writeJSON(filepath.Join(attemptDir, statusJSONName), wrapperStatus); err != nil {
			t.Fatal(err)
		}
	}
}

// writeOriginAttemptRun writes a run whose summary records attempt 1 of job
// "source" as the latest result, so attempt 0 is only available on disk.
func writeOriginAttemptRun(t *testing.T) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, originAttemptRunID, model.Queue{}, []model.JobResult{
		{ID: "source", AttemptID: makeAttemptID(originAttemptRunID, "source", 1), ExitCode: 1, Error: "latest failed"},
	})
	return paths
}

func importedOriginQueue(attemptID string, accepted bool) model.Queue {
	return model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{
		ID: "destination", Command: []string{"work"}, Accepted: accepted,
		Origin: &model.JobOrigin{RunID: originAttemptRunID, JobID: "source", AttemptID: attemptID},
	}}}
}

func TestImportedWorkflowCarriesNonLatestLocalAttempt(t *testing.T) {
	paths := writeOriginAttemptRun(t)
	olderAttempt := makeAttemptID(originAttemptRunID, "source", 0)
	writeOriginAttempt(t, paths, "source", olderAttempt, "0", nil)
	plan, err := planRerunSelection(paths, importedOriginQueue(olderAttempt, false), "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	result := plan.CarriedResults["destination"]
	if plan.Execute["destination"] || result.ExitCode != 0 || result.AttemptID != olderAttempt || result.ID != "destination" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestImportedWorkflowExecutesNonLatestAttemptWithoutResult(t *testing.T) {
	paths := writeOriginAttemptRun(t)
	olderAttempt := makeAttemptID(originAttemptRunID, "source", 0)
	writeOriginAttempt(t, paths, "source", olderAttempt, "", nil)
	plan, err := planRerunSelection(paths, importedOriginQueue(olderAttempt, false), "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["destination"] {
		t.Fatalf("unfinished origin attempt was not executed: %#v", plan)
	}
}

func TestImportedWorkflowAcceptsNonLatestWrapperStatusAttempt(t *testing.T) {
	paths := writeOriginAttemptRun(t)
	olderAttempt := makeAttemptID(originAttemptRunID, "source", 0)
	writeOriginAttempt(t, paths, "source", olderAttempt, "", map[string]any{"phase": "failed", "exit_code": 3, "error": "boom", "hosts": []string{"node1"}})
	plan, err := planRerunSelection(paths, importedOriginQueue(olderAttempt, true), "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	result := plan.CarriedResults["destination"]
	if plan.Execute["destination"] || !result.Accepted || result.ExitCode != 0 || result.Error != "" || result.AttemptID != olderAttempt || len(result.Hosts) != 1 {
		t.Fatalf("accepted result = %#v", result)
	}
}

func TestImportedWorkflowRejectsBrokenOriginState(t *testing.T) {
	olderAttempt := makeAttemptID(originAttemptRunID, "source", 0)
	tests := map[string]struct {
		accepted bool
		setup    func(*testing.T, state.ProjectPaths)
		want     string
	}{
		"missing attempt command": {setup: func(t *testing.T, paths state.ProjectPaths) {
			attemptDir, err := specificAttemptJobDir(filepath.Join(paths.RunsDir, originAttemptRunID), "source", olderAttempt)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(attemptDir, 0o700); err != nil {
				t.Fatal(err)
			}
		}, want: "command"},
		"missing origin summary": {setup: func(t *testing.T, paths state.ProjectPaths) {
			if err := os.Remove(filepath.Join(paths.RunsDir, originAttemptRunID, "summary.json")); err != nil {
				t.Fatal(err)
			}
		}, want: "summary"},
		"accepted attempt without result": {accepted: true, setup: func(t *testing.T, paths state.ProjectPaths) {
			writeOriginAttempt(t, paths, "source", olderAttempt, "", nil)
		}, want: "not found"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			paths := writeOriginAttemptRun(t)
			test.setup(t, paths)
			_, err := planRerunSelection(paths, importedOriginQueue(olderAttempt, test.accepted), "", nil, "", true)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("planRerunSelection error = %v, want %q", err, test.want)
			}
		})
	}
}

func writeWorkflowMatrixRetryRuns(t *testing.T, baseDir string) (state.ProjectPaths, string, string) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	firstRun, retryRun := "20260925-120000-11111111", "20260925-130000-22222222"
	firstAttempt := makeAttemptID(firstRun, "seed-1", 0)
	writeWorkflowSourceRun(t, paths, firstRun, model.Queue{Commands: testMatrixQueueCommands("group")}, []model.JobResult{
		{ID: "seed-1", AttemptID: firstAttempt, ExitCode: 0},
		{ID: "seed-2", AttemptID: makeAttemptID(firstRun, "seed-2", 0), ExitCode: 1},
	})
	retried := testMatrixQueueCommands("group")
	retried[0].Origin = &model.JobOrigin{RunID: firstRun, JobID: "seed-1", AttemptID: firstAttempt, Status: "success"}
	writeWorkflowSourceRun(t, paths, retryRun, model.Queue{Commands: retried}, []model.JobResult{
		{ID: "seed-1", AttemptID: firstAttempt, ExitCode: 0},
		{ID: "seed-2", AttemptID: makeAttemptID(retryRun, "seed-2", 0), ExitCode: 0},
	})
	return paths, firstRun, retryRun
}

func TestWorkflowUnchangedMatrixRunImportReusesEveryCombination(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260925-170000-12345678"
	writeWorkflowSourceRun(t, paths, runID, model.Queue{Commands: testMatrixQueueCommands("group")}, []model.JobResult{
		{ID: "seed-1", AttemptID: makeAttemptID(runID, "seed-1", 0)},
		{ID: "seed-2", AttemptID: makeAttemptID(runID, "seed-2", 0)},
	})
	manifest := mustExportWorkflow(t, baseDir, runID)
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	for _, command := range loadCarryStateQueue(t, paths).Commands {
		if command.Force || command.Accepted || command.Origin == nil || command.Origin.JobID != command.ID || command.Origin.AttemptID != makeAttemptID(runID, command.ID, 0) {
			t.Fatalf("matrix member = %#v, origin = %#v", command, command.Origin)
		}
	}
}

func TestWorkflowExportImportOfFilteredMatrixRetryKeepsEachOrigin(t *testing.T) {
	baseDir := t.TempDir()
	paths, firstRun, retryRun := writeWorkflowMatrixRetryRuns(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, retryRun)
	if job := manifest.Jobs[0]; job.Status != "success" || len(job.Instances) != 0 {
		t.Fatalf("exported retried matrix = %#v", job)
	}
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	queue := loadCarryStateQueue(t, paths)
	want := map[string]string{"seed-1": makeAttemptID(firstRun, "seed-1", 0), "seed-2": makeAttemptID(retryRun, "seed-2", 0)}
	for _, command := range queue.Commands {
		if command.Force || command.Accepted || command.Origin == nil || command.Origin.AttemptID != want[command.ID] || command.Origin.Status != "success" {
			t.Fatalf("matrix member %q = %#v, origin = %#v", command.ID, command, command.Origin)
		}
	}
	if origin := queue.Commands[0].Origin; origin.RunID != firstRun {
		t.Fatalf("carried member origin run = %q, want %q", origin.RunID, firstRun)
	}
}

func TestCmdImportRejectsMalformedCarriedAttempt(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowArrayRun(t, baseDir)
	manifest := mustExportWorkflow(t, baseDir, workflowArrayRunID)
	summaryPath := filepath.Join(paths.RunsDir, workflowArrayRunID, "summary.json")
	summary, err := loadRunSummary(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	for index := range summary.Results {
		if summary.Results[index].ID == "array-1" {
			summary.Results[index].AttemptID = "not-an-attempt"
		}
	}
	if err := writeJSON(summaryPath, summary); err != nil {
		t.Fatal(err)
	}
	manifest.Jobs[0].AttemptID = makeAttemptID(workflowArrayRunID, "array-2", 0)
	if code := importEditedWorkflow(t, baseDir, manifest); code != 1 {
		t.Fatalf("cmdImport exit code = %d, want 1", code)
	}
}

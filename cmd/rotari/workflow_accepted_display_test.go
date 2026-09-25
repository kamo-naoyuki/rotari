package main

import (
	"bytes"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const acceptedDisplaySourceRunID = "20260925-220000-12345678"

// writeLocalSourceAttempt records a finished local attempt with the given exit code.
func writeLocalSourceAttempt(t *testing.T, paths state.ProjectPaths, jobID, attemptID string, exitCode string) {
	t.Helper()
	attemptDir, err := specificAttemptJobDir(filepath.Join(paths.RunsDir, acceptedDisplaySourceRunID), jobID, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"status": exitCode, stateFileFinishedAt: "2026-09-25T22:00:00Z"} {
		if err := os.WriteFile(filepath.Join(attemptDir, name), []byte(content+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeJSON(filepath.Join(attemptDir, commandJSONName), model.JobSpec{ID: jobID, Command: []string{"false"}}); err != nil {
		t.Fatal(err)
	}
}

// writeAcceptedDisplaySource writes a failed source job with two local
// attempts: attempt 0 exited 3 and the latest attempt 1 exited 5.
func writeAcceptedDisplaySource(t *testing.T, baseDir string) (state.ProjectPaths, string, string) {
	t.Helper()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	olderAttempt := makeAttemptID(acceptedDisplaySourceRunID, "prepare-id", 0)
	latestAttempt := makeAttemptID(acceptedDisplaySourceRunID, "prepare-id", 1)
	writeWorkflowSourceRun(t, paths, acceptedDisplaySourceRunID, model.Queue{Commands: []model.QueuedCommand{{ID: "prepare-id", Name: "prepare", Command: []string{"false"}}}},
		[]model.JobResult{{ID: "prepare-id", AttemptID: latestAttempt, ExitCode: 5}})
	writeLocalSourceAttempt(t, paths, "prepare-id", olderAttempt, "3")
	writeLocalSourceAttempt(t, paths, "prepare-id", latestAttempt, "5")
	return paths, olderAttempt, latestAttempt
}

func runAcceptedImport(t *testing.T, baseDir string, paths state.ProjectPaths, attemptID string) {
	t.Helper()
	manifest := mustExportWorkflow(t, baseDir, acceptedDisplaySourceRunID)
	manifest.Jobs[0].AttemptID = attemptID
	manifest.Jobs[0].Status = "success"
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	if code := executeMixedRun(paths, "accepted-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit code = %d", code)
	}
}

func TestAcceptedResultDisplaysConsistentlyInShowAndWeb(t *testing.T) {
	baseDir := t.TempDir()
	paths, _, latestAttempt := writeAcceptedDisplaySource(t, baseDir)
	runAcceptedImport(t, baseDir, paths, latestAttempt)

	var jobOutput bytes.Buffer
	if code := showJob(&jobOutput, paths, "accepted-run", "prepare-id"); code != 0 {
		t.Fatalf("showJob exit code = %d", code)
	}
	text := jobOutput.String()
	for _, want := range []string{
		"success (accepted)",
		"manually accepted from run " + acceptedDisplaySourceRunID + " attempt " + latestAttempt,
		"Status: 5",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("showJob output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "carried forward") {
		t.Fatalf("showJob described an accepted result as an ordinary carry:\n%s", text)
	}

	code, runOutput := captureWorkflowStdout(t, func() int { return showRun(paths, "accepted-run", false) })
	if code != 0 || !strings.Contains(string(runOutput), "success (accepted)") {
		t.Fatalf("showRun code = %d, output:\n%s", code, runOutput)
	}

	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "accepted-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := loadWebJobs(filepath.Join(paths.RunsDir, "accepted-run"), summary)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Result == nil || !jobs[0].Result.Accepted || jobs[0].Result.ExitCode != 0 {
		t.Fatalf("web jobs = %#v", jobs)
	}
	if origin := jobs[0].Origin; origin == nil || origin.Status != "failed" || origin.AttemptID != latestAttempt {
		t.Fatalf("web origin = %#v", origin)
	}
}

func TestShowJobAcceptedResultLinksSelectedSourceAttempt(t *testing.T) {
	baseDir := t.TempDir()
	paths, olderAttempt, _ := writeAcceptedDisplaySource(t, baseDir)
	runAcceptedImport(t, baseDir, paths, olderAttempt)
	var output bytes.Buffer
	if code := showJob(&output, paths, "accepted-run", "prepare-id"); code != 0 {
		t.Fatalf("showJob exit code = %d", code)
	}
	text := output.String()
	if !strings.Contains(text, "Attempt ID: "+olderAttempt) || !strings.Contains(text, "Status: 3") {
		t.Fatalf("showJob did not show the accepted source attempt:\n%s", text)
	}
}

func TestShowJobOrdinaryCarryStillShowsCarriedNote(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeWorkflowPipelineRun(t, baseDir)
	if code := importEditedWorkflow(t, baseDir, mustExportWorkflow(t, baseDir, workflowPipelineRunID)); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	if code := executeMixedRun(paths, "carried-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit code = %d", code)
	}
	var output bytes.Buffer
	if code := showJob(&output, paths, "carried-run", "prepare-id"); code != 0 {
		t.Fatalf("showJob exit code = %d", code)
	}
	if text := output.String(); !strings.Contains(text, "carried forward from run "+workflowPipelineRunID) || strings.Contains(text, "accepted") {
		t.Fatalf("showJob output:\n%s", text)
	}
}

func TestCmdImportResolvesNonLatestLocalAttempt(t *testing.T) {
	baseDir := t.TempDir()
	paths, olderAttempt, _ := writeAcceptedDisplaySource(t, baseDir)
	writeLocalSourceAttempt(t, paths, "prepare-id", olderAttempt, "0")
	manifest := mustExportWorkflow(t, baseDir, acceptedDisplaySourceRunID)
	manifest.Jobs[0].AttemptID = olderAttempt
	manifest.Jobs[0].Status = ""
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	command := loadCarryStateQueue(t, paths).Commands[0]
	if command.Force || command.Accepted || command.Origin == nil || command.Origin.AttemptID != olderAttempt || command.Origin.Status != "success" {
		t.Fatalf("imported command = %#v, origin = %#v", command, command.Origin)
	}
}

func TestAcceptedArrayTaskDisplaysConsistentlyInShowAndWeb(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	firstTask := makeAttemptID(acceptedDisplaySourceRunID, "array-1", 0)
	secondTask := makeAttemptID(acceptedDisplaySourceRunID, "array-2", 0)
	writeWorkflowSourceRun(t, paths, acceptedDisplaySourceRunID, model.Queue{Commands: []model.QueuedCommand{{ID: "array", Name: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2}}}},
		[]model.JobResult{{ID: "array-1", AttemptID: firstTask, ExitCode: 0}, {ID: "array-2", AttemptID: secondTask, ExitCode: 4}})
	writeLocalSourceAttempt(t, paths, "array-1", firstTask, "0")
	writeLocalSourceAttempt(t, paths, "array-2", secondTask, "4")
	manifest := mustExportWorkflow(t, baseDir, acceptedDisplaySourceRunID)
	if len(manifest.Jobs[0].Instances) != 1 {
		t.Fatalf("exported array job = %#v", manifest.Jobs[0])
	}
	manifest.Jobs[0].Instances[0].Status = "success"
	if code := importEditedWorkflow(t, baseDir, manifest); code != 0 {
		t.Fatalf("cmdImport exit code = %d", code)
	}
	if code := executeMixedRun(paths, "accepted-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit code = %d", code)
	}

	var accepted bytes.Buffer
	if code := showJob(&accepted, paths, "accepted-run", "array-2"); code != 0 {
		t.Fatalf("showJob array-2 exit code = %d", code)
	}
	for _, want := range []string{"success (accepted)", "manually accepted from run " + acceptedDisplaySourceRunID + " attempt " + secondTask, "Status: 4"} {
		if !strings.Contains(accepted.String(), want) {
			t.Fatalf("accepted task output missing %q:\n%s", want, accepted.String())
		}
	}
	var carried bytes.Buffer
	if code := showJob(&carried, paths, "accepted-run", "array-1"); code != 0 {
		t.Fatalf("showJob array-1 exit code = %d", code)
	}
	if text := carried.String(); !strings.Contains(text, "carried forward from run "+acceptedDisplaySourceRunID) || strings.Contains(text, "accepted") {
		t.Fatalf("carried task output:\n%s", text)
	}

	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "accepted-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := loadWebJobs(filepath.Join(paths.RunsDir, "accepted-run"), summary)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]webprojection.Job, len(jobs))
	for _, job := range jobs {
		byID[job.ID] = job
	}
	if task := byID["array-2"]; task.Result == nil || !task.Result.Accepted || task.Origin == nil || task.Origin.Status != "failed" {
		t.Fatalf("web accepted task = %#v", task)
	}
	if task := byID["array-1"]; task.Result == nil || task.Result.Accepted || task.Result.ExitCode != 0 {
		t.Fatalf("web carried task = %#v", task)
	}
}

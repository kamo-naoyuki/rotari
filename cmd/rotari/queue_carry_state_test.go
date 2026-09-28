package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func testMatrixQueueCommands(groupID string) []model.QueuedCommand {
	dimensions := []model.MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	commands := make([]model.QueuedCommand, 0, 2)
	for _, value := range []string{"1", "2"} {
		combination := []model.MatrixValue{{Name: "SEED", Value: value}}
		commands = append(commands, model.QueuedCommand{
			ID: "seed-" + value, Name: model.MatrixJobName("train", combination), Command: []string{"train"},
			Environment: model.MatrixEnvironment(nil, combination),
			Matrix:      &model.MatrixSpec{GroupID: groupID, Dimensions: dimensions, Values: combination, BaseName: "train"},
		})
	}
	return commands
}

func writeCarryStateRun(t *testing.T, paths state.ProjectPaths, runID string, queue model.Queue, results []model.JobResult) {
	t.Helper()
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: results}); err != nil {
		t.Fatal(err)
	}
}

func loadCarryStateQueue(t *testing.T, paths state.ProjectPaths) model.Queue {
	t.Helper()
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	return queue
}

func writeImportedArraySource(t *testing.T, paths state.ProjectPaths) {
	t.Helper()
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: []model.QueuedCommand{{ID: "source", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2}}}}, []model.JobResult{
		{ID: "source-1", AttemptID: "attempt-1", ExitCode: 0},
		{ID: "source-2", AttemptID: "attempt-2", ExitCode: 3, Error: "task failed"},
	})
}

// retrySelection is the selection of rotari retry.
const retrySelection = "failed,unfinished"

func TestMarkedSuccessAcceptsFailedArrayTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeImportedArraySource(t, paths)
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2},
		TaskOrigins: map[string]*model.JobOrigin{
			"array-1": {RunID: "source-run", JobID: "source-1", AttemptID: "attempt-1", Status: "success"},
			"array-2": {RunID: "source-run", JobID: "source-2", AttemptID: "attempt-2", Status: "failed"},
		},
		TaskMarkedStatus: map[string]string{"array-2": model.StatusSuccess},
	}}}
	plan, err := planRerunSelection(paths, queue, retrySelection, nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || plan.Execute["array-2"] {
		t.Fatalf("execute = %#v, want no execution", plan.Execute)
	}
	accepted := plan.CarriedResults["array-2"]
	if !accepted.Accepted || accepted.ExitCode != 0 || accepted.Error != "" || accepted.ID != "array-2" || accepted.AttemptID != "attempt-2" {
		t.Fatalf("accepted task result = %#v", accepted)
	}
	if carried := plan.CarriedResults["array-1"]; carried.Accepted || carried.AttemptID != "attempt-1" {
		t.Fatalf("successful task result = %#v", carried)
	}
}

func TestMarkedSuccessAcceptsArrayTaskThroughCommandOrigin(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeImportedArraySource(t, paths)
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2},
		Origin:           &model.JobOrigin{RunID: "source-run", JobID: "source"},
		TaskMarkedStatus: map[string]string{"array-2": model.StatusSuccess},
	}}}
	plan, err := planRerunSelection(paths, queue, retrySelection, nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-2"] || !plan.CarriedResults["array-2"].Accepted || plan.CarriedResults["array-2"].AttemptID != "attempt-2" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestUnfinishedArrayTaskExecutesDownstream(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{}, []model.JobResult{
		{ID: "source-1", ExitCode: 0}, {ID: "source-2", ExitCode: 0}, {ID: "source-downstream", ExitCode: 0}, {ID: "source-independent", ExitCode: 0},
	})
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "array", Name: "work", Stage: "compute", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2},
			Origin: &model.JobOrigin{RunID: "source-run", JobID: "source"}, TaskMarkedStatus: map[string]string{"array-2": model.StatusUnfinished}},
		{ID: "downstream", Name: "downstream", Command: []string{"true"}, DependsOn: []string{"compute"},
			Origin: &model.JobOrigin{RunID: "source-run", JobID: "source-downstream"}},
		{ID: "independent", Name: "independent", Command: []string{"true"},
			Origin: &model.JobOrigin{RunID: "source-run", JobID: "source-independent"}},
	}}
	plan, err := planRerunSelection(paths, queue, retrySelection, nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || !plan.Execute["array-2"] || !plan.Execute["downstream"] || plan.Execute["independent"] {
		t.Fatalf("execute = %#v", plan.Execute)
	}
	if _, carried := plan.CarriedResults["downstream"]; carried {
		t.Fatal("downstream job kept a carried result while executing")
	}
}

func TestMarkWithoutRecordedResultFails(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "previous-run", model.Queue{}, []model.JobResult{{ID: "other", ExitCode: 1}})
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: "previous-run", Phase: "collecting"}); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, MarkedStatus: model.StatusSuccess}}}
	_, err = planRerunSelection(paths, queue, retrySelection, nil, "", true)
	if err == nil || !strings.Contains(err.Error(), "has no recorded result") {
		t.Fatalf("planRerunSelection error = %v, want missing result", err)
	}
}

func TestExecuteMixedRunPersistsAcceptedArrayTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeImportedArraySource(t, paths)
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"must-not-run"}, Array: &model.ArraySpec{First: 1, Last: 2},
		TaskOrigins: map[string]*model.JobOrigin{
			"array-1": {RunID: "source-run", JobID: "source-1", AttemptID: "attempt-1", Status: "success"},
			"array-2": {RunID: "source-run", JobID: "source-2", AttemptID: "attempt-2", Status: "failed"},
		},
		TaskMarkedStatus: map[string]string{"array-2": model.StatusSuccess},
	}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "accepted-array-run", "", 1, 1, 0, "", nil, retrySelection, nil, "source-run", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit code = %d, want 0", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "accepted-array-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	results := model.ResultsByID(summary.Results)
	if results["array-1"].Accepted || results["array-1"].ExitCode != 0 || !results["array-2"].Accepted || results["array-2"].ExitCode != 0 {
		t.Fatalf("summary results = %#v", summary.Results)
	}
}

func testMatrixQueueWithDependent(groupID string) []model.QueuedCommand {
	return append(testMatrixQueueCommands(groupID), model.QueuedCommand{ID: "evaluate-id", Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}})
}

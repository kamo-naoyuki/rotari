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

func TestEnqueueCommandForcesOnlyInImportedQueue(t *testing.T) {
	for _, imported := range []bool{false, true} {
		baseDir := t.TempDir()
		paths, err := state.ResolveProjectPaths(baseDir, "default")
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.QueueFile, model.Queue{WorkflowImport: imported, Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"true"}}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := enqueueCommand(baseDir, "default", []string{"added"}, "", nil, nil, "", nil); err != nil {
			t.Fatal(err)
		}
		queue := loadCarryStateQueue(t, paths)
		if queue.WorkflowImport != imported || queue.Commands[0].Force || queue.Commands[1].Force != imported {
			t.Fatalf("imported=%v queue = %#v", imported, queue)
		}
	}
}

func TestResetQueueCommandsClearsWorkflowImport(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, Force: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := resetQueueCommands(paths); err != nil {
		t.Fatal(err)
	}
	if _, err := enqueueCommand(baseDir, "default", []string{"added"}, "", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if queue.WorkflowImport || len(queue.Commands) != 1 || queue.Commands[0].Force {
		t.Fatalf("queue after reset and add = %#v", queue)
	}
}

func writeImportedArraySource(t *testing.T, paths state.ProjectPaths) {
	t.Helper()
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: []model.QueuedCommand{{ID: "source", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2}}}}, []model.JobResult{
		{ID: "source-1", AttemptID: "attempt-1", ExitCode: 0},
		{ID: "source-2", AttemptID: "attempt-2", ExitCode: 3, Error: "task failed"},
	})
}

func TestImportedWorkflowAcceptsFailedArrayTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeImportedArraySource(t, paths)
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2},
		TaskOrigins: map[string]*model.JobOrigin{
			"array-1": {RunID: "source-run", JobID: "source-1", AttemptID: "attempt-1", Status: "success"},
			"array-2": {RunID: "source-run", JobID: "source-2", AttemptID: "attempt-2", Status: "failed"},
		},
		TaskAccepted: map[string]bool{"array-2": true},
	}}}
	plan, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || plan.Execute["array-2"] {
		t.Fatalf("execute = %#v, want no execution", plan.Execute)
	}
	accepted := plan.CarriedResults["array-2"]
	if !accepted.Accepted || accepted.ExitCode != 0 || accepted.Error != "" || accepted.ID != "array-2" || plan.CarriedOrigins["array-2"].Status != "failed" {
		t.Fatalf("accepted task result = %#v, origin = %#v", accepted, plan.CarriedOrigins["array-2"])
	}
	if carried := plan.CarriedResults["array-1"]; carried.Accepted || carried.AttemptID != "attempt-1" {
		t.Fatalf("successful task result = %#v", carried)
	}
}

func TestImportedWorkflowAcceptsArrayTaskThroughCommandOrigin(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeImportedArraySource(t, paths)
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2},
		Origin:       &model.JobOrigin{RunID: "source-run", JobID: "source"},
		TaskAccepted: map[string]bool{"array-2": true},
	}}}
	plan, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-2"] || !plan.CarriedResults["array-2"].Accepted || plan.CarriedOrigins["array-2"].JobID != "source-2" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestImportedWorkflowForcedArrayTaskExecutesDownstream(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{}, []model.JobResult{
		{ID: "source-1", ExitCode: 0}, {ID: "source-2", ExitCode: 0}, {ID: "source-downstream", ExitCode: 0}, {ID: "source-independent", ExitCode: 0},
	})
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{
		{ID: "array", Name: "work", Stage: "compute", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2},
			Origin: &model.JobOrigin{RunID: "source-run", JobID: "source"}, TaskForce: map[string]bool{"array-2": true}},
		{ID: "downstream", Name: "downstream", Command: []string{"true"}, DependsOn: []string{"compute"},
			Origin: &model.JobOrigin{RunID: "source-run", JobID: "source-downstream"}},
		{ID: "independent", Name: "independent", Command: []string{"true"},
			Origin: &model.JobOrigin{RunID: "source-run", JobID: "source-independent"}},
	}}
	plan, err := planRerunSelection(paths, queue, "", nil, "", true)
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

func TestImportedWorkflowForceOverridesAcceptance(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{}, []model.JobResult{{ID: "source", AttemptID: "attempt", ExitCode: 1}})
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{
		ID: "job", Command: []string{"true"}, Accepted: true, Force: true,
		Origin: &model.JobOrigin{RunID: "source-run", JobID: "source", AttemptID: "attempt", Status: "failed"},
	}}}
	plan, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["job"] {
		t.Fatalf("forced job was not executed: %#v", plan)
	}
	if _, carried := plan.CarriedResults["job"]; carried {
		t.Fatal("forced job kept an accepted carried result")
	}
}

func TestImportedWorkflowAcceptRequiresOrigin(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "default")
	if err != nil {
		t.Fatal(err)
	}
	// Give the fallback lookup a previous run so planning reaches acceptance.
	writeCarryStateRun(t, paths, "previous-run", model.Queue{}, []model.JobResult{{ID: "job", ExitCode: 1}})
	if err := writeJSON(paths.MetaFile, model.Meta{LastRunID: "previous-run", Phase: "collecting"}); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}, Accepted: true}}}
	_, err = planRerunSelection(paths, queue, "", nil, "", true)
	if err == nil || !strings.Contains(err.Error(), "has no source origin") {
		t.Fatalf("planRerunSelection error = %v, want missing source origin", err)
	}
}

func TestExecuteMixedRunPersistsAcceptedArrayTask(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeImportedArraySource(t, paths)
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"must-not-run"}, Array: &model.ArraySpec{First: 1, Last: 2},
		TaskOrigins: map[string]*model.JobOrigin{
			"array-1": {RunID: "source-run", JobID: "source-1", AttemptID: "attempt-1", Status: "success"},
			"array-2": {RunID: "source-run", JobID: "source-2", AttemptID: "attempt-2", Status: "failed"},
		},
		TaskAccepted: map[string]bool{"array-2": true},
	}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "accepted-array-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
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

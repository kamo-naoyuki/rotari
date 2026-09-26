package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
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

func TestCopyRunToQueueKeepsCompleteMatrixGroupUnderNewGroupID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: testMatrixQueueCommands("source-group")}, []model.JobResult{
		{ID: "seed-1", ExitCode: 0}, {ID: "seed-2", ExitCode: 0},
	})
	if _, err := queueEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "all"}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 2 || queue.Commands[0].Matrix == nil || queue.Commands[1].Matrix == nil {
		t.Fatalf("copied matrix group lost provenance: %#v", queue.Commands)
	}
	groupID := queue.Commands[0].Matrix.GroupID
	if groupID == "source-group" || groupID != queue.Commands[1].Matrix.GroupID {
		t.Fatalf("copied group IDs = %q, %q; want one new shared ID", groupID, queue.Commands[1].Matrix.GroupID)
	}
}

func TestCopyRunToQueueClearsPartialMatrixGroup(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: testMatrixQueueCommands("source-group")}, []model.JobResult{
		{ID: "seed-1", ExitCode: 0}, {ID: "seed-2", ExitCode: 1},
	})
	if _, err := queueEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "failed"}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "seed-2" || queue.Commands[0].Matrix != nil {
		t.Fatalf("partially copied matrix group = %#v", queue.Commands)
	}
}

func TestCopyRunToQueueDropsImportCarryFlagsFromSnapshot(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{
		{ID: "scalar", Command: []string{"true"}, Accepted: true, Force: true},
		{ID: "array", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2},
			TaskAccepted: map[string]bool{"array-1": true}, TaskForce: map[string]bool{"array-2": true}},
	}}
	writeCarryStateRun(t, paths, "imported-run", snapshot, []model.JobResult{
		{ID: "scalar", ExitCode: 0, Accepted: true}, {ID: "array-1", ExitCode: 0, Accepted: true}, {ID: "array-2", ExitCode: 0},
	})
	if _, err := queueEditor().Copy(baseDir, "default", "imported-run", queueedit.CopyRequest{Selection: "all"}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if queue.WorkflowImport {
		t.Fatal("copy into an empty queue kept the workflow import marker")
	}
	for _, command := range queue.Commands {
		if command.Accepted || command.Force || command.TaskAccepted != nil || command.TaskForce != nil {
			t.Fatalf("copied command kept import carry flags: %#v", command)
		}
	}
}

func TestCopyRunToQueueAppendToImportedQueueForcesCopiedJobs(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: []model.QueuedCommand{{ID: "copied", Command: []string{"true"}}}}, []model.JobResult{{ID: "copied", ExitCode: 0}})
	imported := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "imported", Command: []string{"true"}, Origin: &model.JobOrigin{RunID: "source-run", JobID: "copied"}}}}
	if err := writeJSON(paths.QueueFile, imported); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "all", Append: true}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if !queue.WorkflowImport || len(queue.Commands) != 2 {
		t.Fatalf("appended queue = %#v", queue)
	}
	if queue.Commands[0].Force {
		t.Fatalf("existing imported job was forced: %#v", queue.Commands[0])
	}
	if !queue.Commands[1].Force {
		t.Fatalf("copied job appended to an imported queue was not forced: %#v", queue.Commands[1])
	}
}

func TestCopyRunToQueueOverwriteClearsWorkflowImport(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: []model.QueuedCommand{{ID: "copied", Command: []string{"true"}}}}, []model.JobResult{{ID: "copied", ExitCode: 0}})
	if err := writeJSON(paths.QueueFile, model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "imported", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "all", Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if queue.WorkflowImport || len(queue.Commands) != 1 || queue.Commands[0].Force {
		t.Fatalf("overwritten queue = %#v", queue)
	}
}

func TestRemoveBatchClearsRemainingMatrixProvenance(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueCommands("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-2"}}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "seed-1" || queue.Commands[0].Matrix != nil {
		t.Fatalf("queue after removing one matrix member = %#v", queue.Commands)
	}
}

func TestChangeBatchClearsWholeMatrixGroup(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueCommands("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-1"}}, queueops.Mutation{Command: []string{"changed"}}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 2 || queue.Commands[0].Command[0] != "changed" {
		t.Fatalf("changed queue = %#v", queue.Commands)
	}
	for _, command := range queue.Commands {
		if command.Matrix != nil {
			t.Fatalf("change kept matrix provenance on %q: %#v", command.ID, command.Matrix)
		}
	}
}

func TestChangeBatchForcesChangedJobInImportedQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	origin := &model.JobOrigin{RunID: "source-run", JobID: "source", Status: "failed"}
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{
		{ID: "accepted", Command: []string{"false"}, Accepted: true, Origin: origin},
		{ID: "untouched", Command: []string{"false"}, Accepted: true, Origin: origin},
	}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"accepted"}}, queueops.Mutation{Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	changed := loadCarryStateQueue(t, paths)
	if !changed.WorkflowImport {
		t.Fatal("change cleared the workflow import marker")
	}
	if !changed.Commands[0].Force || changed.Commands[0].Accepted {
		t.Fatalf("changed imported job = %#v", changed.Commands[0])
	}
	if changed.Commands[1].Force || !changed.Commands[1].Accepted {
		t.Fatalf("untouched imported job = %#v", changed.Commands[1])
	}
}

// TestChangeBatchForcesChangedCommandInOrdinaryQueue checks that a changed
// command drops the job's recorded result, and that a rename keeps it.
func TestChangeBatchForcesChangedCommandInOrdinaryQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{
		{ID: "job", Name: "job", Command: []string{"false"}},
		{ID: "other", Name: "other", Command: []string{"true"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"job"}}, queueops.Mutation{Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"other"}}, queueops.Mutation{SetJobName: "renamed"}); err != nil {
		t.Fatal(err)
	}
	commands := loadCarryStateQueue(t, paths).Commands
	if !commands[0].Force {
		t.Fatalf("changed command was not forced: %#v", commands[0])
	}
	if commands[1].Force {
		t.Fatalf("rename forced the job: %#v", commands[1])
	}
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

func TestChangeMatrixMemberRewritesBaseNameDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueWithDependent("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-1"}}, queueops.Mutation{Environment: []string{"X=1"}}); err != nil {
		t.Fatalf("Change: %v", err)
	}
	queue := loadCarryStateQueue(t, paths)
	if got := queuedCommandByName(t, queue, "evaluate").DependsOn; !reflect.DeepEqual(got, []string{"train-SEED1", "train-SEED2"}) {
		t.Fatalf("evaluate dependencies = %#v", got)
	}
}

func TestRemoveMatrixMemberRewritesBaseNameDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueWithDependent("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := queueEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-2"}}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	queue := loadCarryStateQueue(t, paths)
	if got := queuedCommandByName(t, queue, "evaluate").DependsOn; !reflect.DeepEqual(got, []string{"train-SEED1"}) {
		t.Fatalf("evaluate dependencies = %#v", got)
	}
}

func TestCopyRunToQueueHandlesMatrixBaseNameDependency(t *testing.T) {
	results := []model.JobResult{{ID: "seed-1", ExitCode: 0}, {ID: "seed-2", ExitCode: 1}, {ID: "evaluate-id", ExitCode: 1, Error: "blocked by failed dependency"}}
	tests := []struct {
		selection string
		jobIDs    []string
		wantIDs   []string
		wantDeps  []string
		wantGroup bool
	}{
		{selection: "all", wantIDs: []string{"seed-1", "seed-2", "evaluate-id"}, wantDeps: []string{"train"}, wantGroup: true},
		{selection: "failed", wantIDs: []string{"seed-2", "evaluate-id"}, wantDeps: []string{"train-SEED2"}},
	}
	for _, test := range tests {
		t.Run(test.selection, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, err := state.ResolveProjectPaths(baseDir, "default")
			if err != nil {
				t.Fatal(err)
			}
			writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: testMatrixQueueWithDependent("group")}, results)
			if _, err := queueEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: test.selection, JobIDs: test.jobIDs}); err != nil {
				t.Fatalf("Copy: %v", err)
			}
			queue := loadCarryStateQueue(t, paths)
			var ids []string
			for _, command := range queue.Commands {
				ids = append(ids, command.ID)
			}
			if !reflect.DeepEqual(ids, test.wantIDs) {
				t.Fatalf("copied IDs = %#v, want %#v", ids, test.wantIDs)
			}
			evaluate := queuedCommandByName(t, queue, "evaluate")
			if !reflect.DeepEqual(evaluate.DependsOn, test.wantDeps) {
				t.Fatalf("evaluate dependencies = %#v, want %#v", evaluate.DependsOn, test.wantDeps)
			}
			if hasGroup := queue.Commands[0].Matrix != nil; hasGroup != test.wantGroup {
				t.Fatalf("matrix provenance kept = %v, want %v", hasGroup, test.wantGroup)
			}
		})
	}
}

func TestCopyRunToQueueRejectsExcludedFailedMatrixMember(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: testMatrixQueueWithDependent("group")}, []model.JobResult{
		{ID: "seed-1", ExitCode: 0}, {ID: "seed-2", ExitCode: 1}, {ID: "evaluate-id", ExitCode: 0},
	})
	_, err = queueEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "job-id", JobIDs: []string{"evaluate-id"}})
	if err == nil || !strings.Contains(err.Error(), `excluded dependency "train-SEED2" did not succeed`) {
		t.Fatalf("Copy error = %v", err)
	}
}

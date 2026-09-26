package queueops

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCopyKeepsCompleteMatrixGroupUnderNewGroupID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: testMatrixQueueCommands("source-group")}, []model.JobResult{
		{ID: "seed-1", ExitCode: 0}, {ID: "seed-2", ExitCode: 0},
	})
	if _, err := testEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "all"}); err != nil {
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

func TestCopyClearsPartialMatrixGroup(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: testMatrixQueueCommands("source-group")}, []model.JobResult{
		{ID: "seed-1", ExitCode: 0}, {ID: "seed-2", ExitCode: 1},
	})
	if _, err := testEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "failed"}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "seed-2" || queue.Commands[0].Matrix != nil {
		t.Fatalf("partially copied matrix group = %#v", queue.Commands)
	}
}

func TestCopyDropsImportCarryFlagsFromSnapshot(t *testing.T) {
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
	if _, err := testEditor().Copy(baseDir, "default", "imported-run", queueedit.CopyRequest{Selection: "all"}); err != nil {
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

func TestCopyAppendToImportedQueueForcesCopiedJobs(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: []model.QueuedCommand{{ID: "copied", Command: []string{"true"}}}}, []model.JobResult{{ID: "copied", ExitCode: 0}})
	imported := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "imported", Command: []string{"true"}, Origin: &model.JobOrigin{RunID: "source-run", JobID: "copied"}}}}
	if err := state.WriteJSON(paths.QueueFile, imported); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "all", Append: true}); err != nil {
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

func TestCopyOverwriteClearsWorkflowImport(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: []model.QueuedCommand{{ID: "copied", Command: []string{"true"}}}}, []model.JobResult{{ID: "copied", ExitCode: 0}})
	if err := state.WriteJSON(paths.QueueFile, model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "imported", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "all", Overwrite: true}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if queue.WorkflowImport || len(queue.Commands) != 1 || queue.Commands[0].Force {
		t.Fatalf("overwritten queue = %#v", queue)
	}
}

func TestRemoveClearsRemainingMatrixProvenance(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueCommands("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-2"}}); err != nil {
		t.Fatal(err)
	}
	queue := loadCarryStateQueue(t, paths)
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "seed-1" || queue.Commands[0].Matrix != nil {
		t.Fatalf("queue after removing one matrix member = %#v", queue.Commands)
	}
}

func TestChangeClearsWholeMatrixGroup(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueCommands("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-1"}}, Mutation{Command: []string{"changed"}}); err != nil {
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

func TestChangeForcesChangedJobInImportedQueue(t *testing.T) {
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
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"accepted"}}, Mutation{Command: []string{"true"}}); err != nil {
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

// TestChangeForcesChangedCommandInOrdinaryQueue checks that a changed
// command drops the job's recorded result, and that a rename keeps it.
func TestChangeForcesChangedCommandInOrdinaryQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{
		{ID: "job", Name: "job", Command: []string{"false"}},
		{ID: "other", Name: "other", Command: []string{"true"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"job"}}, Mutation{Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"other"}}, Mutation{SetJobName: "renamed"}); err != nil {
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

func TestChangeMatrixMemberRewritesBaseNameDependency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueWithDependent("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-1"}}, Mutation{Environment: []string{"X=1"}}); err != nil {
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
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: testMatrixQueueWithDependent("group")}); err != nil {
		t.Fatal(err)
	}
	if _, err := testEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"seed-2"}}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	queue := loadCarryStateQueue(t, paths)
	if got := queuedCommandByName(t, queue, "evaluate").DependsOn; !reflect.DeepEqual(got, []string{"train-SEED1"}) {
		t.Fatalf("evaluate dependencies = %#v", got)
	}
}

func TestCopyHandlesMatrixBaseNameDependency(t *testing.T) {
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
			if _, err := testEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: test.selection, JobIDs: test.jobIDs}); err != nil {
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

func TestCopyRejectsExcludedFailedMatrixMember(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	writeCarryStateRun(t, paths, "source-run", model.Queue{Commands: testMatrixQueueWithDependent("group")}, []model.JobResult{
		{ID: "seed-1", ExitCode: 0}, {ID: "seed-2", ExitCode: 1}, {ID: "evaluate-id", ExitCode: 0},
	})
	_, err = testEditor().Copy(baseDir, "default", "source-run", queueedit.CopyRequest{Selection: "job-id", JobIDs: []string{"evaluate-id"}})
	if err == nil || !strings.Contains(err.Error(), `excluded dependency "train-SEED2" did not succeed`) {
		t.Fatalf("Copy error = %v", err)
	}
}

func loadCarryStateQueue(t *testing.T, paths state.ProjectPaths) model.Queue {
	t.Helper()
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	return queue
}

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
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: results}); err != nil {
		t.Fatal(err)
	}
}

func testMatrixQueueWithDependent(groupID string) []model.QueuedCommand {
	return append(testMatrixQueueCommands(groupID), model.QueuedCommand{ID: "evaluate-id", Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}})
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

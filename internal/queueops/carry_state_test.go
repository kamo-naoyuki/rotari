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

func TestCopyDropsMarksFromSnapshot(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "scalar", Command: []string{"true"}, MarkedStatus: model.StatusSuccess},
		{ID: "array", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2},
			TaskMarkedStatus: map[string]string{"array-1": model.StatusSuccess, "array-2": model.StatusUnfinished}},
	}}
	writeCarryStateRun(t, paths, "marked-run", snapshot, []model.JobResult{
		{ID: "scalar", ExitCode: 0, Accepted: true}, {ID: "array-1", ExitCode: 0, Accepted: true}, {ID: "array-2", ExitCode: 0},
	})
	if _, err := testEditor().Copy(baseDir, "default", "marked-run", queueedit.CopyRequest{Selection: "all"}); err != nil {
		t.Fatal(err)
	}
	for _, command := range loadCarryStateQueue(t, paths).Commands {
		if command.MarkedStatus != "" || command.TaskMarkedStatus != nil {
			t.Fatalf("copied command kept the source run's mark: %#v", command)
		}
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

// TestChangeKeepsRecordedResult checks that editing a job, even its command,
// leaves its origin and mark alone: only --status and --clear-status change
// the status a filtered run reads.
func TestChangeKeepsRecordedResult(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	origin := &model.JobOrigin{RunID: "source-run", JobID: "source", Status: "failed"}
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{
		{ID: "job", Name: "job", Command: []string{"false"}, Origin: origin, MarkedStatus: model.StatusSuccess},
		{ID: "array", Name: "array", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2},
			TaskMarkedStatus: map[string]string{"array-1": model.StatusSuccess}},
	}}); err != nil {
		t.Fatal(err)
	}
	change := func(id string, mutation Mutation) {
		t.Helper()
		if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{id}}, mutation); err != nil {
			t.Fatal(err)
		}
	}
	change("job", Mutation{Command: []string{"true"}, Environment: []string{"A=1"}, WorkingDirectory: "/elsewhere"})
	commands := loadCarryStateQueue(t, paths).Commands
	if commands[0].Origin == nil || commands[0].MarkedStatus != model.StatusSuccess {
		t.Fatalf("edited job lost its origin or mark: %#v", commands[0])
	}
	change("job", Mutation{Status: model.StatusFailed})
	change("array", Mutation{Status: model.StatusUnfinished})
	commands = loadCarryStateQueue(t, paths).Commands
	if commands[0].MarkedStatus != model.StatusFailed {
		t.Fatalf("job mark = %q, want failed", commands[0].MarkedStatus)
	}
	if commands[1].MarkedStatus != model.StatusUnfinished || commands[1].TaskMarkedStatus != nil {
		t.Fatalf("array marks = %#v, want the whole array marked unfinished", commands[1])
	}
	change("job", Mutation{ClearStatus: true})
	if commands = loadCarryStateQueue(t, paths).Commands; commands[0].MarkedStatus != "" {
		t.Fatalf("cleared mark = %q", commands[0].MarkedStatus)
	}
	for _, mutation := range []Mutation{{Status: "done"}, {Status: model.StatusFailed, ClearStatus: true}} {
		if _, err := testEditor().Change(baseDir, "default", "", model.CommandSelector{IDs: []string{"job"}}, mutation); err == nil {
			t.Fatalf("Change(%#v) succeeded", mutation)
		}
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

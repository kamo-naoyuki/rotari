package queueedit

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func testRun(commands []model.QueuedCommand, results ...model.JobResult) Run {
	byID := make(map[string]model.JobResult, len(results))
	for _, result := range results {
		byID[result.ID] = result
	}
	return Run{
		ID: "run-1", Snapshot: model.Queue{Commands: commands}, Results: byID, CWD: "/work",
		Timestamps: func(jobID string) (string, string) { return "s-" + jobID, "f-" + jobID },
	}
}

func sequentialIDs() func() string {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("new-%d", next)
	}
}

func commandIDs(queue model.Queue) []string {
	ids := make([]string, 0, len(queue.Commands))
	for _, command := range queue.Commands {
		ids = append(ids, command.ID)
	}
	return ids
}

func TestCopySelectsFailedJobsAndDropsSucceededPrerequisites(t *testing.T) {
	source := testRun([]model.QueuedCommand{
		{ID: "prep", Name: "prep", Command: []string{"true"}},
		{ID: "work", Name: "work", Command: []string{"false"}, DependsOn: []string{"prep"}},
	}, model.JobResult{ID: "prep", ExitCode: 0, AttemptID: "att-prep"}, model.JobResult{ID: "work", ExitCode: 1, AttemptID: "att-work"})
	queue, copied, err := Copy(model.Queue{}, "demo", source, CopyRequest{Selection: "failed"}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	if copied != 1 || !reflect.DeepEqual(commandIDs(queue), []string{"work"}) || len(queue.Commands[0].DependsOn) != 0 {
		t.Fatalf("queue = %#v, want only work without its succeeded prerequisite", queue.Commands)
	}
	origin := queue.Commands[0].Origin
	want := &model.JobOrigin{RunID: "run-1", JobID: "work", AttemptID: "att-work", Status: "failed", CWD: "/work", SubmittedAt: "s-work", FinishedAt: "f-work"}
	if !reflect.DeepEqual(origin, want) {
		t.Fatalf("origin = %#v, want %#v", origin, want)
	}
}

func TestCopyRejectsOmittedFailedPrerequisite(t *testing.T) {
	source := testRun([]model.QueuedCommand{
		{ID: "prep", Name: "prep", Command: []string{"true"}},
		{ID: "work", Name: "work", Command: []string{"true"}, DependsOn: []string{"prep"}},
	}, model.JobResult{ID: "prep", ExitCode: 1}, model.JobResult{ID: "work", ExitCode: 0})
	_, _, err := Copy(model.Queue{}, "demo", source, CopyRequest{Selection: "job-id", JobIDs: []string{"work"}}, sequentialIDs())
	if err == nil || err.Error() != `cannot copy job "work": excluded dependency "prep" did not succeed in run run-1` {
		t.Fatalf("error = %v", err)
	}
}

func TestCopyKeepsStageDependencyWhileAnyMemberIsCopied(t *testing.T) {
	source := testRun([]model.QueuedCommand{
		{ID: "a", Name: "a", Stage: "build", Command: []string{"true"}},
		{ID: "b", Name: "b", Stage: "build", Command: []string{"false"}},
		{ID: "test", Name: "test", Command: []string{"true"}, DependsOn: []string{"build"}},
	}, model.JobResult{ID: "a", ExitCode: 0}, model.JobResult{ID: "b", ExitCode: 1})
	queue, _, err := Copy(model.Queue{}, "demo", source, CopyRequest{Selection: "failed,unfinished"}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(commandIDs(queue), []string{"b", "test"}) || !reflect.DeepEqual(queue.Commands[1].DependsOn, []string{"build"}) {
		t.Fatalf("queue = %#v, want b and test still waiting on the build stage", queue.Commands)
	}
}

func TestCopyRewritesPartialMatrixDependency(t *testing.T) {
	group := func(value string) *model.MatrixSpec {
		return &model.MatrixSpec{GroupID: "g1", BaseName: "train", Dimensions: []model.MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}, Values: []model.MatrixValue{{Name: "SEED", Value: value}}}
	}
	source := testRun([]model.QueuedCommand{
		{ID: "t1", Name: "train-SEED1", Environment: []string{"SEED=1"}, Command: []string{"true"}, Matrix: group("1")},
		{ID: "t2", Name: "train-SEED2", Environment: []string{"SEED=2"}, Command: []string{"true"}, Matrix: group("2")},
		{ID: "eval", Name: "eval", Command: []string{"true"}, DependsOn: []string{"train"}},
	}, model.JobResult{ID: "t1", ExitCode: 0}, model.JobResult{ID: "t2", ExitCode: 1})
	queue, _, err := Copy(model.Queue{}, "demo", source, CopyRequest{Selection: "failed,unfinished"}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(commandIDs(queue), []string{"t2", "eval"}) || queue.Commands[0].Matrix != nil || !reflect.DeepEqual(queue.Commands[1].DependsOn, []string{"train-SEED2"}) {
		t.Fatalf("queue = %#v, want the incomplete group cleared and eval rewritten to train-SEED2", queue.Commands)
	}

	queue, _, err = Copy(model.Queue{}, "demo", source, CopyRequest{Selection: "all"}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	if queue.Commands[0].Matrix == nil || queue.Commands[0].Matrix.GroupID != "new-1" || queue.Commands[1].Matrix.GroupID != "new-1" {
		t.Fatalf("queue = %#v, want a complete group with a new group ID", queue.Commands)
	}
}

func TestCopyAttemptNarrowsArray(t *testing.T) {
	array := &model.ArraySpec{First: 1, Last: 3}
	source := testRun([]model.QueuedCommand{{ID: "arr", Name: "arr", Command: []string{"run"}, Array: array}},
		model.JobResult{ID: "arr-1", ExitCode: 0, AttemptID: "att-1"}, model.JobResult{ID: "arr-2", ExitCode: 1, AttemptID: "att-2"})
	queue, _, err := Copy(model.Queue{}, "demo", source, CopyRequest{Selection: "job-id", Attempts: []Attempt{{ID: "att-2-old", JobID: "arr-2"}}}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	command := queue.Commands[0]
	if !reflect.DeepEqual(command.Array, &model.ArraySpec{First: 2, Last: 2, Tasks: []int{2}}) {
		t.Fatalf("array = %#v, want only task 2", command.Array)
	}
	if origin := command.TaskOrigins["arr-2"]; origin == nil || origin.AttemptID != "att-2-old" || origin.Status != "failed" {
		t.Fatalf("task origins = %#v, want the selected attempt", command.TaskOrigins)
	}
	if command.Origin.SubmittedAt != "s-arr-2" || command.Origin.FinishedAt != "f-arr-2" {
		t.Fatalf("origin = %#v, want copied task times", command.Origin)
	}

	if _, _, err := Copy(model.Queue{}, "demo", source, CopyRequest{Selection: "job-id", Attempts: []Attempt{{ID: "att-x", JobID: "missing"}}}, sequentialIDs()); err == nil || err.Error() != `attempt "att-x" job "missing" not found in run run-1` {
		t.Fatalf("error = %v", err)
	}
}

func TestCopyDestinationRules(t *testing.T) {
	source := testRun([]model.QueuedCommand{{ID: "job", Name: "job", Command: []string{"true"}}}, model.JobResult{ID: "job", ExitCode: 0})
	existing := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{ID: "job", Name: "other", Command: []string{"true"}}}}

	if _, _, err := Copy(existing, "demo", source, CopyRequest{Selection: "all"}, sequentialIDs()); err == nil || !strings.Contains(err.Error(), `project "demo" has queued jobs`) {
		t.Fatalf("error = %v, want queued-jobs error", err)
	}
	queue, _, err := Copy(existing, "demo", source, CopyRequest{Selection: "all", Append: true}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(commandIDs(queue), []string{"job", "new-1"}) || !queue.Commands[1].Force || !queue.WorkflowImport {
		t.Fatalf("queue = %#v, want appended job with a new ID forced into the imported queue", queue)
	}
	queue, _, err = Copy(existing, "demo", source, CopyRequest{Selection: "all", Overwrite: true}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(commandIDs(queue), []string{"job"}) || queue.Commands[0].Force || queue.WorkflowImport {
		t.Fatalf("queue = %#v, want only the copied job in a plain queue", queue)
	}

	for request, want := range map[string]CopyRequest{
		"job IDs not found in run run-1: nope":     {Selection: "job-id", JobIDs: []string{"nope"}},
		"run run-1 has no jobs matching selection": {Selection: "failed"},
	} {
		if _, _, err := Copy(model.Queue{}, "demo", source, want, sequentialIDs()); err == nil || err.Error() != request {
			t.Fatalf("error = %v, want %q", err, request)
		}
	}
}

package workflow

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	sourceRunID = "20260925-000000-00000000"
	laterRunID  = "20260925-010000-00000000"
)

type fakeSourceStore struct {
	runs     map[string]SourceRun
	attempts map[string]model.JobResult
}

func (store fakeSourceStore) Run(runID string) (SourceRun, error) {
	run, ok := store.runs[runID]
	if !ok {
		return SourceRun{}, errors.New("missing run " + runID)
	}
	return run, nil
}

func (store fakeSourceStore) Attempt(_, _, attemptID string, _ []string) (model.JobResult, bool, error) {
	result, ok := store.attempts[attemptID]
	if !ok {
		return model.JobResult{}, false, errors.New("attempt not found")
	}
	return result, result.ID != "", nil
}

func (store fakeSourceStore) AttemptTimestamps(_, jobID, attemptID string) (string, string) {
	return "s-" + attemptID, "f-" + attemptID
}

func noTimestamps(string) (string, string) { return "", "" }

func sourceStore(commands []model.QueuedCommand, results ...model.JobResult) (fakeSourceStore, map[string]string) {
	attempts := make(map[string]model.JobResult)
	ids := make(map[string]string)
	for index := range results {
		results[index].AttemptID = state.MakeAttemptID(sourceRunID, results[index].ID, 1)
		ids[results[index].ID] = results[index].AttemptID
		attempts[results[index].AttemptID] = model.JobResult{}
	}
	run := SourceRun{ID: sourceRunID, Queue: model.Queue{Commands: commands}, Summary: model.RunSummary{Results: results}, CWD: "/work", JobTimestamps: noTimestamps}
	return fakeSourceStore{runs: map[string]SourceRun{sourceRunID: run}, attempts: attempts}, ids
}

func reconcileManifest(t *testing.T, store fakeSourceStore, jobs ...Job) (model.Queue, []RemovedJob, error) {
	t.Helper()
	manifest := Manifest{Version: Version, Source: &Source{Project: "demo", RunIDs: []string{sourceRunID}}, Jobs: jobs}
	next := 0
	queue, err := Compile(manifest, func() string { next++; return "new-" + string(rune('0'+next)) })
	if err != nil {
		t.Fatal(err)
	}
	return Reconcile(manifest, queue, store)
}

func TestReconcileLinksSourcesAndMarksEditedStatuses(t *testing.T) {
	store, attempts := sourceStore([]model.QueuedCommand{
		{ID: "ok-id", Name: "ok", Command: []string{"true"}},
		{ID: "bad-id", Name: "bad", Command: []string{"false"}},
		{ID: "edit-id", Name: "edit", Command: []string{"old"}},
		{ID: "redo-id", Name: "redo", Command: []string{"true"}},
	},
		model.JobResult{ID: "ok-id", ExitCode: 0},
		model.JobResult{ID: "bad-id", ExitCode: 1},
		model.JobResult{ID: "edit-id", ExitCode: 0},
		model.JobResult{ID: "redo-id", ExitCode: 0},
	)
	queue, removed, err := reconcileManifest(t, store,
		Job{Name: "ok", Command: []string{"true"}, AttemptID: attempts["ok-id"], Status: "success"},
		Job{Name: "bad", Command: []string{"false"}, AttemptID: attempts["bad-id"], Status: "success"},
		Job{Name: "edit", Command: []string{"new"}, AttemptID: attempts["edit-id"], Status: "success"},
		Job{Name: "redo", Command: []string{"true"}, AttemptID: attempts["redo-id"], Status: "unfinished"},
		Job{Name: "fresh", Command: []string{"true"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %#v", removed)
	}
	ok, bad, edit, redo, fresh := queue.Commands[0], queue.Commands[1], queue.Commands[2], queue.Commands[3], queue.Commands[4]
	if ok.ID != "ok-id" || ok.MarkedStatus != "" || ok.Origin == nil || ok.Origin.Status != "success" || ok.Origin.CWD != "/work" || ok.Origin.SubmittedAt != "s-"+attempts["ok-id"] {
		t.Fatalf("unchanged success = %#v / %#v, want linked without a mark", ok, ok.Origin)
	}
	if bad.MarkedStatus != model.StatusSuccess || bad.Origin.Status != "failed" {
		t.Fatalf("accepted failure = %#v, want marked success", bad)
	}
	if edit.ID != "edit-id" || edit.MarkedStatus != "" || edit.Origin == nil || edit.Origin.Status != "success" {
		t.Fatalf("changed command = %#v, want its source result kept", edit)
	}
	if redo.MarkedStatus != model.StatusUnfinished || redo.Origin == nil {
		t.Fatalf("success marked unfinished = %#v", redo)
	}
	if fresh.Origin != nil || fresh.MarkedStatus != "" {
		t.Fatalf("new job = %#v, want no origin or mark", fresh)
	}
}

// A group's job-level status is an aggregate, so apart from accepting every
// leaf with success it does not apply to leaves that instances omit, even
// when instances is empty; those leaves keep their source result.
func TestReconcileGroupStatusDoesNotOverrideUnlistedLeaves(t *testing.T) {
	dimensions := []model.MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	combinations := model.ExpandMatrix(dimensions)
	commands := make([]model.QueuedCommand, 0, len(combinations)+1)
	for index, combination := range combinations {
		commands = append(commands, model.QueuedCommand{
			ID: fmt.Sprintf("seed-%d", index+1), Name: model.MatrixJobName("train", combination), Command: []string{"train"},
			Environment: model.MatrixEnvironment(nil, combination),
			Matrix:      &model.MatrixSpec{GroupID: "source-group", Dimensions: dimensions, Values: combination, BaseName: "train"},
		})
	}
	commands = append(commands, model.QueuedCommand{ID: "tasks", Name: "tasks", Command: []string{"work"}, Array: &model.ArraySpec{First: 1, Last: 2}})
	store, attempts := sourceStore(commands,
		model.JobResult{ID: "seed-1", ExitCode: 0},
		model.JobResult{ID: "seed-2", ExitCode: 3},
		model.JobResult{ID: "tasks-1", ExitCode: 0},
		model.JobResult{ID: "tasks-2", ExitCode: 2},
	)
	queue, _, err := reconcileManifest(t, store,
		Job{Name: "train", Command: []string{"train"}, Matrix: []string{"SEED=1,2"}, AttemptID: attempts["seed-1"], Status: "unfinished"},
		Job{Name: "tasks", Command: []string{"work"}, Array: "1-2", AttemptID: attempts["tasks-1"], Status: "unfinished"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range queue.Commands {
		if command.MarkedStatus != "" || len(command.TaskMarkedStatus) != 0 {
			t.Errorf("%s marked %q / %v, want its source results unmarked", command.Name, command.MarkedStatus, command.TaskMarkedStatus)
		}
	}
}

func TestReconcileCountsMatrixExcludedMembers(t *testing.T) {
	dimensions := []model.MatrixDimension{
		{Name: "SEED", Values: []string{"1", "2"}},
		{Name: "MODEL", Values: []string{"small", "large"}},
	}
	exclusions := []model.MatrixExclusion{{Values: []model.MatrixValue{{Name: "SEED", Value: "2"}, {Name: "MODEL", Value: "large"}}}}
	combinations, exclusions, err := model.ExpandMatrixWithExclusions(dimensions, exclusions)
	if err != nil {
		t.Fatal(err)
	}
	sourceCommands := make([]model.QueuedCommand, 0, len(combinations)+1)
	results := make([]model.JobResult, 0, len(combinations)+1)
	for index, combination := range combinations {
		id := fmt.Sprintf("matrix-%d", index)
		sourceCommands = append(sourceCommands, model.QueuedCommand{
			ID: id, Name: model.MatrixJobName("train", combination), Command: []string{"train"},
			Environment: model.MatrixEnvironment(nil, combination),
			Matrix: &model.MatrixSpec{
				GroupID: "source-group", Dimensions: dimensions, Values: combination, Exclusions: exclusions, BaseName: "train",
			},
		})
		results = append(results, model.JobResult{ID: id, ExitCode: 0})
	}
	sourceCommands = append(sourceCommands, model.QueuedCommand{ID: "tail", Name: "tail", Command: []string{"true"}})
	results = append(results, model.JobResult{ID: "tail", ExitCode: 0})
	store, attempts := sourceStore(sourceCommands, results...)
	queue, removed, err := reconcileManifest(t, store,
		Job{
			Name: "train", Command: []string{"train"}, Matrix: []string{"SEED=1,2", "MODEL=small,large"},
			MatrixExclude: []map[string]string{{"SEED": "2", "MODEL": "large"}}, AttemptID: attempts["matrix-0"],
		},
		Job{Name: "tail", Command: []string{"true"}, AttemptID: attempts["tail"]},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %#v, want none", removed)
	}
	if len(queue.Commands) != len(combinations)+1 {
		t.Fatalf("reconciled commands = %d, want %d", len(queue.Commands), len(combinations)+1)
	}
	if queue.Commands[len(queue.Commands)-1].ID != "tail" || queue.Commands[len(queue.Commands)-1].Origin == nil {
		t.Fatalf("tail command = %#v, want its source origin", queue.Commands[len(queue.Commands)-1])
	}
}

func TestReconcileListsExcludedMatrixSourceMemberAsRemoved(t *testing.T) {
	dimensions := []model.MatrixDimension{
		{Name: "SEED", Values: []string{"1", "2"}},
		{Name: "MODEL", Values: []string{"small", "large"}},
	}
	combinations := model.ExpandMatrix(dimensions)
	sourceCommands := make([]model.QueuedCommand, 0, len(combinations))
	results := make([]model.JobResult, 0, len(combinations))
	for index, combination := range combinations {
		id := fmt.Sprintf("matrix-%d", index)
		sourceCommands = append(sourceCommands, model.QueuedCommand{
			ID: id, Name: model.MatrixJobName("train", combination), Command: []string{"train"},
			Environment: model.MatrixEnvironment(nil, combination),
			Matrix: &model.MatrixSpec{
				GroupID: "source-group", Dimensions: dimensions, Values: combination, BaseName: "train",
			},
		})
		results = append(results, model.JobResult{ID: id, ExitCode: 0})
	}
	store, attempts := sourceStore(sourceCommands, results...)
	_, removed, err := reconcileManifest(t, store, Job{
		Name: "train", Command: []string{"train"}, Matrix: []string{"SEED=1,2", "MODEL=small,large"},
		MatrixExclude: []map[string]string{{"SEED": "2", "MODEL": "large"}}, AttemptID: attempts["matrix-0"],
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []RemovedJob{{RunID: sourceRunID, JobID: "matrix-3", Name: "train-SEED2-MODELlarge", Command: []string{"train"}}}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed = %#v, want %#v", removed, want)
	}
}

func TestReconcileCarriesExcludedMatrixMembersAcrossRunExport(t *testing.T) {
	dimensions := []model.MatrixDimension{
		{Name: "SEED", Values: []string{"1", "2"}},
		{Name: "MODEL", Values: []string{"small", "large"}},
	}
	combinations := model.ExpandMatrix(dimensions)
	sourceCommands := make([]model.QueuedCommand, 0, len(combinations))
	results := make([]model.JobResult, 0, len(combinations))
	for index, combination := range combinations {
		id := fmt.Sprintf("matrix-%d", index)
		sourceCommands = append(sourceCommands, model.QueuedCommand{
			ID: id, Name: model.MatrixJobName("train", combination), Command: []string{"train"},
			Environment: model.MatrixEnvironment(nil, combination),
			Matrix: &model.MatrixSpec{
				GroupID: "original-group", Dimensions: dimensions, Values: combination, BaseName: "train",
			},
		})
		results = append(results, model.JobResult{ID: id, ExitCode: 0})
	}
	store, attempts := sourceStore(sourceCommands, results...)
	job := Job{
		Name: "train", Command: []string{"train"}, Matrix: []string{"SEED=1,2", "MODEL=small,large"},
		MatrixExclude: []map[string]string{{"SEED": "2", "MODEL": "large"}}, AttemptID: attempts["matrix-0"],
	}
	firstQueue, _, err := reconcileManifest(t, store, job)
	if err != nil {
		t.Fatal(err)
	}
	carriedResults := make([]model.JobResult, 0, len(firstQueue.Commands))
	for _, command := range firstQueue.Commands {
		carriedResults = append(carriedResults, model.JobResult{ID: command.ID, AttemptID: attempts[command.ID], ExitCode: 0})
	}
	secondRun := SourceRun{
		ID: laterRunID, Queue: model.Queue{Commands: firstQueue.Commands},
		Summary: model.RunSummary{Results: carriedResults}, CWD: "/work", JobTimestamps: noTimestamps,
	}
	store.runs[laterRunID] = secondRun
	exported, err := MergeRuns("demo", []SourceRun{secondRun})
	if err != nil {
		t.Fatalf("MergeRuns: %v", err)
	}
	queue, err := Compile(exported, sequentialIDs())
	if err != nil {
		t.Fatalf("Compile exported manifest: %v", err)
	}
	reconciled, _, err := Reconcile(exported, queue, store)
	if err != nil {
		t.Fatalf("Reconcile exported manifest: %v", err)
	}
	if len(reconciled.Commands) != len(firstQueue.Commands) {
		t.Fatalf("reconciled commands = %d, want %d", len(reconciled.Commands), len(firstQueue.Commands))
	}
	for index, command := range reconciled.Commands {
		if command.ID != firstQueue.Commands[index].ID || command.Origin == nil {
			t.Errorf("command %d = %#v, want carried ID %q and origin", index, command, firstQueue.Commands[index].ID)
		}
	}
}

func TestMergeRunsKeepsExcludedHistoricalMatrixMemberAsStandalone(t *testing.T) {
	dimensions := []model.MatrixDimension{
		{Name: "SEED", Values: []string{"1", "2"}},
		{Name: "MODEL", Values: []string{"small", "large"}},
	}
	combinations := model.ExpandMatrix(dimensions)
	sourceCommands := make([]model.QueuedCommand, 0, len(combinations))
	results := make([]model.JobResult, 0, len(combinations))
	for index, combination := range combinations {
		id := fmt.Sprintf("matrix-%d", index)
		sourceCommands = append(sourceCommands, model.QueuedCommand{
			ID: id, Name: model.MatrixJobName("train", combination), Command: []string{"train"},
			Environment: model.MatrixEnvironment(nil, combination),
			Matrix: &model.MatrixSpec{
				GroupID: "original-group", Dimensions: dimensions, Values: combination, BaseName: "train",
			},
		})
		results = append(results, model.JobResult{ID: id, ExitCode: 0})
	}
	sourceCommands = append(sourceCommands, model.QueuedCommand{ID: "evaluate", Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}})
	results = append(results, model.JobResult{ID: "evaluate", ExitCode: 0})
	store, attempts := sourceStore(sourceCommands, results...)
	firstQueue, _, err := reconcileManifest(t, store,
		Job{
			Name: "train", Command: []string{"train"}, Matrix: []string{"SEED=1,2", "MODEL=small,large"},
			MatrixExclude: []map[string]string{{"SEED": "2", "MODEL": "large"}}, AttemptID: attempts["matrix-0"],
		},
		Job{Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}, AttemptID: attempts["evaluate"]},
	)
	if err != nil {
		t.Fatal(err)
	}
	carriedResults := make([]model.JobResult, 0, len(firstQueue.Commands))
	for _, command := range firstQueue.Commands {
		carriedResults = append(carriedResults, model.JobResult{ID: command.ID, AttemptID: attempts[command.ID], ExitCode: 0})
	}
	secondRun := SourceRun{
		ID: laterRunID, Queue: model.Queue{Commands: firstQueue.Commands},
		Summary: model.RunSummary{Results: carriedResults}, CWD: "/work", JobTimestamps: noTimestamps,
	}
	merged, err := MergeRuns("demo", []SourceRun{secondRun, store.runs[sourceRunID]})
	if err != nil {
		t.Fatalf("MergeRuns: %v", err)
	}
	var matrixJob, historicalJob, evaluateJob *Job
	for index := range merged.Jobs {
		job := &merged.Jobs[index]
		switch job.Name {
		case "train":
			matrixJob = job
		case "train-SEED2-MODELlarge":
			historicalJob = job
		case "evaluate":
			evaluateJob = job
		}
	}
	if matrixJob == nil || !reflect.DeepEqual(matrixJob.MatrixExclude, []map[string]string{{"MODEL": "large", "SEED": "2"}}) || historicalJob == nil || len(historicalJob.Matrix) != 0 || evaluateJob == nil || !reflect.DeepEqual(evaluateJob.DependsOn, []string{"train"}) {
		t.Fatalf("merged jobs = %#v, want excluded matrix, standalone historical leaf, and intact base-name dependency", merged.Jobs)
	}
	queue, err := Compile(merged, sequentialIDs())
	if err != nil {
		t.Fatalf("Compile merged manifest: %v", err)
	}
	if len(queue.Commands) != 5 {
		t.Fatalf("merged queue commands = %d, want 5", len(queue.Commands))
	}
}

func TestReconcileRejectsUnreachableAndUnfinishedAttempts(t *testing.T) {
	store, _ := sourceStore([]model.QueuedCommand{{ID: "job-id", Name: "job", Command: []string{"true"}}})
	foreign := state.MakeAttemptID("20260101-000000-00000000", "other", 1)
	if _, _, err := reconcileManifest(t, store, Job{Name: "job", Command: []string{"true"}, AttemptID: foreign}); err == nil || !strings.Contains(err.Error(), "is not reachable from the exported source runs") {
		t.Fatalf("error = %v, want unreachable attempt", err)
	}
	pending := state.MakeAttemptID(sourceRunID, "job-id", 1)
	store.attempts[pending] = model.JobResult{}
	if _, _, err := reconcileManifest(t, store, Job{Name: "job", Command: []string{"true"}, AttemptID: pending}); err == nil || !strings.Contains(err.Error(), "has no completed result") {
		t.Fatalf("error = %v, want unfinished attempt", err)
	}
}

func TestReconcileListsRemovedSourceJobs(t *testing.T) {
	store, attempts := sourceStore([]model.QueuedCommand{
		{ID: "kept-id", Name: "kept", Command: []string{"true"}},
		{ID: "gone-id", Name: "gone", Command: []string{"true"}},
	}, model.JobResult{ID: "kept-id"}, model.JobResult{ID: "gone-id"})
	_, removed, err := reconcileManifest(t, store, Job{Name: "kept", Command: []string{"true"}, AttemptID: attempts["kept-id"], Status: "success"})
	if err != nil {
		t.Fatal(err)
	}
	want := []RemovedJob{{RunID: sourceRunID, JobID: "gone-id", Name: "gone", Command: []string{"true"}}}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed = %#v, want %#v", removed, want)
	}
}

func TestReconcileWithoutSourceLeavesQueue(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "a"}}}
	got, removed, err := Reconcile(Manifest{}, queue, fakeSourceStore{})
	if err != nil || !reflect.DeepEqual(got, queue) || removed != nil {
		t.Fatalf("Reconcile() = %#v, %#v, %v", got, removed, err)
	}
}

func TestMergeRunsUsesLatestSnapshot(t *testing.T) {
	times := func(finished string) func(string) (string, string) {
		return func(string) (string, string) { return "", finished }
	}
	older := SourceRun{ID: sourceRunID, Queue: model.Queue{Commands: []model.QueuedCommand{
		{ID: "a", Name: "a", Command: []string{"old"}}, {ID: "b", Name: "b", Command: []string{"true"}},
	}}, Summary: model.RunSummary{Results: []model.JobResult{{ID: "a", ExitCode: 1}, {ID: "b", ExitCode: 0}}}, JobTimestamps: times("2026-09-25T00:00:00Z")}
	newer := SourceRun{ID: laterRunID, Queue: model.Queue{Commands: []model.QueuedCommand{
		{ID: "a", Name: "a", Command: []string{"new"}},
	}}, Summary: model.RunSummary{Results: []model.JobResult{{ID: "a", ExitCode: 0}}}, JobTimestamps: times("2026-09-25T01:00:00Z")}
	manifest, err := MergeRuns("demo", []SourceRun{newer, older})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Source == nil || !reflect.DeepEqual(manifest.Source.RunIDs, []string{laterRunID, sourceRunID}) {
		t.Fatalf("source = %#v", manifest.Source)
	}
	if len(manifest.Jobs) != 2 || manifest.Jobs[0].Name != "a" || !reflect.DeepEqual(manifest.Jobs[0].Command, []string{"new"}) || manifest.Jobs[0].Status != "success" || manifest.Jobs[1].Name != "b" {
		t.Fatalf("jobs = %#v, want latest a then b", manifest.Jobs)
	}
}

func TestFlattenQueueDefaults(t *testing.T) {
	queue := FlattenQueueDefaults(model.Queue{DefaultExecutor: "slurm", DefaultExecutorOptions: []string{"-p", "gpu"}, Commands: []model.QueuedCommand{
		{ID: "a"}, {ID: "b", Executor: "local", ExecutorOptions: []string{"x"}},
	}})
	if queue.DefaultExecutor != "" || queue.DefaultExecutorOptions != nil ||
		queue.Commands[0].Executor != "slurm" || !reflect.DeepEqual(queue.Commands[0].ExecutorOptions, []string{"-p", "gpu"}) ||
		queue.Commands[1].Executor != "local" || !reflect.DeepEqual(queue.Commands[1].ExecutorOptions, []string{"x"}) {
		t.Fatalf("queue = %#v", queue)
	}
}

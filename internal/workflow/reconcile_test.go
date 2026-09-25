package workflow

import (
	"errors"
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

func TestReconcileCarriesForcesAndAccepts(t *testing.T) {
	store, attempts := sourceStore([]model.QueuedCommand{
		{ID: "ok-id", Name: "ok", Command: []string{"true"}},
		{ID: "bad-id", Name: "bad", Command: []string{"false"}},
		{ID: "edit-id", Name: "edit", Command: []string{"old"}},
	},
		model.JobResult{ID: "ok-id", ExitCode: 0},
		model.JobResult{ID: "bad-id", ExitCode: 1},
		model.JobResult{ID: "edit-id", ExitCode: 0},
	)
	queue, removed, err := reconcileManifest(t, store,
		Job{Name: "ok", Command: []string{"true"}, AttemptID: attempts["ok-id"], Status: "success"},
		Job{Name: "bad", Command: []string{"false"}, AttemptID: attempts["bad-id"], Status: "success"},
		Job{Name: "edit", Command: []string{"new"}, AttemptID: attempts["edit-id"], Status: "success"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !queue.WorkflowImport || len(removed) != 0 {
		t.Fatalf("queue = %#v, removed = %#v", queue, removed)
	}
	ok, bad, edit := queue.Commands[0], queue.Commands[1], queue.Commands[2]
	if ok.ID != "ok-id" || ok.Force || ok.Accepted || ok.Origin == nil || ok.Origin.Status != "success" || ok.Origin.CWD != "/work" || ok.Origin.SubmittedAt != "s-"+attempts["ok-id"] {
		t.Fatalf("unchanged success = %#v / %#v, want carried", ok, ok.Origin)
	}
	if bad.Force || !bad.Accepted || bad.Origin.Status != "failed" {
		t.Fatalf("accepted failure = %#v, want manual acceptance", bad)
	}
	if !edit.Force || edit.Origin != nil {
		t.Fatalf("changed command = %#v, want forced without origin", edit)
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

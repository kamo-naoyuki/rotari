package run

import (
	"errors"
	"sync"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type testExecutor struct {
	name        string
	submitError error
	array       bool
	options     [][]string
}

func (fake *testExecutor) Name() string { return fake.name }

func (fake *testExecutor) Submit(_ string, job model.JobSpec, options []string) (executor.JobHandle, error) {
	fake.options = append(fake.options, append([]string(nil), options...))
	if fake.submitError != nil {
		return executor.JobHandle{}, fake.submitError
	}
	return executor.JobHandle{Job: job, Native: job.ID}, nil
}

func (fake *testExecutor) SubmitArray(_ string, jobs []model.JobSpec, options []string) ([]executor.JobHandle, error) {
	fake.options = append(fake.options, append([]string(nil), options...))
	if fake.submitError != nil {
		return nil, fake.submitError
	}
	handles := make([]executor.JobHandle, 0, len(jobs))
	for _, job := range jobs {
		handles = append(handles, executor.JobHandle{Job: job, Native: job.ID})
	}
	return handles, nil
}

func (fake *testExecutor) Wait(_ string, handle executor.JobHandle) model.JobResult {
	return model.JobResult{ID: handle.Job.ID, Command: handle.Job.Command, ExitCode: 0}
}

func TestCompleteArrayGroup(t *testing.T) {
	task := func(id int) model.JobSpec {
		return model.JobSpec{ID: "array-" + string(rune('0'+id)), ArrayTaskID: &id}
	}
	if !CompleteArrayGroup([]model.JobSpec{task(1), task(2)}, 1, 2) {
		t.Fatal("CompleteArrayGroup() rejected a complete group")
	}
	for name, jobs := range map[string][]model.JobSpec{
		"missing":      {task(1)},
		"duplicate":    {task(1), task(1)},
		"out of range": {task(1), task(3)},
		"not array":    {{ID: "plain"}, task(2)},
	} {
		t.Run(name, func(t *testing.T) {
			if CompleteArrayGroup(jobs, 1, 2) {
				t.Fatalf("CompleteArrayGroup() accepted %s", name)
			}
		})
	}
}

func TestRemoveAndFinalizeResults(t *testing.T) {
	jobs := []model.JobSpec{{ID: "done", Command: []string{"true"}}, {ID: "blocked", Command: []string{"run"}}}
	results := map[string]model.JobResult{"done": {ID: "done", ExitCode: 0}}
	remaining := RemoveFinishedJobs(jobs, results)
	if len(remaining) != 1 || remaining[0].ID != "blocked" {
		t.Fatalf("RemoveFinishedJobs() = %#v", remaining)
	}
	FinalizePendingResults(remaining, results)
	result := results["blocked"]
	if result.ExitCode != 1 || result.Error != "blocked by failed dependency" || result.Command[0] != "run" {
		t.Fatalf("finalized result = %#v", result)
	}
	completed, succeeded, failed := SummarizeResults(results)
	if completed != 2 || succeeded != 1 || failed != 1 {
		t.Fatalf("SummarizeResults() = %d, %d, %d", completed, succeeded, failed)
	}
}

func TestExpandArrayPlanAndApplyCarriedOrigins(t *testing.T) {
	commands := []model.QueuedCommand{
		{ID: "plain"},
		{ID: "array", Array: &model.ArraySpec{First: 1, Last: 2}},
	}
	jobs := []model.JobSpec{{ID: "array-1", ArrayGroup: "array"}, {ID: "array-2", ArrayGroup: "array"}}
	execute := map[string]bool{"plain": true, "array": true}
	ExpandArrayPlan(commands, jobs, execute)
	if execute["array"] || !execute["plain"] || !execute["array-1"] || !execute["array-2"] {
		t.Fatalf("execute = %#v", execute)
	}

	origin := &model.JobOrigin{RunID: "run-1", JobID: "array-1"}
	ApplyCarriedOrigins(commands, map[string]*model.JobOrigin{"array-1": origin})
	if commands[1].TaskOrigins["array-1"] != origin {
		t.Fatalf("task origins = %#v", commands[1].TaskOrigins)
	}
}

func TestPrepareJobEnvironments(t *testing.T) {
	taskID := 3
	jobs := []model.JobSpec{{ID: "job-1", Name: "train", Executor: "slurm", ArrayTaskID: &taskID, ArrayFirst: 1, ArrayLast: 4, ArraySize: 4, Environment: []string{"CUSTOM=job", "ROTARI_RUN_ID=old"}}}
	names := EnvironmentNames{
		BaseDir: "ROTARI_BASE", ProjectName: "ROTARI_PROJECT", RunID: "ROTARI_RUN_ID", JobID: "ROTARI_JOB_ID",
		Executor: "ROTARI_EXECUTOR", Bin: "ROTARI_BIN", RunDir: "ROTARI_RUN_DIR", JobDir: "ROTARI_JOB_DIR", CWD: "ROTARI_CWD",
		JobName: "ROTARI_JOB_NAME", ArrayTaskID: "ROTARI_ARRAY_TASK_ID", ArrayFirst: "ROTARI_ARRAY_FIRST", ArrayLast: "ROTARI_ARRAY_LAST", ArraySize: "ROTARI_ARRAY_SIZE",
		RunName: "ROTARI_RUN_NAME", LocalConcurrency: "ROTARI_LOCAL_CONCURRENCY", BatchConcurrency: "ROTARI_BATCH_CONCURRENCY", Retry: "ROTARI_RETRY", ExecutorOptions: "ROTARI_EXECUTOR_OPTIONS",
	}
	PrepareJobEnvironments(jobs, EnvironmentConfig{
		Names: names, BaseDir: "/base", ProjectName: "demo", RunID: "run-1", RunDir: "/base/run-1", RunName: "nightly", Bin: "/bin/rotari", CWD: "/work",
		LocalConcurrency: 2, BatchConcurrency: 8, Retry: 1, ExecutorOptions: []string{"--partition", "short"}, Inherited: map[string]string{"CUSTOM": "inherited", "PATH": "/bin"},
		JobDir: func(string, model.JobSpec) (string, error) { return "/base/run-1/job-1", nil },
	})
	values := make(map[string]string)
	for _, entry := range jobs[0].Environment {
		for index := 0; index < len(entry); index++ {
			if entry[index] == '=' {
				values[entry[:index]] = entry[index+1:]
				break
			}
		}
	}
	for name, want := range map[string]string{"ROTARI_PROJECT": "demo", "ROTARI_RUN_ID": "run-1", "ROTARI_ARRAY_TASK_ID": "3", "ROTARI_ARRAY_SIZE": "4", "ROTARI_EXECUTOR_OPTIONS": "--partition short", "CUSTOM": "inherited", "PATH": "/bin"} {
		if values[name] != want {
			t.Errorf("environment[%q] = %q, want %q", name, values[name], want)
		}
	}
}

func TestPrepareJobEnvironmentsSkipsJobDirectoryErrors(t *testing.T) {
	jobs := []model.JobSpec{{ID: "job-1"}}
	PrepareJobEnvironments(jobs, EnvironmentConfig{JobDir: func(string, model.JobSpec) (string, error) { return "", errors.New("missing") }})
	if len(jobs[0].Environment) != 0 {
		t.Fatalf("environment = %#v", jobs[0].Environment)
	}
}

func TestBuildRunSummary(t *testing.T) {
	jobs := []model.JobSpec{{ID: "ok"}, {ID: "failed"}, {ID: "missing"}}
	summary := BuildRunSummary("run-1", "nightly", "started", jobs, map[string]model.JobResult{
		"ok": {ID: "ok", ExitCode: 0}, "failed": {ID: "failed", ExitCode: 2},
	}, func(result model.JobResult) model.JobResult {
		result.Error = "diagnosed"
		return result
	})
	if summary.Status != "failed" || summary.ExitCode != 1 || len(summary.Results) != 2 || summary.Results[1].Error != "diagnosed" || summary.Results[0].ID != "ok" {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestPlanSelectionReportsUnknownJobIDs(t *testing.T) {
	_, err := PlanSelection(model.Queue{Commands: []model.QueuedCommand{{ID: "known"}}}, "job-id", []string{"missing", "also-missing"}, false, Reference{})
	if err == nil || err.Error() != "job IDs not found in queue: also-missing, missing" {
		t.Fatalf("PlanSelection() error = %v", err)
	}
}

func TestPlanSelectionRecordsOriginMetadata(t *testing.T) {
	plan, err := PlanSelection(model.Queue{Commands: []model.QueuedCommand{{ID: "done", Command: []string{"true"}}}}, "failed", nil, false, Reference{
		RunID: "run-1", CWD: "/work", Results: map[string]model.JobResult{"done": {ID: "done", AttemptID: "att-1", ExitCode: 0}},
		SubmittedAt: func(string) string { return "submitted" }, FinishedAt: func(string) string { return "finished" },
	})
	if err != nil {
		t.Fatal(err)
	}
	origin := plan.CarriedOrigins["done"]
	if origin == nil || origin.Status != "success" || origin.AttemptID != "att-1" || origin.CWD != "/work" || origin.SubmittedAt != "submitted" || origin.FinishedAt != "finished" {
		t.Fatalf("origin = %#v", origin)
	}
}

func TestAssignAttemptIDsUpdatesAttemptEnvironment(t *testing.T) {
	jobs := []model.JobSpec{{ID: "job-1", Environment: []string{"ROTARI_RUN_DIR=/runs", "ROTARI_ATTEMPT_ID=old"}}}
	AssignAttemptIDs(jobs, "run-1", 2, AttemptIDCallbacks{
		MakeAttemptID: func(runID, jobID string, number int) string {
			return runID + "/" + jobID + "/" + string(rune('0'+number))
		},
		AttemptJobDir: func(runDir string, job model.JobSpec) (string, error) { return runDir + "/" + job.AttemptID, nil },
		AttemptIDName: "ROTARI_ATTEMPT_ID", RunDirName: "ROTARI_RUN_DIR", JobDirName: "ROTARI_JOB_DIR",
	})
	if jobs[0].AttemptID != "run-1/job-1/2" {
		t.Fatalf("attempt ID = %q", jobs[0].AttemptID)
	}
	if value, ok := EnvironmentEntry(jobs[0].Environment, "ROTARI_JOB_DIR"); !ok || value != "ROTARI_JOB_DIR=/runs/run-1/job-1/2" {
		t.Fatalf("job directory environment = %q, %t", value, ok)
	}
}

func TestWasExplicitlyCancelled(t *testing.T) {
	for _, test := range []struct {
		name  string
		error string
		phase string
		check bool
		want  bool
	}{
		{name: "callback", check: true, want: true},
		{name: "scheduler", phase: " CANCELED ", want: true},
		{name: "error prefix", error: "cancelled by scheduler", want: true},
		{name: "ordinary failure", error: "exit code 1", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := WasExplicitlyCancelled(test.error, func() bool { return test.check }, func() string { return test.phase }); got != test.want {
				t.Fatalf("WasExplicitlyCancelled() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRunAttemptRunsLocalBatchArrayAndUnsupportedJobs(t *testing.T) {
	local := &testExecutor{name: "local"}
	batch := &testExecutor{name: "slurm", array: true}
	started := make(map[string]bool)
	var startedMu sync.Mutex
	jobs := []model.JobSpec{
		{ID: "local-1", Executor: "local", Command: []string{"true"}},
		{ID: "batch-1", Executor: "slurm", Command: []string{"run"}},
		{ID: "array-1", Executor: "slurm", Command: []string{"run"}, ArrayGroup: "array", ArrayTaskID: intPointer(1), ArrayFirst: 1, ArrayLast: 2},
		{ID: "array-2", Executor: "slurm", Command: []string{"run"}, ArrayGroup: "array", ArrayTaskID: intPointer(2), ArrayFirst: 1, ArrayLast: 2},
		{ID: "unknown", Executor: "pbs", Command: []string{"run"}},
	}
	results := RunAttempt("/runs/run-1", model.Queue{DefaultExecutorOptions: []string{"--default"}}, jobs, AttemptOptions{
		LocalConcurrency: 1, BatchMaxActive: 1, ExecutorOptions: []string{"--batch"},
		ResolveExecutor: func(name string) (executor.JobExecutor, bool) {
			switch name {
			case "local":
				return local, true
			case "slurm":
				return batch, true
			default:
				return nil, false
			}
		},
		Callbacks: BatchLaneCallbacks{ValidatedJobDir: func(string, string) (string, error) { return "/job", nil }, JobCancelled: func(string) bool { return false }},
	}, func(job model.JobSpec) {
		startedMu.Lock()
		started[job.ID] = true
		startedMu.Unlock()
	})
	byID := make(map[string]model.JobResult)
	for _, result := range results {
		byID[result.ID] = result
	}
	if len(byID) != len(jobs) || byID["local-1"].ExitCode != 0 || byID["array-2"].ExitCode != 0 || byID["unknown"].Error != "unsupported executor: pbs" {
		t.Fatalf("results = %#v", byID)
	}
	startedMu.Lock()
	localStarted := started["local-1"]
	batchStarted := started["batch-1"]
	arrayOneStarted := started["array-1"]
	arrayTwoStarted := started["array-2"]
	startedMu.Unlock()
	if !localStarted || !batchStarted || !arrayOneStarted || !arrayTwoStarted {
		t.Fatalf("started = %#v", started)
	}
	if len(batch.options) != 2 || len(batch.options[0]) != 1 || batch.options[0][0] != "--batch" || len(batch.options[1]) != 1 || batch.options[1][0] != "--batch" {
		t.Fatalf("batch options = %#v", batch.options)
	}
}

func TestRunBatchLaneHandlesSubmissionAndCancellation(t *testing.T) {
	executor := &testExecutor{name: "slurm", submitError: errors.New("submit failed")}
	results := make(chan model.JobResult, 2)
	callbacks := BatchLaneCallbacks{
		ValidatedJobDir: func(_, jobID string) (string, error) { return "/job/" + jobID, nil },
		JobCancelled:    func(jobDir string) bool { return jobDir == "/job/cancelled" },
		RecordCancelled: func(_ string, job model.JobSpec) model.JobResult {
			return model.JobResult{ID: job.ID, ExitCode: 130, Error: "cancelled"}
		},
	}
	jobs := []model.JobSpec{{ID: "failed-submit"}, {ID: "cancelled"}}
	workers := new(sync.WaitGroup)
	workers.Add(1)
	go RunBatchLane(workers, "/runs/run-1", model.Queue{}, executor, jobs, 2, nil, results, callbacks, nil)
	workers.Wait()
	close(results)
	byID := make(map[string]model.JobResult)
	for result := range results {
		byID[result.ID] = result
	}
	if byID["failed-submit"].Error != "submit failed" || byID["cancelled"].Error != "cancelled" {
		t.Fatalf("results = %#v", byID)
	}
}

func intPointer(value int) *int { return &value }

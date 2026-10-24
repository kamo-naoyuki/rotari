package projectrun

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rotari-dev/rotari/internal/executor"
	"github.com/rotari-dev/rotari/internal/jobcontrol"
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
)

// cancelDuringSubmitExecutor runs a cancel while it submits jobs, after the
// run's pre-start check and before the jobs are recorded as submitted, which
// is where a cancel used to be lost.
type cancelDuringSubmitExecutor struct {
	name             string
	attemptDirBefore bool
	cancel           func()
	once             sync.Once
}

const fakeCancelledFile = "fake-cancelled"

func (fake *cancelDuringSubmitExecutor) Name() string { return fake.name }

func (fake *cancelDuringSubmitExecutor) Submit(runDir string, job model.JobSpec, _ []string) (executor.JobHandle, error) {
	handles, err := fake.submit(runDir, []model.JobSpec{job})
	if err != nil {
		return executor.JobHandle{}, err
	}
	return handles[0], nil
}

func (fake *cancelDuringSubmitExecutor) submit(runDir string, jobs []model.JobSpec) ([]executor.JobHandle, error) {
	jobDirs := make([]string, len(jobs))
	for index, job := range jobs {
		jobDir, err := state.AttemptJobDir(runDir, job)
		if err != nil {
			return nil, err
		}
		jobDirs[index] = jobDir
		if fake.attemptDirBefore {
			if err := os.MkdirAll(jobDir, 0o755); err != nil {
				return nil, err
			}
		}
	}
	fake.once.Do(fake.cancel)
	handles := make([]executor.JobHandle, len(jobs))
	for index, job := range jobs {
		if err := os.MkdirAll(jobDirs[index], 0o755); err != nil {
			return nil, err
		}
		if err := state.WriteJSON(filepath.Join(jobDirs[index], "job.json"), map[string]string{"executor": fake.name}); err != nil {
			return nil, err
		}
		handles[index] = executor.JobHandle{Job: job, Native: jobDirs[index]}
	}
	return handles, nil
}

func (fake *cancelDuringSubmitExecutor) Wait(_ string, handle executor.JobHandle) model.JobResult {
	result := model.JobResult{ID: handle.Job.ID, Command: handle.Job.Command}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(handle.Native, fakeCancelledFile)); err == nil {
			result.ExitCode, result.Error = 143, "cancelled"
			return result
		}
	}
	return result
}

func (fake *cancelDuringSubmitExecutor) Cancel(jobDir string) error {
	return os.WriteFile(filepath.Join(jobDir, fakeCancelledFile), nil, 0o644)
}

type cancelDuringArraySubmitExecutor struct {
	*cancelDuringSubmitExecutor
}

func (fake cancelDuringArraySubmitExecutor) SubmitArray(runDir string, jobs []model.JobSpec, _ []string) ([]executor.JobHandle, error) {
	return fake.submit(runDir, jobs)
}

func TestCancelReachingJobDuringSubmissionStopsIt(t *testing.T) {
	for _, lane := range []struct {
		name     string
		executor string
		array    bool
	}{
		{name: "local", executor: "local"},
		{name: "batch", executor: "fake"},
		{name: "native array", executor: "fake", array: true},
	} {
		for _, test := range []struct {
			name             string
			jobIDs           []string
			attemptDirBefore bool
		}{
			{name: "whole run before attempt directory"},
			{name: "whole run with attempt directory", attemptDirBefore: true},
			{name: "job before attempt directory", jobIDs: []string{"job"}},
			{name: "job with attempt directory", jobIDs: []string{"job"}, attemptDirBefore: true},
		} {
			t.Run(lane.name+"/"+test.name, func(t *testing.T) {
				runner, paths := testRunner(t)
				fake := &cancelDuringSubmitExecutor{name: lane.executor, attemptDirBefore: test.attemptDirBefore}
				var registered executor.JobExecutor = fake
				command := model.QueuedCommand{ID: "job", Command: []string{"work"}, Executor: lane.executor}
				if lane.array {
					registered = cancelDuringArraySubmitExecutor{fake}
					command.Array = &model.ArraySpec{First: 1, Last: 2}
				}
				runner.Executors = executor.Registry{lane.executor: registered}
				controller := jobcontrol.Controller{Store: runner.Store, Executors: runner.Executors}
				var cancelErr error
				fake.cancel = func() {
					_, cancelErr = controller.Cancel(paths.BaseDir, paths.ProjectName, "run-1", test.jobIDs, false)
				}
				if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{command}}); err != nil {
					t.Fatal(err)
				}
				if err := runner.Begin(paths, Start{RunID: "run-1"}); err != nil {
					t.Fatal(err)
				}
				if _, err := runner.Execute(paths, Options{RunID: "run-1", LocalConcurrency: 2, BatchMaxActive: 2}, Observer{}); err != nil {
					t.Fatal(err)
				}
				if cancelErr != nil {
					t.Fatalf("cancel: %v", cancelErr)
				}
				summary, err := state.LoadRunSummary(filepath.Join(paths.RunsDir, "run-1", "summary.json"))
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if lane.array {
					want = 2
				}
				if len(summary.Results) != want {
					t.Fatalf("results = %+v, want %d", summary.Results, want)
				}
				for _, result := range summary.Results {
					if result.ExitCode != 143 || !model.IsCancelledError(result.Error) {
						t.Errorf("result = %+v; a job submitted while a cancel reached it kept running", result)
					}
				}
			})
		}
	}
}

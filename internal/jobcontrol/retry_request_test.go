package jobcontrol_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestRetryRequestIsPersistedAcceptedAndReopensTheJob(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o755, 0o644)
	runner := projectrun.Runner{Store: store, Executors: executor.NewRegistry(store, nil)}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "failed", Name: "failed", Command: []string{"false"}},
		{ID: "slow", Name: "slow", Command: []string{"true"}},
	}}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := runner.Begin(paths, projectrun.Start{RunID: "20261006-010203-aabbccdd", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	runID := "20261006-010203-aabbccdd"
	runDir := filepath.Join(paths.RunsDir, runID)
	jobs := model.QueueToJobs(queue.Commands)
	jobsByName := map[string]model.JobSpec{"failed": jobs[0], "slow": jobs[1]}
	results := make(map[string]model.JobResult)
	requests := make(chan run.ManualRetryRequest, 4)
	stopRequests := jobcontrol.WatchRetryRequests(paths, runID, requests)
	t.Cleanup(stopRequests)

	slowGate := make(chan struct{})
	retryGate := make(chan struct{})
	firstFailed := make(chan struct{}, 1)
	completed := make(chan struct{})
	var mu sync.Mutex
	attempts := map[string]int{}
	go func() {
		run.ExecuteJobs(jobs, jobsByName, results, run.EngineOptions{
			ManualRetries: requests,
			Start: func(ready []model.JobSpec, done func(model.JobResult)) {
				for _, job := range ready {
					mu.Lock()
					attempts[job.ID]++
					attempt := attempts[job.ID]
					mu.Unlock()
					go func(job model.JobSpec, attempt int) {
						if job.ID == "slow" {
							<-slowGate
						}
						if job.ID == "failed" && attempt == 2 {
							<-retryGate
						}
						exitCode := 0
						if job.ID == "failed" && attempt == 1 {
							exitCode = 7
						}
						done(model.JobResult{ID: job.ID, ExitCode: exitCode})
					}(job, attempt)
				}
			},
			AssignAttemptID: func(job *model.JobSpec, attempt int) { job.AttemptID = state.MakeAttemptID(runID, job.ID, attempt) },
			FinalResult: func(job model.JobSpec, result model.JobResult) model.JobResult {
				if job.ID == "failed" && result.ExitCode == 7 {
					firstFailed <- struct{}{}
				}
				if job.ID == "failed" && result.ExitCode == 0 {
					_ = os.Remove(filepath.Join(runDir, job.ID, state.ManualRetryPendingFileName))
				}
				return result
			},
		})
		close(completed)
	}()

	select {
	case <-firstFailed:
	case <-time.After(5 * time.Second):
		t.Fatal("job did not reach its first final failure")
	}
	revision, err := project.Revision(paths)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		response run.ManualRetryResponse
		err      error
	}
	responseCh := make(chan result, 1)
	go func() {
		controller := jobcontrol.Controller{Store: store, Executors: runner.Executors}
		response, err := controller.SubmitSelectedRetry(paths.BaseDir, paths.ProjectName, runID, jobcontrol.RetrySelection{JobIDs: []string{"failed"}, Selection: "job-id", PartialArray: true}, revision, 5*time.Second)
		responseCh <- result{response: response, err: err}
	}()
	var submitted result
	select {
	case submitted = <-responseCh:
	case <-time.After(5 * time.Second):
		t.Fatal("retry request was not answered")
	}
	if submitted.err != nil || len(submitted.response.Accepted) != 1 || submitted.response.Accepted[0] != "failed" {
		t.Fatalf("retry response = %+v, %v", submitted.response, submitted.err)
	}
	if _, err := os.Stat(filepath.Join(runDir, "failed", state.ManualRetryPendingFileName)); err != nil {
		t.Fatalf("retrying marker was not persisted: %v", err)
	}
	if status := jobstatus.ReadJob(store, filepath.Join(runDir, "failed"), model.JobResult{ID: "failed", ExitCode: 7}, true); status.Finished() {
		t.Fatalf("superseded result still appears final: %+v", status)
	}
	close(retryGate)
	close(slowGate)
	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not finish after manual retry")
	}
	if attempts["failed"] != 2 || results["failed"].ExitCode != 0 {
		t.Fatalf("attempts=%v results=%v", attempts, results)
	}
	if _, err := os.Stat(filepath.Join(runDir, "failed", state.ManualRetryPendingFileName)); !os.IsNotExist(err) {
		t.Fatalf("retrying marker remains after final result: %v", err)
	}
	stopRequests()
	if err := runner.Finish(paths, runID, 0); err != nil {
		t.Fatal(err)
	}
}

func TestCloseRetryRequestsRejectsUnacceptedRequestAsRunEnded(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20261006-010203-aabbccdd"
	requestDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(requestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(requestDir, state.ManualRetryAcceptingFileName), map[string]any{"state_version": model.StateVersion, "run_id": runID}); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"state_version": model.StateVersion, "id": "request-1", "run_id": runID, "job_ids": []string{"job-1"}, "partial_array": true}
	if err := state.WriteJSON(filepath.Join(requestDir, state.ManualRetryRequestPrefix+"request-1.json"), request); err != nil {
		t.Fatal(err)
	}
	if err := jobcontrol.CloseRetryRequests(paths, runID); err != nil {
		t.Fatal(err)
	}
	var response struct {
		StateVersion int `json:"state_version"`
		Response     struct {
			RunEnded bool `json:"run_ended"`
		} `json:"response"`
	}
	if err := state.NewStore(0o755, 0o644).ReadJSON(filepath.Join(requestDir, state.ManualRetryRequestPrefix+"request-1"+state.ManualRetryResponseSuffix), &response); err != nil {
		t.Fatal(err)
	}
	if response.StateVersion != model.StateVersion || !response.Response.RunEnded {
		t.Fatalf("end-race response = %+v, want version %d and run_ended", response, model.StateVersion)
	}
}

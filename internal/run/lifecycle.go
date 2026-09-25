package run

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type AttemptCallbacks struct {
	Execute          func(attempt int, jobs []model.JobSpec) []model.JobResult
	AssignAttemptIDs func(jobs []model.JobSpec, attempt int)
	ShouldRetry      func(job model.JobSpec, result model.JobResult) bool
	Progress         func(result model.JobResult, completed, total, succeeded, failed int)
}

func ExecuteDependencyRetries(pending []model.JobSpec, jobsByName map[string]model.JobSpec, results map[string]model.JobResult, retry int, callbacks AttemptCallbacks) []model.JobSpec {
	total := len(pending)
	for attempt := 0; (retry == -1 || attempt <= retry) && len(pending) > 0; attempt++ {
		pendingByID := make(map[string]bool, len(pending))
		for _, job := range pending {
			pendingByID[job.ID] = true
		}
		unresolved := pending
		var attemptResults []model.JobResult
		lastAttempt := retry >= 0 && attempt >= retry
		failureFinal := func(job model.JobSpec, result model.JobResult) bool {
			if !pendingByID[job.ID] || lastAttempt {
				return true
			}
			return callbacks.ShouldRetry != nil && !callbacks.ShouldRetry(job, result)
		}
		for {
			ready, blocked, stillUnresolved := ResolveDependencyWave(unresolved, jobsByName, results, pendingByID, failureFinal)
			for _, job := range blocked {
				results[job.ID] = model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "blocked by failed dependency"}
			}
			if len(ready) == 0 {
				// A blocked job has a final result now, which may let a
				// DependsOnFinished dependent start.
				if len(blocked) == 0 {
					break
				}
				unresolved = stillUnresolved
				continue
			}
			if callbacks.AssignAttemptIDs != nil {
				callbacks.AssignAttemptIDs(ready, attempt)
			}
			waveResults := callbacks.Execute(attempt, ready)
			for _, result := range waveResults {
				results[result.ID] = result
			}
			attemptResults = append(attemptResults, waveResults...)
			unresolved = stillUnresolved
		}
		pending = RetryPendingJobs(pending, results, attempt, retry, callbacks.ShouldRetry)
		completed, succeeded, failed := SummarizeResults(results)
		if callbacks.Progress != nil {
			if len(pending) > 0 && (retry == -1 || attempt < retry) {
				for _, job := range pending {
					callbacks.Progress(model.JobResult{ID: job.ID, Command: job.Command, Error: fmt.Sprintf("retry:%d", attempt+1)}, completed, total, succeeded, failed)
				}
			}
			for _, result := range attemptResults {
				progressResult := result
				if result.ExitCode != 0 && !JobIsPending(pending, result.ID) {
					progressResult.Error = "final-failure"
				}
				callbacks.Progress(progressResult, completed, total, succeeded, failed)
			}
		}
	}
	return pending
}

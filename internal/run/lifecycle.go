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
	// Each job has its own retry limit (JobSpec.RetryLimit), so the loop runs
	// while any job is still pending: RetryPendingJobs drops a failed job
	// whose retries are used up.
	for attempt := 0; len(pending) > 0; attempt++ {
		pendingByID := make(map[string]bool, len(pending))
		for _, job := range pending {
			pendingByID[job.ID] = true
		}
		unresolved := pending
		var attemptResults []model.JobResult
		failureFinal := func(job model.JobSpec, result model.JobResult) bool {
			if limit := job.RetryLimit(retry); !pendingByID[job.ID] || (limit >= 0 && attempt >= limit) {
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
		previousPending := len(pending)
		pending = RetryPendingJobs(pending, results, attempt, retry, callbacks.ShouldRetry)
		completed, succeeded, failed := SummarizeResults(results)
		// Stop when an attempt neither ran nor resolved anything, so jobs that
		// can never become ready do not loop forever; FinalizePendingResults
		// then blocks them.
		stalled := len(attemptResults) == 0 && len(pending) == previousPending
		if callbacks.Progress != nil {
			if len(pending) > 0 && !stalled {
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
		if stalled {
			break
		}
	}
	return pending
}

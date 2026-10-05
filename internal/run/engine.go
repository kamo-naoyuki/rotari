package run

import (
	"fmt"
	"sort"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// EngineOptions configures ExecuteJobs.
type EngineOptions struct {
	// RunRetry is the run's --retry limit for jobs without their own; -1
	// means no limit.
	RunRetry int
	// Start runs jobs that became ready together and calls done once per job,
	// from another goroutine, with its result.
	Start func(jobs []model.JobSpec, done func(model.JobResult))
	// AssignAttemptID sets the attempt ID and environment of a job's attempt,
	// numbered from 0 for each job.
	AssignAttemptID func(job *model.JobSpec, attempt int)
	// ShouldRetry reports whether a failed result may be retried at all, for
	// example false for an explicit cancellation.
	ShouldRetry func(job model.JobSpec, result model.JobResult) bool
	// Stopped reports that the run is being cancelled: no new retries start,
	// and jobs that have not started are recorded as cancelled.
	Stopped  func() bool
	Progress func(result model.JobResult, completed, total, succeeded, failed int)
	// FinalResult is called whenever a job reaches a final result. A manual
	// retry can reopen a job, so each completed generation is reported.
	FinalResult func(job model.JobSpec, result model.JobResult) model.JobResult
	// After runs f after d; it defaults to time.AfterFunc.
	After func(d time.Duration, f func())
	// ManualRetries carries requests to reopen final jobs in this run. Commit
	// persists and returns the canonical response before the engine mutates
	// in-memory state; it can fence an acceptance after rechecking state.
	ManualRetries <-chan ManualRetryRequest
	// ManualRetryAccepted reports the progress reset after a retry request is
	// durably accepted and before its jobs are scheduled again.
	ManualRetryAccepted func(response ManualRetryResponse, completed, total, succeeded, failed int)
}

// ManualRetryRequest selects final jobs already owned by this run.
type ManualRetryRequest struct {
	ID           string
	JobIDs       []string
	PartialArray bool
	Commit       func(ManualRetryResponse) (ManualRetryResponse, error)
}

// ManualRetryResponse records which requested jobs were accepted. Dependent
// jobs reopened because a prerequisite was blocked are listed separately.
type ManualRetryResponse struct {
	ID       string                 `json:"id"`
	Accepted []string               `json:"accepted_job_ids"`
	Reopened []string               `json:"reopened_dependent_job_ids,omitempty"`
	Rejected []ManualRetryRejection `json:"rejected,omitempty"`
	RunEnded bool                   `json:"run_ended,omitempty"`
}

type ManualRetryRejection struct {
	JobID  string `json:"job_id,omitempty"`
	Reason string `json:"reason"`
}

// cancelledBeforeStart is the result of a job the run never started because
// it was cancelled.
const cancelledBeforeStart = "cancelled before start"

type engineEvent struct {
	result model.JobResult
	// retry carries a job whose retry delay has passed.
	retry  *model.JobSpec
	manual *ManualRetryRequest
}

// ExecuteJobs runs pending jobs as soon as their dependencies allow and
// records each result in results, which may already hold results carried
// from earlier runs. A failed job with retries left is started again at
// once, or after its retry delay; a job is never held back by unrelated jobs.
//
// A DependsOn prerequisite must succeed; once its failure is final (no
// retries left) its dependents are blocked. A DependsOnFinished prerequisite
// only needs a final result. ExecuteJobs returns the jobs that could never
// become ready, for FinalizePendingResults.
func ExecuteJobs(pending []model.JobSpec, jobsByName map[string]model.JobSpec, results map[string]model.JobResult, options EngineOptions) []model.JobSpec {
	after := options.After
	if after == nil {
		after = func(delay time.Duration, f func()) { time.AfterFunc(delay, f) }
	}
	stopped := func() bool { return options.Stopped != nil && options.Stopped() }
	total := len(pending)
	jobsByID := make(map[string]model.JobSpec, len(pending))
	for _, job := range pending {
		jobsByID[job.ID] = job
	}
	// A result is final when its job will not run again: carried results,
	// successes, and failures without retries left.
	final := make(map[string]bool, len(results)+len(pending))
	for id := range results {
		if _, executable := jobsByID[id]; !executable {
			final[id] = true
		}
	}
	attempts := make(map[string]int, len(pending))
	waiting := append([]model.JobSpec(nil), pending...)
	events := make(chan engineEvent)
	running, delayed := 0, 0

	readiness := func(job model.JobSpec) (ready, blocked bool) {
		for _, dependency := range job.DependsOn {
			id := jobsByName[dependency].ID
			result, done := results[id]
			if !done || !final[id] {
				return false, false
			}
			if result.ExitCode != 0 {
				return false, true
			}
		}
		for _, dependency := range job.DependsOnFinished {
			id := jobsByName[dependency].ID
			if _, done := results[id]; !done || !final[id] {
				return false, false
			}
		}
		return true, false
	}
	// progress counts only the jobs this run executes, and only once their
	// result is final, so carried results and failures awaiting a retry stay
	// out of the counts that total bounds.
	progress := func(result model.JobResult) {
		if options.Progress == nil {
			return
		}
		completed, succeeded, failed := 0, 0, 0
		for id := range jobsByID {
			if !final[id] {
				continue
			}
			completed++
			if results[id].ExitCode == 0 {
				succeeded++
			} else {
				failed++
			}
		}
		options.Progress(result, completed, total, succeeded, failed)
	}
	finalize := func(job model.JobSpec, result model.JobResult, reportProgress bool) {
		if options.FinalResult != nil {
			result = options.FinalResult(job, result)
		}
		results[job.ID] = result
		final[job.ID] = true
		if reportProgress {
			reported := result
			if result.ExitCode != 0 {
				reported.Error = "final-failure"
			}
			progress(reported)
		}
	}
	// schedule starts every waiting job that is ready and blocks those whose
	// DependsOn prerequisite failed for good, repeating while blocking one job
	// may settle another.
	schedule := func() {
		for {
			changed := false
			var ready []model.JobSpec
			next := make([]model.JobSpec, 0, len(waiting))
			for _, job := range waiting {
				switch isReady, isBlocked := readiness(job); {
				case isBlocked:
					finalize(job, model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "blocked by failed dependency"}, false)
					changed = true
				case isReady && stopped():
					finalize(job, model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 143, Error: cancelledBeforeStart}, false)
					changed = true
				case isReady:
					attempt := job
					attempt.Environment = append([]string(nil), job.Environment...)
					if options.AssignAttemptID != nil {
						options.AssignAttemptID(&attempt, attempts[job.ID])
					}
					attempts[job.ID]++
					running++
					ready = append(ready, attempt)
				default:
					next = append(next, job)
				}
			}
			waiting = next
			if len(ready) > 0 {
				options.Start(ready, func(result model.JobResult) { events <- engineEvent{result: result} })
			}
			if !changed {
				return
			}
		}
	}

	schedule()
	for running > 0 || delayed > 0 {
		var event engineEvent
		if options.ManualRetries == nil {
			event = <-events
		} else {
			select {
			case event = <-events:
			case request, ok := <-options.ManualRetries:
				if !ok {
					options.ManualRetries = nil
					continue
				}
				event.manual = &request
			}
		}
		if event.manual != nil {
			response, reopened := reopenFinalJobs(*event.manual, jobsByID, jobsByName, results, final, stopped())
			if event.manual.Commit != nil {
				committed, err := event.manual.Commit(response)
				if err != nil {
					response = ManualRetryResponse{ID: event.manual.ID, Rejected: rejectAll(event.manual.JobIDs, "failed to commit request: "+err.Error())}
					reopened = nil
					_, _ = event.manual.Commit(response)
				} else {
					response = committed
					if len(response.Accepted) == 0 {
						reopened = nil
					}
				}
			}
			if len(reopened) > 0 {
				for _, job := range reopened {
					delete(results, job.ID)
					delete(final, job.ID)
					waiting = append(waiting, job)
				}
				if options.ManualRetryAccepted != nil {
					completed, succeeded, failed := progressCounts(jobsByID, final, results)
					options.ManualRetryAccepted(response, completed, total, succeeded, failed)
				}
				schedule()
			}
			continue
		}
		if event.retry != nil {
			delayed--
			if stopped() {
				finalize(*event.retry, results[event.retry.ID], false)
			} else {
				waiting = append(waiting, *event.retry)
			}
			schedule()
			continue
		}
		running--
		result := event.result
		job := jobsByID[result.ID]
		results[job.ID] = result
		used := attempts[job.ID]
		limit := job.RetryLimit(options.RunRetry)
		retrying := result.ExitCode != 0 && (limit < 0 || used <= limit) && !stopped() &&
			(options.ShouldRetry == nil || options.ShouldRetry(job, result))
		if retrying {
			progress(model.JobResult{ID: job.ID, Command: job.Command, Error: fmt.Sprintf("retry:%d", used)})
			progress(result)
			delayed++
			retry := job
			after(job.RetryDelayFor(used), func() { events <- engineEvent{retry: &retry} })
		} else {
			finalize(job, result, true)
		}
		schedule()
	}
	if stopped() {
		for _, job := range waiting {
			finalize(job, model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 143, Error: cancelledBeforeStart}, false)
		}
		return nil
	}
	return waiting
}

func progressCounts(jobs map[string]model.JobSpec, final map[string]bool, results map[string]model.JobResult) (completed, succeeded, failed int) {
	for id := range jobs {
		if !final[id] {
			continue
		}
		completed++
		if results[id].ExitCode == 0 {
			succeeded++
		} else {
			failed++
		}
	}
	return completed, succeeded, failed
}

func reopenFinalJobs(request ManualRetryRequest, jobsByID, jobsByName map[string]model.JobSpec, results map[string]model.JobResult, final map[string]bool, stopped bool) (ManualRetryResponse, []model.JobSpec) {
	response := ManualRetryResponse{ID: request.ID, Accepted: []string{}, Reopened: []string{}, Rejected: []ManualRetryRejection{}}
	if stopped {
		response.Rejected = rejectAll(request.JobIDs, "run is cancelling")
		return response, nil
	}
	selected := make(map[string]bool, len(request.JobIDs))
	for _, id := range request.JobIDs {
		if selected[id] {
			response.Rejected = append(response.Rejected, ManualRetryRejection{JobID: id, Reason: "selected more than once"})
			continue
		}
		selected[id] = true
		_, owned := jobsByID[id]
		if !owned {
			response.Rejected = append(response.Rejected, ManualRetryRejection{JobID: id, Reason: "job is not executed by this run"})
			continue
		}
		if !final[id] {
			response.Rejected = append(response.Rejected, ManualRetryRejection{JobID: id, Reason: "job does not have a final result"})
			continue
		}
		response.Accepted = append(response.Accepted, id)
	}
	if !request.PartialArray {
		selected := make(map[string]bool, len(response.Accepted))
		for _, id := range response.Accepted {
			selected[id] = true
		}
		invalid := make(map[string]string)
		for _, id := range response.Accepted {
			job := jobsByID[id]
			if job.ArrayGroup == "" {
				continue
			}
			for memberID, member := range jobsByID {
				if member.ArrayGroup != job.ArrayGroup {
					continue
				}
				switch {
				case !selected[memberID]:
					invalid[job.ArrayGroup] = "whole-array retry did not select every task"
				case !final[memberID]:
					invalid[job.ArrayGroup] = "whole-array retry requires every task to have a final result"
				}
			}
		}
		if len(invalid) > 0 {
			accepted := response.Accepted[:0]
			for _, id := range response.Accepted {
				job := jobsByID[id]
				if reason := invalid[job.ArrayGroup]; reason != "" {
					response.Rejected = append(response.Rejected, ManualRetryRejection{JobID: id, Reason: reason})
					continue
				}
				accepted = append(accepted, id)
			}
			response.Accepted = accepted
		}
	}
	if len(response.Accepted) == 0 {
		return response, nil
	}
	toReopen := make(map[string]bool, len(response.Accepted))
	for _, id := range response.Accepted {
		toReopen[id] = true
	}
	changed := true
	for changed {
		changed = false
		for id, job := range jobsByID {
			if !final[id] || results[id].Error != "blocked by failed dependency" || toReopen[id] {
				continue
			}
			for _, dependency := range job.DependsOn {
				prerequisite, ok := jobsByName[dependency]
				if !ok {
					continue
				}
				if toReopen[prerequisite.ID] {
					toReopen[id] = true
					response.Reopened = append(response.Reopened, id)
					changed = true
					break
				}
			}
		}
	}
	sort.Strings(response.Reopened)
	return response, jobsForIDs(toReopen, jobsByID)
}

func rejectAll(jobIDs []string, reason string) []ManualRetryRejection {
	rejected := make([]ManualRetryRejection, 0, len(jobIDs))
	for _, id := range jobIDs {
		rejected = append(rejected, ManualRetryRejection{JobID: id, Reason: reason})
	}
	return rejected
}

func jobsForIDs(ids map[string]bool, jobs map[string]model.JobSpec) []model.JobSpec {
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	selected := make([]model.JobSpec, 0, len(ids))
	for _, id := range keys {
		if job, ok := jobs[id]; ok {
			selected = append(selected, job)
		}
	}
	return selected
}

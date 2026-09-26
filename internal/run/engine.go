package run

import (
	"fmt"
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
	// After runs f after d; it defaults to time.AfterFunc.
	After func(d time.Duration, f func())
}

// cancelledBeforeStart is the result of a job the run never started because
// it was cancelled.
const cancelledBeforeStart = "cancelled before start"

type engineEvent struct {
	result model.JobResult
	// retry carries a job whose retry delay has passed.
	retry *model.JobSpec
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
					results[job.ID] = model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "blocked by failed dependency"}
					final[job.ID] = true
					changed = true
				case isReady && stopped():
					results[job.ID] = model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 143, Error: cancelledBeforeStart}
					final[job.ID] = true
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
		event := <-events
		if event.retry != nil {
			delayed--
			if stopped() {
				final[event.retry.ID] = true
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
			final[job.ID] = true
			reported := result
			if result.ExitCode != 0 {
				reported.Error = "final-failure"
			}
			progress(reported)
		}
		schedule()
	}
	if stopped() {
		for _, job := range waiting {
			results[job.ID] = model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 143, Error: cancelledBeforeStart}
		}
		return nil
	}
	return waiting
}

package jobstatus

import (
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Job is the resolved outcome of a job in one run: its latest attempt first,
// then the run's `summary.json` result.
type Job struct {
	ExitCode   int
	Source     Source
	Attempt    Attempt
	Summary    model.JobResult
	HasSummary bool
}

// ResolveJob applies the job-level fallback chain to an attempt already read
// with ReadAttempt and the job's summary result, if any.
func ResolveJob(attempt Attempt, summary model.JobResult, hasSummary bool) Job {
	job := Job{Attempt: attempt, Summary: summary, HasSummary: hasSummary}
	switch {
	case attempt.Finished():
		job.ExitCode, job.Source = attempt.ExitCode, attempt.Source
	case hasSummary:
		job.ExitCode, job.Source = summary.ExitCode, SourceSummary
	}
	return job
}

// ResolveAttempt resolves a displayed attempt of a job. The run's summary
// result belongs to the job's latest attempt, so an older attempt resolves
// from its own files only and stays unfinished without a terminal state.
func ResolveAttempt(attempt Attempt, latest bool, summary model.JobResult, hasSummary bool) Job {
	if !latest {
		return ResolveJob(attempt, model.JobResult{}, false)
	}
	return ResolveJob(attempt, summary, hasSummary)
}

// ReadJob reads the job's attempt directory and resolves it against the
// job's summary result, if any.
func ReadJob(store state.Store, jobDir string, summary model.JobResult, hasSummary bool) Job {
	return ResolveJob(ReadAttempt(store, jobDir), summary, hasSummary)
}

// Finished reports whether any step of the chain has a terminal exit code.
func (job Job) Finished() bool {
	return job.Source != SourceNone
}

// Accepted reports whether the run accepted the job's earlier failure as a
// success.
func (job Job) Accepted() bool {
	return job.HasSummary && job.Summary.Accepted
}

// Blocked reports whether the job never ran because a dependency failed.
func (job Job) Blocked() bool {
	return job.Source == SourceSummary && strings.HasPrefix(job.Summary.Error, "blocked")
}

// StatusNotStarted is the display status of a job with no dispatched attempt.
const StatusNotStarted = "not started"

// DisplayStatus classifies the latest persisted state for human/API views.
// Nonterminal executor states are explicitly marked as recorded, not live.
// A job directory missing from its run is not started; other missing or
// unusable records are unknown.
func (job Job) DisplayStatus(spec model.JobSpec) string {
	if job.Blocked() {
		return "blocked"
	}
	phase, schedulerState := job.recordedPhases()
	if job.Finished() {
		status := job.terminalStatus(spec, schedulerState)
		if job.ExitCode != 0 && status != model.StatusSuccess && job.recordedCancellation(phase, schedulerState) {
			return model.StatusCancelled
		}
		return status
	}
	if state := recordedState(phase, schedulerState); state != "" {
		return state
	}
	if job.Attempt.Undispatched {
		return StatusNotStarted
	}
	return "unknown"
}

func (job Job) recordedCancellation(phase, schedulerState string) bool {
	switch job.Source {
	case SourceStatus, SourceWrapper:
		return job.Attempt.HasWrapper && WrapperTerminal(job.Attempt.Wrapper) && isCancelledState(phase)
	case SourceScheduler:
		return isCancelledState(schedulerState)
	default:
		return false
	}
}

func (job Job) recordedPhases() (string, string) {
	phase := ""
	if job.Attempt.HasWrapper {
		phase = strings.ToLower(strings.TrimSpace(job.Attempt.Wrapper.Phase))
	}
	return phase, strings.ToLower(strings.TrimSpace(job.Attempt.SchedulerState))
}

func (job Job) terminalStatus(spec model.JobSpec, schedulerState string) string {
	if job.Source == SourceScheduler && schedulerState == "unknown" {
		return "unknown"
	}
	result, ok := job.Result(spec)
	if !ok {
		result = model.JobResult{ID: spec.ID, Command: spec.Command, ExitCode: job.ExitCode}
		if job.Attempt.HasWrapper {
			result.Error = job.Attempt.Wrapper.Error
		}
	}
	return model.ResultStatus(result, true)
}

func isCancelledState(value string) bool {
	return value == "cancelled" || value == "canceled"
}

func recordedState(values ...string) string {
	for _, value := range values {
		switch value {
		case "unknown":
			return "unknown"
		case "running":
			return "running (recorded)"
		case "pending", "queued", "waiting", "submitted", "configuring", "launching", "held", "queued_and_held":
			return "waiting (recorded)"
		case "suspended", "suspending":
			return "suspended (recorded)"
		}
	}
	return ""
}

// Hosts returns the summary's hosts, falling back to the wrapper's.
func (job Job) Hosts() []string {
	if job.HasSummary && len(job.Summary.Hosts) > 0 {
		return job.Summary.Hosts
	}
	if job.Attempt.HasWrapper {
		return job.Attempt.Wrapper.Hosts
	}
	return nil
}

// Result returns the resolved outcome as a job result, or false while the job
// has not finished. A summary result keeps its metadata, such as acceptance
// and diagnoses, with the resolved exit code.
func (job Job) Result(spec model.JobSpec) (model.JobResult, bool) {
	if job.HasSummary {
		result := job.Summary
		result.ExitCode = job.ExitCode
		return result, true
	}
	return job.Attempt.Result(spec)
}

// RecordedResults returns the results a run has recorded for its jobs, which
// the chain reads as each job's run result: its summary's once it has one,
// and before that the results it carries forward from earlier runs, which it
// records before dispatching any job. summary is the run's summary, or nil
// when it has none yet. A job the run executes has no recorded result until
// the summary.
func RecordedResults(runDir string, summary *model.RunSummary) map[string]model.JobResult {
	if summary != nil {
		return model.ResultsByID(summary.Results)
	}
	carried, err := state.LoadRunSummary(filepath.Join(runDir, state.CarriedResultsFileName))
	if err != nil {
		return map[string]model.JobResult{}
	}
	return model.ResultsByID(carried.Results)
}

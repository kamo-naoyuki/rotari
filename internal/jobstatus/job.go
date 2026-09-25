package jobstatus

import (
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

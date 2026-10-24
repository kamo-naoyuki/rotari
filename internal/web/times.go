package web

import (
	"time"

	"github.com/rotari-dev/rotari/internal/joblist"
	"github.com/rotari-dev/rotari/internal/jobstatus"
	"github.com/rotari-dev/rotari/internal/model"
)

// jobElapsed formats how long a job ran from its raw submission and finish
// times, by the rule `show` and `jobs` measure with; "" when unknown.
func jobElapsed(submittedAt, finishedAt string, done bool, now time.Time) string {
	submitted, _ := time.Parse(time.RFC3339Nano, submittedAt)
	finished, _ := time.Parse(time.RFC3339Nano, finishedAt)
	elapsed := jobstatus.Elapsed(submitted, finished, done, now)
	if elapsed < 0 {
		return ""
	}
	return joblist.FormatElapsed(elapsed)
}

func FormatQueueDisplayTimes(state *QueueState) {
	now := time.Now()
	state.RunnerStartedAt = model.FormatDisplayTimestamp(state.RunnerStartedAt)
	for index := range state.Queue.Commands {
		origin := state.Queue.Commands[index].Origin
		if origin == nil {
			continue
		}
		origin.SubmittedAt = model.FormatDisplayTimestamp(origin.SubmittedAt)
		origin.FinishedAt = model.FormatDisplayTimestamp(origin.FinishedAt)
	}
	for runIndex := range state.Runs {
		run := &state.Runs[runIndex]
		run.StartedAt = model.FormatDisplayTimestamp(run.StartedAt)
		run.FinishedAt = model.FormatDisplayTimestamp(run.FinishedAt)
		for jobIndex := range run.Jobs {
			job := &run.Jobs[jobIndex]
			job.Elapsed = jobElapsed(job.SubmittedAt, job.FinishedAt, job.Result != nil, now)
			job.SubmittedAt = model.FormatDisplayTimestamp(job.SubmittedAt)
			job.FinishedAt = model.FormatDisplayTimestamp(job.FinishedAt)
			if job.Origin != nil {
				job.Origin.SubmittedAt = model.FormatDisplayTimestamp(job.Origin.SubmittedAt)
				job.Origin.FinishedAt = model.FormatDisplayTimestamp(job.Origin.FinishedAt)
			}
			for attemptIndex := range job.Attempts {
				attempt := &job.Attempts[attemptIndex]
				attempt.Elapsed = jobElapsed(attempt.SubmittedAt, attempt.FinishedAt, attempt.Result != nil, now)
				attempt.SubmittedAt = model.FormatDisplayTimestamp(attempt.SubmittedAt)
				attempt.FinishedAt = model.FormatDisplayTimestamp(attempt.FinishedAt)
			}
		}
	}
}

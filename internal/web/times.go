package web

import "github.com/kamo-naoyuki/rotari/internal/model"

func FormatQueueDisplayTimes(state *QueueState) {
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
			job.SubmittedAt = model.FormatDisplayTimestamp(job.SubmittedAt)
			job.FinishedAt = model.FormatDisplayTimestamp(job.FinishedAt)
			if job.Origin != nil {
				job.Origin.SubmittedAt = model.FormatDisplayTimestamp(job.Origin.SubmittedAt)
				job.Origin.FinishedAt = model.FormatDisplayTimestamp(job.Origin.FinishedAt)
			}
			for attemptIndex := range job.Attempts {
				attempt := &job.Attempts[attemptIndex]
				attempt.SubmittedAt = model.FormatDisplayTimestamp(attempt.SubmittedAt)
				attempt.FinishedAt = model.FormatDisplayTimestamp(attempt.FinishedAt)
			}
		}
	}
}

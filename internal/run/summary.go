package run

import (
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func BuildRunSummary(runID, runName, startedAt string, jobs []model.JobSpec, finalResults map[string]model.JobResult, cancelled bool, diagnose func(model.JobResult) model.JobResult) model.RunSummary {
	summary := model.RunSummary{
		RunID: runID, RunName: runName, Status: "finished", StartedAt: startedAt,
		FinishedAt: time.Now().UTC().Format(time.RFC3339), Results: make([]model.JobResult, 0, len(jobs)),
	}
	for _, job := range jobs {
		result, ok := finalResults[job.ID]
		if !ok {
			continue
		}
		if diagnose != nil {
			result = diagnose(result)
		}
		result.Name = job.Name
		summary.Results = append(summary.Results, result)
		if result.ExitCode != 0 {
			summary.ExitCode = 1
		}
	}
	summary.Status = model.RunStatus(summary.ExitCode)
	// A cancelled run keeps exit code 1, so callers that read only the exit
	// code still see it fail; a run whose jobs all succeeded stays finished.
	if cancelled && summary.ExitCode != 0 {
		summary.Status = model.StatusCancelled
	}
	return summary
}

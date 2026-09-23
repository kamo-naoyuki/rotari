package run

import (
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func BuildRunSummary(runID, runName, startedAt string, jobs []model.JobSpec, finalResults map[string]model.JobResult, diagnose func(model.JobResult) model.JobResult) model.RunSummary {
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
		summary.Results = append(summary.Results, result)
		if result.ExitCode != 0 {
			summary.ExitCode = 1
		}
	}
	summary.Status = model.RunStatus(summary.ExitCode)
	return summary
}

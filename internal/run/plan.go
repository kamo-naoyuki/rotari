package run

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Plan describes how a rerun selects work and carries finished results forward.
// It is deliberately independent of filesystem paths; see PlanRerun.
type Plan struct {
	Execute        map[string]bool
	CarriedResults map[string]model.JobResult
	CarriedOrigins map[string]*model.JobOrigin
}

func RetryPendingJobs(jobs []model.JobSpec, results map[string]model.JobResult, attempt, retry int, shouldRetry func(model.JobSpec, model.JobResult) bool) []model.JobSpec {
	remaining := make([]model.JobSpec, 0, len(jobs))
	for _, job := range jobs {
		result, ok := results[job.ID]
		if !ok {
			remaining = append(remaining, job)
			continue
		}
		if result.ExitCode == 0 {
			continue
		}
		if retry >= 0 && attempt >= retry {
			continue
		}
		if shouldRetry != nil && !shouldRetry(job, result) {
			continue
		}
		remaining = append(remaining, job)
	}
	return remaining
}

func ResolveDependencyWave(unresolved []model.JobSpec, jobsByName map[string]model.JobSpec, finalResults map[string]model.JobResult, pendingByID map[string]bool) (ready, blocked, stillUnresolved []model.JobSpec) {
	blocked = make([]model.JobSpec, 0)
	ready = make([]model.JobSpec, 0, len(unresolved))
	stillUnresolved = make([]model.JobSpec, 0, len(unresolved))
	for _, job := range unresolved {
		blockedBy := ""
		readyForRun := true
		for _, dependency := range job.DependsOn {
			dependencyJob := jobsByName[dependency]
			result, done := finalResults[dependencyJob.ID]
			if !done || (result.ExitCode != 0 && pendingByID[dependencyJob.ID]) {
				readyForRun = false
				continue
			}
			if result.ExitCode != 0 {
				blockedBy = dependency
				break
			}
		}
		if blockedBy != "" {
			blocked = append(blocked, job)
		} else if readyForRun {
			ready = append(ready, job)
		} else {
			stillUnresolved = append(stillUnresolved, job)
		}
	}
	return ready, blocked, stillUnresolved
}

func joinStrings(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += separator + value
	}
	return result
}

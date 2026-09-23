package run

import "github.com/kamo-naoyuki/rotari/internal/model"

func CompleteArrayGroup(jobs []model.JobSpec, first, last int) bool {
	if len(jobs) != last-first+1 {
		return false
	}
	seen := make(map[int]bool, len(jobs))
	for _, job := range jobs {
		if job.ArrayTaskID == nil || *job.ArrayTaskID < first || *job.ArrayTaskID > last || seen[*job.ArrayTaskID] {
			return false
		}
		seen[*job.ArrayTaskID] = true
	}
	return len(seen) == len(jobs)
}

func JobIsPending(jobs []model.JobSpec, jobID string) bool {
	for _, job := range jobs {
		if job.ID == jobID {
			return true
		}
	}
	return false
}

func RemoveFinishedJobs(jobs []model.JobSpec, results map[string]model.JobResult) []model.JobSpec {
	remaining := make([]model.JobSpec, 0, len(jobs))
	for _, job := range jobs {
		if _, done := results[job.ID]; !done {
			remaining = append(remaining, job)
		}
	}
	return remaining
}

func SummarizeResults(results map[string]model.JobResult) (completed, succeeded, failed int) {
	for _, result := range results {
		completed++
		if result.ExitCode == 0 {
			succeeded++
		} else {
			failed++
		}
	}
	return completed, succeeded, failed
}

func ExpandArrayPlan(commands []model.QueuedCommand, jobs []model.JobSpec, execute map[string]bool) {
	for _, command := range commands {
		if command.Array == nil || !execute[command.ID] {
			continue
		}
		delete(execute, command.ID)
		for _, job := range jobs {
			if job.ArrayGroup == command.ID {
				execute[job.ID] = true
			}
		}
	}
}

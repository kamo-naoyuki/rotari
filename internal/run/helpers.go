package run

import (
	"strconv"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

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

func FinalizePendingResults(pending []model.JobSpec, results map[string]model.JobResult) {
	for _, job := range pending {
		if _, ok := results[job.ID]; ok {
			continue
		}
		results[job.ID] = model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "blocked by failed dependency"}
	}
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

func ApplyCarriedOrigins(commands []model.QueuedCommand, origins map[string]*model.JobOrigin) {
	for index := range commands {
		command := &commands[index]
		if origin, ok := origins[command.ID]; ok {
			command.Origin = origin
		}
		if command.Array == nil {
			continue
		}
		for _, task := range model.ArrayTaskIDs(command.Array) {
			taskID := command.ID + "-" + strconv.Itoa(task)
			if origin, ok := origins[taskID]; ok {
				if command.TaskOrigins == nil {
					command.TaskOrigins = make(map[string]*model.JobOrigin)
				}
				command.TaskOrigins[taskID] = origin
			}
		}
	}
}

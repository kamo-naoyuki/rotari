package run

import (
	"fmt"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// Plan describes how a rerun selects work and carries finished results forward.
// It is deliberately independent of filesystem paths; callers provide the
// reference data used to build it.
type Plan struct {
	Execute        map[string]bool
	CarriedResults map[string]model.JobResult
	CarriedOrigins map[string]*model.JobOrigin
}

type Reference struct {
	RunID       string
	CWD         string
	Results     map[string]model.JobResult
	SubmittedAt func(jobID string) string
	FinishedAt  func(jobID string) string
}

func PlanSelection(queue model.Queue, selection string, jobIDs []string, partialArray bool, reference Reference) (Plan, error) {
	plan := Plan{Execute: make(map[string]bool, len(queue.Commands))}
	if selection == "" {
		for _, command := range queue.Commands {
			plan.Execute[command.ID] = true
		}
		return plan, nil
	}
	requested := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		requested[jobID] = true
	}
	plan.CarriedResults = make(map[string]model.JobResult)
	plan.CarriedOrigins = make(map[string]*model.JobOrigin)
	for _, command := range queue.Commands {
		if command.Array != nil && partialArray && selection != "job-id" {
			planArrayTasks(command, selection, reference, &plan)
			continue
		}
		result, finished := model.AggregatedJobResult(command.ID, command.Array, reference.Results)
		include := requested[command.ID]
		if selection != "job-id" {
			include = model.ResultSelectionMatches(selection, finished, result.ExitCode)
		}
		if requested[command.ID] {
			include = true
			delete(requested, command.ID)
		}
		if include {
			plan.Execute[command.ID] = true
			continue
		}
		if !finished {
			continue
		}
		status := "failed"
		if result.ExitCode == 0 {
			status = "success"
		}
		if command.Array == nil {
			plan.CarriedResults[command.ID] = result
		} else {
			for _, task := range model.ArrayTaskIDs(command.Array) {
				taskID := fmt.Sprintf("%s-%d", command.ID, task)
				plan.CarriedResults[taskID] = reference.Results[taskID]
			}
		}
		plan.CarriedOrigins[command.ID] = origin(reference, command.ID, result, status)
	}
	if len(requested) > 0 {
		missing := make([]string, 0, len(requested))
		for jobID := range requested {
			missing = append(missing, jobID)
		}
		sort.Strings(missing)
		return Plan{}, fmt.Errorf("job IDs not found in queue: %s", joinStrings(missing, ", "))
	}
	return plan, nil
}

func planArrayTasks(command model.QueuedCommand, selection string, reference Reference, plan *Plan) {
	for _, task := range model.ArrayTaskIDs(command.Array) {
		taskID := fmt.Sprintf("%s-%d", command.ID, task)
		result, finished := reference.Results[taskID]
		if model.ResultSelectionMatches(selection, finished, result.ExitCode) {
			plan.Execute[taskID] = true
			continue
		}
		if !finished {
			continue
		}
		plan.CarriedResults[taskID] = result
		status := "failed"
		if result.ExitCode == 0 {
			status = "success"
		}
		plan.CarriedOrigins[taskID] = origin(reference, taskID, result, status)
	}
}

func origin(reference Reference, jobID string, result model.JobResult, status string) *model.JobOrigin {
	var submittedAt, finishedAt string
	if reference.SubmittedAt != nil {
		submittedAt = reference.SubmittedAt(jobID)
	}
	if reference.FinishedAt != nil {
		finishedAt = reference.FinishedAt(jobID)
	}
	return &model.JobOrigin{RunID: reference.RunID, JobID: jobID, AttemptID: result.AttemptID, Status: status, CWD: reference.CWD, SubmittedAt: submittedAt, FinishedAt: finishedAt}
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

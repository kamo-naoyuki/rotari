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

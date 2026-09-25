package run

import (
	"fmt"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// OriginResults reads the source results that queued commands' origins name.
// Implementations own the filesystem access and error wording for loading.
type OriginResults interface {
	// LastRunID returns the run that commands without an origin fall back to.
	// It reports an error when the project has no previous run.
	LastRunID() (string, error)
	// RunResults returns a run's summary results by job ID.
	RunResults(runID string) (map[string]model.JobResult, error)
	// AttemptResult returns the result recorded by the attempt named by
	// origin.AttemptID.
	AttemptResult(origin model.JobOrigin) (model.JobResult, bool, error)
	// Origin describes a result carried from runID for a job that has no
	// origin of its own.
	Origin(runID, jobID string, result model.JobResult) *model.JobOrigin
}

// PlanRerun decides which of the queue's jobs a new run executes.
//
// Without a selection every command executes, except in an imported workflow
// queue, whose commands carry explicit dispositions. With a selection only
// matching jobs execute; a job that does not match but has a finished result
// carries that result forward (Origin recorded, no re-execution), and one
// without a result is left untouched. Each command's result comes from its
// origin; a command without one falls back to referenceRunID, or to the
// project's last run when referenceRunID is empty.
//
// When partialArray is true, array jobs are evaluated per task, so only
// matching tasks execute and the rest carry their own results. When false,
// the whole array executes if its aggregate result matches.
func PlanRerun(queue model.Queue, selection string, jobIDs []string, referenceRunID string, partialArray bool, source OriginResults) (Plan, error) {
	if selection == "" && queue.WorkflowImport {
		return planImportedWorkflow(queue, source)
	}
	if selection == "" {
		plan := Plan{Execute: make(map[string]bool, len(queue.Commands))}
		for _, command := range queue.Commands {
			plan.Execute[command.ID] = true
		}
		return plan, nil
	}
	return planByOrigin(queue, selection, jobIDs, referenceRunID, partialArray, source)
}

func planImportedWorkflow(queue model.Queue, source OriginResults) (Plan, error) {
	// Imported jobs without an origin are new work. They must not fall back to
	// the project's last run, which the run server has already replaced with
	// the run being planned.
	withOrigin := model.Queue{Commands: make([]model.QueuedCommand, 0, len(queue.Commands))}
	var fresh []model.QueuedCommand
	for _, command := range queue.Commands {
		if command.Origin == nil && len(command.TaskOrigins) == 0 {
			fresh = append(fresh, command)
		} else {
			withOrigin.Commands = append(withOrigin.Commands, command)
		}
	}
	plan, err := planByOrigin(withOrigin, "failed,unfinished", nil, "", true, source)
	if err != nil {
		return Plan{}, err
	}
	for _, command := range fresh {
		if command.Array == nil {
			plan.Execute[command.ID] = true
			continue
		}
		for _, task := range model.ArrayTaskIDs(command.Array) {
			plan.Execute[taskID(command.ID, task)] = true
		}
	}
	for _, command := range queue.Commands {
		if err := applyImportedCommandPlan(command, &plan, source); err != nil {
			return Plan{}, err
		}
	}
	expandImportedDownstream(queue, &plan)
	return plan, nil
}

func applyImportedCommandPlan(command model.QueuedCommand, plan *Plan, source OriginResults) error {
	if command.Array == nil {
		if command.Accepted {
			if err := acceptImportedResult(command.ID, command.Origin, plan, source); err != nil {
				return err
			}
		}
		if command.Force {
			forceExecution(command.ID, plan)
		}
		return nil
	}
	for _, task := range model.ArrayTaskIDs(command.Array) {
		id := taskID(command.ID, task)
		if command.TaskAccepted[id] {
			if err := acceptImportedResult(id, TaskOrigin(command, task), plan, source); err != nil {
				return err
			}
		}
		if command.TaskForce[id] || command.Force {
			forceExecution(id, plan)
		}
	}
	return nil
}

func forceExecution(jobID string, plan *Plan) {
	plan.Execute[jobID] = true
	delete(plan.CarriedResults, jobID)
	delete(plan.CarriedOrigins, jobID)
}

func acceptImportedResult(destinationID string, origin *model.JobOrigin, plan *Plan, source OriginResults) error {
	if origin == nil {
		return fmt.Errorf("accepted job %q has no source origin", destinationID)
	}
	results, err := source.RunResults(origin.RunID)
	if err != nil {
		return fmt.Errorf("failed to load accepted source result: %w", err)
	}
	result, ok := results[origin.JobID]
	if origin.AttemptID != "" && (!ok || result.AttemptID != origin.AttemptID) {
		result, ok, err = source.AttemptResult(*origin)
		if err != nil {
			return err
		}
	}
	if !ok {
		return fmt.Errorf("accepted source result %q not found in run %q", origin.JobID, origin.RunID)
	}
	result.ID = destinationID
	result.ExitCode = 0
	result.Accepted = true
	result.Error = ""
	result.Diagnoses = nil
	delete(plan.Execute, destinationID)
	plan.CarriedResults[destinationID] = result
	plan.CarriedOrigins[destinationID] = origin
	return nil
}

// expandImportedDownstream executes every job that depends, directly or
// transitively, on an executing job.
func expandImportedDownstream(queue model.Queue, plan *Plan) {
	jobs := model.QueueToJobs(queue.Commands)
	executingNames := make(map[string]bool)
	for _, job := range jobs {
		if plan.Execute[job.ID] && job.Name != "" {
			executingNames[job.Name] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, job := range jobs {
			if plan.Execute[job.ID] || !dependsOnAny(job, executingNames) {
				continue
			}
			forceExecution(job.ID, plan)
			if job.Name != "" {
				executingNames[job.Name] = true
			}
			changed = true
		}
	}
}

func dependsOnAny(job model.JobSpec, names map[string]bool) bool {
	for _, dependency := range job.DependsOn {
		if names[dependency] {
			return true
		}
	}
	return false
}

// originResolver looks up the result each queued job's origin names, caching
// run summaries and resolving the fallback run on first use.
type originResolver struct {
	source        OriginResults
	fallbackRunID string
	resultsByRun  map[string]map[string]model.JobResult
}

func (resolver *originResolver) result(origin *model.JobOrigin, fallbackJobID string) (model.JobResult, bool, error) {
	if origin == nil || origin.RunID == "" || origin.JobID == "" {
		if resolver.fallbackRunID == "" {
			runID, err := resolver.source.LastRunID()
			if err != nil {
				return model.JobResult{}, false, err
			}
			resolver.fallbackRunID = runID
		}
		origin = &model.JobOrigin{RunID: resolver.fallbackRunID, JobID: fallbackJobID}
	}
	results, ok := resolver.resultsByRun[origin.RunID]
	if !ok {
		loaded, err := resolver.source.RunResults(origin.RunID)
		if err != nil {
			return model.JobResult{}, false, fmt.Errorf("failed to load run summary for origin run %q: %w", origin.RunID, err)
		}
		results = loaded
		resolver.resultsByRun[origin.RunID] = results
	}
	result, ok := results[origin.JobID]
	if origin.AttemptID != "" && (!ok || result.AttemptID != origin.AttemptID) {
		return resolver.source.AttemptResult(*origin)
	}
	return result, ok, nil
}

func (resolver *originResolver) fallbackOrigin(jobID string, result model.JobResult) *model.JobOrigin {
	return resolver.source.Origin(resolver.fallbackRunID, jobID, result)
}

func (resolver *originResolver) commandResult(command model.QueuedCommand) (model.JobResult, bool, error) {
	if command.Array == nil {
		return resolver.result(command.Origin, command.ID)
	}
	results := make(map[string]model.JobResult)
	for _, task := range model.ArrayTaskIDs(command.Array) {
		id := taskID(command.ID, task)
		result, finished, err := resolver.result(TaskOrigin(command, task), id)
		if err != nil {
			return model.JobResult{}, false, err
		}
		if finished {
			results[id] = result
		}
	}
	result, finished := model.AggregatedJobResult(command.ID, command.Array, results)
	return result, finished, nil
}

// planByOrigin applies a selection to the result recorded by each command's
// origin. Commands added directly to the queue have no origin and fall back
// to the reference run.
func planByOrigin(queue model.Queue, selection string, jobIDs []string, referenceRunID string, partialArray bool, source OriginResults) (Plan, error) {
	requested := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		requested[jobID] = true
	}
	plan := Plan{
		Execute:        make(map[string]bool, len(queue.Commands)),
		CarriedResults: make(map[string]model.JobResult),
		CarriedOrigins: make(map[string]*model.JobOrigin),
	}
	resolver := &originResolver{source: source, fallbackRunID: referenceRunID, resultsByRun: make(map[string]map[string]model.JobResult)}
	for _, command := range queue.Commands {
		if command.Array != nil && partialArray && selection != "job-id" {
			for _, task := range model.ArrayTaskIDs(command.Array) {
				id := taskID(command.ID, task)
				origin := TaskOrigin(command, task)
				result, finished, err := resolver.result(origin, id)
				if err != nil {
					return Plan{}, err
				}
				if model.ResultSelectionMatches(selection, finished, result.ExitCode) {
					plan.Execute[id] = true
					continue
				}
				if finished {
					result.ID = id
					plan.CarriedResults[id] = result
					if origin == nil {
						plan.CarriedOrigins[id] = resolver.fallbackOrigin(id, result)
					}
				}
			}
			continue
		}

		result, finished, err := resolver.commandResult(command)
		if err != nil {
			return Plan{}, err
		}
		include := requested[command.ID]
		if selection != "job-id" {
			include = model.ResultSelectionMatches(selection, finished, result.ExitCode)
		}
		delete(requested, command.ID)
		if include {
			plan.Execute[command.ID] = true
			continue
		}
		if !finished {
			continue
		}
		if command.Array == nil {
			result.ID = command.ID
			plan.CarriedResults[command.ID] = result
			if command.Origin == nil {
				plan.CarriedOrigins[command.ID] = resolver.fallbackOrigin(command.ID, result)
			}
			continue
		}
		for _, task := range model.ArrayTaskIDs(command.Array) {
			id := taskID(command.ID, task)
			origin := TaskOrigin(command, task)
			taskResult, taskFinished, err := resolver.result(origin, id)
			if err != nil {
				return Plan{}, err
			}
			if taskFinished {
				taskResult.ID = id
				plan.CarriedResults[id] = taskResult
				if origin == nil {
					plan.CarriedOrigins[id] = resolver.fallbackOrigin(id, taskResult)
				}
			}
		}
		if command.Origin == nil {
			plan.CarriedOrigins[command.ID] = resolver.fallbackOrigin(command.ID, result)
		}
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

// TaskOrigin returns the origin of one task of an array command: its own
// task origin, or the command's origin narrowed to the task.
func TaskOrigin(command model.QueuedCommand, task int) *model.JobOrigin {
	if origin := command.TaskOrigins[taskID(command.ID, task)]; origin != nil {
		return origin
	}
	if command.Origin == nil {
		return nil
	}
	origin := *command.Origin
	origin.JobID = taskID(command.Origin.JobID, task)
	return &origin
}

func taskID(commandID string, task int) string {
	return fmt.Sprintf("%s-%d", commandID, task)
}

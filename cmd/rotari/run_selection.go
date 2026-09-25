package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

var errNoPreviousRun = errors.New("no previous run")

// rerunPlan splits a queue's commands between jobs that must be executed and
// jobs whose previous result should be carried forward into the new run
// instead of being re-executed.
type rerunPlan = runcontract.Plan

// planRerunSelection decides, for a run of the given queue, which jobs must
// actually execute. When selection is empty every job executes normally.
// When selection filters are provided, only matching jobs execute; jobs that
// do not match but already have a finished result in referenceRunID are
// carried forward (Origin recorded, no re-execution); jobs that neither
// match nor have a previous result are left untouched (no result recorded,
// shown as still unfinished).
//
// When partialArray is true (the default), array jobs are evaluated per
// task instead of as one unit: only tasks matching the selection (e.g. the
// failed ones) execute, and the rest carry forward their own result. When
// false, the whole array re-executes if any of its tasks match, restoring
// the older, coarser behavior.
func planRerunSelection(paths pathSet, queue Queue, selection string, jobIDs []string, referenceRunID string, partialArray bool) (rerunPlan, error) {
	if selection == "" {
		plan, err := runcontract.PlanSelection(model.Queue(queue), selection, jobIDs, partialArray, runcontract.Reference{})
		return rerunPlan(plan), err
	}
	return planQueuedSelectionByOrigin(paths, queue, selection, jobIDs, referenceRunID, partialArray)
}

// planQueuedSelectionByOrigin applies filters to the result recorded by each
// copied command's origin. Commands added directly to the queue have no origin
// and are consequently unfinished until they are executed.
func planQueuedSelectionByOrigin(paths pathSet, queue Queue, selection string, jobIDs []string, referenceRunID string, partialArray bool) (rerunPlan, error) {
	requested := make(map[string]bool, len(jobIDs))
	for _, jobID := range jobIDs {
		requested[jobID] = true
	}
	plan := rerunPlan{
		Execute:        make(map[string]bool, len(queue.Commands)),
		CarriedResults: make(map[string]JobResult),
		CarriedOrigins: make(map[string]*JobOrigin),
	}
	resultsByRun := make(map[string]map[string]JobResult)
	fallbackRunID := referenceRunID
	fallbackOrigin := func(jobID string, result JobResult) *JobOrigin {
		runDir := filepath.Join(paths.RunsDir, fallbackRunID)
		cwd := ""
		if context, err := state.LoadContext(jsonStore(), runDir); err == nil {
			cwd = context.CWD
		}
		status := "failed"
		if result.ExitCode == 0 {
			status = "success"
		}
		return &JobOrigin{
			RunID: fallbackRunID, JobID: jobID, AttemptID: result.AttemptID, Status: status, CWD: cwd,
			SubmittedAt: state.ReadJobTimestamp(runDir, jobID, "submitted_at"),
			FinishedAt:  state.ReadJobTimestamp(runDir, jobID, "finished_at"),
		}
	}
	resultFor := func(origin *JobOrigin, fallbackJobID string) (JobResult, bool, error) {
		if origin == nil || origin.RunID == "" || origin.JobID == "" {
			if fallbackRunID == "" {
				meta, err := state.LoadMeta(paths.MetaFile)
				if err != nil {
					if errors.Is(err, os.ErrNotExist) {
						return JobResult{}, false, fmt.Errorf("project '%s' has no previous run: %w", paths.ProjectName, errNoPreviousRun)
					}
					return JobResult{}, false, fmt.Errorf("failed to load metadata: %w", err)
				}
				if meta.LastRunID == "" {
					return JobResult{}, false, fmt.Errorf("project '%s' has no previous run: %w", paths.ProjectName, errNoPreviousRun)
				}
				fallbackRunID = meta.LastRunID
			}
			origin = &JobOrigin{RunID: fallbackRunID, JobID: fallbackJobID}
		}
		results, ok := resultsByRun[origin.RunID]
		if !ok {
			runDir := filepath.Join(paths.RunsDir, origin.RunID)
			summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
			if err != nil {
				return JobResult{}, false, fmt.Errorf("failed to load run summary for origin run %q: %w", origin.RunID, err)
			}
			results = model.ResultsByID(summary.Results)
			resultsByRun[origin.RunID] = results
		}
		result, ok := results[origin.JobID]
		return result, ok, nil
	}
	for _, command := range queue.Commands {
		if command.Array != nil && partialArray && selection != "job-id" {
			for _, task := range model.ArrayTaskIDs(command.Array) {
				taskID := fmt.Sprintf("%s-%d", command.ID, task)
				origin := queuedTaskOrigin(command, taskID, task)
				result, finished, err := resultFor(origin, taskID)
				if err != nil {
					return rerunPlan{}, err
				}
				if model.ResultSelectionMatches(selection, finished, result.ExitCode) {
					plan.Execute[taskID] = true
					continue
				}
				if finished {
					result.ID = taskID
					plan.CarriedResults[taskID] = result
					if origin == nil {
						plan.CarriedOrigins[taskID] = fallbackOrigin(taskID, result)
					}
				}
			}
			continue
		}

		result, finished, err := queuedCommandResult(command, resultFor)
		if err != nil {
			return rerunPlan{}, err
		}
		include := requested[command.ID]
		if selection != "job-id" {
			include = model.ResultSelectionMatches(selection, finished, result.ExitCode)
		}
		if requested[command.ID] {
			delete(requested, command.ID)
		}
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
				plan.CarriedOrigins[command.ID] = fallbackOrigin(command.ID, result)
			}
			continue
		}
		for _, task := range model.ArrayTaskIDs(command.Array) {
			taskID := fmt.Sprintf("%s-%d", command.ID, task)
			taskResult, taskFinished, taskErr := resultFor(queuedTaskOrigin(command, taskID, task), taskID)
			if taskErr != nil {
				return rerunPlan{}, taskErr
			}
			if taskFinished {
				taskResult.ID = taskID
				plan.CarriedResults[taskID] = taskResult
				if queuedTaskOrigin(command, taskID, task) == nil {
					plan.CarriedOrigins[taskID] = fallbackOrigin(taskID, taskResult)
				}
			}
		}
		if command.Origin == nil {
			plan.CarriedOrigins[command.ID] = fallbackOrigin(command.ID, result)
		}
	}
	if len(requested) > 0 {
		missing := make([]string, 0, len(requested))
		for jobID := range requested {
			missing = append(missing, jobID)
		}
		sort.Strings(missing)
		return rerunPlan{}, fmt.Errorf("job IDs not found in queue: %s", strings.Join(missing, ", "))
	}
	return plan, nil
}

func queuedCommandResult(command QueuedCommand, resultFor func(*JobOrigin, string) (JobResult, bool, error)) (JobResult, bool, error) {
	if command.Array == nil {
		return resultFor(command.Origin, command.ID)
	}
	results := make(map[string]JobResult)
	for _, task := range model.ArrayTaskIDs(command.Array) {
		taskID := fmt.Sprintf("%s-%d", command.ID, task)
		result, finished, err := resultFor(queuedTaskOrigin(command, taskID, task), taskID)
		if err != nil {
			return JobResult{}, false, err
		}
		if finished {
			results[taskID] = result
		}
	}
	result, finished := model.AggregatedJobResult(command.ID, command.Array, results)
	return result, finished, nil
}

func queuedTaskOrigin(command QueuedCommand, taskID string, task int) *JobOrigin {
	if origin := command.TaskOrigins[taskID]; origin != nil {
		return origin
	}
	if command.Origin == nil {
		return nil
	}
	origin := *command.Origin
	origin.JobID = fmt.Sprintf("%s-%d", command.Origin.JobID, task)
	return &origin
}

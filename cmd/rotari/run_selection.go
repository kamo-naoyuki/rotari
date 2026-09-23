package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
)

var errNoPreviousRun = errors.New("no previous run")

func resultSelection(failed, unfinished, success bool) string {
	return model.ResultSelection(failed, unfinished, success)
}

func resultSelectionMatches(selection string, finished bool, exitCode int) bool {
	return model.ResultSelectionMatches(selection, finished, exitCode)
}

// aggregatedJobResult returns the result and finished state for a command
// ID, aggregating per-task results when array is non-nil: results are keyed
// per task (e.g. "id-1", "id-2", ...) by queueToJobs, not by the array
// command's own ID, so it is only considered finished once every task has a
// result, and any non-zero task exit code marks the whole command as failed.
func aggregatedJobResult(id string, array *ArraySpec, results map[string]JobResult) (JobResult, bool) {
	return model.AggregatedJobResult(id, array, results)
}

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
	runID := referenceRunID
	if runID == "" {
		meta, err := loadMeta(paths.metaFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return rerunPlan{}, fmt.Errorf("project '%s' has no previous run: %w", paths.queueName, errNoPreviousRun)
			}
			return rerunPlan{}, fmt.Errorf("failed to load metadata: %w", err)
		}
		if meta.LastRunID == "" {
			return rerunPlan{}, fmt.Errorf("project '%s' has no previous run: %w", paths.queueName, errNoPreviousRun)
		}
		runID = meta.LastRunID
	}
	runDir := filepath.Join(paths.runsDir, runID)
	summary, err := loadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return rerunPlan{}, fmt.Errorf("failed to load run summary: %w", err)
	}
	results := jobResultsByID(summary.Results)
	originCWD := ""
	if data, contextErr := os.ReadFile(filepath.Join(runDir, "context.json")); contextErr == nil {
		var context RunContext
		if json.Unmarshal(data, &context) == nil {
			originCWD = context.CWD
		}
	}
	plan, err := runcontract.PlanSelection(model.Queue(queue), selection, jobIDs, partialArray, runcontract.Reference{
		RunID: runID, CWD: originCWD, Results: results,
		SubmittedAt: func(jobID string) string { return readJobTimestamp(runDir, jobID, "submitted_at") },
		FinishedAt:  func(jobID string) string { return readJobTimestamp(runDir, jobID, "finished_at") },
	})
	return rerunPlan(plan), err
}

// planArrayTaskSelection evaluates each task of an array command on its own
// (unlike aggregatedJobResult, which treats the whole array as one unit):
// only tasks matching selection are added to Execute, and finished
// non-matching tasks carry their own result forward instead of the whole
// array re-executing. The per-task Origin (keyed by task ID, e.g. "id-1")
// lets show/web follow a carried task back to its own job directory in the
// reference run, since QueuedCommand.Origin only covers the whole command.
func planArrayTaskSelection(command QueuedCommand, selection string, results map[string]JobResult, runID, runDir, originCWD string, plan *rerunPlan) {
	for _, task := range arrayTaskIDs(command.Array) {
		taskID := fmt.Sprintf("%s-%d", command.ID, task)
		result, finished := results[taskID]
		if resultSelectionMatches(selection, finished, result.ExitCode) {
			plan.Execute[taskID] = true
			continue
		}
		if !finished {
			// No previous result to carry forward; leave it unfinished.
			continue
		}
		plan.CarriedResults[taskID] = result
		status := "failed"
		if result.ExitCode == 0 {
			status = "success"
		}
		plan.CarriedOrigins[taskID] = &JobOrigin{
			RunID:       runID,
			JobID:       taskID,
			AttemptID:   result.AttemptID,
			Status:      status,
			CWD:         originCWD,
			SubmittedAt: readJobTimestamp(runDir, taskID, "submitted_at"),
			FinishedAt:  readJobTimestamp(runDir, taskID, "finished_at"),
		}
	}
}

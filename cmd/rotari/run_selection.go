package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	runID := referenceRunID
	if runID == "" {
		meta, err := state.LoadMeta(paths.MetaFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return rerunPlan{}, fmt.Errorf("project '%s' has no previous run: %w", paths.ProjectName, errNoPreviousRun)
			}
			return rerunPlan{}, fmt.Errorf("failed to load metadata: %w", err)
		}
		if meta.LastRunID == "" {
			return rerunPlan{}, fmt.Errorf("project '%s' has no previous run: %w", paths.ProjectName, errNoPreviousRun)
		}
		runID = meta.LastRunID
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return rerunPlan{}, fmt.Errorf("failed to load run summary: %w", err)
	}
	results := model.ResultsByID(summary.Results)
	originCWD := ""
	if context, contextErr := state.LoadContext(jsonStore(), runDir); contextErr == nil {
		originCWD = context.CWD
	}
	plan, err := runcontract.PlanSelection(model.Queue(queue), selection, jobIDs, partialArray, runcontract.Reference{
		RunID: runID, CWD: originCWD, Results: results,
		SubmittedAt: func(jobID string) string { return state.ReadJobTimestamp(runDir, jobID, "submitted_at") },
		FinishedAt:  func(jobID string) string { return state.ReadJobTimestamp(runDir, jobID, "finished_at") },
	})
	return rerunPlan(plan), err
}

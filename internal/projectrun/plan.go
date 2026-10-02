package projectrun

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// PlanRequest is what planning a run of a queue needs.
type PlanRequest struct {
	Executor        string
	ExecutorOptions []string
	Settings        executor.RunSettingsMap
	Selection       string
	JobIDs          []string
	Scope           model.CommandSelector
	Filter          jobfilter.Filter
	SourceRunID     string
	PartialArray    bool
	MatchBy         string
}

// PlannedRun is a planned run: the queue it runs, with fingerprint matches
// applied, which of its jobs execute and which carry results, and the
// resolved reference run.
type PlannedRun struct {
	Queue       model.Queue
	Plan        run.Plan
	SourceRunID string
}

// PlanRun plans a run of queue as the run itself will: it validates the
// queue, resolves the reference run, applies fingerprint matching, and plans
// the selection. A run request and a preview of it both use it, so the
// preview shows what the run would do. The caller holds the project's state
// lock and has checked that the project is idle.
func (runner Runner) PlanRun(paths state.ProjectPaths, queue model.Queue, request PlanRequest) (PlannedRun, error) {
	if err := runner.ValidateQueue(queue, request.Executor, request.ExecutorOptions, request.Settings); err != nil {
		return PlannedRun{}, err
	}
	if len(queue.Commands) == 0 {
		return PlannedRun{}, fmt.Errorf("queue %q has no queued commands", paths.ProjectName)
	}
	// A scope also needs validation for unfiltered runs, which do not use it
	// when planning their execution.
	if request.Scope.Kinds() > 0 {
		if _, err := model.SelectCommands(queue.Commands, request.Scope); err != nil {
			return PlannedRun{}, err
		}
	}
	sourceRunID, err := ReferenceRun(paths, request.Selection, request.SourceRunID)
	if err != nil {
		return PlannedRun{}, err
	}
	if request.MatchBy != "" || request.Filter.Changed || request.Filter.New {
		sourceRunID, err = FingerprintReferenceRun(paths, sourceRunID)
		if err != nil {
			return PlannedRun{}, err
		}
	}
	if request.MatchBy != "" {
		queue, err = runner.MatchFingerprintQueue(paths, queue, request.MatchBy, sourceRunID)
		if err != nil {
			return PlannedRun{}, err
		}
	}
	filter, err := ClassifyDefinitions(paths, queue, request.Filter, sourceRunID, request.MatchBy)
	if err != nil {
		return PlannedRun{}, err
	}
	plan, err := runner.PlanSelection(paths, queue, request.Selection, request.JobIDs, request.Scope, filter, sourceRunID, request.PartialArray)
	if err != nil {
		return PlannedRun{}, err
	}
	return PlannedRun{Queue: queue, Plan: plan, SourceRunID: sourceRunID}, nil
}

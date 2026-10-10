package projectrun

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
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
	SourcePolicy    SourcePolicy
	CopyAttempts    bool
	CopyJobIDs      []string
	PartialArray    bool
	MatchBy         string
}

// PlannedRun is a planned run: the queue it runs, with fingerprint matches
// applied, which of its jobs execute and which carry results, and the
// resolved reference run.
type PlannedRun struct {
	Queue               model.Queue
	Plan                run.Plan
	SourceRunID         string
	SnapshotFromSource  bool
	UsedQueueWithSource bool
	// OmittedSourceJobs are failed or unfinished jobs from SourceRunID that
	// are not represented by the non-empty queue used for this run.
	OmittedSourceJobs []string
}

// PlanRun plans a run of queue as the run itself will: it validates the
// queue, resolves the reference run, applies fingerprint matching, and plans
// the selection. A run request and a preview of it both use it, so the
// preview shows what the run would do. The caller holds the project's state
// lock and has checked that the project is idle.
func (runner Runner) PlanRun(paths state.ProjectPaths, queue model.Queue, request PlanRequest) (PlannedRun, error) {
	if !validSourcePolicy(request.SourcePolicy) {
		return PlannedRun{}, fmt.Errorf("invalid run source policy %q", request.SourcePolicy)
	}
	if request.SourcePolicy == SourceCopyIfEmpty {
		// Automatic latest-run selection must be refreshed under this lock,
		// not pinned to the client's earlier metadata read.
		request.SourceRunID = ""
	}
	queueWasNonEmpty := len(queue.Commands) > 0
	queue, request, copySource, err := runner.sourceSnapshot(paths, queue, request)
	if err != nil {
		return PlannedRun{}, err
	}
	planned, err := runner.planQueue(paths, queue, request)
	if err != nil {
		return PlannedRun{}, err
	}
	planned.SnapshotFromSource = copySource
	planned.UsedQueueWithSource = queueWasNonEmpty && request.SourcePolicy == SourceCopyIfEmpty
	if planned.UsedQueueWithSource && planned.SourceRunID != "" {
		planned.OmittedSourceJobs, err = runner.OmittedFailedOrUnfinished(paths, planned.Queue, planned.SourceRunID)
		if err != nil {
			return PlannedRun{}, err
		}
	}
	return planned, nil
}

func validSourcePolicy(policy SourcePolicy) bool {
	switch policy {
	case "", SourceKeepQueue, SourceCopyIfEmpty, SourceCopyRun:
		return true
	default:
		return false
	}
}

func (runner Runner) sourceSnapshot(paths state.ProjectPaths, queue model.Queue, request PlanRequest) (model.Queue, PlanRequest, bool, error) {
	copySource := request.SourcePolicy == SourceCopyRun || (request.SourcePolicy == SourceCopyIfEmpty && len(queue.Commands) == 0)
	if !copySource {
		return queue, request, false, nil
	}
	sourceRunID, err := sourceRunIDForPlan(paths, request)
	if err != nil {
		return model.Queue{}, request, false, err
	}
	copyRequest := queueedit.CopyRequest{Selection: "all", Overwrite: true}
	if request.CopyAttempts {
		copyRequest.Selection, copyRequest.JobIDs = "job-id", request.CopyJobIDs
	}
	copied, _, err := (queueops.Editor{Store: runner.Store, Executors: runner.Executors, NewJobID: runner.NewJobID}).CopySnapshot(paths.BaseDir, paths.ProjectName, sourceRunID, queue, copyRequest)
	if err != nil {
		return model.Queue{}, request, false, err
	}
	request.SourceRunID = sourceRunID
	return copied, request, true, nil
}

func sourceRunIDForPlan(paths state.ProjectPaths, request PlanRequest) (string, error) {
	if request.SourceRunID != "" {
		return request.SourceRunID, nil
	}
	sourceRunID, err := ReferenceRun(paths, request.Selection, "")
	if err != nil {
		return "", err
	}
	if sourceRunID == "" {
		return "", fmt.Errorf("queue %q has no previous run", paths.ProjectName)
	}
	return sourceRunID, nil
}

func (runner Runner) planQueue(paths state.ProjectPaths, queue model.Queue, request PlanRequest) (PlannedRun, error) {
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

// OmittedFailedOrUnfinished reports source-run jobs selected by the shared
// failed/unfinished selection rule that are not represented in queue by ID or
// provenance. It selects recorded results, not an expanded execution plan:
// successful dependents must not be counted as failed or unfinished work.
func (runner Runner) OmittedFailedOrUnfinished(paths state.ProjectPaths, queue model.Queue, sourceRunID string) ([]string, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, sourceRunID)
	if err != nil {
		return nil, err
	}
	sourceQueue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to load source run commands: %w", err)
	}
	selection := model.ResultSelection(true, true, false)
	results, err := (originResults{paths: paths, store: runner.Store}).RunResults(sourceRunID)
	if err != nil {
		return nil, err
	}
	sourceIDs := make(map[string]bool)
	for _, job := range model.QueueToJobs(sourceQueue.Commands) {
		result, finished := results[job.ID]
		if (jobfilter.Filter{}).Selects(selection, jobfilter.Job{ID: job.ID, Result: result, Finished: finished}) {
			sourceIDs[job.ID] = true
		}
	}
	included := make(map[string]bool)
	for _, command := range queue.Commands {
		for _, job := range model.QueueToJobs([]model.QueuedCommand{command}) {
			origin := command.Origin
			if command.Array != nil {
				origin = run.TaskOrigin(command, *job.ArrayTaskID)
			}
			if origin == nil {
				included[job.ID] = true
			} else if origin.RunID == sourceRunID {
				included[origin.JobID] = true
			}
		}
	}
	var omitted []string
	for id := range sourceIDs {
		if !included[id] {
			omitted = append(omitted, id)
		}
	}
	sort.Strings(omitted)
	return omitted, nil
}

// PreviewRun plans a run of the project without starting it, as a run would
// plan it under the state lock: the project must be idle and, when
// ifRevision is set, still at that revision, with the same plan when the
// revision is a preview's. queue, when set, replaces the project's queue,
// such as the queue a copy from the reference run would leave. It returns
// the plan and its revision, PlanRevision.
func (runner Runner) PreviewRun(paths state.ProjectPaths, queue *model.Queue, request PlanRequest, ifRevision string) (PlannedRun, string, error) {
	release, err := state.AcquireStateReadLock(paths.StateLockFile)
	if err != nil {
		return PlannedRun{}, "", fmt.Errorf("failed to lock project state: %w", err)
	}
	defer release()
	if err := project.EnsureIdle(paths, "run"); err != nil {
		return PlannedRun{}, "", err
	}
	revision, err := CheckRunProjectRevision(paths, ifRevision)
	if err != nil {
		return PlannedRun{}, "", err
	}
	if queue == nil {
		loaded, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			return PlannedRun{}, "", fmt.Errorf("failed to load queue: %w", err)
		}
		queue = &loaded
	}
	planned, err := runner.PlanRun(paths, *queue, request)
	if err != nil {
		return PlannedRun{}, "", err
	}
	if err := CheckRunPlanRevision(ifRevision, revision, planned); err != nil {
		return PlannedRun{}, "", err
	}
	return planned, PlanRevision(revision, planned), nil
}

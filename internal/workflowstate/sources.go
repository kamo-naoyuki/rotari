package workflowstate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

const (
	submittedAtFileName = "submitted_at"
	finishedAtFileName  = "finished_at"
	wrapperStatusName   = "status.json"
)

// Reconcile matches a queue compiled from manifest to the source runs it was
// exported from; see workflow.Reconcile. A manifest without a source is
// returned unchanged.
func Reconcile(store state.Store, baseDir string, manifest workflow.Manifest, queue model.Queue) (model.Queue, []workflow.RemovedJob, error) {
	if manifest.Source == nil {
		return queue, nil, nil
	}
	paths, err := state.ResolveProjectPaths(baseDir, manifest.Source.Project)
	if err != nil {
		return model.Queue{}, nil, err
	}
	queue, removed, err := workflow.Reconcile(manifest, queue, Sources{Paths: paths, Store: store})
	if err != nil {
		return model.Queue{}, nil, err
	}
	if err := queueops.ValidateJobs(queue); err != nil {
		return model.Queue{}, nil, err
	}
	return queue, removed, nil
}

// Sources reads a project's runs and attempts as workflow sources.
type Sources struct {
	Paths state.ProjectPaths
	Store state.Store
}

// Run reads runID as a workflow source run.
func (sources Sources) Run(runID string) (workflow.SourceRun, error) {
	if !state.IsValidPathElement(runID) {
		return workflow.SourceRun{}, fmt.Errorf("invalid source run ID %q", runID)
	}
	runDir := filepath.Join(sources.Paths.RunsDir, runID)
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load source run %q commands: %w", runID, err)
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load source run %q summary: %w", runID, err)
	}
	run := SourceRun(runID, runDir, queue, summary)
	if context, contextErr := state.LoadContext(sources.Store, runDir); contextErr == nil {
		run.CWD = context.CWD
	}
	return run, nil
}

// Attempt reads one attempt's result, or reports false while it has none.
func (sources Sources) Attempt(runID, jobID, attemptID string, command []string) (model.JobResult, bool, error) {
	runDir, err := state.SafeJoin(sources.Paths.RunsDir, runID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, jobID, attemptID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	if info, err := os.Stat(attemptDir); err != nil || !info.IsDir() {
		return model.JobResult{}, false, fmt.Errorf("attempt %q not found", attemptID)
	}
	if local, ok := state.LoadLocalJobResult(attemptDir, model.JobSpec{ID: jobID, Command: command}); ok {
		local.AttemptID = attemptID
		return local, true, nil
	}
	status, ok := executor.LoadWrapperStatus(sources.Store, filepath.Join(attemptDir, wrapperStatusName))
	if !ok || (status.Phase != "finished" && status.Phase != "failed" && status.Phase != "cancelled") {
		return model.JobResult{}, false, nil
	}
	return model.JobResult{ID: jobID, AttemptID: attemptID, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}, true, nil
}

// AttemptTimestamps returns when an attempt was submitted and finished.
func (sources Sources) AttemptTimestamps(runID, jobID, attemptID string) (string, string) {
	runDir, err := state.SafeJoin(sources.Paths.RunsDir, runID)
	if err != nil {
		return "", ""
	}
	if attemptDir, err := state.SpecificAttemptJobDir(runDir, jobID, attemptID); err == nil {
		return state.ReadAttemptTimestamp(attemptDir, submittedAtFileName), state.ReadAttemptTimestamp(attemptDir, finishedAtFileName)
	}
	return state.ReadJobTimestamp(runDir, jobID, submittedAtFileName), state.ReadJobTimestamp(runDir, jobID, finishedAtFileName)
}

// SourceRun describes a loaded run as a workflow source.
func SourceRun(runID, runDir string, queue model.Queue, summary model.RunSummary) workflow.SourceRun {
	return workflow.SourceRun{
		ID: runID, Queue: queue, Summary: summary,
		JobTimestamps: func(jobID string) (string, string) {
			return state.ReadJobTimestamp(runDir, jobID, submittedAtFileName), state.ReadJobTimestamp(runDir, jobID, finishedAtFileName)
		},
	}
}

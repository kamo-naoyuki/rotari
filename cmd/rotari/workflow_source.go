package main

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

// reconcileWorkflowManifest matches a queue compiled from manifest to the
// source runs it was exported from; see workflow.Reconcile.
func reconcileWorkflowManifest(baseDir string, manifest workflow.Manifest, queue model.Queue) (model.Queue, []workflow.RemovedJob, error) {
	if manifest.Source == nil {
		return queue, nil, nil
	}
	paths, err := state.ResolveProjectPaths(baseDir, manifest.Source.Project)
	if err != nil {
		return model.Queue{}, nil, err
	}
	queue, removed, err := workflow.Reconcile(manifest, queue, projectWorkflowSources{paths: paths})
	if err != nil {
		return model.Queue{}, nil, err
	}
	if err := queueops.ValidateJobs(queue); err != nil {
		return model.Queue{}, nil, err
	}
	return queue, removed, nil
}

// projectWorkflowSources reads a project's runs and attempts for workflow
// import.
type projectWorkflowSources struct {
	paths state.ProjectPaths
}

func (sources projectWorkflowSources) Run(runID string) (workflow.SourceRun, error) {
	if !state.IsValidPathElement(runID) {
		return workflow.SourceRun{}, fmt.Errorf("invalid source run ID %q", runID)
	}
	runDir := filepath.Join(sources.paths.RunsDir, runID)
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load source run %q commands: %w", runID, err)
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load source run %q summary: %w", runID, err)
	}
	run := workflowSourceRun(runID, runDir, queue, summary)
	if context, contextErr := state.LoadContext(jsonStore(), runDir); contextErr == nil {
		run.CWD = context.CWD
	}
	return run, nil
}

func (sources projectWorkflowSources) Attempt(runID, jobID, attemptID string, command []string) (model.JobResult, bool, error) {
	runDir, err := state.SafeJoin(sources.paths.RunsDir, runID)
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
	status, ok := executor.LoadWrapperStatus(jsonStore(), filepath.Join(attemptDir, statusJSONName))
	if !ok || (status.Phase != "finished" && status.Phase != "failed" && status.Phase != "cancelled") {
		return model.JobResult{}, false, nil
	}
	return model.JobResult{ID: jobID, AttemptID: attemptID, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}, true, nil
}

func (sources projectWorkflowSources) AttemptTimestamps(runID, jobID, attemptID string) (string, string) {
	runDir, err := state.SafeJoin(sources.paths.RunsDir, runID)
	if err != nil {
		return "", ""
	}
	if attemptDir, err := state.SpecificAttemptJobDir(runDir, jobID, attemptID); err == nil {
		return state.ReadAttemptTimestamp(attemptDir, stateFileSubmittedAt), state.ReadAttemptTimestamp(attemptDir, stateFileFinishedAt)
	}
	return state.ReadJobTimestamp(runDir, jobID, stateFileSubmittedAt), state.ReadJobTimestamp(runDir, jobID, stateFileFinishedAt)
}

func workflowSourceRun(runID, runDir string, queue model.Queue, summary model.RunSummary) workflow.SourceRun {
	return workflow.SourceRun{
		ID: runID, Queue: queue, Summary: summary,
		JobTimestamps: func(jobID string) (string, string) {
			return state.ReadJobTimestamp(runDir, jobID, stateFileSubmittedAt), state.ReadJobTimestamp(runDir, jobID, stateFileFinishedAt)
		},
	}
}

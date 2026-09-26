package main

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Test shorthands for the projectrun lifecycle with this command's wiring.

var errNoPreviousRun = projectrun.ErrNoPreviousRun

// executeMixedRun executes the project's queue as run runID without the
// surrounding Begin and Finish steps.
func executeMixedRun(paths state.ProjectPaths, runID, runName string, localConcurrency, batchMaxActive, retry int, requestedExecutor string, executorOptions []string, selection string, jobIDs []string, referenceRunID string, partialArray bool, progress func(model.JobResult, int, int, int, int), onStart func(model.JobSpec), settings ...executor.RunSettingsMap) int {
	options := projectrun.Options{
		RunID: runID, RunName: runName, LocalConcurrency: localConcurrency, BatchMaxActive: batchMaxActive, Retry: retry,
		Executor: requestedExecutor, ExecutorOptions: executorOptions,
		Selection: selection, JobIDs: jobIDs, SourceRunID: referenceRunID, PartialArray: partialArray,
	}
	if len(settings) > 0 {
		options.Settings = settings[0]
	}
	exitCode, err := projectRunner().Execute(paths, options, projectrun.Observer{Progress: progress, Started: onStart})
	if err != nil {
		printError(err)
	}
	return exitCode
}

func writeRunContext(paths state.ProjectPaths, runID, cwd string) error {
	return projectRunner().WriteContext(paths, runID, cwd)
}

func finishRunContext(paths state.ProjectPaths, runID string) error {
	return projectRunner().FinishContext(paths, runID)
}

func prepareJobEnvironments(paths state.ProjectPaths, runID string, jobs []model.JobSpec, runName string, localConcurrency, batchConcurrency, retry int, executorOptions []string) {
	projectRunner().PrepareJobEnvironments(paths, projectrun.Options{
		RunID: runID, RunName: runName, LocalConcurrency: localConcurrency, BatchMaxActive: batchConcurrency,
		Retry: retry, ExecutorOptions: executorOptions,
	}, jobs)
}

func assignAttemptIDs(jobs []model.JobSpec, runID string, attempt int) {
	projectRunner().AssignAttemptIDs(jobs, runID, attempt)
}

func jobWasExplicitlyCancelled(runDir, jobID string, result model.JobResult) bool {
	return projectRunner().WasExplicitlyCancelled(runDir, jobID, result)
}

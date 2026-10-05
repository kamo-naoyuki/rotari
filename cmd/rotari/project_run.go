package main

import (
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// projectRunner connects the shared run lifecycle to this command's executor
// registry, environment names, config discovery, run registry, webhooks, and
// diagnosis.
func projectRunner() projectrun.Runner {
	return projectrun.Runner{
		Store:     jsonStore(),
		Executors: executorRegistry,
		NewJobID:  makeJobID,
		// jobLogf is replaced in tests, so it is looked up on every call.
		Logf:   func(format string, args ...any) { jobLogf(format, args...) },
		Errorf: printErrorf,
		Environment: runcontract.EnvironmentNames{
			BaseDir: envBaseDir, ProjectName: envProjectName, RunID: envRunID, JobID: envJobID,
			Executor: envExecutor, Bin: envBin, RunDir: envRunDir, JobDir: envJobDir, CWD: envCWD,
			JobName: envJobName, ArrayTaskID: envArrayTaskID, ArrayFirst: envArrayFirst, ArrayLast: envArrayLast,
			ArraySize: envArraySize, RunName: envRunName, LocalConcurrency: envRunLocalConc,
			BatchConcurrency: envRunBatchConc, Retry: envRunRetry, ExecutorOptions: envExecutorOpts,
		},
		AttemptIDName:       envAttemptID,
		PropagatedVariables: propagatedEnvironmentVariables,
		ConfigPaths: func(paths state.ProjectPaths, configPath string) []string {
			var pathsForRun []string
			if configPath != "" {
				pathsForRun = append(pathsForRun, configPath)
			}
			if loaded, err := notification.Load(paths.BaseDir, paths.ProjectName); err == nil && loaded.Path != "" {
				pathsForRun = append(pathsForRun, loaded.Path)
			}
			return pathsForRun
		},
		RegisterRun: registerRun,
		Diagnose:    diagnoseJobResult,
		JobFinished: webhookNotifications.JobFinished,
		RunFinished: webhookNotifications.RunFinished,
	}
}

// planRerunSelection decides which of the queue's jobs a run executes, as a
// run request planned before its run is recorded would; see
// projectrun.ReferenceRun and projectrun.Runner.PlanSelection.
func planRerunSelection(paths state.ProjectPaths, queue model.Queue, selection string, jobIDs []string, referenceRunID string, partialArray bool) (runcontract.Plan, error) {
	referenceRunID, err := projectrun.ReferenceRun(paths, selection, referenceRunID)
	if err != nil {
		return runcontract.Plan{}, err
	}
	return projectRunner().PlanSelection(paths, queue, selection, jobIDs, model.CommandSelector{}, jobfilter.Filter{}, referenceRunID, partialArray)
}

// finishRun finalizes a run whose jobs have ended; see
// projectrun.Runner.Finalize.
func finishRun(paths state.ProjectPaths, runID string, exitCode int) error {
	return projectRunner().Finalize(paths, runID, exitCode)
}

func loadSamplesPath(paths state.ProjectPaths, runID string) string {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return ""
	}
	return filepath.Join(runDir, state.LoadSamplesFileName)
}

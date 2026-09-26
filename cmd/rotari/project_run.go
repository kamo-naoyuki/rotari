package main

import (
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/model"
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
		ConfigPaths: func(paths state.ProjectPaths) []string {
			return config.PathsForRun(paths.BaseDir, paths.ProjectName)
		},
		RegisterRun: registerRun,
		Diagnose:    diagnoseJobResult,
		RunFinished: notifyRunWebhook,
	}
}

// planRerunSelection decides which of the queue's jobs a run executes; see
// projectrun.Runner.PlanSelection.
func planRerunSelection(paths state.ProjectPaths, queue model.Queue, selection string, jobIDs []string, referenceRunID string, partialArray bool) (runcontract.Plan, error) {
	return projectRunner().PlanSelection(paths, queue, selection, jobIDs, model.CommandSelector{}, referenceRunID, partialArray)
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

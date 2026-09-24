package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// executeMixedRun snapshots a queue, plans selected work, runs local and batch
// jobs through the shared run engine, and persists final run state.
func executeMixedRun(paths pathSet, runID, runName string, localConcurrency, batchMaxActive, retry int, requestedExecutor string, executorOptions []string, selection string, jobIDs []string, referenceRunID string, partialArray bool, progress func(JobResult, int, int, int, int), onStart func(JobSpec), settings ...executorRunSettingsMap) int {
	var executorSettings executorRunSettingsMap
	if len(settings) > 0 {
		executorSettings = settings[0]
	}
	if !state.IsValidPathElement(runID) {
		printErrorf("invalid run ID %q", runID)
		return 1
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		printErrorf("failed to load queue: %v", err)
		return 1
	}
	jobs := model.QueueToJobs(queue.Commands)
	if len(jobs) == 0 {
		printErrorf("queue '%s' has no valid commands", paths.ProjectName)
		return 1
	}
	for _, job := range jobs {
		if !state.IsValidPathElement(job.ID) {
			printErrorf("invalid job ID %q", job.ID)
			return 1
		}
	}
	if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
		printErrorf("invalid dependencies: %v", err)
		return 1
	}
	defaultExecutor := requestedExecutor
	if defaultExecutor == "" {
		defaultExecutor = queue.DefaultExecutor
	}
	if defaultExecutor == "" {
		defaultExecutor = "local"
	}
	for index := range jobs {
		if jobs[index].Executor == "" {
			jobs[index].Executor = defaultExecutor
		}
	}
	prepareJobEnvironments(paths, runID, jobs, runName, localConcurrency, batchMaxActive, retry, executorOptions)

	plan, err := planRerunSelection(paths, queue, selection, jobIDs, referenceRunID, partialArray)
	if err != nil {
		printErrorf("failed to prepare job selection: %v", err)
		return 1
	}
	runcontract.ExpandArrayPlan(queue.Commands, jobs, plan.Execute)
	runcontract.ApplyCarriedOrigins(queue.Commands, plan.CarriedOrigins)

	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, stateDirMode()); err != nil {
		return 1
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		return 1
	}

	finalResults := make(map[string]JobResult, len(jobs))
	for id, result := range plan.CarriedResults {
		finalResults[id] = result
	}
	executable := make([]JobSpec, 0, len(jobs))
	for _, job := range jobs {
		if plan.Execute[job.ID] {
			executable = append(executable, job)
		}
	}
	pending := append([]JobSpec(nil), executable...)
	jobsByName := make(map[string]JobSpec, len(jobs))
	for _, job := range jobs {
		if job.Name != "" {
			jobsByName[job.Name] = job
		}
	}
	pending = runcontract.ExecuteDependencyRetries(pending, jobsByName, finalResults, retry, runcontract.AttemptCallbacks{
		Execute: func(_ int, ready []model.JobSpec) []model.JobResult {
			return executeMixedAttempt(runDir, queue, ready, localConcurrency, batchMaxActive, requestedExecutor, executorOptions, executorSettings, onStart)
		},
		AssignAttemptIDs: func(ready []model.JobSpec, attempt int) {
			assignAttemptIDs(ready, runID, attempt)
		},
		ShouldRetry: func(job model.JobSpec, result model.JobResult) bool {
			return !jobWasExplicitlyCancelled(runDir, job.ID, result)
		},
		Progress: func(result model.JobResult, completed, total, succeeded, failed int) {
			if progress != nil {
				progress(result, completed, total, succeeded, failed)
			}
		},
	})
	runcontract.FinalizePendingResults(pending, finalResults)

	summary := runcontract.BuildRunSummary(runID, runName, nowRFC3339(), jobs, finalResults, func(result JobResult) JobResult {
		return diagnoseJobResult(runDir, result)
	})
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		return 1
	}
	return summary.ExitCode
}

func jobWasExplicitlyCancelled(runDir, jobID string, result JobResult) bool {
	jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
	if err != nil {
		return false
	}
	return runcontract.WasExplicitlyCancelled(result.Error,
		func() bool { return jobCancellationRequested(jobDir) },
		func() string {
			if status, ok := loadSlurmStatus(filepath.Join(jobDir, "status.json")); ok {
				return status.Phase
			}
			return ""
		},
	)
}

func prepareJobEnvironments(paths pathSet, runID string, jobs []JobSpec, runName string, localConcurrency, batchConcurrency, retry int, executorOptions []string) {
	runDir := filepath.Join(paths.RunsDir, runID)
	cwd := ""
	if data, err := os.ReadFile(filepath.Join(runDir, "context.json")); err == nil {
		var context RunContext
		if json.Unmarshal(data, &context) == nil {
			cwd = context.CWD
		}
	}
	bin, _ := os.Executable()
	inherited := make(map[string]string)
	for _, name := range propagatedEnvironmentVariables {
		if value, exists := os.LookupEnv(name); exists {
			inherited[name] = value
		}
	}
	runcontract.PrepareJobEnvironments(jobs, runcontract.EnvironmentConfig{
		Names: runcontract.EnvironmentNames{
			BaseDir: envBaseDir, ProjectName: envProjectName, RunID: envRunID, JobID: envJobID,
			Executor: envExecutor, Bin: envBin, RunDir: envRunDir, JobDir: envJobDir, CWD: envCWD,
			JobName: envJobName, ArrayTaskID: envArrayTaskID, ArrayFirst: envArrayFirst, ArrayLast: envArrayLast,
			ArraySize: envArraySize, RunName: envRunName, LocalConcurrency: envRunLocalConc,
			BatchConcurrency: envRunBatchConc, Retry: envRunRetry, ExecutorOptions: envExecutorOpts,
		}, BaseDir: paths.BaseDir, ProjectName: paths.ProjectName, RunID: runID, RunDir: runDir,
		RunName: runName, Bin: bin, CWD: cwd, LocalConcurrency: localConcurrency,
		BatchConcurrency: batchConcurrency, Retry: retry, ExecutorOptions: executorOptions,
		Inherited: inherited, JobDir: func(runDir string, job JobSpec) (string, error) {
			return state.SafeJoin(runDir, job.ID)
		},
	})
}

func assignAttemptIDs(jobs []JobSpec, runID string, attempt int) {
	runcontract.AssignAttemptIDs(jobs, runID, attempt, runcontract.AttemptIDCallbacks{
		MakeAttemptID: state.MakeAttemptID, AttemptJobDir: func(runDir string, job JobSpec) (string, error) {
			return state.AttemptJobDir(runDir, model.JobSpec(job))
		},
		AttemptIDName: envAttemptID, RunDirName: envRunDir, JobDirName: envJobDir,
	})
}

func environmentEntry(environment []string, name string) (string, bool) {
	return runcontract.EnvironmentEntry(environment, name)
}

func executeMixedAttempt(runDir string, queue Queue, jobs []JobSpec, localConcurrency, batchMaxActive int, requestedExecutor string, executorOptions []string, executorSettings executorRunSettingsMap, onStart func(JobSpec)) []JobResult {
	return runcontract.RunAttempt(runDir, queue, jobs, runcontract.AttemptOptions{
		LocalConcurrency: localConcurrency, BatchMaxActive: batchMaxActive,
		RequestedExecutor: requestedExecutor, ExecutorOptions: executorOptions,
		Settings: executorSettings, ResolveExecutor: lookupExecutor,
		Callbacks: runcontract.BatchLaneCallbacks{
			ValidatedJobDir: state.SafeJoin,
			JobCancelled:    jobCancellationRequested,
			RecordCancelled: recordCancelledJob,
			Logf:            jobLogf,
		},
	}, onStart)
}

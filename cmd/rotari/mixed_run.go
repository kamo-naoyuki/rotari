package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
)

func executeMixedRun(paths pathSet, runID, runName string, localConcurrency, batchMaxActive, retry int, requestedExecutor string, executorOptions []string, selection string, jobIDs []string, referenceRunID string, partialArray bool, progress func(JobResult, int, int, int, int), onStart func(JobSpec), settings ...executorRunSettingsMap) int {
	var executorSettings executorRunSettingsMap
	if len(settings) > 0 {
		executorSettings = settings[0]
	}
	if !isValidPathElement(runID) {
		printErrorf("invalid run ID %q", runID)
		return 1
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		printErrorf("failed to load queue: %v", err)
		return 1
	}
	jobs := queueToJobs(queue.Commands)
	if len(jobs) == 0 {
		printErrorf("queue '%s' has no valid commands", paths.ProjectName)
		return 1
	}
	for _, job := range jobs {
		if !isValidPathElement(job.ID) {
			printErrorf("invalid job ID %q", job.ID)
			return 1
		}
	}
	if err := model.ValidateDependencies(jobs); err != nil {
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
	expandArrayPlan(queue.Commands, jobs, plan.Execute)
	for index := range queue.Commands {
		command := &queue.Commands[index]
		if origin, ok := plan.CarriedOrigins[command.ID]; ok {
			command.Origin = origin
		}
		if command.Array == nil {
			continue
		}
		for _, task := range arrayTaskIDs(command.Array) {
			taskID := fmt.Sprintf("%s-%d", command.ID, task)
			if origin, ok := plan.CarriedOrigins[taskID]; ok {
				if command.TaskOrigins == nil {
					command.TaskOrigins = make(map[string]*JobOrigin)
				}
				command.TaskOrigins[taskID] = origin
			}
		}
	}

	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, stateDirMode()); err != nil {
		return 1
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
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
	for attempt := 0; (retry == -1 || attempt <= retry) && len(pending) > 0; attempt++ {
		pendingByID := make(map[string]bool, len(pending))
		for _, job := range pending {
			pendingByID[job.ID] = true
		}
		// Resolve dependency waves within this attempt: a job whose
		// dependency finishes in an earlier wave of the same attempt must
		// run in this attempt too, instead of waiting for the next retry.
		unresolved := pending
		var attemptResults []JobResult
		for {
			blocked := make([]JobSpec, 0)
			ready := make([]JobSpec, 0, len(unresolved))
			stillUnresolved := make([]JobSpec, 0, len(unresolved))
			for _, job := range unresolved {
				blockedBy := ""
				readyForRun := true
				for _, dependency := range job.DependsOn {
					dependencyJob := jobsByName[dependency]
					result, done := finalResults[dependencyJob.ID]
					if !done || (result.ExitCode != 0 && pendingByID[dependencyJob.ID]) {
						readyForRun = false
						continue
					}
					if result.ExitCode != 0 {
						blockedBy = dependency
						break
					}
				}
				if blockedBy != "" {
					blocked = append(blocked, job)
				} else if readyForRun {
					ready = append(ready, job)
				} else {
					stillUnresolved = append(stillUnresolved, job)
				}
			}
			for _, job := range blocked {
				finalResults[job.ID] = JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "blocked by failed dependency"}
			}
			if len(ready) == 0 {
				break
			}
			assignAttemptIDs(ready, runID, attempt)
			waveResults := executeMixedAttempt(runDir, queue, ready, localConcurrency, batchMaxActive, requestedExecutor, executorOptions, executorSettings, onStart)
			for _, result := range waveResults {
				finalResults[result.ID] = result
			}
			attemptResults = append(attemptResults, waveResults...)
			unresolved = stillUnresolved
		}
		nextPending := make([]JobSpec, 0, len(jobs))
		for _, job := range pending {
			result, ok := finalResults[job.ID]
			if !ok || (result.ExitCode != 0 && attempt < retry && !jobWasExplicitlyCancelled(runDir, job.ID, result)) {
				nextPending = append(nextPending, job)
			}
		}
		pending = nextPending
		if len(pending) > 0 && (retry == -1 || attempt < retry) && progress != nil {
			completed, succeeded, failed := summarizeResults(finalResults)
			for _, job := range pending {
				progress(JobResult{ID: job.ID, Command: job.Command, Error: fmt.Sprintf("retry:%d", attempt+1)}, completed, len(jobs), succeeded, failed)
			}
		}
		if progress != nil {
			completed, succeeded, failed := summarizeResults(finalResults)
			for _, result := range attemptResults {
				progressResult := result
				if result.ExitCode != 0 && !jobIsPending(pending, result.ID) {
					progressResult.Error = "final-failure"
				}
				progress(progressResult, completed, len(jobs), succeeded, failed)
			}
		}
	}
	for _, job := range pending {
		if _, ok := finalResults[job.ID]; !ok {
			finalResults[job.ID] = JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: "blocked by failed dependency"}
		}
	}

	summary := runcontract.BuildRunSummary(runID, runName, nowRFC3339(), jobs, finalResults, func(result JobResult) JobResult {
		return diagnoseJobResult(runDir, result)
	})
	if err := writeJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		return 1
	}
	return summary.ExitCode
}

func jobWasExplicitlyCancelled(runDir, jobID string, result JobResult) bool {
	jobDir, err := latestAttemptJobDir(runDir, jobID)
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

func expandArrayPlan(commands []QueuedCommand, jobs []JobSpec, execute map[string]bool) {
	runcontract.ExpandArrayPlan(commands, jobs, execute)
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
			return validatedJobDir(runDir, job.ID)
		},
	})
}

func assignAttemptIDs(jobs []JobSpec, runID string, attempt int) {
	runcontract.AssignAttemptIDs(jobs, runID, attempt, runcontract.AttemptIDCallbacks{
		MakeAttemptID: makeAttemptID, AttemptJobDir: attemptJobDir,
		AttemptIDName: envAttemptID, RunDirName: envRunDir, JobDirName: envJobDir,
	})
}

func environmentEntry(environment []string, name string) (string, bool) {
	return runcontract.EnvironmentEntry(environment, name)
}

func jobIsPending(jobs []JobSpec, jobID string) bool {
	return runcontract.JobIsPending(jobs, jobID)
}

func removeFinishedJobs(jobs []JobSpec, results map[string]JobResult) []JobSpec {
	return runcontract.RemoveFinishedJobs(jobs, results)
}

func executeMixedAttempt(runDir string, queue Queue, jobs []JobSpec, localConcurrency, batchMaxActive int, requestedExecutor string, executorOptions []string, executorSettings executorRunSettingsMap, onStart func(JobSpec)) []JobResult {
	return runcontract.RunAttempt(runDir, queue, jobs, runcontract.AttemptOptions{
		LocalConcurrency: localConcurrency, BatchMaxActive: batchMaxActive,
		RequestedExecutor: requestedExecutor, ExecutorOptions: executorOptions,
		Settings: executorSettings, ResolveExecutor: lookupExecutor,
		Callbacks: runcontract.BatchLaneCallbacks{
			ValidatedJobDir: validatedJobDir,
			JobCancelled:    jobCancellationRequested,
			RecordCancelled: recordCancelledJob,
			Logf:            jobLogf,
		},
	}, onStart)
}

func completeArrayGroup(jobs []JobSpec, first, last int) bool {
	return runcontract.CompleteArrayGroup(jobs, first, last)
}

func summarizeResults(results map[string]JobResult) (int, int, int) {
	return runcontract.SummarizeResults(results)
}

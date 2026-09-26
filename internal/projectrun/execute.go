package projectrun

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Options selects and configures the work of one run.
type Options struct {
	RunID            string
	RunName          string
	LocalConcurrency int
	BatchMaxActive   int
	Retry            int
	// Executor overrides the queue's default executor for jobs without one.
	Executor        string
	ExecutorOptions []string
	Settings        executor.RunSettingsMap
	// Selection, JobIDs, Scope, SourceRunID, and PartialArray choose which
	// jobs execute and which carry a result forward; see run.PlanRerun.
	Selection    string
	JobIDs       []string
	Scope        model.CommandSelector
	SourceRunID  string
	PartialArray bool
}

// Observer receives run progress. Both fields are optional.
type Observer struct {
	// Progress is called for every job result, including retries.
	Progress func(result model.JobResult, completed, total, succeeded, failed int)
	// Started is called when a job attempt starts.
	Started func(job model.JobSpec)
}

// Execute snapshots the queue into the run directory, plans the selected
// work, runs local and batch jobs through the shared run engine, and writes
// the run summary. It returns the run's exit code. An error means the run
// could not be prepared or recorded; its exit code is then 1.
func (runner Runner) Execute(paths state.ProjectPaths, options Options, observer Observer) (int, error) {
	runID := options.RunID
	if !state.IsValidPathElement(runID) {
		return 1, fmt.Errorf("invalid run ID %q", runID)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return 1, fmt.Errorf("failed to load queue: %w", err)
	}
	jobs := model.QueueToJobs(queue.Commands)
	if len(jobs) == 0 {
		return 1, fmt.Errorf("queue '%s' has no valid commands", paths.ProjectName)
	}
	for _, job := range jobs {
		if !state.IsValidPathElement(job.ID) {
			return 1, fmt.Errorf("invalid job ID %q", job.ID)
		}
	}
	if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
		return 1, fmt.Errorf("invalid dependencies: %w", err)
	}
	defaultExecutor := options.Executor
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
	runner.PrepareJobEnvironments(paths, options, jobs)

	plan, err := runner.PlanSelection(paths, queue, options.Selection, options.JobIDs, options.Scope, options.SourceRunID, options.PartialArray)
	if err != nil {
		return 1, fmt.Errorf("failed to prepare job selection: %w", err)
	}
	run.ExpandArrayPlan(queue.Commands, jobs, plan.Execute)
	run.ApplyCarriedOrigins(queue.Commands, plan.CarriedOrigins)

	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, state.DirectoryMode()); err != nil {
		return 1, fmt.Errorf("failed to create run directory: %w", err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		return 1, fmt.Errorf("failed to save run commands: %w", err)
	}

	finalResults := make(map[string]model.JobResult, len(jobs))
	for id, result := range plan.CarriedResults {
		finalResults[id] = result
	}
	pending := make([]model.JobSpec, 0, len(jobs))
	jobsByName := make(map[string]model.JobSpec, len(jobs))
	for _, job := range jobs {
		if plan.Execute[job.ID] {
			pending = append(pending, job)
		}
		if job.Name != "" {
			jobsByName[job.Name] = job
		}
	}
	dispatcher := run.NewDispatcher(runDir, queue, run.DispatchOptions{
		LocalConcurrency: options.LocalConcurrency, BatchMaxActive: options.BatchMaxActive,
		RequestedExecutor: options.Executor, ExecutorOptions: options.ExecutorOptions,
		Settings: options.Settings, ResolveExecutor: runner.Executors.Lookup,
		Callbacks: run.BatchLaneCallbacks{
			ValidatedJobDir: state.SafeJoin,
			JobCancelled:    cancellationRequested,
			RecordCancelled: func(jobDir string, job model.JobSpec) model.JobResult {
				return executor.RecordCancelledJob(jobDir, job, runner.Store)
			},
			Logf: runner.logf,
		},
	}, observer.Started)
	pending = run.ExecuteJobs(pending, jobsByName, finalResults, run.EngineOptions{
		RunRetry: options.Retry,
		Start:    dispatcher.Start,
		AssignAttemptID: func(job *model.JobSpec, attempt int) {
			attempts := []model.JobSpec{*job}
			runner.AssignAttemptIDs(attempts, runID, attempt)
			*job = attempts[0]
		},
		ShouldRetry: func(job model.JobSpec, result model.JobResult) bool {
			return !runner.WasExplicitlyCancelled(runDir, job.ID, result)
		},
		// run cancel marks the project cancelling; stop retrying and starting
		// jobs then, since jobs not yet started have no attempt to cancel.
		Stopped: func() bool {
			meta, err := state.LoadMeta(paths.MetaFile)
			return err == nil && meta.Phase == "cancelling"
		},
		Progress: func(result model.JobResult, completed, total, succeeded, failed int) {
			if observer.Progress != nil {
				observer.Progress(result, completed, total, succeeded, failed)
			}
		},
	})
	run.FinalizePendingResults(pending, finalResults)

	summary := run.BuildRunSummary(runID, options.RunName, runner.timestamp(), jobs, finalResults, func(result model.JobResult) model.JobResult {
		if runner.Diagnose == nil {
			return result
		}
		return runner.Diagnose(runDir, result)
	})
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		return 1, fmt.Errorf("failed to save run summary: %w", err)
	}
	return summary.ExitCode, nil
}

// WasExplicitlyCancelled reports whether a job's latest attempt was cancelled
// on purpose, which ends its retries for this run.
func (runner Runner) WasExplicitlyCancelled(runDir, jobID string, result model.JobResult) bool {
	jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
	if err != nil {
		return false
	}
	return run.WasExplicitlyCancelled(result.Error,
		func() bool { return cancellationRequested(jobDir) },
		func() string {
			if status, ok := executor.LoadWrapperStatus(runner.Store, filepath.Join(jobDir, "status.json")); ok {
				return status.Phase
			}
			return ""
		},
	)
}

func cancellationRequested(jobDir string) bool {
	_, err := os.Stat(filepath.Join(jobDir, "cancelled"))
	return err == nil
}

// PrepareJobEnvironments sets the rotari variables every job of the run sees.
func (runner Runner) PrepareJobEnvironments(paths state.ProjectPaths, options Options, jobs []model.JobSpec) {
	runDir := filepath.Join(paths.RunsDir, options.RunID)
	cwd := ""
	if context, err := state.LoadContext(runner.Store, runDir); err == nil {
		cwd = context.CWD
	}
	bin, _ := os.Executable()
	inherited := make(map[string]string)
	for _, name := range runner.PropagatedVariables {
		if value, exists := os.LookupEnv(name); exists {
			inherited[name] = value
		}
	}
	run.PrepareJobEnvironments(jobs, run.EnvironmentConfig{
		Names: runner.Environment, BaseDir: paths.BaseDir, ProjectName: paths.ProjectName,
		RunID: options.RunID, RunDir: runDir, RunName: options.RunName, Bin: bin, CWD: cwd,
		LocalConcurrency: options.LocalConcurrency, BatchConcurrency: options.BatchMaxActive,
		Retry: options.Retry, ExecutorOptions: options.ExecutorOptions, Inherited: inherited,
		JobDir: func(runDir string, job model.JobSpec) (string, error) {
			return state.SafeJoin(runDir, job.ID)
		},
	})
}

// AssignAttemptIDs gives each job a new attempt ID for the given attempt
// number and points its environment at the attempt directory.
func (runner Runner) AssignAttemptIDs(jobs []model.JobSpec, runID string, attempt int) {
	run.AssignAttemptIDs(jobs, runID, attempt, run.AttemptIDCallbacks{
		MakeAttemptID: state.MakeAttemptID, AttemptJobDir: state.AttemptJobDir,
		AttemptIDName: runner.AttemptIDName, RunDirName: runner.Environment.RunDir, JobDirName: runner.Environment.JobDir,
	})
}

package projectrun

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
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
	EnvMode         string
	Settings        executor.RunSettingsMap
	// Selection, JobIDs, Scope, Filter, SourceRunID, and PartialArray choose
	// which jobs execute and which carry a result forward; see run.PlanRerun.
	Selection    string
	JobIDs       []string
	Scope        model.CommandSelector
	Filter       jobfilter.Filter
	SourceRunID  string
	PartialArray bool
	MatchBy      string
}

// Observer receives run progress. All fields are optional.
type Observer struct {
	// Progress is called for every job result, including retries.
	Progress func(result model.JobResult, completed, total, succeeded, failed int)
	// Started is called when a job attempt starts.
	Started func(job model.JobSpec)
	// Finished is called once when a job reaches its final result.
	Finished func(job model.JobSpec, result model.JobResult)
}

// Execute snapshots the queue into the run directory, plans the selected
// work, runs local and batch jobs through the shared run engine, and writes
// the run summary. It returns the run's exit code. An error means the run
// could not be prepared or recorded; its exit code is then 1.
func (runner Runner) Execute(paths state.ProjectPaths, options Options, observer Observer) (int, error) {
	if options.EnvMode == "" {
		options.EnvMode = model.EnvModeAll
	}
	if options.EnvMode != model.EnvModeAll && options.EnvMode != model.EnvModeNone {
		return 1, fmt.Errorf("invalid environment mode %q (choose ALL or NONE)", options.EnvMode)
	}
	runID := options.RunID
	if !state.IsValidPathElement(runID) {
		return 1, fmt.Errorf("invalid run ID %q", runID)
	}
	startedAt := runner.timestamp()
	if lock, err := state.LoadLock(paths.LockFile); err == nil && lock.RunID == runID && lock.StartedAt != "" {
		startedAt = lock.StartedAt
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return 1, fmt.Errorf("failed to load queue: %w", err)
	}
	jobs := model.QueueToJobs(queue.Commands)
	if len(jobs) == 0 {
		return 1, fmt.Errorf("queue '%s' has no valid commands", paths.ProjectName)
	}
	options.Filter, err = ClassifyDefinitions(paths, queue, options.Filter, options.SourceRunID, options.MatchBy)
	if err != nil {
		return 1, err
	}
	if options.MatchBy != "" {
		queue, err = runner.MatchFingerprintQueue(paths, queue, options.MatchBy, options.SourceRunID)
		if err != nil {
			return 1, err
		}
		jobs = model.QueueToJobs(queue.Commands)
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
		jobs[index].EnvMode = options.EnvMode
	}
	runner.ResolveJobWorkingDirectories(paths, options, jobs)
	runner.PrepareJobEnvironments(paths, options, jobs)

	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, state.DirectoryMode()); err != nil {
		return 1, fmt.Errorf("failed to create run directory: %w", err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		return 1, fmt.Errorf("failed to save run commands: %w", err)
	}

	plan, err := runner.PlanSelection(paths, queue, options.Selection, options.JobIDs, options.Scope, options.Filter, options.SourceRunID, options.PartialArray)
	if err != nil {
		return 1, fmt.Errorf("failed to prepare job selection: %w", err)
	}
	run.ApplyCarriedOrigins(queue.Commands, plan.CarriedOrigins)
	// The first snapshot keeps a failed plan inspectable; this one records origins.
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		return 1, fmt.Errorf("failed to save run commands: %w", err)
	}
	// Readers tell carried jobs from pending ones by these results until the
	// run writes its summary; see jobstatus.RecordedResults.
	carried := model.RunSummary{RunID: runID, Results: make([]model.JobResult, 0, len(plan.CarriedResults))}
	for _, result := range plan.CarriedResults {
		carried.Results = append(carried.Results, result)
	}
	sort.Slice(carried.Results, func(i, j int) bool { return carried.Results[i].ID < carried.Results[j].ID })
	if err := state.WriteJSON(filepath.Join(runDir, state.CarriedResultsFileName), carried); err != nil {
		return 1, fmt.Errorf("failed to save carried results: %w", err)
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
		FinalResult: func(job model.JobSpec, result model.JobResult) model.JobResult {
			if result.ExitCode != 0 && runner.WasExplicitlyCancelled(runDir, job.ID, result) {
				result.Error = model.CancelledError(result.Error)
			}
			if runner.Diagnose != nil {
				result = runner.Diagnose(runDir, result)
			}
			if jobDir, err := state.LatestAttemptJobDir(runDir, job.ID); err == nil {
				if err := os.MkdirAll(jobDir, state.DirectoryMode()); err == nil {
					if err := state.WriteJSON(filepath.Join(jobDir, state.FinalResultFileName), result); err != nil {
						runner.logf("WARNING: failed to record final result for job %s: %v", job.ID, err)
					}
				}
			}
			if runner.JobFinished != nil {
				runner.JobFinished(paths, runID, options.RunName, job, result)
			}
			if observer.Finished != nil {
				observer.Finished(job, result)
			}
			return result
		},
	})
	run.FinalizePendingResults(pending, finalResults)

	summary := run.BuildRunSummary(runID, options.RunName, startedAt, jobs, finalResults, nil)
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		return 1, fmt.Errorf("failed to save run summary: %w", err)
	}
	return summary.ExitCode, nil
}

// ClassifyDefinitions sets filter's changed and new job IDs by comparing
// queue with the command snapshot of referenceRunID, matched by mode (the
// --match-by modes; empty means id-and-fingerprint). Without a reference run
// every job is new. It returns filter unchanged when neither --filter-changed
// nor --filter-new is set. Callers resolve referenceRunID before Begin makes
// the new run the project's last run.
func ClassifyDefinitions(paths state.ProjectPaths, queue model.Queue, filter jobfilter.Filter, referenceRunID, mode string) (jobfilter.Filter, error) {
	if !filter.Changed && !filter.New {
		return filter, nil
	}
	currentJobs, err := model.QueueFingerprintJobs(queue)
	if err != nil {
		return filter, err
	}
	var sourceJobs []model.FingerprintJob
	if referenceRunID != "" {
		runDir, err := state.SafeJoin(paths.RunsDir, referenceRunID)
		if err != nil {
			return filter, err
		}
		sourceQueue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
		if err != nil {
			return filter, fmt.Errorf("failed to load reference run %q: %w", referenceRunID, err)
		}
		if sourceJobs, err = model.QueueFingerprintJobs(sourceQueue); err != nil {
			return filter, err
		}
	}
	if mode == "" {
		mode = model.MatchByIDAndFingerprint
	}
	filter.ChangedIDs, filter.NewIDs = model.ClassifyFingerprintJobs(currentJobs, sourceJobs, mode)
	return filter, nil
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
	callerEnvironment := make(map[string]string)
	if options.EnvMode == model.EnvModeAll {
		for _, entry := range os.Environ() {
			name, value, ok := strings.Cut(entry, "=")
			if ok && model.ValidEnvironmentName(name) {
				callerEnvironment[name] = value
			}
		}
	}
	run.PrepareJobEnvironments(jobs, run.EnvironmentConfig{
		Names: runner.Environment, BaseDir: paths.BaseDir, ProjectName: paths.ProjectName,
		RunID: options.RunID, RunDir: runDir, RunName: options.RunName, Bin: bin, CWD: cwd,
		LocalConcurrency: options.LocalConcurrency, BatchConcurrency: options.BatchMaxActive,
		Retry: options.Retry, ExecutorOptions: options.ExecutorOptions, Inherited: inherited,
		CallerEnvironment: callerEnvironment,
		JobDir: func(runDir string, job model.JobSpec) (string, error) {
			return state.SafeJoin(runDir, job.ID)
		},
	})
}

func (runner Runner) ResolveJobWorkingDirectories(paths state.ProjectPaths, options Options, jobs []model.JobSpec) {
	runDir := filepath.Join(paths.RunsDir, options.RunID)
	context, err := state.LoadContext(runner.Store, runDir)
	if err != nil {
		return
	}
	for index := range jobs {
		jobDirectory := jobs[index].WorkingDirectory
		if jobDirectory == "" {
			jobs[index].WorkingDirectory = context.CWD
		} else if !filepath.IsAbs(jobDirectory) {
			jobs[index].WorkingDirectory = filepath.Join(context.CWD, jobDirectory)
		} else {
			jobs[index].WorkingDirectory = filepath.Clean(jobDirectory)
		}
	}
}

// AssignAttemptIDs gives each job a new attempt ID for the given attempt
// number and points its environment at the attempt directory.
func (runner Runner) AssignAttemptIDs(jobs []model.JobSpec, runID string, attempt int) {
	run.AssignAttemptIDs(jobs, runID, attempt, run.AttemptIDCallbacks{
		MakeAttemptID: state.MakeAttemptID, AttemptJobDir: state.AttemptJobDir,
		AttemptIDName: runner.AttemptIDName, RunDirName: runner.Environment.RunDir, JobDirName: runner.Environment.JobDir,
	})
}

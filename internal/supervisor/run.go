package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// StartRun starts an async run inside the supervisor and returns at once.
// onDone, if not nil, is called once the run has finished.
func (ops Operations) StartRun(request server.Request, onDone func()) (string, string, error) {
	started, err := ops.beginRun(request)
	if err != nil {
		return "", "", err
	}
	runDir, err := state.SafeJoin(started.paths.RunsDir, started.runID)
	if err != nil {
		return "", "", err
	}
	go func() {
		if onDone != nil {
			defer onDone()
		}
		if _, err := ops.Runner.Run(started.paths, started.options, projectrun.Observer{}); err != nil {
			ops.logf("async run %s failed: %v", started.runID, err)
		}
	}()
	target := fmt.Sprintf("--basedir %s --project-name %s", executor.ShellQuote(started.paths.BaseDir), executor.ShellQuote(request.QueueName))
	message := fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run: %s\n  Directory: %s\n\nWait for it:\n  rotari wait %s --run-id %s\n\nCheck status:\n  rotari show --run-id %s\n\nCancel run:\n  rotari cancel %s %s\n",
		request.QueueName, model.RunLabel(started.runID, request.RunName), runDir, target, started.runID, started.runID, target, started.runID)
	return started.runID, message + sourceNotice(started), nil
}

// Run executes a run inside the supervisor, reporting progress to the
// attached client, and returns once it has finished.
func (ops Operations) Run(request server.Request, progress func(server.Response)) (string, int, error) {
	started, err := ops.beginRun(request)
	if err != nil {
		return "", 1, err
	}
	paths, runID := started.paths, started.runID
	if progress != nil {
		progress(server.Response{Progress: true, Message: fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run ID: %s\n  Submitted: %d\n  Excluded: %d\n  Total: %d", request.QueueName, runID, started.submitted, started.total-started.submitted, started.total)})
	}
	exitCode, err := ops.Runner.Run(paths, started.options, runObserver(request, runID, progress))
	if err != nil {
		return "", 1, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", 1, err
	}
	if summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		return CompletionMessage(paths, runID, summary) + sourceNotice(started), exitCode, nil
	}
	return fmt.Sprintf("=== Run finished ===\n  Project: %s\n  Run: %s\n  Exit code: %d", request.QueueName, runID, exitCode) + sourceNotice(started), exitCode, nil
}

// startedRun is a run that Begin has recorded and that is ready to execute.
type startedRun struct {
	paths   state.ProjectPaths
	runID   string
	options projectrun.Options
	// submitted and total count the jobs the run executes and the queue's
	// jobs.
	submitted, total    int
	sourceRunID         string
	usedQueueWithSource bool
	omittedSourceJobs   []string
}

// beginRun validates a run request and records the new run with Begin, under
// the project's state lock.
func (ops Operations) beginRun(request server.Request) (startedRun, error) {
	prepared, err := ops.prepareRun(request)
	if err != nil {
		return startedRun{}, err
	}
	defer prepared.release()
	request.SourceRunID = prepared.sourceRunID
	runID := ops.NewRunID()
	start := projectrun.Start{RunID: runID, RunName: request.RunName, CWD: request.CWD, ConfigPath: request.ConfigPath}
	if prepared.snapshotFromSource {
		start.Snapshot = &prepared.queue
	}
	if err := ops.Runner.Begin(prepared.paths, start); err != nil {
		return startedRun{}, err
	}
	options := runRequestOptions(request, runID)
	options.Executor = prepared.executor
	return startedRun{
		paths: prepared.paths, runID: runID, options: options,
		submitted: len(prepared.plan.Execute), total: len(model.QueueToJobs(prepared.queue.Commands)),
		sourceRunID: prepared.sourceRunID, usedQueueWithSource: prepared.usedQueueWithSource,
		omittedSourceJobs: prepared.omittedSourceJobs,
	}, nil
}

// preparedRun is a run request that passed validation while its caller holds
// the project's state lock.
type preparedRun struct {
	paths               state.ProjectPaths
	queue               model.Queue
	plan                run.Plan
	executor            string
	snapshotFromSource  bool
	usedQueueWithSource bool
	omittedSourceJobs   []string
	// sourceRunID is the request's reference run, resolved before the new
	// run is recorded; see projectrun.ReferenceRun.
	sourceRunID string
	release     func()
}

// prepareRun validates a run request and takes the project's state lock. On
// success the caller must call release.
func (ops Operations) prepareRun(request server.Request) (preparedRun, error) {
	if request.EnvMode != "" && request.EnvMode != model.EnvModeAll && request.EnvMode != model.EnvModeNone {
		return preparedRun{}, fmt.Errorf("invalid environment mode %q (choose ALL or NONE)", request.EnvMode)
	}
	if ops.Project != "" && request.QueueName != ops.Project {
		return preparedRun{}, fmt.Errorf("this supervisor runs project %q, not %q", ops.Project, request.QueueName)
	}
	if request.LocalConcurrency < 1 {
		return preparedRun{}, errors.New("local concurrency must be >= 1")
	}
	paths, err := state.ResolveProjectPaths(ops.BaseDir, request.QueueName)
	if err != nil {
		return preparedRun{}, err
	}
	if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
		return preparedRun{}, err
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return preparedRun{}, err
	}
	if err := project.EnsureIdle(paths, "run"); err != nil {
		release()
		return preparedRun{}, err
	}
	if _, err := project.CheckRevision(paths, project.Guard{IfRevision: request.IfRevision}); err != nil {
		release()
		return preparedRun{}, err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		release()
		return preparedRun{}, err
	}
	// Resolve the reference and plan under the state lock, before Begin changes
	// the project's last run. A planning error must not create an incomplete run.
	planned, err := ops.Runner.PlanRun(paths, queue, planRequest(request))
	if err != nil {
		release()
		return preparedRun{}, err
	}
	queue, plan, sourceRunID := planned.Queue, planned.Plan, planned.SourceRunID
	resolvedExecutor, err := resolveQueueExecutor(ops.Runner.Executors, queue, request.Executor)
	if err != nil {
		release()
		return preparedRun{}, err
	}
	return preparedRun{
		paths: paths, queue: queue, plan: plan, executor: resolvedExecutor,
		snapshotFromSource: planned.SnapshotFromSource, usedQueueWithSource: planned.UsedQueueWithSource,
		omittedSourceJobs: planned.OmittedSourceJobs, sourceRunID: sourceRunID, release: release,
	}, nil
}

func sourceNotice(started startedRun) string {
	if !started.usedQueueWithSource {
		return ""
	}
	message := fmt.Sprintf("\nRetry source: current queue; latest run %s has %d failed or unfinished job(s) not included", started.sourceRunID, len(started.omittedSourceJobs))
	if len(started.omittedSourceJobs) > 0 {
		message += ": " + strings.Join(started.omittedSourceJobs, ", ")
		message += fmt.Sprintf("\nInclude them with: rotari copy --basedir %s --project-name %s --run-id %s --failed --unfinished --append, then retry",
			executor.ShellQuote(started.paths.BaseDir), executor.ShellQuote(started.paths.ProjectName), executor.ShellQuote(started.sourceRunID))
	}
	return message + "\n"
}

// resolveQueueExecutor returns the run's default executor, requested or the
// planned snapshot's, and checks that it and every job's executor are known.
func resolveQueueExecutor(executors executor.Registry, queue model.Queue, requested string) (string, error) {
	if requested == "" {
		requested = queue.DefaultExecutor
		if requested == "" {
			requested = "local"
		}
	}
	if !executors.Known(requested) {
		return "", fmt.Errorf("unsupported executor: %s", requested)
	}
	for _, queued := range queue.Commands {
		executor := queued.Executor
		if executor == "" {
			executor = requested
		}
		if !executors.Known(executor) {
			return "", fmt.Errorf("unsupported executor: %s", executor)
		}
	}
	return requested, nil
}

// planRequest returns what planning request's run needs.
func planRequest(request server.Request) projectrun.PlanRequest {
	return projectrun.PlanRequest{
		Executor: request.Executor, ExecutorOptions: request.ExecutorOptions, Settings: request.ExecutorSettings,
		Selection: request.Selection, JobIDs: request.JobIDs, Scope: requestScope(request), Filter: request.Filter,
		SourceRunID: request.SourceRunID, SourcePolicy: projectrun.SourcePolicy(request.SourcePolicy),
		CopyAttempts: request.CopyAttempts, CopyJobIDs: request.CopyJobIDs,
		PartialArray: request.PartialArray, MatchBy: request.MatchBy,
	}
}

// requestScope returns the stage or matrix that narrows a run request's
// selection.
func requestScope(request server.Request) model.CommandSelector {
	return model.CommandSelector{Stage: request.ScopeStage, Matrix: request.ScopeMatrix}
}

// runRequestOptions converts a run request into the options of run runID.
func runRequestOptions(request server.Request, runID string) projectrun.Options {
	envMode := request.EnvMode
	if envMode == "" {
		envMode = model.EnvModeAll
	}
	return projectrun.Options{
		RunID: runID, RunName: request.RunName,
		LocalConcurrency: request.LocalConcurrency, BatchMaxActive: request.BatchMaxActive, Retry: request.Retry,
		Executor: request.Executor, ExecutorOptions: request.ExecutorOptions, Settings: request.ExecutorSettings,
		EnvMode:   envMode,
		Selection: request.Selection, JobIDs: request.JobIDs, Scope: requestScope(request), Filter: request.Filter,
		SourceRunID: request.SourceRunID, PartialArray: request.PartialArray,
		MatchBy: request.MatchBy,
	}
}

// runObserver turns job starts and results into progress responses for an
// attached client.
func runObserver(request server.Request, runID string, progress func(server.Response)) projectrun.Observer {
	if progress == nil {
		return projectrun.Observer{}
	}
	return projectrun.Observer{
		Progress: func(result model.JobResult, completed, total, succeeded, failed int) {
			message := ""
			if result.ExitCode != 0 && result.Error == "final-failure" {
				failureTitle := "Job failed:"
				if request.Retry > 0 {
					failureTitle = "Job failed after retry:"
				}
				attemptID := result.AttemptID
				if attemptID == "" {
					attemptID = result.ID
				}
				message = fmt.Sprintf("%s\n  ID: %s\n  Attempt ID: %s\n  Command: %s\n  Show output:\n    rotari show --run-id %s --job-id %s",
					failureTitle,
					result.ID, attemptID, strings.Join(result.Command, " "), runID, attemptID)
			} else if strings.HasPrefix(result.Error, "retry:") {
				message = fmt.Sprintf("Retrying job: attempt=%s job=%s command=%v", strings.TrimPrefix(result.Error, "retry:"), result.ID, result.Command)
			}
			progress(server.Response{OK: true, Progress: true, Message: message, JobID: result.ID, Completed: completed, Total: total, Succeeded: succeeded, Failed: failed})
		},
		Started: func(job model.JobSpec) {
			name := job.Name
			if name == "" {
				name = "-"
			}
			message := fmt.Sprintf("Job running:\n  ID: %s\n  Attempt ID: %s\n  Name: %s\n  Command: %s\n  Show:\n    rotari show -j %s",
				job.ID, job.AttemptID, name, strings.Join(job.Command, " "), job.AttemptID)
			progress(server.Response{OK: true, Progress: true, Message: message, JobID: job.ID})
		},
	}
}

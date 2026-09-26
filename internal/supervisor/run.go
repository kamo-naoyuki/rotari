package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// StartRun starts an async run in a detached worker and returns at once.
// onDone is called once the worker exits.
func (ops Operations) StartRun(request server.Request, onDone func()) (string, error) {
	prepared, err := ops.prepareRun(request)
	if err != nil {
		return "", err
	}
	defer prepared.release()
	paths := prepared.paths
	runID := ops.NewRunID()
	options := runRequestOptions(request, runID)
	options.OnDone = onDone
	if err := ops.launchWorker(paths, options); err != nil {
		return "", err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run: %s\n  Directory: %s\n\nCheck status:\n  rotari show --run-id %s\n\nCancel run:\n  rotari cancel --basedir %s --project-name %s",
		request.QueueName, model.RunLabel(runID, request.RunName), runDir, runID, paths.BaseDir, request.QueueName), nil
}

// Run executes a run inside the supervisor, reporting progress to the
// attached client, and returns once it has finished.
func (ops Operations) Run(request server.Request, progress func(server.Response)) (string, int, error) {
	prepared, err := ops.prepareRun(request)
	if err != nil {
		return "", 1, err
	}
	paths, queue := prepared.paths, prepared.queue
	runner := ops.Runner
	runID := ops.NewRunID()
	if err := runner.Begin(paths, projectrun.Start{RunID: runID, RunName: request.RunName, CWD: request.CWD}); err != nil {
		prepared.release()
		return "", 1, err
	}
	plan, err := runner.PlanSelection(paths, queue, request.Selection, request.JobIDs, requestScope(request), request.SourceRunID, request.PartialArray)
	if err != nil {
		_ = os.Remove(paths.LockFile)
		prepared.release()
		return "", 1, err
	}
	submitted := len(plan.Execute)
	excluded := len(queue.Commands) - submitted
	prepared.release()
	if progress != nil {
		progress(server.Response{Progress: true, Message: fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run ID: %s\n  Submitted: %d\n  Excluded: %d\n  Total: %d", request.QueueName, runID, submitted, excluded, len(queue.Commands))})
	}

	options := projectrun.OptionsFrom(runRequestOptions(request, runID))
	options.Executor = prepared.executor
	exitCode, err := runner.Run(paths, options, runObserver(request, runID, progress))
	if err != nil {
		return "", 1, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", 1, err
	}
	if summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		return CompletionMessage(paths, runID, summary), exitCode, nil
	}
	return fmt.Sprintf("=== Run finished ===\n  Project: %s\n  Run: %s\n  Exit code: %d", request.QueueName, runID, exitCode), exitCode, nil
}

// preparedRun is a run request that passed validation while its caller holds
// the project's state lock.
type preparedRun struct {
	paths    state.ProjectPaths
	queue    model.Queue
	executor string
	release  func()
}

// prepareRun validates a run request and takes the project's state lock. On
// success the caller must call release.
func (ops Operations) prepareRun(request server.Request) (preparedRun, error) {
	resolvedExecutor, err := ops.resolveQueueExecutor(request.QueueName, request.Executor)
	if err != nil {
		return preparedRun{}, err
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
	queue, err := ops.Runner.LoadQueue(paths, request.Executor, request.ExecutorOptions, request.ExecutorSettings)
	if err != nil {
		release()
		return preparedRun{}, err
	}
	if len(queue.Commands) == 0 {
		release()
		return preparedRun{}, fmt.Errorf("queue %q has no queued commands", request.QueueName)
	}
	// Check the scope before the run is created; planning happens after, and
	// its errors leave the new run behind.
	if scope := requestScope(request); scope.Kinds() > 0 {
		if _, err := model.SelectCommands(queue.Commands, scope); err != nil {
			release()
			return preparedRun{}, err
		}
	}
	return preparedRun{paths: paths, queue: queue, executor: resolvedExecutor, release: release}, nil
}

// resolveQueueExecutor returns the run's default executor, requested or the
// queue's, and checks that it and every job's executor are known.
func (ops Operations) resolveQueueExecutor(queueName, requested string) (string, error) {
	paths, err := state.ResolveProjectPaths(ops.BaseDir, queueName)
	if err != nil {
		return "", err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return "", err
	}
	if requested == "" {
		requested = queue.DefaultExecutor
		if requested == "" {
			requested = "local"
		}
	}
	executors := ops.Runner.Executors
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

// requestScope returns the stage or matrix that narrows a run request's
// selection.
func requestScope(request server.Request) model.CommandSelector {
	return model.CommandSelector{Stage: request.ScopeStage, Matrix: request.ScopeMatrix}
}

// runRequestOptions converts a run request into worker options for runID.
func runRequestOptions(request server.Request, runID string) run.Options {
	return run.Options{
		QueueName: request.QueueName, RunID: runID, RunName: request.RunName,
		LocalConcurrency: request.LocalConcurrency, BatchMaxActive: request.BatchMaxActive, Retry: request.Retry,
		Executor: request.Executor, ExecutorOptions: request.ExecutorOptions, Selection: request.Selection,
		Scope:  requestScope(request),
		JobIDs: request.JobIDs, SourceRunID: request.SourceRunID, PartialArray: request.PartialArray,
		CWD: request.CWD, ExecutorSettings: request.ExecutorSettings,
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
					result.ID, result.AttemptID, strings.Join(result.Command, " "), runID, attemptID)
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
			message := fmt.Sprintf("Job running:\n  ID: %s\n  Attempt ID: %s\n  Name: %s\n  Show:\n    rotari show --run-id %s --job-id %s",
				job.ID, job.AttemptID, name, runID, job.AttemptID)
			progress(server.Response{OK: true, Progress: true, Message: message, JobID: job.ID})
		},
	}
}

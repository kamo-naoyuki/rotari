package supervisor

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/attachment"
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
	observer := ops.progressObserver(request, started, nil)
	go func() {
		if onDone != nil {
			defer onDone()
		}
		if _, err := ops.runWithAttachmentMonitor(started.paths, started.runID, started.options, observer); err != nil {
			ops.logf("async run %s failed: %v", started.runID, err)
		}
	}()
	message := fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run: %s\n  Directory: %s\n\nWait for it:\n  rotari wait -r %s\n\nCheck status:\n  rotari show -r %s\n\nCancel run:\n  rotari cancel -p %s %s\n",
		request.QueueName, model.RunLabel(started.runID, request.RunName), runDir, started.runID, started.runID, request.QueueName, started.runID)
	return started.runID, message + sourceNotice(started), nil
}

// runWithAttachmentMonitor keeps cancellation-on-disconnect enforceable even
// when a client is killed without running cleanup. Session locks remain held
// across suspension; an unverifiable remote session is conservatively live.
func (ops Operations) runWithAttachmentMonitor(paths state.ProjectPaths, runID string, options projectrun.Options, observer projectrun.Observer) (int, error) {
	stop := make(chan struct{})
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				live, stale, err := attachment.Scan(paths, runID)
				_ = live // Unknown liveness remains attached; only proved-stale sessions act.
				if err != nil {
					ops.logf("run %s attachment scan failed: %v", runID, err)
					continue
				}
				for _, client := range stale {
					status := model.RunClientStatus{State: model.RunClientDetached, Reason: model.RunClientReasonEOF}
					handled := true
					if client.Disconnect == server.DisconnectActionCancel {
						status.State, status.Reason = model.RunClientCancelling, model.RunClientReasonCancel
						if _, cancelErr := ops.Controller.Cancel(paths.BaseDir, paths.ProjectName, runID, nil, false); cancelErr != nil {
							phase, phaseErr := project.RunPhaseOf(paths, runID)
							handled = phaseErr == nil && phase != project.RunPhaseRunning
							if !handled {
								ops.logf("run %s cancellation after client %s disconnect failed: %v", runID, client.ID, cancelErr)
							}
						}
					}
					if client.Initiator {
						if err := ops.Runner.SetRunClientStatus(paths, runID, status); err != nil {
							ops.logf("run %s client status after disconnect failed: %v", runID, err)
						}
					}
					if handled {
						if err := attachment.Forget(paths, client.ID); err != nil {
							ops.logf("run %s failed to acknowledge disconnected client %s: %v", runID, client.ID, err)
						}
					}
				}
			}
		}
	}()
	exitCode, err := ops.Runner.Run(paths, options, observer)
	close(stop)
	<-monitorDone
	return exitCode, err
}

// Run executes a run inside the supervisor, reporting progress to the
// attached client, and returns once it has finished.
func (ops Operations) Run(request server.Request, progress func(server.Response)) (string, int, error) {
	started, err := ops.beginRun(request)
	if err != nil {
		return "", 1, err
	}
	paths, runID := started.paths, started.runID
	exitCode, err := ops.runWithAttachmentMonitor(paths, runID, started.options, ops.progressObserver(request, started, progress))
	if err != nil {
		return "", 1, err
	}
	// The client reads the completion from the run's files, as wait does
	// (cmd/rotari run_completion.go), so this message only closes the request.
	return fmt.Sprintf("=== Run finished ===\n  Project: %s\n  Run: %s\n  Exit code: %d", request.QueueName, runID, exitCode), exitCode, nil
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
	// A valid job ID may occupy the optional journal's path.
	progressJobIDCollision bool
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
	clientStatus := model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientAttached}
	if request.Async {
		clientStatus = model.RunClientStatus{Mode: model.RunClientModeAsync, State: model.RunClientDetached, Reason: model.RunClientReasonAsync}
	}
	if err := attachment.EnableRun(prepared.paths, runID); err != nil {
		return startedRun{}, fmt.Errorf("failed to initialize run attachments: %w", err)
	}
	if !request.Async && request.ClientSessionID != "" {
		if err := attachment.Bind(prepared.paths, request.ClientSessionID, runID); err != nil {
			return startedRun{}, fmt.Errorf("failed to accept client attachment: %w", err)
		}
	}
	start := projectrun.Start{RunID: runID, RunName: request.RunName, ClientStatus: clientStatus, CWD: request.CWD, ConfigPath: request.ConfigPath, FileConfig: request.FileConfig}
	if prepared.snapshotFromSource {
		start.Snapshot = &prepared.queue
	}
	if err := ops.Runner.Begin(prepared.paths, start); err != nil {
		return startedRun{}, err
	}
	options := runRequestOptions(request, runID)
	options.Executor = prepared.executor
	jobs := model.QueueToJobs(prepared.queue.Commands)
	progressJobIDCollision := false
	for _, job := range jobs {
		if job.ID == state.ProgressFileName {
			progressJobIDCollision = true
			break
		}
	}
	return startedRun{
		paths: prepared.paths, runID: runID, options: options,
		submitted: len(prepared.plan.Execute), total: len(jobs),
		sourceRunID: prepared.sourceRunID, usedQueueWithSource: prepared.usedQueueWithSource,
		omittedSourceJobs:      prepared.omittedSourceJobs,
		progressJobIDCollision: progressJobIDCollision,
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

// progressObserver journals the same stream for sync and async runs, regardless
// of Quiet, and optionally forwards it to the attached client. A journal error
// disables further writes (a failed write may have left a partial line), logs
// once, and never prevents execution or forwarding.
func (ops Operations) progressObserver(request server.Request, started startedRun, progress func(server.Response)) projectrun.Observer {
	var journal *state.ProgressJournal
	if started.progressJobIDCollision {
		ops.logf("run %s progress journal disabled: job ID %s uses the journal path", started.runID, state.ProgressFileName)
	} else {
		runDir, err := state.SafeJoin(started.paths.RunsDir, started.runID)
		if err == nil {
			journal, err = state.NewProgressJournal(runDir)
		}
		if err != nil {
			ops.logf("run %s progress journal failed: %v", started.runID, err)
		}
	}
	var mu sync.Mutex
	emit := func(response server.Response) {
		mu.Lock()
		defer mu.Unlock()
		if journal != nil {
			event := model.ProgressEvent{
				OK: response.OK, Progress: response.Progress, Message: response.Message,
				RunID: response.RunID, Notice: response.Notice,
				JobID: response.JobID, Completed: response.Completed, Total: response.Total,
				Succeeded: response.Succeeded, Failed: response.Failed,
			}
			if err := journal.Append(event); err != nil {
				ops.logf("run %s progress journal failed: %v", started.runID, err)
				journal = nil
			}
		}
		if progress != nil {
			progress(response)
		}
	}
	emit(server.Response{Progress: true, RunID: started.runID, Notice: sourceNotice(started), Total: started.submitted, Message: fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run ID: %s\n  Submitted: %d\n  Excluded: %d\n  Total: %d", request.QueueName, started.runID, started.submitted, started.total-started.submitted, started.total)})
	return runObserver(request, started.runID, emit)
}

// runObserver turns job starts and results into progress responses.
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
				// An attempt ID locates its run, as in the "Job running" hint;
				// a bare job ID needs the run.
				attemptID, show := result.AttemptID, "rotari show -j "+result.AttemptID
				if attemptID == "" {
					attemptID, show = result.ID, fmt.Sprintf("rotari show --run-id %s --job-id %s", runID, result.ID)
				}
				message = fmt.Sprintf("%s\n  ID: %s\n  Attempt ID: %s\n  Command: %s\n  Show output:\n    %s",
					failureTitle,
					result.ID, attemptID, strings.Join(result.Command, " "), show)
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

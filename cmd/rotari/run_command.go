package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdRun starts a run, optionally repopulating the queue from historical run
// results or selected attempts before submitting work to the background server.
func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	jobNameOption := cliString(fs, "job-name", "")
	overwriteQueue := cliBool(fs, "overwrite", false)
	runName := cliString(fs, "run-name", "")
	localConcurrency := cliInt(fs, "local-concurrency", 8)
	batchConcurrency := cliInt(fs, "batch-concurrency", 8)
	executorSettings := cliExecutorRunSettings(fs)
	retry := cliInt(fs, "retry", 0)
	failed := cliBool(fs, "failed", false)
	unfinished := cliBool(fs, "unfinished", false)
	success := cliBool(fs, "success", false)
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	partialArray := cliBool(fs, "partial-array", true)
	async := cliBool(fs, "async", false)
	quiet := cliBool(fs, "quiet", false)
	executor := cliString(fs, "executor", "")
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	left := fs.Args()
	selection := model.ResultSelection(*failed, *unfinished, *success)
	if len(jobIDs) > 0 && selection == "" {
		selection = "job-id"
	}
	if len(left) != 0 || *localConcurrency < 1 || *batchConcurrency < 1 || *retry < -1 {
		printError("usage: " + cliUsage("run"))
		return 1
	}
	if *jobNameOption != "" && len(jobIDs) > 0 {
		printError("--job-name cannot be combined with --job-id")
		return 1
	}
	if *jobNameOption != "" {
		if *runIDOption != "" {
			baseDir, projectName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runIDOption)
			if err != nil {
				printError(err)
				return 1
			}
			paths, err := state.ResolveProjectPaths(baseDir, projectName)
			if err != nil {
				printError(err)
				return 1
			}
			target, found, err := findShowJobInRun(paths, *runIDOption, *jobNameOption, true)
			if err != nil || !found {
				printErrorf("job name %q not found in run %q", *jobNameOption, *runIDOption)
				return 1
			}
			jobIDs = stringSliceFlag{target.jobID}
		} else {
			targets, err := resolveJobTargets(*basedir, *queueNameOption, *jobNameOption, true, false)
			if err != nil {
				printError(err)
				return 1
			}
			if len(targets) != 1 {
				printErrorf("job name %q is %s", *jobNameOption, map[bool]string{true: "ambiguous across latest runs", false: "not found"}[len(targets) > 1])
				return 1
			}
			*basedir, *queueNameOption, *runIDOption = targets[0].baseDir, targets[0].projectName, targets[0].runID
			jobIDs = stringSliceFlag{targets[0].jobID}
		}
	}
	attemptSelection := false
	for _, jobID := range jobIDs {
		if !strings.HasPrefix(jobID, "att_") {
			continue
		}
		payload, decodeErr := state.DecodeAttemptID(jobID)
		if decodeErr != nil {
			printError(decodeErr)
			return 1
		}
		if *runIDOption != "" && *runIDOption != payload.RunID {
			printErrorf("attempt %q belongs to run %q, not %q", jobID, payload.RunID, *runIDOption)
			return 1
		}
		if !attemptSelection {
			*runIDOption = payload.RunID
			attemptSelection = true
		}
	}
	if *runIDOption == "" && len(jobIDs) > 0 && !attemptSelection {
		target, resolveErr := resolveLatestJobIDSelection(*basedir, *queueNameOption, jobIDs)
		if resolveErr != nil {
			printError(resolveErr)
			return 1
		}
		*basedir, *queueNameOption, *runIDOption = target.baseDir, target.projectName, target.runID
	}
	if *overwriteQueue && *runIDOption == "" {
		printError("usage: " + cliUsage("run"))
		return 1
	}
	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureProjectIdle(baseDir, queueName, "run"); err != nil {
		printError(err)
		return 1
	}

	// A source run is needed whenever a selection filter is used, so the
	// filter can be matched against that run's results. An explicit
	// --run-id always repopulates the queue first ("run --run-id X"
	// behaves like "copy --run-id X --overwrite" followed by "run"). When
	// --run-id is omitted, the queue is only repopulated from the latest
	// run if it is currently empty; a non-empty queue (e.g. already
	// restored and edited via "change") is used as-is.
	sourceRunID := *runIDOption
	forceCopy := sourceRunID != ""
	if attemptSelection {
		selection = "job-id"
		forceCopy = true
	}
	if sourceRunID == "" && selection != "" {
		paths, pathErr := state.ResolveProjectPaths(baseDir, queueName)
		if pathErr != nil {
			printError(pathErr)
			return 1
		}
		meta, metaErr := state.LoadMeta(paths.MetaFile)
		if metaErr != nil {
			printErrorf("failed to load metadata: %v", metaErr)
			return 1
		}
		if meta.LastRunID == "" {
			printErrorf("queue %q has no previous run", queueName)
			return 1
		}
		sourceRunID = meta.LastRunID
		queue, queueErr := state.LoadQueue(paths.QueueFile)
		if queueErr != nil {
			printErrorf("failed to load queue: %v", queueErr)
			return 1
		}
		// Explicit job selection is equivalent to copying the selected job
		// from the latest run and then running it. Other filtered runs keep
		// the current queue when it is already populated.
		forceCopy = len(queue.Commands) == 0 || selection == "job-id"
	}
	if forceCopy {
		// Only prompts when the queue actually has jobs to lose; an empty
		// queue (the common "auto-copy" case) is overwritten silently.
		overwriteConfirmed, confirmErr := confirmQueueOverwrite(baseDir, queueName, false, *overwriteQueue)
		if confirmErr != nil {
			printError(confirmErr)
			return 1
		}
		copySelection := "all"
		copyJobIDs := []string(nil)
		if attemptSelection {
			copySelection = "job-id"
			copyJobIDs = jobIDs
		}
		message, copyErr := copyRunToQueue(baseDir, queueName, sourceRunID, copySelection, copyJobIDs, false, overwriteConfirmed)
		if copyErr != nil {
			printError(copyErr)
			return 1
		}
		fmt.Println(colorKeyValueMessage(message, green))
	}
	if attemptSelection {
		selection = ""
		jobIDs = nil
	}

	if err := ensureServer(baseDir); err != nil {
		printError(err)
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		printErrorf("failed to determine working directory: %v", err)
		return 1
	}
	request := serverinternal.Request{
		Op: serverinternal.OpRun, QueueName: queueName, LocalConcurrency: *localConcurrency, BatchMaxActive: *batchConcurrency, ExecutorSettings: executorSettings(), Retry: *retry, Async: *async, Quiet: *quiet,
		RunName: *runName, Executor: *executor, ExecutorOptions: executorOptions, CWD: cwd,
		Selection: selection, JobIDs: jobIDs, SourceRunID: sourceRunID, PartialArray: *partialArray,
	}
	var response serverinternal.Response
	if *async {
		response, err = serverinternal.SendRequest(baseDir, request)
	} else {
		response, err = sendRunRequest(baseDir, request)
	}
	if err != nil {
		printErrorf("failed to contact server: %v", err)
		return 1
	}
	if !response.OK {
		printError(response.Message)
		return 1
	}
	if !*quiet {
		fmt.Print(colorMessage(response.Message))
	}
	return response.ExitCode
}

// cmdRetry reruns failed and unfinished jobs by delegating to cmdRun with the
// retry selection flags.
func cmdRetry(args []string) int {
	return cmdRun(append([]string{"--failed", "--unfinished"}, args...))
}

func sendRunRequest(baseDir string, request serverinternal.Request) (serverinternal.Response, error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	detach := make(chan struct{}, 1)
	if isTerminal(os.Stdin) {
		go func() {
			var buffer [1]byte
			n, err := os.Stdin.Read(buffer[:])
			if (n == 0 && err == nil) || (n > 0 && buffer[0] == serverinternal.DetachControl) {
				detach <- struct{}{}
			}
		}()
	}
	printer := runProgressPrinter{quiet: request.Quiet, lastCompleted: -1, lastSucceeded: -1, lastFailed: -1}
	response, outcome, err := serverinternal.StreamRun(baseDir, request, detach, signals, printer.print)
	if err != nil {
		return serverinternal.Response{}, err
	}
	switch outcome {
	case serverinternal.RunDetached:
		if !request.Quiet {
			fmt.Println(cyan(serverinternal.DetachedMessage))
		}
	case serverinternal.RunInterrupted:
		if !request.Quiet {
			fmt.Println(yellow("Cancellation requested; stopping running jobs..."))
		}
	}
	return response, nil
}

// runProgressPrinter renders a synchronous run's progress responses.
type runProgressPrinter struct {
	quiet                                    bool
	lastCompleted, lastSucceeded, lastFailed int
}

func (printer *runProgressPrinter) print(response serverinternal.Response) {
	if printer.quiet && !strings.HasPrefix(response.Message, "Job failed") {
		return
	}
	if response.Message == "" {
		if response.Completed == printer.lastCompleted && response.Succeeded == printer.lastSucceeded && response.Failed == printer.lastFailed {
			return
		}
		fmt.Printf("%s\n", colorKeyValueMessage(fmt.Sprintf("progress: %d/%d completed=%d failed=%d", response.Completed, response.Total, response.Succeeded, response.Failed), cyan))
		printer.lastCompleted, printer.lastSucceeded, printer.lastFailed = response.Completed, response.Succeeded, response.Failed
		return
	}
	switch {
	case strings.HasPrefix(response.Message, "Job failed"):
		title, details, _ := strings.Cut(response.Message, "\n")
		fmt.Printf("%s\n%s\n", red(title), colorLabeledDetails(details, true))
	case strings.HasPrefix(response.Message, "Retrying job"):
		fmt.Printf("%s\n", colorKeyValueMessage(response.Message, yellow))
	case strings.HasPrefix(response.Message, "=== Run started ==="):
		fmt.Printf("%s\n", colorMessage(response.Message))
		fmt.Println(cyan("Press Ctrl-D to detach; Ctrl-C to cancel."))
	case strings.HasPrefix(response.Message, "Job running:"):
		title, details, _ := strings.Cut(response.Message, "\n")
		fmt.Printf("%s\n%s\n", cyan(title), colorLabeledDetails(details, false))
	default:
		fmt.Printf("%s\n", yellow(response.Message))
	}
}

func resolveQueueExecutor(baseDir, queueName, requested string) (string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
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
	if !executorRegistry.Known(requested) {
		return "", fmt.Errorf("unsupported executor: %s", requested)
	}
	resolved := requested
	for _, queued := range queue.Commands {
		executor := queued.Executor
		if executor == "" {
			executor = requested
		}
		if !executorRegistry.Known(executor) {
			return "", fmt.Errorf("unsupported executor: %s", executor)
		}
	}
	return resolved, nil
}

func startServerRun(baseDir string, request serverinternal.Request, onDone func()) (string, error) {
	_, err := resolveQueueExecutor(baseDir, request.QueueName, request.Executor)
	if err != nil {
		return "", err
	}
	if request.LocalConcurrency < 1 {
		return "", errors.New("local concurrency must be >= 1")
	}
	paths, err := state.ResolveProjectPaths(baseDir, request.QueueName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
		return "", err
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", err
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "run"); err != nil {
		return "", err
	}
	queue, err := loadRunQueue(paths, request.Executor, request.ExecutorOptions, request.ExecutorSettings)
	if err != nil {
		return "", err
	}
	if len(queue.Commands) == 0 {
		return "", fmt.Errorf("queue %q has no queued commands", request.QueueName)
	}
	runID := makeRunID()
	if err := launchAsyncRun(paths, runcontract.Options{
		QueueName: request.QueueName, RunID: runID, RunName: request.RunName,
		LocalConcurrency: request.LocalConcurrency, BatchMaxActive: request.BatchMaxActive, Retry: request.Retry,
		Executor: request.Executor, ExecutorOptions: request.ExecutorOptions, Selection: request.Selection,
		JobIDs: request.JobIDs, SourceRunID: request.SourceRunID, PartialArray: request.PartialArray,
		CWD: request.CWD, OnDone: onDone, ExecutorSettings: request.ExecutorSettings,
	}); err != 0 {
		return "", errors.New("queue is already running")
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run: %s\n  Directory: %s\n\nCheck status:\n  rotari show --run-id %s\n\nCancel run:\n  rotari cancel --basedir %s --project-name %s",
		request.QueueName, formatRunLabel(runID, request.RunName), runDir, runID, paths.BaseDir, request.QueueName), nil
}

func runServerSync(baseDir string, request serverinternal.Request, progress func(serverinternal.Response)) (string, int, error) {
	resolvedExecutor, err := resolveQueueExecutor(baseDir, request.QueueName, request.Executor)
	if err != nil {
		return "", 1, err
	}
	if request.LocalConcurrency < 1 {
		return "", 1, errors.New("local concurrency must be >= 1")
	}
	paths, err := state.ResolveProjectPaths(baseDir, request.QueueName)
	if err != nil {
		return "", 1, err
	}
	if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
		return "", 1, err
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", 1, err
	}
	if err := ensureProjectIdleForPaths(paths, "run"); err != nil {
		release()
		return "", 1, err
	}
	queue, err := loadRunQueue(paths, request.Executor, request.ExecutorOptions, request.ExecutorSettings)
	if err != nil {
		release()
		return "", 1, err
	}
	if len(queue.Commands) == 0 {
		release()
		return "", 1, fmt.Errorf("queue %q has no queued commands", request.QueueName)
	}
	runID := makeRunID()
	if err := writeRunContext(paths, runID, request.CWD); err != nil {
		release()
		return "", 1, err
	}
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: runID, RunName: request.RunName, StartedAt: nowRFC3339()}); err != nil {
		release()
		return "", 1, fmt.Errorf("project %q is already running", request.QueueName)
	}
	if err := registerRun(paths, runID); err != nil {
		_ = os.Remove(paths.LockFile)
		if runDir, pathErr := state.SafeJoin(paths.RunsDir, runID); pathErr == nil {
			_ = os.RemoveAll(runDir)
		}
		release()
		return "", 1, fmt.Errorf("failed to register run: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		_ = os.Remove(paths.LockFile)
		release()
		return "", 1, err
	}
	meta.Phase = "running"
	meta.LastRunID = runID
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		_ = os.Remove(paths.LockFile)
		release()
		return "", 1, err
	}
	plan, err := planRerunSelection(paths, queue, request.Selection, request.JobIDs, request.SourceRunID, request.PartialArray)
	if err != nil {
		_ = os.Remove(paths.LockFile)
		release()
		return "", 1, err
	}
	submitted := len(plan.Execute)
	excluded := len(queue.Commands) - submitted
	release()
	if progress != nil {
		progress(serverinternal.Response{Progress: true, Message: fmt.Sprintf("=== Run started ===\n  Project: %s\n  Run ID: %s\n  Submitted: %d\n  Excluded: %d\n  Total: %d", request.QueueName, runID, submitted, excluded, len(queue.Commands))})
	}

	stopLoadSampling := startRunLoadSampling(paths, runID)
	exitCode := executeMixedRun(paths, runID, request.RunName, request.LocalConcurrency, request.BatchMaxActive, request.Retry, resolvedExecutor, request.ExecutorOptions, request.Selection, request.JobIDs, request.SourceRunID, request.PartialArray, func(result model.JobResult, completed, total, succeeded, failed int) {
		if progress != nil {
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
			progress(serverinternal.Response{OK: true, Progress: true, Message: message, JobID: result.ID, Completed: completed, Total: total, Succeeded: succeeded, Failed: failed})
		}
	}, func(job model.JobSpec) {
		if progress == nil {
			return
		}
		name := job.Name
		if name == "" {
			name = "-"
		}
		message := fmt.Sprintf("Job running:\n  ID: %s\n  Attempt ID: %s\n  Name: %s\n  Show:\n    rotari show --run-id %s --job-id %s",
			job.ID, job.AttemptID, name, runID, job.AttemptID)
		progress(serverinternal.Response{OK: true, Progress: true, Message: message, JobID: job.ID})
	}, request.ExecutorSettings)
	stopLoadSampling()
	if err := finishRunContext(paths, runID); err != nil {
		_ = os.Remove(paths.LockFile)
		return "", 1, err
	}
	if err := finishRun(paths, runID, exitCode); err != nil {
		_ = os.Remove(paths.LockFile)
		return "", 1, err
	}
	if err := os.Remove(paths.LockFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", 1, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", 1, err
	}
	data, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
	if err == nil {
		var summary model.RunSummary
		if json.Unmarshal(data, &summary) == nil {
			return formatRunCompletion(paths, runID, summary), exitCode, nil
		}
	}
	return fmt.Sprintf("=== Run finished ===\n  Project: %s\n  Run: %s\n  Exit code: %d", request.QueueName, runID, exitCode), exitCode, nil
}

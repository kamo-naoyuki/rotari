package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdRun starts a run, optionally building its snapshot from historical run
// results or selected attempts before submitting work to the supervisor.
func cmdRun(args []string) int {
	return runJobs(args, "")
}

// errJobsWithResultFilter rejects a job named directly together with a result
// filter: naming the job already says what to execute.
const errJobsWithResultFilter = "--job-id or --job-name cannot be combined with --failed, --unfinished, or --success"

// errJobsWithFilter rejects a job named directly together with a --filter-*
// condition, for the same reason.
const errJobsWithFilter = "--job-id or --job-name cannot be combined with --filter-* options"

// runJobs is run, and retry with defaultSelection: the result selection used
// when neither a result filter nor a job is given.
func runJobs(args []string, defaultSelection string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	jobNameOption := cliString(fs, "job-name", "")
	if rejectRunOverwrite(args) {
		return 1
	}
	runName := cliString(fs, "run-name", "")
	localConcurrency := cliInt(fs, "local-concurrency", 8)
	batchConcurrency := cliInt(fs, "batch-concurrency", 8)
	executorSettings := cliExecutorRunSettings(fs)
	retry := cliInt(fs, "retry", 0)
	filterOptions := cliJobFilterOptions(fs, queueRunJobFilters)
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	partialArray := cliBool(fs, "partial-array", true)
	async := cliBool(fs, "async", false)
	quiet := cliBool(fs, "quiet", false)
	executor := cliString(fs, "executor", "")
	envMode := cliString(fs, "env", model.EnvModeAll)
	matchBy := cliString(fs, "match-by", model.MatchByIDAndFingerprint)
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	guard := cliGuardFlags(fs)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if *async && *guard.dryRun {
		printError("--async cannot be combined with --dry-run, which starts no run; preview without --async, then start the run with --async --if-revision REVISION, taking REVISION from the preview")
		return 1
	}
	if *matchBy != model.MatchByJobID && *matchBy != model.MatchByFingerprint && *matchBy != model.MatchByIDAndFingerprint {
		printErrorf("invalid match mode %q (choose %s, %s, or %s)", *matchBy, model.MatchByJobID, model.MatchByFingerprint, model.MatchByIDAndFingerprint)
		return 1
	}
	left := fs.Args()
	selection := filterOptions.resultSelection()
	if len(left) > 1 || (len(left) == 1 && *runIDOption != "") || *localConcurrency < 1 || *batchConcurrency < 1 || *retry < -1 {
		printError("usage: " + cliUsage("run"))
		return 1
	}
	if len(left) == 1 {
		*runIDOption = left[0]
	}
	if *jobNameOption != "" && len(jobIDs) > 0 {
		printError("--job-name cannot be combined with --job-id")
		return 1
	}
	if err := model.ValidateReservedName("run name", *runName); err != nil {
		printError(err)
		return 1
	}
	scope, err := filterOptions.scope()
	if err != nil {
		printError(err)
		return 1
	}
	filter := filterOptions.filter()
	directJobs := *jobNameOption != "" || len(jobIDs) > 0
	if scope.Kinds() > 1 || (scope.Kinds() > 0 && directJobs) {
		printError("--stage, --matrix, and --job-id or --job-name cannot be combined")
		return 1
	}
	if selection != "" && directJobs {
		printError(errJobsWithResultFilter)
		return 1
	}
	if !filter.Empty() && directJobs {
		printError(errJobsWithFilter)
		return 1
	}
	if selection == "" && !directJobs {
		selection = defaultSelection
	}
	if (scope.Kinds() > 0 || !filter.Empty()) && selection == "" {
		// A scope or filter alone re-executes every job it keeps, whatever its
		// result.
		selection = model.ResultSelection(true, true, true)
	}
	if len(jobIDs) > 0 && selection == "" {
		selection = "job-id"
	}
	if defaultSelection != "" {
		if code, handled := tryActiveRetry(fs, activeRetryOptions{
			baseDir: *basedir, projectName: *queueNameOption, requestedRunID: *runIDOption, jobName: *jobNameOption,
			jobIDs: jobIDs, selection: selection, scope: scope, filter: filter, partialArray: *partialArray,
			dryRun: *guard.dryRun, ifRevision: *guard.ifRevision, quiet: *quiet,
		}); handled {
			return code
		}
	}
	if *jobNameOption != "" {
		if *runIDOption != "" {
			baseDir, projectName, resolvedRunID, err := resolve.ExistingRunID(*basedir, *queueNameOption, *runIDOption)
			if err != nil {
				printError(err)
				return 1
			}
			*runIDOption = resolvedRunID
			paths, err := state.ResolveProjectPaths(baseDir, projectName)
			if err != nil {
				printError(err)
				return 1
			}
			target, found, err := resolve.JobInRun(paths, *runIDOption, *jobNameOption, true)
			if err != nil {
				printError(err)
				return 1
			}
			if !found {
				printErrorf("job name %q not found in run %q", *jobNameOption, *runIDOption)
				return 1
			}
			jobIDs = stringSliceFlag{target.JobID}
		} else {
			// A non-empty queue is preferred to the latest run, as in show.
			targets, err := resolve.Jobs(*basedir, *queueNameOption, *jobNameOption, true, true)
			if err != nil {
				printError(err)
				return 1
			}
			if len(targets) == 0 {
				printErrorf("job name %q not found", *jobNameOption)
				return 1
			}
			if len(targets) > 1 {
				printError(resolve.AmbiguousError(fmt.Sprintf("job name %q", *jobNameOption), targets))
				return 1
			}
			*basedir, *queueNameOption = targets[0].BaseDir, targets[0].ProjectName
			if !targets[0].FromQueue {
				*runIDOption = targets[0].RunID
			}
			jobIDs = stringSliceFlag{targets[0].JobID}
		}
	}
	// Set after --job-name has been resolved to a job ID.
	if len(jobIDs) > 0 && selection == "" {
		selection = "job-id"
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
		// Jobs in a non-empty queue run from it, keeping its edits; otherwise
		// the latest run that holds them is restored first.
		target, resolveErr := resolve.QueuedOrLatestJobIDs(*basedir, *queueNameOption, jobIDs)
		if resolveErr != nil {
			printError(resolveErr)
			return 1
		}
		*basedir, *queueNameOption = target.BaseDir, target.ProjectName
		if !target.FromQueue {
			*runIDOption = target.RunID
		}
	}
	baseDir, queueName, resolvedRunID, err := resolve.ExistingRunID(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	*runIDOption = resolvedRunID
	if err := ensureProjectIdle(baseDir, queueName, "run"); err != nil {
		printError(err)
		return 1
	}

	if attemptSelection {
		selection = "job-id"
	}
	sourcePaths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printError(err)
		return 1
	}
	sourceRunID, sourcePolicy, err := projectrun.RunSource(sourcePaths, selection, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	runRevision := *guard.ifRevision
	copyJobIDs := []string(nil)
	if attemptSelection {
		copyJobIDs = append(copyJobIDs, jobIDs...)
		selection = ""
		jobIDs = nil
	}

	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printError(err)
		return 1
	}
	if *guard.dryRun {
		return previewRun(paths, nil, projectrun.PlanRequest{
			Executor: *executor, ExecutorOptions: executorOptions, Settings: executorSettings(),
			Selection: selection, JobIDs: jobIDs, Scope: scope, Filter: filter, SourceRunID: sourceRunID,
			SourcePolicy: sourcePolicy, CopyAttempts: attemptSelection, CopyJobIDs: copyJobIDs,
			PartialArray: *partialArray, MatchBy: *matchBy,
		}, *guard.ifRevision, *runName)
	}
	cwd, err := os.Getwd()
	if err != nil {
		printErrorf("failed to determine working directory: %v", err)
		return 1
	}
	client, err := startSupervisor(paths)
	if err != nil {
		printError(err)
		return 1
	}
	defer client.Close()
	request := serverinternal.Request{
		Op: serverinternal.OpRun, QueueName: queueName, LocalConcurrency: *localConcurrency, BatchMaxActive: *batchConcurrency, ExecutorSettings: executorSettings(), Retry: *retry, Async: *async, Quiet: *quiet,
		RunName: *runName, Executor: *executor, ExecutorOptions: executorOptions, EnvMode: *envMode, CWD: cwd, ConfigPath: cliConfigPath,
		Selection: selection, JobIDs: jobIDs, ScopeStage: scope.Stage, ScopeMatrix: scope.Matrix, Filter: filter, SourceRunID: sourceRunID,
		SourcePolicy: string(sourcePolicy), CopyAttempts: attemptSelection, CopyJobIDs: copyJobIDs,
		PartialArray: *partialArray, MatchBy: *matchBy,
		IfRevision: runRevision,
	}
	var response serverinternal.Response
	if *async {
		response, err = client.Send(request)
	} else {
		response, err = sendRunRequest(client, request)
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

func rejectRunOverwrite(args []string) bool {
	flagsWithValues := make(map[string]bool)
	for _, spec := range runCommandFlags(false) {
		if spec.ValueName != "" {
			flagsWithValues[spec.Name] = true
		}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			return false
		}
		if arg == "--overwrite" || arg == "-overwrite" || strings.HasPrefix(arg, "--overwrite=") || strings.HasPrefix(arg, "-overwrite=") {
			printError("run and retry no longer accept --overwrite; saved-run retries leave the next queue untouched")
			return true
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if flagsWithValues[name] && !hasValue && index+1 < len(args) {
			index++
		}
	}
	return false
}

// cmdRetry reruns failed and unfinished jobs, or, given a result filter or a
// job, what they select, as run does.
func cmdRetry(args []string) int {
	return runJobs(args, model.ResultSelection(true, true, false))
}

// sendRunRequest sends a synchronous run request to the supervisor on client
// and prints its progress until the run finishes or the client detaches or is
// interrupted.
func sendRunRequest(client *serverinternal.Client, request serverinternal.Request) (serverinternal.Response, error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	detach := make(chan struct{}, 1)
	if isTerminal(os.Stdin) {
		go watchDetach(os.Stdin, detach)
	}
	printer := runProgressPrinter{quiet: request.Quiet, lastCompleted: -1, lastSucceeded: -1, lastFailed: -1}
	response, outcome, err := client.StreamRun(request, detach, signals, printer.print)
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

// watchDetach signals detach when input reaches EOF, which is how a canonical
// mode terminal reports Ctrl-D at the start of a line, or carries the detach
// byte. Other input is discarded.
func watchDetach(input io.Reader, detach chan<- struct{}) {
	var buffer [256]byte
	for {
		n, err := input.Read(buffer[:])
		if bytes.IndexByte(buffer[:n], serverinternal.DetachControl) >= 0 || errors.Is(err, io.EOF) {
			detach <- struct{}{}
			return
		}
		if err != nil {
			return
		}
	}
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
		fmt.Printf("%s\n", colorKeyValueMessage(fmt.Sprintf("progress: %d/%d succeeded=%d failed=%d", response.Completed, response.Total, response.Succeeded, response.Failed), cyan))
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

// previewRun prints what a run of the project would execute, planned with
// projectrun.Runner.PlanRun as the run itself is, without starting it. queue,
// when set, is an explicitly supplied replacement queue; saved-run snapshots
// are prepared by the shared planner without changing queue.json.
func previewRun(paths state.ProjectPaths, queue *model.Queue, request projectrun.PlanRequest, ifRevision, runName string) int {
	planned, revision, err := projectRunner().PreviewRun(paths, queue, request, ifRevision)
	if err != nil {
		printError(err)
		return 1
	}
	jobs := model.QueueToJobs(planned.Queue.Commands)
	executed := 0
	for _, job := range jobs {
		if planned.Plan.Execute[job.ID] {
			executed++
		}
	}
	fmt.Println(colorKeyValueMessage(fmt.Sprintf("dry run: run project=%s%s would execute %d of %d job(s), carrying %d result(s)", paths.ProjectName, strings.Join(optionalField(" run_name", runName), ""), executed, len(jobs), len(planned.Plan.CarriedResults)), green))
	for _, job := range jobs {
		if planned.Plan.Execute[job.ID] {
			fmt.Printf("  execute job_id=%s%s%s\n", job.ID, strings.Join(optionalField(" job_name", job.Name), ""), strings.Join(optionalField(" depends_on_rerun", planned.Plan.RerunDependencies[job.ID]), ""))
		}
	}
	if planned.UsedQueueWithSource {
		fmt.Printf("Retry source: current queue; latest run %s has %d failed or unfinished job(s) not included", planned.SourceRunID, len(planned.OmittedSourceJobs))
		if len(planned.OmittedSourceJobs) > 0 {
			fmt.Printf(": %s\nInclude them with: rotari copy --basedir %s --project-name %s --run-id %s --failed --unfinished --append, then retry\n",
				strings.Join(planned.OmittedSourceJobs, ", "), executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(planned.SourceRunID))
		} else {
			fmt.Println()
		}
	}
	fmt.Printf("revision=%s\n", revision)
	return 0
}

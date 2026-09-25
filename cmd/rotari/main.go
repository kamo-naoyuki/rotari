package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const jobIDLen = 9
const runIDLen = len("20060102-150405-00000000")
const defaultProjectName = "default"
const commandJSONName = "command.json"
const authBearerPrefix = "Bearer "
const headerContentType = "Content-Type"
const mimeApplicationJSON = "application/json"

func validateQueueJobs(queue model.Queue) error {
	if err := model.ValidateMatrixGroups(queue.Commands); err != nil {
		return err
	}
	if err := model.ValidateStageNames(queue.Commands); err != nil {
		return err
	}
	expandedIDs := make(map[string]bool)
	for _, command := range queue.Commands {
		if !state.IsValidPathElement(command.ID) {
			return fmt.Errorf("invalid job ID %q", command.ID)
		}
		if len(command.Command) == 0 || command.Command[0] == "" {
			return fmt.Errorf("job %q has an empty command", command.ID)
		}
		for _, argument := range command.Command {
			if strings.ContainsRune(argument, '\x00') {
				return fmt.Errorf("job %q command contains a NUL byte", command.ID)
			}
		}
		if err := model.ValidateEnvironment(command.Environment); err != nil {
			return fmt.Errorf("job %q has invalid environment: %w", command.ID, err)
		}
		if strings.ContainsRune(command.WorkingDirectory, '\x00') {
			return fmt.Errorf("job %q working directory contains a NUL byte", command.ID)
		}
		if command.Array != nil {
			if err := model.ValidateArraySpec(command.Array); err != nil {
				return fmt.Errorf("job %q has invalid array: %w", command.ID, err)
			}
		}
		jobIDs := []string{command.ID}
		if command.Array != nil {
			jobIDs = make([]string, 0, len(model.ArrayTaskIDs(command.Array)))
			for _, task := range model.ArrayTaskIDs(command.Array) {
				jobIDs = append(jobIDs, fmt.Sprintf("%s-%d", command.ID, task))
			}
		}
		for _, jobID := range jobIDs {
			if expandedIDs[jobID] {
				return fmt.Errorf("duplicate job ID %q", jobID)
			}
			expandedIDs[jobID] = true
		}
	}
	return nil
}

func formatRunLabel(runID, runName string) string {
	if runName == "" {
		return runID
	}
	return fmt.Sprintf("%s (%s)", runName, runID)
}

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	cliConfigCommand = args[0]
	isConfigCommand := args[0] == "config" || (args[0] == "run" && len(args) > 1 && args[1] == "config")
	if !isConfigCommand && args[0] != "schema" && args[0] != "guide" && args[0] != "--version" && args[0] != "version" {
		if err := loadCLIConfig(args[1:]); err != nil {
			printErrorf("failed to load config: %v", err)
			return 1
		}
	}
	if args[0] == "--version" || args[0] == "version" {
		printVersion()
		return 0
	}

	switch args[0] {
	case "config":
		return cmdConfig(args[1:])
	case "check":
		return cmdCheck(args[1:])
	case "reset":
		return cmdReset(args[1:])
	case "cancel":
		return cmdCancel(args[1:])
	case "suspend":
		return cmdJobSignal(args[1:], "suspend")
	case "resume":
		return cmdJobSignal(args[1:], "resume")
	case "delete":
		return cmdDelete(args[1:])
	case "gc":
		return cmdGC(args[1:])
	case "unlock":
		return cmdUnlock(args[1:])
	case "change":
		return cmdChange(args[1:])
	case "export":
		return cmdExport(args[1:])
	case "import":
		return cmdImport(args[1:])
	case "remove":
		return cmdRemove(args[1:])
	case "show":
		return cmdShow(args[1:])
	case "jobs":
		return cmdJobs(args[1:])
	case "diagnose":
		return cmdDiagnose(args[1:])
	case "wait":
		return cmdWait(args[1:])
	case "run":
		if len(args) > 1 && args[1] == "config" {
			return cmdConfig(args[2:])
		}
		return cmdRun(args[1:])
	case "retry":
		return cmdRetry(args[1:])
	case "add":
		return cmdAdd(args[1:])
	case "copy":
		return cmdCopy(args[1:])
	case "server":
		return cmdServer(args[1:])
	case "web":
		return cmdWeb(args[1:])
	case "env":
		return cmdEnvironment(args[1:])
	case "completion":
		return cmdCompletion(args[1:])
	case "schema":
		return cmdSchema(args[1:])
	case "guide":
		return cmdGuide(args[1:])
	case "__complete":
		return cmdComplete(args[1:])
	case "__server":
		return cmdServerProcess(args[1:])
	case "__worker-run":
		return cmdWorkerRun(args[1:])
	default:
		printErrorf("unknown subcommand: %s", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Println("rotari: lightweight local job queue")
	fmt.Println("")
	fmt.Println("Usage:")
	for _, command := range cliCommandSpecs {
		fmt.Printf("  %s\n", cliUsage(command.Name))
	}
}

func recoverInterruptedProject(paths state.ProjectPaths, runID string, discardQueue bool) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	projectState, currentRunID, err := inspectConsistentProjectRunState(paths, true)
	if err != nil {
		return err
	}
	if projectState != projectInterrupted || currentRunID != runID {
		return fmt.Errorf("project %q no longer has interrupted run %q", paths.ProjectName, runID)
	}
	if discardQueue {
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			return err
		}
		queue.Commands = nil
		queue.WorkflowImport = false
		if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
			return err
		}
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return err
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	return state.WriteJSON(paths.MetaFile, meta)
}

func formatProjectRunningError(paths state.ProjectPaths, runID string) string {
	baseDir := paths.BaseDir
	projectName := paths.ProjectName
	return fmt.Sprintf("%s\n  Run: %s\n\nWait for completion:\n  rotari wait --basedir %s --project-name %s --run-id %s\n\nCancel run:\n  rotari cancel --basedir %s --project-name %s\n",
		redError(fmt.Sprintf("project '%s' is running; new jobs are not allowed", projectName)),
		runID, baseDir, projectName, runID, baseDir, projectName)
}

const cancellationWaitTimeout = 5 * time.Minute

func waitForCancellation(paths state.ProjectPaths, projectName string) bool {
	fmt.Println(yellow(fmt.Sprintf("project '%s' is cancelling", projectName)))
	fmt.Println(yellow("Waiting for cancellation to finish..."))
	deadline := time.Now().Add(cancellationWaitTimeout)
	for {
		if finalized, err := finalizeCompletedCancellation(paths); err != nil {
			printErrorf("failed to finalize cancellation: %v", err)
			return false
		} else if finalized {
			fmt.Println(green("Cancellation complete"))
			return true
		}
		running, err := isRunning(paths.LockFile)
		if err != nil {
			printErrorf("failed to check queue: %v", err)
			return false
		}
		if !running {
			fmt.Println(green("Cancellation complete"))
			return true
		}
		if time.Now().After(deadline) {
			printError("cancellation is still in progress")
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func finalizeCompletedCancellation(paths state.ProjectPaths) (bool, error) {
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	runDir, pathErr := state.SafeJoin(paths.RunsDir, lock.RunID)
	if pathErr != nil {
		return false, pathErr
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil || summary.FinishedAt == "" {
		return false, nil
	}
	if err := finishRun(paths, lock.RunID, summary.ExitCode); err != nil {
		return false, err
	}
	if err := os.Remove(paths.LockFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

// cmdWorkerRun executes a single scheduler-dispatched job attempt inside an
// existing run directory.
func cmdWorkerRun(args []string) int {
	options, err := parseWorkerRunArgs(args)
	if err != nil {
		printError(err)
		return 1
	}
	queueName, runID, runName, cwd := options.QueueName, options.RunID, options.RunName, options.CWD

	paths, err := state.ResolveProjectPaths(options.BaseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	exitCode, workerErr := runcontract.RunWorker(runcontract.WorkerCallbacks{
		WriteContext: func() error { return writeRunContext(paths, runID, cwd) },
		MarkRunning: func() error {
			meta, _ := state.LoadMeta(paths.MetaFile)
			meta.Phase = "running"
			meta.LastRunID = runID
			meta.UpdatedAt = nowRFC3339()
			return state.WriteJSON(paths.MetaFile, meta)
		},
		StartSampling: func() func() { return startRunLoadSampling(paths, runID) },
		Execute: func() int {
			return executeMixedRun(paths, runID, runName, options.LocalConcurrency, options.BatchMaxActive, options.Retry, options.Executor, options.ExecutorOptions, options.Selection, options.JobIDs, options.SourceRunID, options.PartialArray, nil, nil, options.ExecutorSettings)
		},
		FinishContext: func() error { return finishRunContext(paths, runID) },
		Finalize:      func(exitCode int) error { return finishRun(paths, runID, exitCode) },
		RemoveLock: func() error {
			err := os.Remove(paths.LockFile)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		},
	})
	if workerErr != nil {
		printErrorf("worker run failed: %v", workerErr)
		return 1
	}
	return exitCode
}

// parseWorkerRunArgs parses the arguments that runcontract.WorkerArgs builds
// for __worker-run.
func parseWorkerRunArgs(args []string) (runcontract.Options, error) {
	fs := flag.NewFlagSet("__worker-run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	basedir := cliString(fs, "basedir", "")
	executor := cliString(fs, "executor", "")
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	executorSettings := cliExecutorRunSettings(fs)
	// selection and source-run-id exist only on this internal command, so they
	// have no CLI metadata, environment, or config defaults.
	selection := fs.String("selection", "", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	sourceRunID := fs.String("source-run-id", "", "")
	partialArray := cliBool(fs, "partial-array", true)
	if err := fs.Parse(args); err != nil {
		return runcontract.Options{}, fmt.Errorf("failed to parse worker args: %w", err)
	}
	left := fs.Args()
	if len(left) != 7 {
		return runcontract.Options{}, errors.New("usage: rotari __worker-run [--basedir DIR] <project_name> <run_id> <run_name> <local_concurrency> <batch_max_active> <retry> <cwd>")
	}
	localConcurrency, err := strconv.Atoi(left[3])
	batchMaxActive, batchErr := strconv.Atoi(left[4])
	retry, retryErr := strconv.Atoi(left[5])
	if err != nil || batchErr != nil || retryErr != nil || localConcurrency < 1 || batchMaxActive < 1 || retry < -1 {
		return runcontract.Options{}, fmt.Errorf("invalid run options: local=%s batch=%s retry=%s", left[3], left[4], left[5])
	}
	return runcontract.Options{
		BaseDir: *basedir, QueueName: left[0], RunID: left[1], RunName: left[2],
		LocalConcurrency: localConcurrency, BatchMaxActive: batchMaxActive, Retry: retry,
		Executor: *executor, ExecutorOptions: executorOptions, Selection: *selection, JobIDs: jobIDs,
		SourceRunID: *sourceRunID, PartialArray: *partialArray, CWD: left[6], ExecutorSettings: executorSettings,
	}, nil
}

func finishRun(paths state.ProjectPaths, runID string, exitCode int) error {
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()

	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		return fmt.Errorf("failed to verify run lock: %w", err)
	}
	if lock.RunID != runID {
		return fmt.Errorf("run lock belongs to %q, not %q", lock.RunID, runID)
	}

	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	queue, meta, err = state.FinalizeRun(queue, meta, runID, exitCode, time.Now())
	if err != nil {
		return err
	}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		return fmt.Errorf("failed to clear queue: %w", err)
	}
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return fmt.Errorf("failed to finalize metadata: %w", err)
	}
	notifyRunWebhook(paths, runID, exitCode)
	return nil
}

func launchAsyncRun(paths state.ProjectPaths, options runcontract.Options) int {
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: options.RunID, RunName: options.RunName, StartedAt: nowRFC3339()}); err != nil {
		printErrorf("project '%s' is running; run is not allowed: %v", options.QueueName, err)
		return 1
	}

	meta, _ := state.LoadMeta(paths.MetaFile)
	meta.Phase = "running"
	meta.LastRunID = options.RunID
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		_ = os.Remove(paths.LockFile)
		printErrorf("failed to update metadata: %v", err)
		return 1
	}
	if err := writeRunContext(paths, options.RunID, options.CWD); err != nil {
		_ = os.Remove(paths.LockFile)
		printErrorf("failed to save run context: %v", err)
		return 1
	}
	if err := registerRun(paths, options.RunID); err != nil {
		_ = os.Remove(paths.LockFile)
		if runDir, pathErr := state.SafeJoin(paths.RunsDir, options.RunID); pathErr == nil {
			_ = os.RemoveAll(runDir)
		}
		printErrorf("failed to register run: %v", err)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		_ = os.Remove(paths.LockFile)
		printErrorf("failed to detect executable path: %v", err)
		return 1
	}

	childOptions := options
	childOptions.BaseDir = paths.BaseDir
	childArgs := runcontract.WorkerArgs(childOptions, paths.BaseDirExplicit, executorRunSettingNames)

	cmd := exec.Command(exe, childArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(paths.LockFile)
		printErrorf("failed to launch async runner: %v", err)
		return 1
	}

	host, err := os.Hostname()
	if err != nil {
		_ = cmd.Process.Kill()
		_ = os.Remove(paths.LockFile)
		printErrorf("failed to determine lock host: %v", err)
		return 1
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: cmd.Process.Pid, RunID: options.RunID, RunName: options.RunName, StartedAt: nowRFC3339(), Host: host}); err != nil {
		_ = cmd.Process.Kill()
		_ = os.Remove(paths.LockFile)
		printErrorf("failed to update lock with child pid: %v", err)
		return 1
	}
	waitForAsyncRun(cmd, options.OnDone)

	fmt.Printf("submitted project=%s run_id=%s pid=%d\n", options.QueueName, options.RunID, cmd.Process.Pid)
	return 0
}

func waitForAsyncRun(cmd *exec.Cmd, onDone func()) {
	go func() {
		_ = cmd.Wait()
		if onDone != nil {
			onDone()
		}
	}()
}

func printFailedJobHints(runID string, results []model.JobResult) {
	if hints := failedJobHints(runID, results); hints != "" {
		fmt.Print(hints)
	}
}

func failedJobHints(runID string, results []model.JobResult) string {
	var hints strings.Builder
	seen := make(map[string]bool)
	for _, result := range results {
		if result.ExitCode == 0 || seen[result.ID] {
			continue
		}
		seen[result.ID] = true
		hosts := strings.Join(result.Hosts, ",")
		if hosts == "" {
			hosts = "-"
		}
		if hints.Len() > 0 {
			hints.WriteString("  ----\n")
		}
		attemptID := result.AttemptID
		if attemptID == "" {
			attemptID = result.ID
		}
		fmt.Fprintf(&hints, "  Job: %s\n  Attempt ID: %s\n  Hosts: %s\n  Command: %s\n  Show output:\n    rotari show --run-id %s --job-id %s\n",
			result.ID, result.AttemptID, hosts, strings.Join(result.Command, " "), runID, attemptID)
	}
	return hints.String()
}

func runOneJob(runDir string, job model.JobSpec) model.JobResult {
	return executor.RunLocalJob(runDir, model.JobSpec(job), jsonStore(), jobLogf)
}

func jobCancellationRequested(jobDir string) bool {
	_, err := os.Stat(filepath.Join(jobDir, stateFileCancelled))
	return err == nil
}

func recordCancelledJob(jobDir string, job model.JobSpec) model.JobResult {
	return executor.RecordCancelledJob(jobDir, model.JobSpec(job), jsonStore())
}

const (
	stateFileCommandsJSON  = "commands.json"
	stateFileSummaryJSON   = "summary.json"
	stateFileOutput        = "output"
	stateFileSchedulerJSON = "scheduler_status.json"
	stateFileStatusJSON    = "status.json"
	stateFileStatus        = "status"
	stateFileSubmittedAt   = "submitted_at"
	stateFileFinishedAt    = "finished_at"
	stateFileJobJSON       = "job.json"
	stateFilePID           = "pid"
	stateFileCancelled     = "cancelled"
	stateFileName          = "name"
)

func isRunning(lockPath string) (bool, error) {
	lockState, _, err := state.InspectLock(lockPath, true)
	return lockState == state.LockActive || lockState == state.LockRemote, err
}

func makeRunID() string {
	var value [4]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Sprintf("failed to generate run id: %v", err))
	}
	timestamp := time.Now().UTC().Format("20060102-150405")
	return fmt.Sprintf("%s-%08x", timestamp, value)
}

func makeJobID() string {
	var value [5]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Sprintf("failed to generate job id: %v", err))
	}
	return hex.EncodeToString(value[:])[:jobIDLen]
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func nowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func jsonStore() state.Store {
	return state.NewStore(state.DirectoryMode(), state.FileMode())
}

func loadWrapperStatus(path string) (executor.WrapperStatus, bool) {
	return executor.LoadWrapperStatus(jsonStore(), path)
}

func loadRunQueue(paths state.ProjectPaths, requestedExecutor string, executorOptions []string, settings executor.RunSettingsMap) (model.Queue, error) {
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return model.Queue{}, err
	}
	if err := validateQueueForRun(queue, requestedExecutor, executorOptions, settings); err != nil {
		return model.Queue{}, err
	}
	return queue, nil
}

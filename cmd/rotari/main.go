package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type Queue = model.Queue
type QueuedCommand = model.QueuedCommand
type ArraySpec = model.ArraySpec
type JobOrigin = model.JobOrigin

type Meta = model.Meta
type LockInfo = model.LockInfo
type JobSpec = model.JobSpec
type JobResult = model.JobResult
type RunSummary = model.RunSummary
type RunContext = model.RunContext
type LoadAverage = model.LoadAverage
type LoadSample = model.LoadSample
type ruleDiagnosis = model.RuleDiagnosis

func runStatus(exitCode int) string {
	return model.RunStatus(exitCode)
}

func parseArrayRange(value string) (ArraySpec, error) {
	return model.ParseArrayRange(value)
}

func arrayTaskIDs(array *ArraySpec) []int {
	return model.ArrayTaskIDs(array)
}

func validateQueueJobs(queue Queue) error {
	expandedIDs := make(map[string]bool)
	for _, command := range queue.Commands {
		if !isValidPathElement(command.ID) {
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
		if err := validateEnvironment(command.Environment); err != nil {
			return fmt.Errorf("job %q has invalid environment: %w", command.ID, err)
		}
		if strings.ContainsRune(command.WorkingDirectory, '\x00') {
			return fmt.Errorf("job %q working directory contains a NUL byte", command.ID)
		}
		if command.Array != nil {
			if err := validateArraySpec(command.Array); err != nil {
				return fmt.Errorf("job %q has invalid array: %w", command.ID, err)
			}
		}
		jobIDs := []string{command.ID}
		if command.Array != nil {
			jobIDs = make([]string, 0, len(arrayTaskIDs(command.Array)))
			for _, task := range arrayTaskIDs(command.Array) {
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

func validateArraySpec(array *ArraySpec) error {
	return model.ValidateArraySpec(array)
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
	if !isConfigCommand && args[0] != "schema" && args[0] != "--version" && args[0] != "version" {
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

func recoverInterruptedProject(paths pathSet, runID string, discardQueue bool) error {
	release, err := acquireStateLock(paths.stateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	state, currentRunID, err := inspectConsistentProjectRunState(paths, true)
	if err != nil {
		return err
	}
	if state != projectInterrupted || currentRunID != runID {
		return fmt.Errorf("project %q no longer has interrupted run %q", paths.queueName, runID)
	}
	if discardQueue {
		queue, err := loadQueue(paths.queueFile)
		if err != nil {
			return err
		}
		queue.Commands = nil
		if err := writeJSON(paths.queueFile, queue); err != nil {
			return err
		}
	}
	meta, err := loadMeta(paths.metaFile)
	if err != nil {
		return err
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	return writeJSON(paths.metaFile, meta)
}

func formatProjectRunningError(paths pathSet, runID string) string {
	return fmt.Sprintf("%s\n  Run: %s\n\nWait for completion:\n  rotari wait --basedir %s --project-name %s --run-id %s\n\nCancel run:\n  rotari cancel --basedir %s --project-name %s\n",
		redError(fmt.Sprintf("project '%s' is running; new jobs are not allowed", paths.queueName)),
		runID, paths.baseDir, paths.queueName, runID, paths.baseDir, paths.queueName)
}

const cancellationWaitTimeout = 5 * time.Minute

func waitForCancellation(paths pathSet, projectName string) bool {
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
		running, err := isRunning(paths.lockFile)
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

func finalizeCompletedCancellation(paths pathSet) (bool, error) {
	lock, err := loadLockInfo(paths.lockFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	runDir, pathErr := validatedRunDir(paths, lock.RunID)
	if pathErr != nil {
		return false, pathErr
	}
	summary, err := loadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil || summary.FinishedAt == "" {
		return false, nil
	}
	if err := finishRun(paths, lock.RunID, summary.ExitCode); err != nil {
		return false, err
	}
	if err := os.Remove(paths.lockFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, nil
}

func cmdWorkerRun(args []string) int {
	fs := flag.NewFlagSet("__worker-run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	basedir := cliString(fs, "basedir", "")
	executor := cliString(fs, "executor", "")
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	executorSettings := cliExecutorRunSettings(fs)
	selection := cliString(fs, "selection", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	sourceRunID := cliString(fs, "source-run-id", "")
	partialArray := cliBool(fs, "partial-array", true)
	if err := fs.Parse(args); err != nil {
		printErrorf("failed to parse worker args: %v", err)
		return 1
	}
	left := fs.Args()
	if len(left) != 7 {
		printError("usage: rotari __worker-run [--basedir DIR] <project_name> <run_id> <run_name> <local_concurrency> <batch_max_active> <retry> <cwd>")
		return 1
	}
	queueName := left[0]
	runID := left[1]
	runName := left[2]
	localConcurrency, err := strconv.Atoi(left[3])
	batchMaxActive, batchErr := strconv.Atoi(left[4])
	retry, retryErr := strconv.Atoi(left[5])
	cwd := left[6]
	if err != nil || batchErr != nil || retryErr != nil || localConcurrency < 1 || batchMaxActive < 1 || retry < -1 {
		printErrorf("invalid run options: local=%s batch=%s retry=%s", left[2], left[3], left[4])
		return 1
	}

	paths, err := resolvePaths(*basedir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	exitCode, workerErr := runcontract.RunWorker(runcontract.WorkerCallbacks{
		WriteContext: func() error { return writeRunContext(paths, runID, cwd) },
		MarkRunning: func() error {
			meta, _ := loadMeta(paths.metaFile)
			meta.Phase = "running"
			meta.LastRunID = runID
			meta.UpdatedAt = nowRFC3339()
			return writeJSON(paths.metaFile, meta)
		},
		StartSampling: func() func() { return startRunLoadSampling(paths, runID) },
		Execute: func() int {
			return executeMixedRun(paths, runID, runName, localConcurrency, batchMaxActive, retry, *executor, executorOptions, *selection, jobIDs, *sourceRunID, *partialArray, nil, nil, executorSettings)
		},
		FinishContext: func() error { return finishRunContext(paths, runID) },
		Finalize:      func(exitCode int) error { return finishRun(paths, runID, exitCode) },
		RemoveLock: func() error {
			err := os.Remove(paths.lockFile)
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

func finishRun(paths pathSet, runID string, exitCode int) error {
	release, err := acquireStateLock(paths.stateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()

	lock, err := loadLockInfo(paths.lockFile)
	if err != nil {
		return fmt.Errorf("failed to verify run lock: %w", err)
	}
	if lock.RunID != runID {
		return fmt.Errorf("run lock belongs to %q, not %q", lock.RunID, runID)
	}

	queue, err := loadQueue(paths.queueFile)
	if err != nil {
		return fmt.Errorf("failed to load queue: %w", err)
	}
	meta, err := loadMeta(paths.metaFile)
	if err != nil {
		return fmt.Errorf("failed to load metadata: %w", err)
	}
	queue, meta, err = state.FinalizeRun(queue, meta, runID, exitCode, time.Now())
	if err != nil {
		return err
	}
	if err := writeJSON(paths.queueFile, queue); err != nil {
		return fmt.Errorf("failed to clear queue: %w", err)
	}
	if err := writeJSON(paths.metaFile, meta); err != nil {
		return fmt.Errorf("failed to finalize metadata: %w", err)
	}
	notifyRunWebhook(paths, runID, exitCode)
	return nil
}

func launchAsyncRun(paths pathSet, options runOptions) int {
	if err := acquireLock(paths.lockFile, LockInfo{PID: os.Getpid(), RunID: options.RunID, RunName: options.RunName, StartedAt: nowRFC3339()}); err != nil {
		printErrorf("project '%s' is running; run is not allowed: %v", options.QueueName, err)
		return 1
	}

	meta, _ := loadMeta(paths.metaFile)
	meta.Phase = "running"
	meta.LastRunID = options.RunID
	meta.UpdatedAt = nowRFC3339()
	if err := writeJSON(paths.metaFile, meta); err != nil {
		_ = os.Remove(paths.lockFile)
		printErrorf("failed to update metadata: %v", err)
		return 1
	}
	if err := writeRunContext(paths, options.RunID, options.CWD); err != nil {
		_ = os.Remove(paths.lockFile)
		printErrorf("failed to save run context: %v", err)
		return 1
	}
	if err := registerRun(paths, options.RunID); err != nil {
		_ = os.Remove(paths.lockFile)
		if runDir, pathErr := validatedRunDir(paths, options.RunID); pathErr == nil {
			_ = os.RemoveAll(runDir)
		}
		printErrorf("failed to register run: %v", err)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		_ = os.Remove(paths.lockFile)
		printErrorf("failed to detect executable path: %v", err)
		return 1
	}

	childOptions := options
	childOptions.BaseDir = paths.baseDir
	childArgs := runcontract.WorkerArgs(childOptions, paths.baseDirExplicit, executorRunSettingNames)

	cmd := exec.Command(exe, childArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(paths.lockFile)
		printErrorf("failed to launch async runner: %v", err)
		return 1
	}

	host, err := os.Hostname()
	if err != nil {
		_ = cmd.Process.Kill()
		_ = os.Remove(paths.lockFile)
		printErrorf("failed to determine lock host: %v", err)
		return 1
	}
	if err := writeJSON(paths.lockFile, LockInfo{PID: cmd.Process.Pid, RunID: options.RunID, RunName: options.RunName, StartedAt: nowRFC3339(), Host: host}); err != nil {
		_ = cmd.Process.Kill()
		_ = os.Remove(paths.lockFile)
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

func countRunResults(results []JobResult) (successCount, failedCount int) {
	return model.CountRunResults(results)
}

func printFailedJobHints(runID string, results []JobResult) {
	if hints := failedJobHints(runID, results); hints != "" {
		fmt.Print(hints)
	}
}

func failedJobHints(runID string, results []JobResult) string {
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

func runOneJob(runDir string, job JobSpec) JobResult {
	return executor.RunLocalJob(runDir, model.JobSpec(job), jsonStore(), jobLogf)
}

func jobCancellationRequested(jobDir string) bool {
	_, err := os.Stat(filepath.Join(jobDir, stateFileCancelled))
	return err == nil
}

func recordCancelledJob(jobDir string, job JobSpec) JobResult {
	return executor.RecordCancelledJob(jobDir, model.JobSpec(job), jsonStore())
}

func queueToJobs(commands []QueuedCommand) []JobSpec {
	return model.QueueToJobs(commands)
}

type pathSet struct {
	baseDir         string
	baseDirExplicit bool
	queueName       string
	projectDir      string
	queueFile       string
	metaFile        string
	stateLockFile   string
	lockFile        string
	runsDir         string
}

const (
	cliProjectNameFlag      = "project-name"
	cliRunIDFlag            = "run-id"
	cliJobIDFlag            = "job-id"
	headerContentType       = "Content-Type"
	mimeApplicationJSON     = "application/json"
	authBearerPrefix        = "Bearer "
	redactedPathPlaceholder = "[REDACTED_PATH]"
	commandJSONName         = "command.json"
	stateFileCommandsJSON   = "commands.json"
	stateFileSummaryJSON    = "summary.json"
	stateFileContextJSON    = "context.json"
	stateFileOutput         = "output"
	stateFileSchedulerJSON  = "scheduler_status.json"
	stateFileStatusJSON     = "status.json"
	stateFileStatus         = "status"
	stateFileSubmittedAt    = "submitted_at"
	stateFileFinishedAt     = "finished_at"
	stateFileJobJSON        = "job.json"
	stateFilePID            = "pid"
	stateFileCancelled      = "cancelled"
	stateFileName           = "name"
)

func resolvePaths(cliBaseDir, projectName string) (pathSet, error) {
	if !isValidProjectName(projectName) {
		return pathSet{}, fmt.Errorf("invalid project name %q", projectName)
	}
	baseDir, explicit, err := resolveBaseDir(cliBaseDir)
	if err != nil {
		return pathSet{}, err
	}
	projectDir := filepath.Join(baseDir, "projects", projectName)
	return pathSet{
		baseDir:         baseDir,
		baseDirExplicit: explicit,
		queueName:       projectName,
		projectDir:      projectDir,
		queueFile:       filepath.Join(projectDir, "queue.json"),
		metaFile:        filepath.Join(projectDir, "meta.json"),
		stateLockFile:   filepath.Join(projectDir, "state.lock"),
		lockFile:        filepath.Join(projectDir, "running.lock"),
		runsDir:         filepath.Join(projectDir, "runs"),
	}, nil
}

func resolveBaseDir(cliBaseDir string) (string, bool, error) {
	if cliBaseDir != "" {
		return cliBaseDir, true, nil
	}
	if v := os.Getenv(envBaseDir); v != "" {
		return v, true, nil
	}
	if cwd, err := os.Getwd(); err == nil {
		localState := filepath.Join(cwd, ".rotari-state")
		if info, err := os.Stat(localState); err == nil && info.IsDir() {
			return localState, false, nil
		}
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "rotari"), false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(home, ".local", "state", "rotari"), false, nil
}

// privateStateEnabled controls whether the state directory tree (queues,
// runs, job output, locks) is created owner-only (0700/0600) instead of the
// default shared (0755/0644) permissions. Shared is the default because
// rotari is commonly used on shared HPC/lab filesystems where colleagues
// point each other at a job's log path.
func privateStateEnabled() bool {
	value, _ := strconv.ParseBool(os.Getenv(envPrivateState))
	return value
}

func stateDirMode() os.FileMode {
	return stateMode(0o700, 0o755)
}

func stateFileMode() os.FileMode {
	return stateMode(0o600, 0o644)
}

// stateScriptMode is for generated wrapper scripts, which must stay executable.
func stateScriptMode() os.FileMode {
	return stateMode(0o700, 0o755)
}

func stateMode(privateMode, sharedMode os.FileMode) os.FileMode {
	if privateStateEnabled() {
		return privateMode
	}
	return sharedMode
}

func resolveProjectName(baseDir string, cliProjectName string) (string, error) {
	if cliProjectName != "" {
		if !isValidProjectName(cliProjectName) {
			return "", fmt.Errorf("invalid project name %q", cliProjectName)
		}
		return cliProjectName, nil
	}
	if value := os.Getenv(envProjectName); value != "" {
		if !isValidProjectName(value) {
			return "", fmt.Errorf("invalid project name %q", value)
		}
		return value, nil
	}
	projectsDir := filepath.Join(baseDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err == nil {
		var available []string
		for _, entry := range entries {
			if entry.IsDir() {
				available = append(available, entry.Name())
			}
		}
		if len(available) == 1 {
			return available[0], nil
		}
		if len(available) > 1 {
			sort.Strings(available)
			var list []string
			for _, q := range available {
				list = append(list, "  - "+q)
			}
			return "", fmt.Errorf("multiple projects exist in state directory %q; please specify one with --project-name or ROTARI_PROJECT_NAME:\n%s", baseDir, strings.Join(list, "\n"))
		}
	}
	return defaultProjectName, nil
}

func isRunning(lockPath string) (bool, error) {
	state, _, err := inspectRunLock(lockPath, true)
	return state == projectLockActive || state == projectLockRemote, err
}

type projectLockState string

const (
	projectLockNone   projectLockState = "none"
	projectLockActive projectLockState = "active"
	projectLockStale  projectLockState = "stale"
	projectLockRemote projectLockState = "remote"
)

func inspectRunLock(lockPath string, cleanupStale bool) (projectLockState, LockInfo, error) {
	lock, err := loadLockInfo(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return projectLockNone, LockInfo{}, nil
		}
		return projectLockNone, LockInfo{}, fmt.Errorf("read run lock: %w", err)
	}

	localHost, err := os.Hostname()
	if err != nil {
		return projectLockNone, LockInfo{}, fmt.Errorf("determine local host: %w", err)
	}
	if lock.Host == "" || lock.Host != localHost {
		return projectLockRemote, lock, nil
	}
	if lock.PID > 0 && processAlive(lock.PID) {
		return projectLockActive, lock, nil
	}
	if cleanupStale {
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) { // NOSONAR: lockPath is the resolved state lock.
			return projectLockStale, lock, err
		}
	}
	return projectLockStale, lock, nil
}

func loadLockInfo(lockPath string) (LockInfo, error) {
	b, err := os.ReadFile(lockPath) // NOSONAR: lockPath is the resolved state lock.
	if err != nil {
		return LockInfo{}, err
	}
	var lock LockInfo
	if err := json.Unmarshal(b, &lock); err != nil {
		return LockInfo{}, err
	}
	return lock, nil
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
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

func formatDisplayTimestamp(value string) string {
	if value == "" || value == "-" {
		return value
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return timestamp.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02 15:04:05 JST")
}

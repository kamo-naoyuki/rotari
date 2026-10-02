package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const jobIDLen = 9
const runIDLen = len("20060102-150405-00000000")
const commandJSONName = "command.json"
const mimeApplicationJSON = "application/json"

func main() {
	code := run(os.Args[1:])
	os.Exit(code)
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	if args[0] == "__log-forward" {
		return executor.RunLogForward(args[1:])
	}
	if isHelpArgument(args[0]) || args[0] == "help" {
		printUsage()
		return 0
	}
	cliConfigCommand = args[0]
	if args[0] != "config" && args[0] != "schema" && args[0] != "guide" && args[0] != "--version" && args[0] != "version" {
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
	case "lineage":
		return cmdLineage(args[1:])
	case "jobs":
		return cmdJobs(args[1:])
	case "diagnose":
		return cmdDiagnose(args[1:])
	case "wait":
		return cmdWait(args[1:])
	case "run":
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
	case "mcp":
		return cmdMCP(args[1:])
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
	default:
		if suggestion := cliSimilarCommand(args[0]); suggestion != "" {
			printErrorf("unknown subcommand: %s; did you mean %s?", args[0], suggestion)
			printError("usage: " + cliUsage(suggestion))
			return 1
		}
		printErrorf("unknown subcommand: %s", args[0])
		printError("available subcommands: " + strings.Join(cliCommandNames(), ", "))
		return 1
	}
}

func printUsage() {
	fmt.Println("rotari: lightweight local job queue")
	fmt.Println("")
	fmt.Println("Coding agents: run `rotari guide` first for the recommended workflow and a command reference.")
	fmt.Println("")
	fmt.Println("Usage:")
	for _, command := range cliCommandSpecs {
		fmt.Printf("  %s\n", cliUsage(command.Name))
	}
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

func runOneJob(runDir string, job model.JobSpec) model.JobResult {
	return executor.RunLocalJob(runDir, model.JobSpec(job), jsonStore(), jobLogf)
}

const (
	stateFileCommandsJSON  = "commands.json"
	stateFileSummaryJSON   = "summary.json"
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

func jobController() jobcontrol.Controller {
	return jobcontrol.Controller{Store: jsonStore(), Executors: executorRegistry}
}

func queueEditor() queueops.Editor {
	return queueops.Editor{Store: jsonStore(), Executors: executorRegistry, NewJobID: makeJobID, UnregisterRun: unregisterRun}
}

func loadWrapperStatus(path string) (executor.WrapperStatus, bool) {
	return executor.LoadWrapperStatus(jsonStore(), path)
}

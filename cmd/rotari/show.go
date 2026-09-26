package main

import (
	"bytes"
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
	"time"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	pagerLineLimit     = 24
	statusJSONName     = "status.json"
	runNotFoundMessage = "run %q not found"
	jobNotFoundMessage = "job %q not found in run %q"
)

func resolveShowJobTargets(cliBaseDir, cliProjectName, runID, selector string, byName bool) ([]resolve.Job, error) {
	if runID != "" {
		baseDir, projectName, err := resolve.ExistingRun(cliBaseDir, cliProjectName, runID)
		if err != nil {
			return nil, err
		}
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			return nil, err
		}
		target, found, err := resolve.JobInRun(paths, runID, selector, byName)
		if err != nil || !found {
			return nil, err
		}
		return []resolve.Job{target}, nil
	}
	return resolve.Jobs(cliBaseDir, cliProjectName, selector, byName, true)
}

func resolveShowSelector(cliBaseDir, cliProjectName, selector string) ([]resolve.Job, error) {
	runTargets, err := resolve.RunsByName(cliBaseDir, cliProjectName, selector, false)
	if err != nil {
		return nil, err
	}
	targets := make([]resolve.Job, 0, len(runTargets))
	for _, target := range runTargets {
		targets = append(targets, resolve.Job{Run: target})
	}

	for _, byName := range []bool{false, true} {
		jobTargets, err := resolveShowJobTargets(cliBaseDir, cliProjectName, "", selector, byName)
		if err != nil {
			return nil, err
		}
		for _, target := range jobTargets {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func showJobNameTargets(cliBaseDir, cliProjectName, runID, jobName string) ([]resolve.Job, error) {
	return resolveShowJobTargets(cliBaseDir, cliProjectName, runID, jobName, true)
}

func applyShowSelectorTarget(target resolve.Job, basedir, projectName, runID, jobID *string, showQueue *bool) {
	*basedir, *projectName, *runID, *jobID = target.BaseDir, target.ProjectName, target.RunID, target.JobID
	if target.FromQueue {
		*showQueue = true
	}
}

// cmdShow displays project, queue, run, job, log, report, and registry views.
func cmdShow(args []string) int {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	showQueueOption := cliBool(fs, "queue", false)
	jobIDOption := cliString(fs, "job-id", "")
	jobNameOption := cliString(fs, "job-name", "")
	failedOnly := cliBool(fs, "failed", false)
	unfinishedOnly := cliBool(fs, "unfinished", false)
	successOnly := cliBool(fs, "success", false)
	stageOption := cliString(fs, "stage", "")
	matrixOption := cliString(fs, "matrix", "")
	lineage := cliBool(fs, "lineage", false)
	showBaseDirsList := cliBool(fs, "basedirs", false)
	masterdir := cliString(fs, "masterdir", "")
	showLogs := cliBool(fs, "logs", false)
	showFailedLogs := cliBool(fs, "failed-logs", false)
	followLogs := cliBool(fs, "follow", false)
	noPager := cliBool(fs, "no-pager", false)
	jsonOutput := cliBool(fs, "json", false)
	reportOutput := cliBool(fs, "report", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	scope := model.CommandSelector{Stage: *stageOption, Matrix: *matrixOption}
	// resultSelection filters the job table by result, like copy and run;
	// logs and reports only know --failed.
	resultSelection := model.ResultSelection(*failedOnly, *unfinishedOnly, *successOnly)
	resultFilter := resultSelection != ""
	if (*unfinishedOnly || *successOnly) && (*showLogs || *showFailedLogs || *followLogs || *reportOutput || *jsonOutput) {
		printError("--unfinished and --success filter the job table; logs, reports, and JSON take --failed only")
		return 1
	}
	if scope.Kinds() > 1 {
		printError("--stage cannot be combined with --matrix")
		return 1
	}
	if len(fs.Args()) > 1 {
		printError("usage: " + cliUsage("show"))
		return 1
	}
	selector := ""
	if len(fs.Args()) == 1 {
		selector = fs.Args()[0]
		if *runIDOption != "" || *jobIDOption != "" || *jobNameOption != "" || *showQueueOption || *showBaseDirsList || *showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput {
			printError("a run name selector cannot be combined with run, job, queue, list, log, follow, JSON, or report options")
			return 1
		}
	}
	if *jobIDOption != "" && *jobNameOption != "" {
		printError("--job-id cannot be combined with --job-name")
		return 1
	}
	attemptID := ""
	if selector != "" && strings.HasPrefix(selector, "att_") {
		attemptID = selector
		baseDir, projectName, resolvedRunID, resolvedJobID, err := resolve.Attempt(selector, *basedir, *queueNameOption, "")
		if err != nil {
			printError(err)
			return 1
		}
		*basedir, *queueNameOption, *runIDOption, *jobIDOption = baseDir, projectName, resolvedRunID, resolvedJobID
		selector = ""
	}
	if selector != "" {
		if location, found, err := resolveRunLocation(selector); err != nil {
			printError(err)
			return 1
		} else if found {
			baseDir, projectName, err := resolve.ExistingRun(*basedir, *queueNameOption, selector)
			if err != nil {
				printError(err)
				return 1
			}
			*basedir, *queueNameOption, *runIDOption = baseDir, projectName, location.RunID
			selector = ""
		}
	}
	if selector == model.Latest {
		*runIDOption, selector = model.Latest, ""
	}
	if selector != "" && !cliOptionSet(fs, "project-name") {
		// A project name comes next, as in wait: "show sweep" is
		// "show -p sweep".
		baseDir, _, err := state.ResolveBaseDir(*basedir)
		if err != nil {
			printError(err)
			return 1
		}
		if resolve.ProjectExists(baseDir, selector) {
			*queueNameOption = selector
			selector = ""
		}
	}
	if strings.HasPrefix(*jobIDOption, "att_") {
		attemptID = *jobIDOption
		baseDir, projectName, resolvedRunID, resolvedJobID, err := resolve.Attempt(*jobIDOption, *basedir, *queueNameOption, *runIDOption)
		if err != nil {
			printError(err)
			return 1
		}
		*basedir, *queueNameOption, *runIDOption, *jobIDOption = baseDir, projectName, resolvedRunID, resolvedJobID
	}
	if *jobIDOption != "" && attemptID == "" && *runIDOption == "" {
		targets, err := resolveShowJobTargets(*basedir, *queueNameOption, "", *jobIDOption, false)
		if err != nil {
			printError(err)
			return 1
		}
		if len(targets) == 0 {
			printErrorf("job %q not found", *jobIDOption)
			return 1
		}
		if len(targets) > 1 {
			printError(resolve.AmbiguousError(fmt.Sprintf("job %q", *jobIDOption), targets))
			return 1
		}
		applyShowSelectorTarget(targets[0], basedir, queueNameOption, runIDOption, jobIDOption, showQueueOption)
	}
	if selector != "" {
		targets, err := resolveShowSelector(*basedir, *queueNameOption, selector)
		if err != nil {
			printError(err)
			return 1
		}
		if len(targets) == 0 {
			printErrorf("selector %q not found", selector)
			return 1
		}
		if len(targets) > 1 {
			printError(resolve.AmbiguousError(fmt.Sprintf("selector %q", selector), targets))
			return 1
		}
		applyShowSelectorTarget(targets[0], basedir, queueNameOption, runIDOption, jobIDOption, showQueueOption)
	}
	if *jobNameOption != "" {
		targets, err := showJobNameTargets(*basedir, *queueNameOption, *runIDOption, *jobNameOption)
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
		applyShowSelectorTarget(targets[0], basedir, queueNameOption, runIDOption, jobIDOption, showQueueOption)
	}
	if *lineage {
		if selector != "" || *runIDOption != "" || *jobIDOption != "" || *jobNameOption != "" || *showQueueOption || resultFilter || scope.Kinds() > 0 ||
			*showBaseDirsList || *showLogs || *showFailedLogs || *followLogs || *reportOutput {
			printError("--lineage cannot be combined with run, job, queue, filter, list, log, follow, or report options")
			return 1
		}
		baseDir, projectName, err := resolve.ExistingRun(*basedir, *queueNameOption, "")
		if err != nil {
			printError(err)
			return 1
		}
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			printErrorf("failed to resolve paths: %v", err)
			return 1
		}
		return showLineage(paths, *jsonOutput)
	}
	if scope.Kinds() > 0 && (*jobIDOption != "" || *showBaseDirsList || *showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput) {
		printError("--stage and --matrix cannot be combined with job, list, log, follow, JSON, or report options")
		return 1
	}
	if *reportOutput && (*showQueueOption || *showBaseDirsList || *showLogs || *showFailedLogs || *followLogs || *jsonOutput) {
		printError("--report cannot be combined with queue, list, log, follow, or JSON options")
		return 1
	}
	if *showQueueOption && (*runIDOption != "" || resultFilter || *showLogs || *showFailedLogs || *followLogs) {
		printError("--queue cannot be combined with run, log, or filter options")
		return 1
	}
	if *showBaseDirsList {
		if *runIDOption != "" || *jobIDOption != "" || resultFilter || *showLogs || *showFailedLogs || *followLogs || *jsonOutput {
			printError("--basedirs cannot be combined with project, run, job, log, filter, or JSON options")
			return 1
		}
		masterDir, err := state.ResolveMasterDir(*masterdir)
		if err != nil {
			printErrorf("failed to resolve master directory: %v", err)
			return 1
		}
		return showBaseDirs(masterDir)
	}
	if *queueNameOption == "" && *runIDOption == "" && *jobIDOption == "" && *jobNameOption == "" && scope.Kinds() == 0 &&
		!(*showQueueOption || *showBaseDirsList || resultFilter || *showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput) {
		return showAllProjects(*basedir, *masterdir)
	}
	baseDir, queueName, err := resolve.ExistingRun(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	projectOverview := (*queueNameOption != "") && *runIDOption == "" && *jobIDOption == "" && *jobNameOption == "" && scope.Kinds() == 0 && !*showQueueOption && !*showBaseDirsList && !*showLogs && !*showFailedLogs && !*followLogs && !*jsonOutput && !*reportOutput && !resultFilter
	if projectOverview {
		return showProjectOverview(paths)
	}
	if *showQueueOption {
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			printErrorf("failed to load queue: %v", err)
			return 1
		}
		if arrayScope, ok := arrayCommandScope(queue.Commands, *jobIDOption); ok {
			return showQueue(paths, queue, arrayScope)
		}
		if *jobIDOption != "" {
			if *jsonOutput {
				printError("--json cannot be combined with --job-id")
				return 1
			}
			return showQueueJob(paths, queue, *jobIDOption)
		}
		if *jsonOutput {
			return showQueueJSON(paths, queue)
		}
		return showQueue(paths, queue, scope)
	}
	selectedRunID := *runIDOption
	if selectedRunID == "" {
		inspection, err := project.Inspect(paths, true)
		projectState, stateRunID := inspection.State, inspection.RunID
		if err != nil {
			printErrorf("failed to check project state: %v", err)
			return 1
		}
		if projectState == project.Running {
			selectedRunID = stateRunID
		} else if projectState == project.Interrupted {
			selectedRunID = stateRunID
			printInterruptedRunNotice(paths, stateRunID)
		} else {
			queue, err := state.LoadQueue(paths.QueueFile)
			if err != nil {
				printErrorf("failed to load queue: %v", err)
				return 1
			}
			// Options that only apply to runs look past a non-empty queue to
			// the latest run; other views show the queue.
			runOnly := *showLogs || *showFailedLogs || *followLogs || resultFilter || *reportOutput
			if len(queue.Commands) > 0 && !runOnly {
				if arrayScope, ok := arrayCommandScope(queue.Commands, *jobIDOption); ok {
					return showQueue(paths, queue, arrayScope)
				}
				if *jobIDOption != "" {
					return showQueueJob(paths, queue, *jobIDOption)
				}
				if *jsonOutput {
					return showQueueJSON(paths, queue)
				}
				return showQueue(paths, queue, scope)
			}
			if countProjectRuns(paths.RunsDir) == 0 && runOnly {
				printErrorf("project %q has no runs; logs, failed filters, and reports need one", paths.ProjectName)
				return 1
			}
			if countProjectRuns(paths.RunsDir) == 0 {
				printErrorf("WARNING: project %q has no runs or queued jobs; nothing to show", paths.ProjectName)
				writeShowTargetHeaderWithMode(os.Stdout, paths, "project")
				fmt.Println("\nNo runs or queued jobs found.")
				return 0
			}
		}
	}
	runID, err := resolve.RunID(paths, selectedRunID)
	if err != nil {
		printError(err)
		return 1
	}
	if runQueue, err := state.LoadQueue(filepath.Join(paths.RunsDir, runID, "commands.json")); err == nil && !*reportOutput {
		if arrayScope, ok := arrayCommandScope(runQueue.Commands, *jobIDOption); ok {
			return showRun(paths, runID, showJobFilter{selection: resultSelection, scope: arrayScope})
		}
	}
	if *jobIDOption != "" {
		if *reportOutput {
			report, err := buildAIReport(paths, runID, *jobIDOption, false, attemptID)
			if err != nil {
				printError(err)
				return 1
			}
			fmt.Print(report)
			return 0
		}
		if *jsonOutput {
			printError("--json cannot be combined with --job-id")
			return 1
		}
		running, err := isRunning(paths.LockFile)
		if err != nil {
			printErrorf("failed to check queue state: %v", err)
			return 1
		}
		if shouldFollowLogs(*followLogs, running, isTerminal(os.Stdout)) {
			return followJobLog(os.Stdout, paths, runID, *jobIDOption)
		}
		return showWithPager(!*noPager, func(writer io.Writer) int {
			return showJobAttempt(writer, paths, runID, *jobIDOption, attemptID)
		})
	}
	if *followLogs {
		printError("--follow requires --job-id")
		return 1
	}
	if *showLogs || *showFailedLogs {
		if *jsonOutput {
			printError("--json cannot be combined with log output")
			return 1
		}
		return showWithPager(!*noPager, func(writer io.Writer) int {
			return showRunLogs(writer, paths, runID, *showFailedLogs)
		})
	}
	if *reportOutput {
		report, err := buildAIReport(paths, runID, "", *failedOnly)
		if err != nil {
			printError(err)
			return 1
		}
		fmt.Print(report)
		return 0
	}
	if *jsonOutput {
		return showRunJSON(paths, runID)
	}
	return showRun(paths, runID, showJobFilter{selection: resultSelection, scope: scope})
}

func hasMultipleProjects(baseDir string) (bool, error) {
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	projectCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			projectCount++
		}
	}
	return projectCount > 1, nil
}

type showJSON struct {
	BaseDir  string            `json:"base_dir"`
	Project  string            `json:"project_name"`
	RunID    string            `json:"run_id"`
	RunDir   string            `json:"run_dir"`
	Summary  *model.RunSummary `json:"summary,omitempty"`
	Commands model.Queue       `json:"commands"`
}

type showJobCounts struct {
	success int
	failed  int
	blocked int
	running int
	pending int
}

// showJobFilter selects the rows of a run job table.
type showJobFilter struct {
	// selection, when set, keeps the jobs whose result matches it, such as
	// "failed" or "failed,unfinished"; see model.ResultSelection.
	selection string
	// scope, when set, keeps only the jobs of one stage or matrix.
	scope model.CommandSelector
}

// arrayCommandScope reports whether jobID names an array command, whose
// tasks show lists as a table instead of one job's details.
func arrayCommandScope(commands []model.QueuedCommand, jobID string) (model.CommandSelector, bool) {
	for _, command := range commands {
		if jobID != "" && command.ID == jobID && command.Array != nil {
			return model.CommandSelector{IDs: []string{jobID}}, true
		}
	}
	return model.CommandSelector{}, false
}

// scopedJobIDs returns the IDs of the jobs, array tasks included, of the
// commands that scope selects, or nil when scope is not set.
func scopedJobIDs(commands []model.QueuedCommand, scope model.CommandSelector) (map[string]bool, error) {
	if scope.Kinds() == 0 {
		return nil, nil
	}
	indexes, err := model.SelectCommands(commands, scope)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool)
	for _, index := range indexes {
		for _, job := range model.QueueToJobs(commands[index : index+1]) {
			ids[job.ID] = true
		}
	}
	return ids, nil
}

func showRunJSON(paths state.ProjectPaths, runID string) int {
	result := showJSON{BaseDir: paths.BaseDir, Project: paths.ProjectName, RunID: runID, RunDir: filepath.Join(paths.RunsDir, runID)}
	if summary, err := state.LoadRunSummary(filepath.Join(result.RunDir, "summary.json")); err == nil {
		result.Summary = &summary
	} else if !os.IsNotExist(err) {
		printErrorf("failed to read summary: %v", err)
		return 1
	}
	commands, err := state.LoadQueue(filepath.Join(result.RunDir, "commands.json"))
	if err != nil {
		printErrorf("failed to read commands: %v", err)
		return 1
	}
	result.Commands = commands
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		printErrorf("failed to write JSON: %v", err)
		return 1
	}
	return 0
}

func showQueueJSON(paths state.ProjectPaths, queue model.Queue) int {
	result := showJSON{BaseDir: paths.BaseDir, Project: paths.ProjectName, Commands: queue}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		printErrorf("failed to write JSON: %v", err)
		return 1
	}
	return 0
}

func shouldFollowLogs(explicit bool, queueRunning bool, isTTY bool) bool {
	if explicit {
		return true
	}
	return queueRunning && isTTY
}

func showWithPager(usePager bool, show func(io.Writer) int) int {
	if !usePager || !isTerminal(os.Stdout) {
		return show(os.Stdout)
	}

	writer := &pagerWriter{output: os.Stdout}
	exitCode := show(writer)
	if err := writer.Close(); err != nil {
		printError(err)
		if exitCode == 0 {
			return 1
		}
	}
	return exitCode
}

type pagerWriter struct {
	output   io.Writer
	buffer   bytes.Buffer
	newlines int
	input    io.WriteCloser
	command  *exec.Cmd
}

func (writer *pagerWriter) Write(data []byte) (int, error) {
	if writer.input != nil {
		return writer.input.Write(data)
	}
	if writer.command != nil {
		return writer.output.Write(data)
	}

	for offset, value := range data {
		if value == '\n' {
			writer.newlines++
		}
		if writer.newlines > pagerLineLimit {
			writer.buffer.Write(data[:offset+1])
			if err := writer.startPager(); err != nil {
				return 0, err
			}
			if offset+1 == len(data) {
				return len(data), nil
			}
			written, err := writer.Write(data[offset+1:])
			return offset + 1 + written, err
		}
	}
	writer.buffer.Write(data)
	return len(data), nil
}

func (writer *pagerWriter) Close() error {
	if writer.input == nil && writer.command == nil && exceedsPagerLineLimit(writer.buffer.Bytes(), writer.newlines) {
		if err := writer.startPager(); err != nil {
			if _, writeErr := writer.output.Write(writer.buffer.Bytes()); writeErr != nil {
				return fmt.Errorf("%v; failed to write output directly: %w", err, writeErr)
			}
			writer.buffer.Reset()
			return nil
		}
	}
	if writer.input == nil {
		_, err := writer.output.Write(writer.buffer.Bytes())
		return err
	}
	if err := writer.input.Close(); err != nil {
		return err
	}
	if err := writer.command.Wait(); err != nil {
		return fmt.Errorf("pager %q failed: %w", writer.command.Args[0], err)
	}
	return nil
}

func exceedsPagerLineLimit(data []byte, newlines int) bool {
	return newlines > pagerLineLimit || newlines == pagerLineLimit && len(data) > 0 && data[len(data)-1] != '\n'
}

func (writer *pagerWriter) startPager() error {
	args := strings.Fields(os.Getenv("PAGER"))
	if len(args) == 0 {
		args = []string{"less", "-R"}
	}
	command := exec.Command(args[0], args[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	input, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to start pager: %w", err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("failed to start pager %q: %w", args[0], err)
	}
	writer.input = input
	writer.command = command
	if _, err := writer.input.Write(writer.buffer.Bytes()); err != nil {
		return err
	}
	writer.buffer.Reset()
	return nil
}

func writeShowTargetHeader(writer io.Writer, paths state.ProjectPaths) {
	writeShowTargetHeaderWithMode(writer, paths, "")
}

func writeShowTargetHeaderWithMode(writer io.Writer, paths state.ProjectPaths, mode string) {
	if mode != "" {
		fmt.Fprintf(writer, "%s\n", cyan("=== SHOW MODE: "+showViewLabel(mode)+" ==="))
	}
	fmt.Fprintf(writer, "%s %s\n%s %s\n", cyan("Base directory:"), paths.BaseDir, cyan("Project:"), paths.ProjectName)
	if queue, err := state.LoadQueue(paths.QueueFile); err == nil {
		fmt.Fprintf(writer, "%s %s (%d jobs)\n", cyan("Queue:"), paths.QueueFile, len(queue.Commands))
	} else {
		fmt.Fprintf(writer, "%s %s\n", cyan("Queue:"), paths.QueueFile)
	}
	if inspection, err := project.Inspect(paths, true); err == nil {
		fmt.Fprintf(writer, "%s %s\n", cyan("Project state:"), projectStateName(inspection.State))
	}
	if response, err := serverinternal.SendRequest(paths.BaseDir, serverinternal.Request{Op: "ping"}); err == nil && response.OK {
		fmt.Fprintf(writer, "%s running (pid=%d)\n", cyan("Runner server:"), response.PID)
	} else {
		fmt.Fprintf(writer, "%s stopped\n", cyan("Runner server:"))
	}
	fmt.Fprintf(writer, "%s %d\n", cyan("Runs:"), countProjectRuns(paths.RunsDir))
	if configPath := effectiveConfigPath(paths.BaseDir, paths.ProjectName); configPath != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Config:"), configPath)
	}
}

func showViewLabel(mode string) string {
	switch mode {
	case "basedirs":
		return "BASE DIRECTORIES"
	case "projects":
		return "PROJECTS"
	case "runs":
		return "PROJECT / RUNS"
	case "queue":
		return "PROJECT / QUEUE"
	case "queue job":
		return "PROJECT / QUEUE / JOB"
	case "run":
		return "PROJECT / RUN"
	case "run job":
		return "PROJECT / RUN / JOB"
	case "project":
		return "PROJECT"
	case "lineage":
		return "PROJECT / LINEAGE"
	default:
		return strings.ToUpper(mode)
	}
}

func countProjectRuns(runsDir string) int {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func printInterruptedRunNotice(paths state.ProjectPaths, runID string) {
	fmt.Printf("%s\n", yellow(fmt.Sprintf("Run %s appears to have been interrupted.", runID)))
	fmt.Printf("Recover the queue before modifying or running it:\n  rotari unlock --basedir %s --project-name %s --run-id %s\n\n",
		executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName), executor.ShellQuote(runID))
}

func showRun(paths state.ProjectPaths, runID string, filter showJobFilter) int {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	summary, summaryErr := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	summaryOK := summaryErr == nil
	if summaryErr != nil && !errors.Is(summaryErr, os.ErrNotExist) {
		printErrorf("failed to read summary: %v", summaryErr)
		return 1
	}
	jobSpecs := loadRunJobSpecs(runDir)
	scoped, err := scopedJobIDs(runScopeCommands(runDir, jobSpecs), filter.scope)
	if err != nil {
		printErrorf("run %s: %v", runID, err)
		return 1
	}

	if summary.RunName == "" {
		// An active run has no summary yet; its lock records the name.
		if lock, err := state.LoadLock(paths.LockFile); err == nil && lock.RunID == runID {
			summary.RunName = lock.RunName
		}
	}
	writeShowTargetHeaderWithMode(os.Stdout, paths, "run")
	fmt.Printf("%s %s\n", cyan("Run:"), formatRunLabel(runID, summary.RunName))
	if summary.RunName != "" {
		fmt.Printf("%s %s\n", cyan("Run name:"), summary.RunName)
	}
	fmt.Printf("%s %s\n", cyan("Directory:"), runDir)
	if summaryOK {
		fmt.Printf("%s %s\n%s %s\n%s %s\n%s %d\n", cyan("Status:"), summary.Status, cyan("Started:"), model.FormatDisplayTimestamp(summary.StartedAt), cyan("Finished:"), model.FormatDisplayTimestamp(summary.FinishedAt), cyan("Exit code:"), summary.ExitCode)
	}
	fmt.Printf("%s %s\n", cyan("Output directory:"), runDir)
	queue, queueErr := state.LoadQueue(paths.QueueFile)
	if queueErr == nil && len(queue.Commands) > 0 {
		if diff, err := compareQueueWithRun(paths.QueueFile, filepath.Join(runDir, "commands.json")); err == nil && diff.HasChanges() {
			fmt.Printf("\n%s\n", yellow("Queue differs from this run:"))
			fmt.Printf("  Added: %d\n  Removed: %d\n  Changed: %d\n", diff.Added, diff.Removed, diff.Changed)
		}
	}
	runQueue, runQueueErr := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	runActive := runIsActive(paths, runID)
	jobCounts := showJobCounts{}
	resultByID := model.ResultsByID(summary.Results)
	jobIDs := make([]string, 0, len(runQueue.Commands))
	originByID := make(map[string]*model.JobOrigin, len(runQueue.Commands))
	if runQueueErr == nil {
		for _, job := range model.QueueToJobs(runQueue.Commands) {
			jobIDs = append(jobIDs, job.ID)
		}
		for _, command := range runQueue.Commands {
			originByID[command.ID] = command.Origin
			for taskID, origin := range command.TaskOrigins {
				originByID[taskID] = origin
			}
		}
	} else {
		entries, err := os.ReadDir(runDir)
		if err != nil {
			printErrorf("failed to read run directory: %v", err)
			return 1
		}
		for _, entry := range entries {
			if entry.IsDir() {
				jobIDs = append(jobIDs, entry.Name())
			}
		}
	}
	fmt.Println("\n" + cyan("Jobs:"))
	changeHints := make([]model.JobSpec, 0)
	fmt.Printf("%s\n", cyan(fmt.Sprintf("%-12s %-42s %-6s %-15s %-15s %-20s %-10s %-30s %-24s %-24s %-24s %s", "JOB ID", "LATEST ATTEMPT", "TASK", "NAME", "STAGE", "DEPENDS ON", "STATUS", "EXECUTOR", "SUBMITTED", "FINISHED", "HOSTS", "COMMAND")))
	for _, jobID := range jobIDs {
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		if err != nil {
			continue
		}
		jobSpec := jobSpecs[jobID]
		if scoped != nil && !scoped[jobID] {
			continue
		}
		latestAttemptLabel := "-"
		if value, readErr := state.LatestAttemptID(runDir, jobID); readErr == nil && value != "" {
			latestAttemptLabel = value
		}
		if latestAttemptLabel == "-" {
			if origin := originByID[jobID]; origin != nil && origin.AttemptID != "" {
				latestAttemptLabel = origin.AttemptID
			} else if result, ok := resultByID[jobSpec.ID]; ok && result.AttemptID != "" {
				latestAttemptLabel = result.AttemptID
			}
		}
		name := readJobName(jobDir)
		if name == "" {
			name = jobSpec.Name
		}
		if name == "" {
			name = "-"
		}
		stage := jobSpec.Stage
		if stage == "" {
			stage = "-"
		}
		taskText := "-"
		if jobSpec.ArrayTaskID != nil {
			taskText = strconv.Itoa(*jobSpec.ArrayTaskID)
		}
		dependsOn := model.FormatDependencies(jobSpec.DependsOn, jobSpec.DependsOnFinished, ",")
		if dependsOn == "" {
			dependsOn = "-"
		}
		summaryResult, hasSummary := resultByID[jobSpec.ID]
		resolved := jobstatus.ReadJob(jsonStore(), jobDir, summaryResult, hasSummary)
		status, statusOK, blocked := resolved.ExitCode, resolved.Finished(), resolved.Blocked()
		if statusOK {
			switch {
			case blocked:
				jobCounts.blocked++
			case status == 0:
				jobCounts.success++
			default:
				jobCounts.failed++
			}
		} else if runActive {
			jobCounts.running++
		} else {
			jobCounts.pending++
		}
		executorText := queueExecutorText(runQueue, jobSpec)
		if filter.selection != "" && !model.ResultSelectionMatches(filter.selection, statusOK, status) {
			continue
		}
		if statusOK && status != 0 {
			changeHints = append(changeHints, jobSpec)
		}
		hosts := "-"
		if resolvedHosts := resolved.Hosts(); len(resolvedHosts) > 0 {
			hosts = strings.Join(resolvedHosts, ",")
		}
		command := readJSONCommand(filepath.Join(jobDir, commandJSONName))
		if command == "" {
			command = strings.Join(jobSpec.Command, " ")
		}
		submittedAt, finishedAt := jobstatus.Timestamps(runDir, jobID, originByID[jobID])
		submittedAt = model.FormatDisplayTimestamp(submittedAt)
		finishedAt = model.FormatDisplayTimestamp(finishedAt)
		if statusOK {
			statusText := green(strconv.Itoa(status))
			if resolved.Accepted() {
				statusText = green("success (accepted)")
			} else if blocked {
				statusText = yellow("blocked")
			} else if status != 0 {
				statusText = red(strconv.Itoa(status))
			}
			fmt.Printf("%-12s %-42s %-6s %-15s %-15s %-20s %-10s %-30s %-24s %-24s %-24s %s\n", jobID, latestAttemptLabel, taskText, name, stage, dependsOn, statusText, executorText, submittedAt, finishedAt, hosts, command)
		} else {
			fmt.Printf("%-12s %-42s %-6s %-15s %-15s %-20s %-10s %-30s %-24s %-24s %-24s %s\n", jobID, latestAttemptLabel, taskText, name, stage, dependsOn, yellow("running"), executorText, submittedAt, finishedAt, hosts, command)
		}
	}
	fmt.Printf("\n%s success: %d, failed: %d, blocked: %d, running: %d, pending: %d\n", cyan("Job status:"), jobCounts.success, jobCounts.failed, jobCounts.blocked, jobCounts.running, jobCounts.pending)
	printChangeHints(paths, runID, runQueue, changeHints)
	printFailedLogHints(runID, changeHints, resultByID)
	fmt.Printf("\n%s\n  rotari delete --run-id %s\n", cyan("To delete this run's saved logs:"), runID)
	return 0
}

func runIsActive(paths state.ProjectPaths, runID string) bool {
	running, err := isRunning(paths.LockFile)
	if err != nil || !running {
		return false
	}
	lock, err := state.LoadLock(paths.LockFile)
	return err == nil && lock.RunID == runID
}

func printFailedLogHints(runID string, failedJobs []model.JobSpec, results map[string]model.JobResult) {
	if len(failedJobs) == 0 {
		return
	}
	selector := "-j JOB_ID"
	if len(failedJobs) == 1 {
		selector = "-j " + failedJobs[0].ID
		if result, ok := results[failedJobs[0].ID]; ok && result.AttemptID != "" {
			selector = "-j " + result.AttemptID
		}
	}
	fmt.Println("\n" + cyan("Logs:"))
	fmt.Println("  " + cyan("e.g., Show logs for every failed job:"))
	fmt.Printf("    rotari show -r %s --failed-logs\n", runID)
	fmt.Println("  " + cyan("e.g., Show the log for one failed job:"))
	if strings.HasPrefix(selector, "-j att_") {
		fmt.Printf("    rotari show %s\n", selector)
	} else {
		fmt.Printf("    rotari show -r %s %s\n", runID, selector)
	}
}

func printChangeHints(paths state.ProjectPaths, runID string, queue model.Queue, jobs []model.JobSpec) {
	if len(jobs) == 0 {
		return
	}
	hasSlurm := false
	hasDependencies := false
	selector := "-j JOB_ID"
	if len(jobs) == 1 && jobs[0].Name != "" {
		selector = "--job-name " + jobs[0].Name
	}
	for _, job := range jobs {
		executor := job.Executor
		if executor == "" {
			executor = queue.DefaultExecutor
		}
		if executor == "slurm" {
			hasSlurm = true
		}
		if len(job.DependsOn) > 0 {
			hasDependencies = true
		}
	}
	fmt.Println("\n" + cyan("Change:"))
	fmt.Println("  " + cyan("e.g., Replace the command:"))
	fmt.Printf("    rotari change -r %s %s -- <new-command ...>\n", runID, selector)
	if hasSlurm {
		fmt.Println("  " + cyan("e.g., Replace the executor options:"))
		fmt.Printf("    rotari change -r %s %s --executor-option=\"<options>\"\n", runID, selector)
	}
	if hasDependencies {
		fmt.Println("  " + cyan("e.g., Replace the dependencies:"))
		fmt.Printf("    rotari change -r %s %s --depends-on <job-name>\n", runID, selector)
	}
	fmt.Println("\n" + cyan("Retry:"))
	fmt.Printf("    rotari retry --basedir %s --project-name %s\n", paths.BaseDir, paths.ProjectName)
}

func showQueue(paths state.ProjectPaths, queue model.Queue, scope model.CommandSelector) int {
	jobs := model.QueueToJobs(queue.Commands)
	scoped, err := scopedJobIDs(queue.Commands, scope)
	if err != nil {
		printErrorf("current queue: %v", err)
		return 1
	}
	if scoped != nil {
		selected := make([]model.JobSpec, 0, len(scoped))
		for _, job := range jobs {
			if scoped[job.ID] {
				selected = append(selected, job)
			}
		}
		jobs = selected
	}
	writeShowTargetHeaderWithMode(os.Stdout, paths, "queue")
	fmt.Println("\n" + cyan("Queue:"))
	return showQueueContent(paths, queue, jobs)
}

// runScopeCommands returns a run's command snapshot. A run saved without
// one is described by its per-job specs, which carry stages but no matrix.
func runScopeCommands(runDir string, jobSpecs map[string]model.JobSpec) []model.QueuedCommand {
	if queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json")); err == nil {
		return queue.Commands
	}
	commands := make([]model.QueuedCommand, 0, len(jobSpecs))
	for id, spec := range jobSpecs {
		commands = append(commands, model.QueuedCommand{ID: id, Name: spec.Name, Stage: spec.Stage, Command: spec.Command})
	}
	return commands
}

// showQueueContent prints jobs, taken from queue, as a table.
func showQueueContent(paths state.ProjectPaths, queue model.Queue, jobs []model.JobSpec) int {
	originByID := model.QueueOriginsByJobID(model.Queue(queue))
	fmt.Printf("%s\n", cyan(fmt.Sprintf("%-12s %-6s %-15s %-15s %-20s %-30s %-24s %-12s %s", "JOB ID", "TASK", "NAME", "STAGE", "DEPENDS ON", "EXECUTOR", "SOURCE RUN", "SOURCE STATUS", "COMMAND")))
	for _, job := range jobs {
		name := job.Name
		if name == "" {
			name = "-"
		}
		stage := job.Stage
		if stage == "" {
			stage = "-"
		}
		dependsOn := model.FormatDependencies(job.DependsOn, job.DependsOnFinished, ",")
		if dependsOn == "" {
			dependsOn = "-"
		}
		taskText := "-"
		if job.ArrayTaskID != nil {
			taskText = strconv.Itoa(*job.ArrayTaskID)
		}
		executorText := queueExecutorText(queue, job)
		sourceRun, sourceStatus := "-", "-"
		if origin := originByID[job.ID]; origin != nil {
			sourceRun = origin.RunID + "/" + origin.JobID
			if origin.Status != "" {
				sourceStatus = origin.Status
			}
		}
		fmt.Printf("%-12s %-6s %-15s %-15s %-20s %-30s %-24s %-12s %s\n", job.ID, taskText, name, stage, dependsOn, executorText, sourceRun, sourceStatus, strings.Join(job.Command, " "))
	}
	fmt.Printf("\n%s\n  rotari run -b %s -p %s\n", cyan("To execute these jobs:"), executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName))
	return 0
}

func showQueueJob(paths state.ProjectPaths, queue model.Queue, jobID string) int {
	if !state.IsValidPathElement(jobID) {
		printErrorf("job %q not found in current queue", jobID)
		return 1
	}
	for _, job := range model.QueueToJobs(queue.Commands) {
		if job.ID != jobID {
			continue
		}
		writeShowTargetHeaderWithMode(os.Stdout, paths, "queue job")
		fmt.Printf("%s %s\n", cyan("Job:"), job.ID)
		if job.Name != "" {
			fmt.Printf("%s %s\n", cyan("Name:"), job.Name)
		}
		if job.Stage != "" {
			fmt.Printf("%s %s\n", cyan("Stage:"), job.Stage)
		}
		if job.ArrayTaskID != nil {
			fmt.Printf("%s %d (range %d-%d)\n", cyan("Array task:"), *job.ArrayTaskID, job.ArrayFirst, job.ArrayLast)
		}
		fmt.Printf("%s %s\n", cyan("Executor:"), queueExecutorText(queue, job))
		if dependencies := model.FormatDependencies(job.DependsOn, job.DependsOnFinished, ", "); dependencies != "" {
			fmt.Printf("%s %s\n", cyan("Depends on:"), dependencies)
		}
		if job.WorkingDirectory != "" {
			fmt.Printf("%s %s\n", cyan("Working directory:"), job.WorkingDirectory)
		}
		if job.Timeout != "" {
			fmt.Printf("%s %s\n", cyan("Timeout:"), job.Timeout)
		}
		if retry := model.FormatRetryPolicy(job); retry != "" {
			fmt.Printf("%s %s\n", cyan("Retry:"), retry)
		}
		fmt.Printf("%s %s\n", cyan("Command:"), strings.Join(job.Command, " "))
		return 0
	}
	printErrorf("job %q not found in current queue", jobID)
	return 1
}

func queueExecutorText(queue model.Queue, job model.JobSpec) string {
	executor := job.Executor
	options := job.ExecutorOptions
	if executor == "" {
		executor = queue.DefaultExecutor
	}
	if executor == "" {
		executor = "local"
	}
	if executor == "slurm" && len(options) == 0 {
		options = queue.DefaultExecutorOptions
	}
	executorText := colorExecutor(executor)
	if len(options) > 0 {
		executorText += " (" + strings.Join(options, " ") + ")"
	}
	return executorText
}

type queueRunDiff struct {
	Added   int
	Removed int
	Changed int
}

func (diff queueRunDiff) HasChanges() bool {
	return diff.Added != 0 || diff.Removed != 0 || diff.Changed != 0
}

func compareQueueWithRun(queuePath, runCommandsPath string) (queueRunDiff, error) {
	current, err := state.LoadQueue(queuePath)
	if err != nil {
		return queueRunDiff{}, err
	}
	runQueue, err := state.LoadQueue(runCommandsPath)
	if err != nil {
		return queueRunDiff{}, err
	}

	currentJobs := model.QueueToJobs(current.Commands)
	runJobs := model.QueueToJobs(runQueue.Commands)
	currentByID := make(map[string]model.JobSpec, len(currentJobs))
	for _, job := range currentJobs {
		currentByID[job.ID] = job
	}
	runByID := make(map[string]model.JobSpec, len(runJobs))
	for _, job := range runJobs {
		runByID[job.ID] = job
	}
	var diff queueRunDiff
	for id, currentJob := range currentByID {
		runJob, ok := runByID[id]
		if !ok {
			diff.Added++
		} else if !sameJobSpec(currentJob, runJob) {
			diff.Changed++
		}
	}
	for id := range runByID {
		if _, ok := currentByID[id]; !ok {
			diff.Removed++
		}
	}
	return diff, nil
}

func sameJobSpec(left, right model.JobSpec) bool {
	if left.ID != right.ID || left.Name != right.Name || left.WorkingDirectory != right.WorkingDirectory || left.Executor != right.Executor || left.ArrayGroup != right.ArrayGroup || left.ArrayFirst != right.ArrayFirst || left.ArrayLast != right.ArrayLast {
		return false
	}
	if (left.ArrayTaskID == nil) != (right.ArrayTaskID == nil) || (left.ArrayTaskID != nil && *left.ArrayTaskID != *right.ArrayTaskID) {
		return false
	}
	if !slicesEqual(left.Command, right.Command) || !slicesEqual(left.ExecutorOptions, right.ExecutorOptions) || !slicesEqual(left.DependsOn, right.DependsOn) || !slicesEqual(left.DependsOnFinished, right.DependsOnFinished) {
		return false
	}
	return true
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func showRuns(paths state.ProjectPaths) int {
	return showRunsWithHint(paths, true)
}

func showRunsOverview(paths state.ProjectPaths) int {
	return showRunsWithMode(paths, false, "project")
}

func showRunsWithHint(paths state.ProjectPaths, showHint bool) int {
	return showRunsWithMode(paths, showHint, "runs")
}

func showRunsWithMode(paths state.ProjectPaths, showHint bool, mode string) int {
	entries, err := os.ReadDir(paths.RunsDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeShowTargetHeaderWithMode(os.Stdout, paths, mode)
			fmt.Println("No runs found.")
			return 0
		}
		printErrorf("failed to read runs directory: %v", err)
		return 1
	}

	type runInfo struct {
		id         string
		name       string
		startedAt  string
		finishedAt string
		exitCode   int
		status     string
		hasSummary bool
		modTime    int64
	}

	var runs []runInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		runDir := filepath.Join(paths.RunsDir, runID)
		info, err := entry.Info()
		modTime := int64(0)
		if err == nil {
			modTime = info.ModTime().UnixNano()
		}

		r := runInfo{id: runID, modTime: modTime}
		if summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
			r.name = summary.RunName
			r.startedAt = summary.StartedAt
			r.finishedAt = summary.FinishedAt
			r.exitCode = summary.ExitCode
			r.status = summary.Status
			if r.status == "" {
				r.status = model.RunStatus(summary.ExitCode)
			}
			r.hasSummary = true
		}
		if !r.hasSummary {
			r.status = "running"
		}
		runs = append(runs, r)
	}

	sort.Slice(runs, func(i, j int) bool {
		return runs[i].modTime > runs[j].modTime
	})

	writeShowTargetHeaderWithMode(os.Stdout, paths, mode)
	fmt.Printf("%s %s\n", cyan("Runs directory:"), paths.RunsDir)
	fmt.Println("\n" + cyan("Runs:"))
	fmt.Printf("%s\n", cyan(fmt.Sprintf("%-36s %-24s %-12s %-12s %-24s %-24s", "RUN ID", "NAME", "STATUS", "EXIT CODE", "STARTED", "FINISHED")))
	for _, r := range runs {
		started := r.startedAt
		if started == "" {
			started = "-"
		}
		started = model.FormatDisplayTimestamp(started)
		finished := r.finishedAt
		if finished == "" {
			finished = "-"
		}
		finished = model.FormatDisplayTimestamp(finished)
		statusText := r.status
		if statusText == "finished" {
			statusText = green(statusText)
		} else if statusText == "failed" {
			statusText = red(statusText)
		} else {
			statusText = yellow(statusText)
		}
		exitCode := "-"
		if r.hasSummary {
			exitCode = strconv.Itoa(r.exitCode)
		}
		name := r.name
		if name == "" {
			name = "-"
		}
		fmt.Printf("%-36s %-24s %-12s %-12s %-24s %-24s\n", r.id, name, statusText, exitCode, started, finished)
	}
	if showHint {
		fmt.Println("\n" + cyan("To show jobs in a run:"))
		fmt.Println("  rotari show -p PROJECT -r RUN_ID")
	}
	return 0
}

func showProjectOverview(paths state.ProjectPaths) int {
	if code := showRunsOverview(paths); code != 0 {
		return code
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		printErrorf("failed to load queue: %v", err)
		return 1
	}
	if len(queue.Commands) == 0 {
		return 0
	}
	fmt.Println("\n" + cyan("Queue:"))
	return showQueueContent(paths, queue, model.QueueToJobs(queue.Commands))
}

func showProjects(baseDir string) int {
	return showProjectsForBaseDirs([]string{baseDir})
}

func showAllProjects(cliBaseDir, cliMasterDir string) int {
	if cliBaseDir != "" {
		baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
		if err != nil {
			printErrorf("failed to resolve state directory: %v", err)
			return 1
		}
		return showProjectsForBaseDirs([]string{baseDir})
	}
	masterDir, err := state.ResolveMasterDir(cliMasterDir)
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	servers, err := listServers(masterDir)
	if err != nil {
		printErrorf("failed to list servers: %v", err)
		return 1
	}
	known, err := listKnownBaseDirs(masterDir, servers)
	if err != nil {
		printErrorf("failed to list known state directories: %v", err)
		return 1
	}
	baseDirs := make([]string, 0, len(known)+1)
	seen := make(map[string]bool, len(known)+1)
	for _, item := range known {
		seen[item.BaseDir] = true
		baseDirs = append(baseDirs, item.BaseDir)
	}
	if current, _, err := state.ResolveBaseDir(""); err == nil && !seen[current] {
		baseDirs = append(baseDirs, current)
	}
	sort.Strings(baseDirs)
	return showProjectsForBaseDirs(baseDirs)
}

func showProjectsForBaseDirs(baseDirs []string) int {
	fmt.Printf("%s\n", cyan("=== SHOW MODE: "+showViewLabel("projects")+" ==="))

	type projectInfo struct {
		baseDir string
		name    string
		queued  int
		runs    int
		state   string
		lastRun string
	}
	projects := make([]projectInfo, 0)
	for _, baseDir := range baseDirs {
		entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			printErrorf("failed to read projects directory %q: %v", baseDir, err)
			return 1
		}
		for _, entry := range entries {
			if !entry.IsDir() || !state.IsValidPathElement(entry.Name()) {
				continue
			}
			paths, err := state.ResolveProjectPaths(baseDir, entry.Name())
			if err != nil {
				printErrorf("failed to resolve project %q: %v", entry.Name(), err)
				return 1
			}
			queue, err := state.LoadQueue(paths.QueueFile)
			if err != nil {
				printErrorf("failed to load queue for project %q: %v", entry.Name(), err)
				return 1
			}
			inspection, err := project.Inspect(paths, true)
			projectState := inspection.State
			if err != nil {
				printErrorf("failed to check project %q state: %v", entry.Name(), err)
				return 1
			}
			meta, err := state.LoadMeta(paths.MetaFile)
			if err != nil {
				printErrorf("failed to load project %q metadata: %v", entry.Name(), err)
				return 1
			}
			lastRun := meta.LastRunID
			if lastRun == "" {
				lastRun = "-"
			}
			projects = append(projects, projectInfo{baseDir: baseDir, name: entry.Name(), queued: len(queue.Commands), runs: countProjectRuns(paths.RunsDir), state: projectStateName(projectState), lastRun: lastRun})
		}
	}

	if len(projects) == 0 {
		fmt.Println("No projects found.")
		return 0
	}
	fmt.Printf("\n%s\n", cyan(fmt.Sprintf("Projects: %d", len(projects))))
	fmt.Println(cyan(fmt.Sprintf("%-36s %-24s %-8s %-8s %-14s %s", "BASEDIR", "PROJECT", "QUEUED", "RUNS", "STATE", "LAST RUN")))
	for _, project := range projects {
		fmt.Printf("%-36s %-24s %-8d %-8d %-14s %s\n", project.baseDir, project.name, project.queued, project.runs, project.state, project.lastRun)
	}
	fmt.Println("\n" + cyan("To show runs in a project:"))
	fmt.Println("  rotari show -p PROJECT")
	return 0
}

func showBaseDirs(masterDir string) int {
	servers, err := listServers(masterDir)
	if err != nil {
		printErrorf("failed to list servers: %v", err)
		return 1
	}
	baseDirs, err := listKnownBaseDirs(masterDir, servers)
	if err != nil {
		printErrorf("failed to list known state directories: %v", err)
		return 1
	}
	fmt.Printf("%s\n%s %s\n", cyan("=== SHOW MODE: "+showViewLabel("basedirs")+" ==="), cyan("Master directory:"), masterDir)
	if len(baseDirs) == 0 {
		fmt.Println("No known state directories.")
		return 0
	}
	fmt.Printf("\n%s\n", cyan(fmt.Sprintf("Known state directories: %d", len(baseDirs))))
	fmt.Println(cyan(fmt.Sprintf("%-8s %-24s %s", "PID", "SOURCE", "BASE DIRECTORY")))
	for _, baseDir := range baseDirs {
		pid := "-"
		if baseDir.PID != 0 {
			pid = strconv.Itoa(baseDir.PID)
		}
		fmt.Printf("%-8s %-24s %s\n", pid, strings.Join(baseDir.Sources, ", "), baseDir.BaseDir)
	}
	return 0
}

func projectStateName(state project.RunState) string {
	switch state {
	case project.Running:
		return "running"
	case project.Interrupted:
		return "interrupted"
	default:
		return "idle"
	}
}

func colorExecutor(executor string) string {
	switch executor {
	case "local":
		return green(executor)
	case "slurm":
		return cyan(executor)
	default:
		return yellow(executor)
	}
}

func loadRunJobSpecs(runDir string) map[string]model.JobSpec {
	specs := make(map[string]model.JobSpec)
	if queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json")); err == nil {
		for _, job := range model.QueueToJobs(queue.Commands) {
			specs[job.ID] = job
		}
	}
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return specs
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobDir, err := state.LatestAttemptJobDir(runDir, entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(jobDir, commandJSONName))
		if err != nil {
			continue
		}
		var job model.JobSpec
		if json.Unmarshal(data, &job) == nil {
			specs[entry.Name()] = job
		}
	}
	return specs
}

func showJob(writer io.Writer, paths state.ProjectPaths, runID, jobID string) int {
	return showJobAttempt(writer, paths, runID, jobID, "")
}

func showJobAttempt(writer io.Writer, paths state.ProjectPaths, runID, jobID, attemptID string) int {
	if !state.IsValidPathElement(runID) {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	if !state.IsValidPathElement(jobID) {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	jobDir, err := state.LatestAttemptJobDir(filepath.Join(paths.RunsDir, runID), jobID)
	if attemptID != "" {
		jobDir, err = state.SpecificAttemptJobDir(filepath.Join(paths.RunsDir, runID), jobID, attemptID)
	}
	if err != nil {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	if info, err := os.Stat(jobDir); err != nil || !info.IsDir() {
		if origin := state.LoadRunOrigin(runDir, jobID); origin != nil {
			if runResultAccepted(runDir, jobID) {
				// The destination result is an accepted success; the source
				// details below keep the original attempt and exit code.
				fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), green("success (accepted)"))
				fmt.Fprintf(writer, "%s manually accepted from run %s attempt %s (no re-execution)\n\n", cyan("Note:"), origin.RunID, origin.AttemptID)
				return showJobAttempt(writer, paths, origin.RunID, origin.JobID, origin.AttemptID)
			}
			fmt.Fprintf(writer, "%s carried forward from run %s (no re-execution)\n\n", cyan("Note:"), origin.RunID)
			return showJobAttempt(writer, paths, origin.RunID, origin.JobID, "")
		}
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	writeShowTargetHeaderWithMode(writer, paths, "run job")
	fmt.Fprintf(writer, "%s %s\n%s %s\n", cyan("Run:"), runID, cyan("Job:"), jobID)
	jobSpecs := loadRunJobSpecs(runDir)
	latestAttemptID, _ := state.LatestAttemptID(runDir, jobID)
	selectedAttemptID := attemptID
	if selectedAttemptID == "" {
		selectedAttemptID = latestAttemptID
	}
	latest := selectedAttemptID == latestAttemptID
	if selectedAttemptID != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Attempt ID:"), selectedAttemptID)
	}
	if attempts := state.ListAttemptIDs(runDir, jobID); len(attempts) > 0 {
		fmt.Fprintln(writer, cyan("Attempts:"))
		for _, listedAttemptID := range attempts {
			labels := make([]string, 0, 2)
			if listedAttemptID == latestAttemptID {
				labels = append(labels, "latest")
			}
			if listedAttemptID == selectedAttemptID && attemptID != "" {
				labels = append(labels, "selected")
			}
			if len(labels) > 0 {
				fmt.Fprintf(writer, "  %s (%s)\n", listedAttemptID, strings.Join(labels, ", "))
			} else {
				fmt.Fprintf(writer, "  %s\n", listedAttemptID)
			}
		}
	}
	name := readJobName(jobDir)
	if name == "" {
		name = jobSpecs[jobID].Name
	}
	if name != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Name:"), name)
	}
	if stage := jobSpecs[jobID].Stage; stage != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Stage:"), stage)
	}
	executor, options := jobSpecs[jobID].Executor, jobSpecs[jobID].ExecutorOptions
	if executor == "" {
		executor = "local"
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Executor:"), executor)
	if len(options) > 0 {
		fmt.Fprintf(writer, "%s %s\n", cyan("Executor options:"), strings.Join(options, " "))
	}
	if dependencies := model.FormatDependencies(jobSpecs[jobID].DependsOn, jobSpecs[jobID].DependsOnFinished, ", "); dependencies != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Depends on:"), dependencies)
	}
	if timeout := jobSpecs[jobID].Timeout; timeout != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Timeout:"), timeout)
	}
	if retry := model.FormatRetryPolicy(jobSpecs[jobID]); retry != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Retry:"), retry)
	}
	submittedAt, finishedAt := state.ReadJobTimestamp(runDir, jobID, "submitted_at"), state.ReadJobTimestamp(runDir, jobID, "finished_at")
	if !latest {
		submittedAt, finishedAt = state.ReadAttemptTimestamp(jobDir, "submitted_at"), state.ReadAttemptTimestamp(jobDir, "finished_at")
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Submitted:"), model.FormatDisplayTimestamp(submittedAt))
	fmt.Fprintf(writer, "%s %s\n", cyan("Finished:"), model.FormatDisplayTimestamp(finishedAt))
	summaryResult, hasSummary := loadRunResult(runDir, jobSpecs[jobID].ID)
	resolved := jobstatus.ResolveAttempt(jobstatus.ReadAttempt(jsonStore(), jobDir), latest, summaryResult, hasSummary)
	if resolved.HasSummary {
		hosts := strings.Join(resolved.Summary.Hosts, ",")
		if hosts == "" {
			hosts = "-"
		}
		fmt.Fprintf(writer, "%s %s\n", cyan("Hosts:"), hosts)
	}
	if status, ok := jobAttemptStatusText(resolved); ok {
		fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), status)
	}
	command := readJSONCommand(filepath.Join(jobDir, commandJSONName))
	if resolved.HasSummary {
		writeJobDiagnoses(writer, resolved.Summary)
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Command:"), command)
	fmt.Fprintf(writer, "%s %s\n\n", cyan("Output:"), filepath.Join(jobDir, "output"))
	output, err := os.ReadFile(filepath.Join(jobDir, "output"))
	if err == nil {
		fmt.Fprint(writer, string(output))
	}
	return 0
}

// jobAttemptStatusText renders a resolved job's status for `show` of one job.
func jobAttemptStatusText(resolved jobstatus.Job) (string, bool) {
	switch resolved.Source {
	case jobstatus.SourceStatus:
		return exitCodeStatusText(resolved.ExitCode, strconv.Itoa(resolved.ExitCode)), true
	case jobstatus.SourceScheduler:
		return exitCodeStatusText(resolved.ExitCode, fmt.Sprintf("%s (exit code %d)", resolved.Attempt.SchedulerState, resolved.ExitCode)), true
	case jobstatus.SourceSummary:
		result := resolved.Summary
		switch {
		case result.Accepted:
			return green("success (accepted)"), true
		case resolved.Blocked():
			return yellow("blocked (dependency failed)"), true
		case result.Error != "":
			// No terminal attempt state was ever recorded for this job (e.g. the
			// scheduler and its accounting were both unreachable), so this is the
			// only place the reason surfaces.
			return red(fmt.Sprintf("%d (%s)", result.ExitCode, result.Error)), true
		default:
			return exitCodeStatusText(result.ExitCode, strconv.Itoa(result.ExitCode)), true
		}
	}
	// A terminal wrapper, or a still-running one, shows its phase.
	if !resolved.Attempt.HasWrapper {
		return "", false
	}
	wrapper := resolved.Attempt.Wrapper
	text := fmt.Sprintf("%s (exit code %d)", wrapper.Phase, wrapper.ExitCode)
	switch {
	case wrapper.Phase == "finished" && wrapper.ExitCode == 0:
		return green(text), true
	case wrapper.Phase == "finished":
		return red(text), true
	default:
		return yellow(text), true
	}
}

func exitCodeStatusText(exitCode int, text string) string {
	if exitCode == 0 {
		return green(text)
	}
	return red(text)
}

func writeJobDiagnoses(writer io.Writer, result model.JobResult) {
	switch {
	case result.DiagnosisStatus == model.DiagnosisNoMatch:
		fmt.Fprintf(writer, "%s no known rule matched\n  Next: %s\n", cyan("Diagnosis:"), noMatchDiagnosisNext)
	case result.DiagnosisStatus == model.DiagnosisUnavailable:
		fmt.Fprintf(writer, "%s unavailable: %s\n  Next: %s\n", cyan("Diagnosis:"), result.DiagnosisNote, unavailableDiagnosisNext)
	case len(result.Diagnoses) > 0:
		fmt.Fprintf(writer, "%s\n", cyan("Diagnosis:"))
		for _, diagnosis := range result.Diagnoses {
			fmt.Fprintf(writer, "  %s\n    Evidence: %s\n    Next: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
		}
	default:
		return
	}
	if diagnose.Outdated(result) {
		fmt.Fprintf(writer, "  Note: %s\n", outdatedDiagnosisNote)
	}
}

func readJSONCommand(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var job model.JobSpec
	if json.Unmarshal(data, &job) != nil {
		return ""
	}
	return strings.Join(job.Command, " ")
}

func readJobName(jobDir string) string {
	data, err := os.ReadFile(filepath.Join(jobDir, "name"))
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

func showRunLogs(writer io.Writer, paths state.ProjectPaths, runID string, failedOnly bool) int {
	runDir := filepath.Join(paths.RunsDir, runID)
	entries, err := os.ReadDir(runDir)
	if err != nil {
		printErrorf("failed to read run directory: %v", err)
		return 1
	}
	jobSpecs := loadRunJobSpecs(runDir)
	writeShowTargetHeaderWithMode(writer, paths, "run")
	fmt.Fprintf(writer, "%s %s\n\n", cyan("Run:"), runID)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jobID := entry.Name()
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		if err != nil {
			continue
		}

		attempt := jobstatus.ReadAttempt(jsonStore(), jobDir)
		status, statusOK := attempt.ExitCode, attempt.Finished()

		if failedOnly && (!statusOK || status == 0) {
			continue
		}

		name := readJobName(jobDir)
		if name == "" {
			name = jobSpecs[jobID].Name
		}
		command := readJSONCommand(filepath.Join(jobDir, commandJSONName))

		header := fmt.Sprintf("=== Job: %s", jobID)
		if name != "" {
			header += fmt.Sprintf(" (Name: %s)", name)
		}
		headerColor := cyan
		if statusOK {
			if status == 0 {
				header += fmt.Sprintf(" [Status: %d (success)]", status)
				headerColor = green
			} else {
				header += fmt.Sprintf(" [Status: %d (failed)]", status)
				headerColor = red
			}
		} else {
			header += " [Status: running]"
		}
		header += " ==="
		fmt.Fprintln(writer, headerColor(header))
		if jobSpecs[jobID].WorkingDirectory != "" {
			fmt.Fprintf(writer, "Working directory: %s\n", jobSpecs[jobID].WorkingDirectory)
		}
		fmt.Fprintf(writer, "Command: %s\n", command)
		fmt.Fprintf(writer, "Output path: %s\n", filepath.Join(jobDir, "output"))

		output, err := os.ReadFile(filepath.Join(jobDir, "output"))
		if err == nil && len(output) > 0 {
			fmt.Fprintln(writer, "--- Log Output ---")
			fmt.Fprint(writer, string(output))
			if !strings.HasSuffix(string(output), "\n") {
				fmt.Fprintln(writer)
			}
		} else {
			fmt.Fprintln(writer, "(No output log)")
		}
		fmt.Fprintln(writer)
	}

	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		seen[entry.Name()] = true
	}
	queue, queueErr := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if queueErr == nil {
		for _, command := range queue.Commands {
			if command.Array == nil {
				printCarriedForwardOutput(writer, paths, command.ID, command.Name, command.Command, command.WorkingDirectory, command.Origin, seen, failedOnly)
				continue
			}
			for _, task := range model.ArrayTaskIDs(command.Array) {
				taskID := fmt.Sprintf("%s-%d", command.ID, task)
				printCarriedForwardOutput(writer, paths, taskID, command.Name, command.Command, command.WorkingDirectory, command.TaskOrigins[taskID], seen, failedOnly)
			}
		}
	}
	return 0
}

// printCarriedForwardOutput prints a carried-forward job's output read from
// its origin run/job, if it was not itself re-executed in this run (i.e. it
// has no directory of its own here).
func printCarriedForwardOutput(writer io.Writer, paths state.ProjectPaths, id, name string, command []string, workingDirectory string, origin *model.JobOrigin, seen map[string]bool, failedOnly bool) {
	if seen[id] || origin == nil {
		return
	}
	if !state.IsValidPathElement(origin.RunID) || !state.IsValidPathElement(origin.JobID) {
		return
	}
	if failedOnly && origin.Status != "failed" {
		return
	}
	header := fmt.Sprintf("=== Job: %s", id)
	if name != "" {
		header += fmt.Sprintf(" (Name: %s)", name)
	}
	header += fmt.Sprintf(" [carried forward from run %s: %s] ===", origin.RunID, origin.Status)
	headerColor := cyan
	switch origin.Status {
	case "failed":
		headerColor = red
	case "success":
		headerColor = green
	}
	fmt.Fprintln(writer, headerColor(header))
	if workingDirectory != "" {
		fmt.Fprintf(writer, "Working directory: %s\n", workingDirectory)
	}
	fmt.Fprintf(writer, "Command: %s\n", strings.Join(command, " "))
	originRunDir, err := state.SafeJoin(paths.RunsDir, origin.RunID)
	if err != nil {
		return
	}
	originDir, err := state.SafeJoin(originRunDir, origin.JobID)
	if err != nil {
		return
	}
	fmt.Fprintf(writer, "Output path: %s\n", filepath.Join(originDir, "output"))
	output, err := os.ReadFile(filepath.Join(originDir, "output"))
	if err == nil && len(output) > 0 {
		fmt.Fprintln(writer, "--- Log Output ---")
		fmt.Fprint(writer, string(output))
		if !strings.HasSuffix(string(output), "\n") {
			fmt.Fprintln(writer)
		}
	} else {
		fmt.Fprintln(writer, "(No output log)")
	}
	fmt.Fprintln(writer)
}

func followJobLog(writer io.Writer, paths state.ProjectPaths, runID, jobID string) int {
	if !state.IsValidPathElement(runID) {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	if !state.IsValidPathElement(jobID) {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	jobDir, err := state.SafeJoin(runDir, jobID)
	if err != nil {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	outputPath := filepath.Join(jobDir, "output")
	output, err := os.ReadFile(outputPath)
	if err != nil {
		if origin := state.LoadRunOrigin(runDir, jobID); origin != nil {
			// Carried forward: it already finished under the origin run, so
			// there is nothing new to follow, just print its output once.
			fmt.Fprintf(writer, "%s carried forward from run %s (no re-execution)\n\n", cyan("Note:"), origin.RunID)
			originRunDir, pathErr := state.SafeJoin(paths.RunsDir, origin.RunID)
			originDir, jobErr := state.SafeJoin(originRunDir, origin.JobID)
			if pathErr != nil || jobErr != nil {
				return 0
			}
			originOutput, readErr := os.ReadFile(filepath.Join(originDir, "output"))
			if readErr == nil {
				_, _ = writer.Write(originOutput)
			}
			return 0
		}
		printErrorf("failed to read job output: %v", err)
		return 1
	}
	if _, err := writer.Write(output); err != nil {
		return 1
	}
	offset := int64(len(output))
	for {
		data, err := os.ReadFile(outputPath)
		if err != nil {
			if !os.IsNotExist(err) {
				printErrorf("failed to read job output: %v", err)
				return 1
			}
			continue
		}
		if len(data) > int(offset) {
			if _, err := writer.Write(data[offset:]); err != nil {
				return 1
			}
			offset = int64(len(data))
		}
		if jobstatus.ReadAttempt(jsonStore(), jobDir).Finished() {
			return 0
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func runResultAccepted(runDir, jobID string) bool {
	result, ok := loadRunResult(runDir, jobID)
	return ok && result.Accepted
}

// loadRunResult returns the job's result from the run's summary.json.
func loadRunResult(runDir, jobID string) (model.JobResult, bool) {
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return model.JobResult{}, false
	}
	result, ok := model.ResultsByID(summary.Results)[jobID]
	return result, ok
}

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
)

const (
	pagerLineLimit     = 24
	statusJSONName     = "status.json"
	runNotFoundMessage = "run %q not found"
	jobNotFoundMessage = "job %q not found in run %q"
)

type showSelectorTarget struct {
	waitTarget
	jobID     string
	fromQueue bool
	priority  int
}

const (
	showPriorityActive = iota
	showPriorityInterrupted
	showPriorityQueue
	showPriorityLatest
)

func showDefaultJobTargets(paths pathSet, selector string, byName bool) ([]showSelectorTarget, error) {
	state, stateRunID, err := inspectProjectRunState(paths)
	if err != nil {
		return nil, err
	}
	if state == projectRunning || state == projectInterrupted {
		target, found, err := findShowJobInRun(paths, stateRunID, selector, byName)
		if err != nil || !found {
			return nil, err
		}
		target.priority = showPriorityActive
		if state == projectInterrupted {
			target.priority = showPriorityInterrupted
		}
		return []showSelectorTarget{target}, nil
	}
	targets := make([]showSelectorTarget, 0, 2)
	queue, err := loadQueue(paths.queueFile)
	if err != nil {
		return nil, err
	}
	if len(queue.Commands) > 0 {
		jobID, found := findShowJobInQueue(queue, selector, byName)
		if found {
			targets = append(targets, showSelectorTarget{
				waitTarget: waitTarget{baseDir: paths.baseDir, projectName: paths.queueName},
				jobID:      jobID, fromQueue: true, priority: showPriorityQueue,
			})
		}
	}
	runID, err := selectRunID(paths, "")
	if err != nil {
		return targets, nil
	}
	target, found, err := findShowJobInRun(paths, runID, selector, byName)
	if err != nil || !found {
		return targets, err
	}
	target.priority = showPriorityLatest
	return append(targets, target), nil
}

func highestPriorityShowTargets(targets []showSelectorTarget) []showSelectorTarget {
	if len(targets) == 0 {
		return targets
	}
	bestPriority := targets[0].priority
	for _, target := range targets[1:] {
		if target.priority < bestPriority {
			bestPriority = target.priority
		}
	}
	best := make([]showSelectorTarget, 0, len(targets))
	for _, target := range targets {
		if target.priority == bestPriority {
			best = append(best, target)
		}
	}
	return best
}

func findShowJobInQueue(queue Queue, selector string, byName bool) (string, bool) {
	for _, job := range queueToJobs(queue.Commands) {
		if (byName && job.Name == selector) || (!byName && job.ID == selector) {
			return job.ID, true
		}
	}
	return "", false
}

func findShowJobInRun(paths pathSet, runID, selector string, byName bool) (showSelectorTarget, bool, error) {
	queue, err := loadQueue(filepath.Join(paths.runsDir, runID, "commands.json"))
	if err != nil {
		return showSelectorTarget{}, false, nil
	}
	jobID, found := findShowJobInQueue(queue, selector, byName)
	if !found {
		return showSelectorTarget{}, false, nil
	}
	return showSelectorTarget{
		waitTarget: waitTarget{baseDir: paths.baseDir, projectName: paths.queueName, runID: runID},
		jobID:      jobID,
	}, true, nil
}

func resolveShowJobTargets(cliBaseDir, cliProjectName, runID, selector string, byName bool) ([]showSelectorTarget, error) {
	if runID != "" {
		baseDir, projectName, err := resolveExistingRunTarget(cliBaseDir, cliProjectName, runID)
		if err != nil {
			return nil, err
		}
		paths, err := resolvePaths(baseDir, projectName)
		if err != nil {
			return nil, err
		}
		target, found, err := findShowJobInRun(paths, runID, selector, byName)
		if err != nil || !found {
			return nil, err
		}
		return []showSelectorTarget{target}, nil
	}
	return resolveJobTargets(cliBaseDir, cliProjectName, selector, byName, true)
}

func resolveJobTargets(cliBaseDir, cliProjectName, selector string, byName, includeQueue bool) ([]showSelectorTarget, error) {
	baseDir, _, err := resolveBaseDir(cliBaseDir)
	if err != nil {
		return nil, err
	}
	projects, err := projectNamesForRunName(baseDir, cliProjectName)
	if err != nil {
		return nil, err
	}
	targets := make([]showSelectorTarget, 0)
	for _, projectName := range projects {
		paths, err := resolvePaths(baseDir, projectName)
		if err != nil {
			return nil, err
		}
		if includeQueue {
			projectTargets, err := showDefaultJobTargets(paths, selector, byName)
			if err != nil {
				return nil, err
			}
			targets = append(targets, projectTargets...)
			continue
		}
		runID, err := selectRunID(paths, "")
		if err != nil {
			continue
		}
		target, found, err := findShowJobInRun(paths, runID, selector, byName)
		if err != nil {
			return nil, err
		}
		if found {
			target.priority = showPriorityLatest
			targets = append(targets, target)
		}
	}
	if includeQueue {
		return highestPriorityShowTargets(targets), nil
	}
	return targets, nil
}

func resolveLatestJobIDSelection(cliBaseDir, cliProjectName string, jobIDs []string) (showSelectorTarget, error) {
	var target showSelectorTarget
	for index, jobID := range jobIDs {
		targets, err := resolveJobTargets(cliBaseDir, cliProjectName, jobID, false, false)
		if err != nil {
			return showSelectorTarget{}, err
		}
		if len(targets) == 0 {
			return showSelectorTarget{}, fmt.Errorf("job %q not found in latest runs", jobID)
		}
		if len(targets) > 1 {
			return showSelectorTarget{}, fmt.Errorf("job %q is ambiguous across latest runs", jobID)
		}
		if index == 0 {
			target = targets[0]
		} else if targets[0].baseDir != target.baseDir || targets[0].projectName != target.projectName || targets[0].runID != target.runID {
			return showSelectorTarget{}, errors.New("job IDs resolve to different latest runs")
		}
	}
	return target, nil
}

func resolveShowSelector(cliBaseDir, cliProjectName, selector string) ([]showSelectorTarget, error) {
	runTargets, err := resolveRunNameTargets(cliBaseDir, cliProjectName, selector, false)
	if err != nil {
		return nil, err
	}
	targets := make([]showSelectorTarget, 0, len(runTargets))
	for _, target := range runTargets {
		targets = append(targets, showSelectorTarget{waitTarget: target})
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

func showJobNameTargets(cliBaseDir, cliProjectName, runID, jobName string) ([]showSelectorTarget, error) {
	return resolveShowJobTargets(cliBaseDir, cliProjectName, runID, jobName, true)
}

func applyShowSelectorTarget(target showSelectorTarget, basedir, projectName, runID, jobID *string, showQueue *bool) {
	*basedir, *projectName, *runID, *jobID = target.baseDir, target.projectName, target.runID, target.jobID
	if target.fromQueue {
		*showQueue = true
	}
}

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
	showBaseDirsList := cliBool(fs, "basedirs", false)
	masterdir := cliString(fs, "masterdir", "")
	showLogs := cliBool(fs, "logs", false)
	showFailedLogs := cliBool(fs, "failed-logs", false)
	followLogs := cliBool(fs, "follow", false)
	noPager := cliBool(fs, "no-pager", false)
	jsonOutput := cliBool(fs, "json", false)
	reportOutput := cliBool(fs, "report", false)
	if err := fs.Parse(args); err != nil {
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
		baseDir, projectName, resolvedRunID, resolvedJobID, err := resolveAttemptTarget(selector, *basedir, *queueNameOption, "")
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
			baseDir, projectName, err := resolveExistingRunTarget(*basedir, *queueNameOption, selector)
			if err != nil {
				printError(err)
				return 1
			}
			*basedir, *queueNameOption, *runIDOption = baseDir, projectName, location.RunID
			selector = ""
		}
	}
	if strings.HasPrefix(*jobIDOption, "att_") {
		attemptID = *jobIDOption
		baseDir, projectName, resolvedRunID, resolvedJobID, err := resolveAttemptTarget(*jobIDOption, *basedir, *queueNameOption, *runIDOption)
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
			printErrorf("job %q is ambiguous", *jobIDOption)
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
			printErrorf("selector %q is ambiguous", selector)
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
			printErrorf("job name %q is ambiguous", *jobNameOption)
			return 1
		}
		applyShowSelectorTarget(targets[0], basedir, queueNameOption, runIDOption, jobIDOption, showQueueOption)
	}
	if *reportOutput && (*showQueueOption || *showBaseDirsList || *showLogs || *showFailedLogs || *followLogs || *jsonOutput) {
		printError("--report cannot be combined with queue, list, log, follow, or JSON options")
		return 1
	}
	if *showQueueOption && (*runIDOption != "" || *failedOnly || *showLogs || *showFailedLogs || *followLogs) {
		printError("--queue cannot be combined with run, log, or filter options")
		return 1
	}
	if *showBaseDirsList {
		if *runIDOption != "" || *jobIDOption != "" || *failedOnly || *showLogs || *showFailedLogs || *followLogs || *jsonOutput {
			printError("--basedirs cannot be combined with project, run, job, log, filter, or JSON options")
			return 1
		}
		masterDir, err := resolveMasterDir(*masterdir)
		if err != nil {
			printErrorf("failed to resolve master directory: %v", err)
			return 1
		}
		return showBaseDirs(masterDir)
	}
	if *queueNameOption == "" && *runIDOption == "" && *jobIDOption == "" && *jobNameOption == "" &&
		!(*showQueueOption || *showBaseDirsList || *failedOnly || *showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput) {
		return showAllProjects(*basedir, *masterdir)
	}
	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	projectOverview := (*queueNameOption != "") && *runIDOption == "" && *jobIDOption == "" && *jobNameOption == "" && !*showQueueOption && !*showBaseDirsList && !*showLogs && !*showFailedLogs && !*followLogs && !*jsonOutput && !*reportOutput && !*failedOnly
	if projectOverview {
		return showProjectOverview(paths)
	}
	if *showQueueOption {
		queue, err := loadQueue(paths.queueFile)
		if err != nil {
			printErrorf("failed to load queue: %v", err)
			return 1
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
		return showQueue(paths, queue)
	}
	selectedRunID := *runIDOption
	if selectedRunID == "" {
		state, stateRunID, err := inspectProjectRunState(paths)
		if err != nil {
			printErrorf("failed to check project state: %v", err)
			return 1
		}
		if state == projectRunning {
			selectedRunID = stateRunID
		} else if state == projectInterrupted {
			selectedRunID = stateRunID
			printInterruptedRunNotice(paths, stateRunID)
		} else {
			queue, err := loadQueue(paths.queueFile)
			if err != nil {
				printErrorf("failed to load queue: %v", err)
				return 1
			}
			if len(queue.Commands) > 0 {
				if *showLogs || *showFailedLogs || *failedOnly {
					printError("logs and failed filters require --run-id")
					return 1
				}
				if *reportOutput {
					printError("--report requires --run-id when the current queue is not empty")
					return 1
				}
				if *jobIDOption != "" {
					return showQueueJob(paths, queue, *jobIDOption)
				}
				if *jsonOutput {
					return showQueueJSON(paths, queue)
				}
				return showQueue(paths, queue)
			}
			if countProjectRuns(paths.runsDir) == 0 {
				printErrorf("WARNING: project %q has no runs or queued jobs; nothing to show", paths.queueName)
				writeShowTargetHeaderWithMode(os.Stdout, paths, "project")
				fmt.Println("\nNo runs or queued jobs found.")
				return 0
			}
		}
	}
	runID, err := selectRunID(paths, selectedRunID)
	if err != nil {
		printError(err)
		return 1
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
		running, err := isRunning(paths.lockFile)
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
	return showRun(paths, runID, *failedOnly)
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
	BaseDir  string      `json:"base_dir"`
	Project  string      `json:"project_name"`
	RunID    string      `json:"run_id"`
	RunDir   string      `json:"run_dir"`
	Summary  *RunSummary `json:"summary,omitempty"`
	Commands Queue       `json:"commands"`
}

type showJobCounts struct {
	success int
	failed  int
	blocked int
	running int
	pending int
}

func showRunJSON(paths pathSet, runID string) int {
	result := showJSON{BaseDir: paths.baseDir, Project: paths.queueName, RunID: runID, RunDir: filepath.Join(paths.runsDir, runID)}
	if summary, err := loadRunSummary(filepath.Join(result.RunDir, "summary.json")); err == nil {
		result.Summary = &summary
	} else if !os.IsNotExist(err) {
		printErrorf("failed to read summary: %v", err)
		return 1
	}
	commands, err := loadQueue(filepath.Join(result.RunDir, "commands.json"))
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

func showQueueJSON(paths pathSet, queue Queue) int {
	result := showJSON{BaseDir: paths.baseDir, Project: paths.queueName, Commands: queue}
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

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func selectRunID(paths pathSet, requested string) (string, error) {
	if requested == "latest" {
		requested = ""
	}
	if requested != "" {
		if !isValidPathElement(requested) {
			return "", fmt.Errorf(runNotFoundMessage, requested)
		}
		if _, err := os.Stat(filepath.Join(paths.runsDir, requested)); err != nil {
			return "", fmt.Errorf(runNotFoundMessage, requested)
		}
		return requested, nil
	}
	meta, err := loadMeta(paths.metaFile)
	if err != nil {
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	if meta.LastRunID != "" {
		if info, err := os.Stat(filepath.Join(paths.runsDir, meta.LastRunID)); err == nil && info.IsDir() {
			return meta.LastRunID, nil
		}
	}
	{
		entries, err := os.ReadDir(paths.runsDir)
		if err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("project %q has no runs (runs_dir=%s)", paths.queueName, paths.runsDir)
			}
			return "", fmt.Errorf("failed to read runs: %w", err)
		}
		runIDs := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				runIDs = append(runIDs, entry.Name())
			}
		}
		if len(runIDs) == 0 {
			return "", fmt.Errorf("project %q has no runs (runs_dir=%s)", paths.queueName, paths.runsDir)
		}
		sort.Slice(runIDs, func(i, j int) bool {
			left, _ := os.Stat(filepath.Join(paths.runsDir, runIDs[i]))
			right, _ := os.Stat(filepath.Join(paths.runsDir, runIDs[j]))
			return left.ModTime().After(right.ModTime())
		})
		return runIDs[0], nil
	}
}

func writeShowTargetHeader(writer io.Writer, paths pathSet) {
	writeShowTargetHeaderWithMode(writer, paths, "")
}

func writeShowTargetHeaderWithMode(writer io.Writer, paths pathSet, mode string) {
	if mode != "" {
		fmt.Fprintf(writer, "%s\n", cyan("=== SHOW MODE: "+showViewLabel(mode)+" ==="))
	}
	fmt.Fprintf(writer, "%s %s\n%s %s\n", cyan("Base directory:"), paths.baseDir, cyan("Project:"), paths.queueName)
	if queue, err := loadQueue(paths.queueFile); err == nil {
		fmt.Fprintf(writer, "%s %s (%d jobs)\n", cyan("Queue:"), paths.queueFile, len(queue.Commands))
	} else {
		fmt.Fprintf(writer, "%s %s\n", cyan("Queue:"), paths.queueFile)
	}
	if state, _, err := inspectProjectRunState(paths); err == nil {
		fmt.Fprintf(writer, "%s %s\n", cyan("Project state:"), projectStateName(state))
	}
	if response, err := sendServerRequest(paths.baseDir, serverRequest{Op: "ping"}); err == nil && response.OK {
		fmt.Fprintf(writer, "%s running (pid=%d)\n", cyan("Runner server:"), response.PID)
	} else {
		fmt.Fprintf(writer, "%s stopped\n", cyan("Runner server:"))
	}
	fmt.Fprintf(writer, "%s %d\n", cyan("Runs:"), countProjectRuns(paths.runsDir))
	if configPath := effectiveConfigPath(paths.baseDir, paths.queueName); configPath != "" {
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

func printInterruptedRunNotice(paths pathSet, runID string) {
	fmt.Printf("%s\n", yellow(fmt.Sprintf("Run %s appears to have been interrupted.", runID)))
	fmt.Printf("Recover the queue before modifying or running it:\n  rotari unlock --basedir %s --project-name %s --run-id %s\n\n",
		shellQuote(paths.baseDir), shellQuote(paths.queueName), shellQuote(runID))
}

func showRun(paths pathSet, runID string, failedOnly bool) int {
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	var summary RunSummary
	summaryData, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
	summaryOK := err == nil
	if err == nil {
		if err := json.Unmarshal(summaryData, &summary); err != nil {
			printErrorf("failed to read summary: %v", err)
			return 1
		}
	}

	writeShowTargetHeaderWithMode(os.Stdout, paths, "run")
	fmt.Printf("%s %s\n", cyan("Run:"), formatRunLabel(runID, summary.RunName))
	if summary.RunName != "" {
		fmt.Printf("%s %s\n", cyan("Run name:"), summary.RunName)
	}
	fmt.Printf("%s %s\n", cyan("Directory:"), runDir)
	if summaryOK {
		fmt.Printf("%s %s\n%s %s\n%s %s\n%s %d\n", cyan("Status:"), summary.Status, cyan("Started:"), formatDisplayTimestamp(summary.StartedAt), cyan("Finished:"), formatDisplayTimestamp(summary.FinishedAt), cyan("Exit code:"), summary.ExitCode)
	}
	fmt.Printf("%s %s\n", cyan("Output directory:"), runDir)
	queue, queueErr := loadQueue(paths.queueFile)
	if queueErr == nil && len(queue.Commands) > 0 {
		if diff, err := compareQueueWithRun(paths.queueFile, filepath.Join(runDir, "commands.json")); err == nil && diff.HasChanges() {
			fmt.Printf("\n%s\n", yellow("Queue differs from this run:"))
			fmt.Printf("  Added: %d\n  Removed: %d\n  Changed: %d\n", diff.Added, diff.Removed, diff.Changed)
		}
	}
	jobSpecs := loadRunJobSpecs(runDir)
	runQueue, runQueueErr := loadQueue(filepath.Join(runDir, "commands.json"))
	runActive := runIsActive(paths, runID)
	jobCounts := showJobCounts{}
	resultByID := make(map[string]JobResult, len(summary.Results))
	for _, result := range summary.Results {
		resultByID[result.ID] = result
	}
	jobIDs := make([]string, 0, len(runQueue.Commands))
	originByID := make(map[string]*JobOrigin, len(runQueue.Commands))
	if runQueueErr == nil {
		for _, job := range queueToJobs(runQueue.Commands) {
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
	changeHints := make([]JobSpec, 0)
	fmt.Printf("%s\n", cyan(fmt.Sprintf("%-12s %-42s %-6s %-15s %-20s %-10s %-30s %-24s %-24s %-24s %s", "JOB ID", "LATEST ATTEMPT", "TASK", "NAME", "DEPENDS ON", "STATUS", "EXECUTOR", "SUBMITTED", "FINISHED", "HOSTS", "COMMAND")))
	for _, jobID := range jobIDs {
		jobDir, err := latestAttemptJobDir(runDir, jobID)
		if err != nil {
			continue
		}
		jobSpec := jobSpecs[jobID]
		latestAttemptLabel := "-"
		if value, readErr := latestAttemptID(runDir, jobID); readErr == nil && value != "" {
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
		taskText := "-"
		if jobSpec.ArrayTaskID != nil {
			taskText = strconv.Itoa(*jobSpec.ArrayTaskID)
		}
		dependsOn := strings.Join(jobSpec.DependsOn, ",")
		if dependsOn == "" {
			dependsOn = "-"
		}
		status, statusOK := readJobStatus(filepath.Join(jobDir, "status"))
		blocked := false
		if !statusOK {
			if slurm, ok := loadSlurmStatus(filepath.Join(jobDir, statusJSONName)); ok && jobStatusTerminal(slurm) {
				status = slurm.ExitCode
				statusOK = true
			}
		}
		if !statusOK {
			if schedulerState, ok := loadTerminalSchedulerState(jobDir); ok {
				status = schedulerState
				statusOK = true
			}
		}
		if !statusOK {
			if result, ok := resultByID[jobSpec.ID]; ok {
				status = result.ExitCode
				statusOK = true
				blocked = strings.HasPrefix(result.Error, "blocked")
			}
		}
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
		if failedOnly && (!statusOK || status == 0) {
			continue
		}
		if statusOK && status != 0 {
			changeHints = append(changeHints, jobSpec)
		}
		hosts := "-"
		if result, ok := resultByID[jobSpec.ID]; ok && len(result.Hosts) > 0 {
			hosts = strings.Join(result.Hosts, ",")
		} else if slurm, ok := loadSlurmStatus(filepath.Join(jobDir, statusJSONName)); ok && len(slurm.Hosts) > 0 {
			hosts = strings.Join(slurm.Hosts, ",")
		}
		command := readJSONCommand(filepath.Join(jobDir, commandJSONName))
		if command == "" {
			command = strings.Join(jobSpec.Command, " ")
		}
		submittedAt, finishedAt := readShowJobTimestamps(runDir, jobID, originByID[jobID])
		submittedAt = formatDisplayTimestamp(submittedAt)
		finishedAt = formatDisplayTimestamp(finishedAt)
		if statusOK {
			statusText := green(strconv.Itoa(status))
			if blocked {
				statusText = yellow("blocked")
			} else if status != 0 {
				statusText = red(strconv.Itoa(status))
			}
			fmt.Printf("%-12s %-42s %-6s %-15s %-20s %-10s %-30s %-24s %-24s %-24s %s\n", jobID, latestAttemptLabel, taskText, name, dependsOn, statusText, executorText, submittedAt, finishedAt, hosts, command)
		} else {
			fmt.Printf("%-12s %-42s %-6s %-15s %-20s %-10s %-30s %-24s %-24s %-24s %s\n", jobID, latestAttemptLabel, taskText, name, dependsOn, yellow("running"), executorText, submittedAt, finishedAt, hosts, command)
		}
	}
	fmt.Printf("\n%s success: %d, failed: %d, blocked: %d, running: %d, pending: %d\n", cyan("Job status:"), jobCounts.success, jobCounts.failed, jobCounts.blocked, jobCounts.running, jobCounts.pending)
	printChangeHints(paths, runID, runQueue, changeHints)
	printFailedLogHints(runID, changeHints, resultByID)
	fmt.Printf("\n%s\n  rotari delete --run-id %s\n", cyan("To delete this run's saved logs:"), runID)
	return 0
}

func runIsActive(paths pathSet, runID string) bool {
	running, err := isRunning(paths.lockFile)
	if err != nil || !running {
		return false
	}
	lock, err := loadLockInfo(paths.lockFile)
	return err == nil && lock.RunID == runID
}

func printFailedLogHints(runID string, failedJobs []JobSpec, results map[string]JobResult) {
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

func printChangeHints(paths pathSet, runID string, queue Queue, jobs []JobSpec) {
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
	fmt.Printf("    rotari retry --basedir %s --project-name %s\n", paths.baseDir, paths.queueName)
}

func showQueue(paths pathSet, queue Queue) int {
	writeShowTargetHeaderWithMode(os.Stdout, paths, "queue")
	fmt.Println("\n" + cyan("Queue:"))
	return showQueueContent(paths, queue)
}

func showQueueContent(paths pathSet, queue Queue) int {
	jobs := queueToJobs(queue.Commands)
	originByID := queueOriginsByJobID(queue)
	fmt.Printf("%s\n", cyan(fmt.Sprintf("%-12s %-6s %-15s %-20s %-30s %-24s %-12s %s", "JOB ID", "TASK", "NAME", "DEPENDS ON", "EXECUTOR", "SOURCE RUN", "SOURCE STATUS", "COMMAND")))
	for _, job := range jobs {
		name := job.Name
		if name == "" {
			name = "-"
		}
		dependsOn := strings.Join(job.DependsOn, ",")
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
		fmt.Printf("%-12s %-6s %-15s %-20s %-30s %-24s %-12s %s\n", job.ID, taskText, name, dependsOn, executorText, sourceRun, sourceStatus, strings.Join(job.Command, " "))
	}
	fmt.Printf("\n%s\n  rotari run -b %s -p %s\n", cyan("To execute these jobs:"), shellQuote(paths.baseDir), shellQuote(paths.queueName))
	return 0
}

func queueOriginsByJobID(queue Queue) map[string]*JobOrigin {
	origins := make(map[string]*JobOrigin)
	for _, command := range queue.Commands {
		if command.Origin != nil {
			origins[command.ID] = command.Origin
		}
		for taskID, origin := range command.TaskOrigins {
			origins[taskID] = origin
		}
	}
	return origins
}

func showQueueJob(paths pathSet, queue Queue, jobID string) int {
	if !isValidPathElement(jobID) {
		printErrorf("job %q not found in current queue", jobID)
		return 1
	}
	for _, job := range queueToJobs(queue.Commands) {
		if job.ID != jobID {
			continue
		}
		writeShowTargetHeaderWithMode(os.Stdout, paths, "queue job")
		fmt.Printf("%s %s\n", cyan("Job:"), job.ID)
		if job.Name != "" {
			fmt.Printf("%s %s\n", cyan("Name:"), job.Name)
		}
		if job.ArrayTaskID != nil {
			fmt.Printf("%s %d (range %d-%d)\n", cyan("Array task:"), *job.ArrayTaskID, job.ArrayFirst, job.ArrayLast)
		}
		fmt.Printf("%s %s\n", cyan("Executor:"), queueExecutorText(queue, job))
		if len(job.DependsOn) > 0 {
			fmt.Printf("%s %s\n", cyan("Depends on:"), strings.Join(job.DependsOn, ", "))
		}
		if job.WorkingDirectory != "" {
			fmt.Printf("%s %s\n", cyan("Working directory:"), job.WorkingDirectory)
		}
		fmt.Printf("%s %s\n", cyan("Command:"), strings.Join(job.Command, " "))
		return 0
	}
	printErrorf("job %q not found in current queue", jobID)
	return 1
}

func queueExecutorText(queue Queue, job JobSpec) string {
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
	current, err := loadQueue(queuePath)
	if err != nil {
		return queueRunDiff{}, err
	}
	runData, err := os.ReadFile(runCommandsPath)
	if err != nil {
		return queueRunDiff{}, err
	}
	var runQueue Queue
	if err := json.Unmarshal(runData, &runQueue); err != nil {
		return queueRunDiff{}, err
	}

	currentJobs := queueToJobs(current.Commands)
	runJobs := queueToJobs(runQueue.Commands)
	currentByID := make(map[string]JobSpec, len(currentJobs))
	for _, job := range currentJobs {
		currentByID[job.ID] = job
	}
	runByID := make(map[string]JobSpec, len(runJobs))
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

func sameJobSpec(left, right JobSpec) bool {
	if left.ID != right.ID || left.Name != right.Name || left.WorkingDirectory != right.WorkingDirectory || left.Executor != right.Executor || left.ArrayGroup != right.ArrayGroup || left.ArrayFirst != right.ArrayFirst || left.ArrayLast != right.ArrayLast {
		return false
	}
	if (left.ArrayTaskID == nil) != (right.ArrayTaskID == nil) || (left.ArrayTaskID != nil && *left.ArrayTaskID != *right.ArrayTaskID) {
		return false
	}
	if !slicesEqual(left.Command, right.Command) || !slicesEqual(left.ExecutorOptions, right.ExecutorOptions) || !slicesEqual(left.DependsOn, right.DependsOn) {
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

func showRuns(paths pathSet) int {
	return showRunsWithHint(paths, true)
}

func showRunsOverview(paths pathSet) int {
	return showRunsWithMode(paths, false, "project")
}

func showRunsWithHint(paths pathSet, showHint bool) int {
	return showRunsWithMode(paths, showHint, "runs")
}

func showRunsWithMode(paths pathSet, showHint bool, mode string) int {
	entries, err := os.ReadDir(paths.runsDir)
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
		runDir := filepath.Join(paths.runsDir, runID)
		info, err := entry.Info()
		modTime := int64(0)
		if err == nil {
			modTime = info.ModTime().UnixNano()
		}

		r := runInfo{id: runID, modTime: modTime}
		summaryData, err := os.ReadFile(filepath.Join(runDir, "summary.json"))
		if err == nil {
			var summary RunSummary
			if json.Unmarshal(summaryData, &summary) == nil {
				r.name = summary.RunName
				r.startedAt = summary.StartedAt
				r.finishedAt = summary.FinishedAt
				r.exitCode = summary.ExitCode
				r.status = summary.Status
				if r.status == "" {
					r.status = runStatus(summary.ExitCode)
				}
				r.hasSummary = true
			}
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
	fmt.Printf("%s %s\n", cyan("Runs directory:"), paths.runsDir)
	fmt.Println("\n" + cyan("Runs:"))
	fmt.Printf("%s\n", cyan(fmt.Sprintf("%-36s %-24s %-12s %-12s %-24s %-24s", "RUN ID", "NAME", "STATUS", "EXIT CODE", "STARTED", "FINISHED")))
	for _, r := range runs {
		started := r.startedAt
		if started == "" {
			started = "-"
		}
		started = formatDisplayTimestamp(started)
		finished := r.finishedAt
		if finished == "" {
			finished = "-"
		}
		finished = formatDisplayTimestamp(finished)
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

func showProjectOverview(paths pathSet) int {
	if code := showRunsOverview(paths); code != 0 {
		return code
	}
	queue, err := loadQueue(paths.queueFile)
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
	return showQueueContent(paths, queue)
}

func showProjects(baseDir string) int {
	return showProjectsForBaseDirs([]string{baseDir})
}

func showAllProjects(cliBaseDir, cliMasterDir string) int {
	if cliBaseDir != "" {
		baseDir, _, err := resolveBaseDir(cliBaseDir)
		if err != nil {
			printErrorf("failed to resolve state directory: %v", err)
			return 1
		}
		return showProjectsForBaseDirs([]string{baseDir})
	}
	masterDir, err := resolveMasterDir(cliMasterDir)
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
	if current, _, err := resolveBaseDir(""); err == nil && !seen[current] {
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
			if !entry.IsDir() || !isValidProjectName(entry.Name()) {
				continue
			}
			paths, err := resolvePaths(baseDir, entry.Name())
			if err != nil {
				printErrorf("failed to resolve project %q: %v", entry.Name(), err)
				return 1
			}
			queue, err := loadQueue(paths.queueFile)
			if err != nil {
				printErrorf("failed to load queue for project %q: %v", entry.Name(), err)
				return 1
			}
			state, _, err := inspectProjectRunState(paths)
			if err != nil {
				printErrorf("failed to check project %q state: %v", entry.Name(), err)
				return 1
			}
			meta, err := loadMeta(paths.metaFile)
			if err != nil {
				printErrorf("failed to load project %q metadata: %v", entry.Name(), err)
				return 1
			}
			lastRun := meta.LastRunID
			if lastRun == "" {
				lastRun = "-"
			}
			projects = append(projects, projectInfo{baseDir: baseDir, name: entry.Name(), queued: len(queue.Commands), runs: countProjectRuns(paths.runsDir), state: projectStateName(state), lastRun: lastRun})
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

func projectStateName(state projectRunState) string {
	switch state {
	case projectRunning:
		return "running"
	case projectInterrupted:
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

func jobStatusTerminal(status slurmStatus) bool {
	return status.FinishedAt != "" || slurmStatusTerminal(status.Phase)
}

func loadTerminalSchedulerState(jobDir string) (int, bool) {
	state := strings.ToLower(loadSchedulerStatus(jobDir))
	switch state {
	case "completed", "complete", "success", "succeeded":
		return 0, true
	case "failed", "cancelled", "canceled", "timeout", "out_of_memory", "oom":
		return 1, true
	default:
		return 0, false
	}
}

func slurmStatusTerminal(phase string) bool {
	switch phase {
	case "finished", "completed", "failed", "cancelled", "timeout", "out_of_memory", "unknown":
		return true
	default:
		return false
	}
}

func readSubmittedAt(runDir, jobID string) string {
	jobDir, err := latestAttemptJobDir(runDir, jobID)
	if err != nil {
		return "-"
	}
	data, err := os.ReadFile(filepath.Join(jobDir, "submitted_at"))
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	data, err = os.ReadFile(filepath.Join(jobDir, "job.json"))
	if err != nil {
		return "-"
	}
	var metadata slurmJobMetadata
	if json.Unmarshal(data, &metadata) != nil || metadata.SubmittedAt == "" {
		return "-"
	}
	return metadata.SubmittedAt
}

func readFinishedAt(runDir, jobID string) string {
	jobDir, err := latestAttemptJobDir(runDir, jobID)
	if err != nil {
		return "-"
	}
	data, err := os.ReadFile(filepath.Join(jobDir, "finished_at"))
	if err == nil {
		return strings.TrimSpace(string(data))
	}
	data, err = os.ReadFile(filepath.Join(jobDir, statusJSONName))
	if err != nil {
		return "-"
	}
	var status slurmStatus
	if json.Unmarshal(data, &status) != nil || status.FinishedAt == "" {
		return "-"
	}
	return status.FinishedAt
}

func readShowJobTimestamps(runDir, jobID string, origin *JobOrigin) (string, string) {
	submittedAt := readSubmittedAt(runDir, jobID)
	finishedAt := readFinishedAt(runDir, jobID)
	if origin == nil {
		return submittedAt, finishedAt
	}
	if submittedAt == "-" {
		submittedAt = origin.SubmittedAt
		if submittedAt == "" && isValidPathElement(origin.RunID) {
			submittedAt = readSubmittedAt(filepath.Join(filepath.Dir(runDir), origin.RunID), origin.JobID)
		}
	}
	if finishedAt == "-" {
		finishedAt = origin.FinishedAt
		if finishedAt == "" && isValidPathElement(origin.RunID) {
			finishedAt = readFinishedAt(filepath.Join(filepath.Dir(runDir), origin.RunID), origin.JobID)
		}
	}
	return submittedAt, finishedAt
}

func loadRunJobSpecs(runDir string) map[string]JobSpec {
	specs := make(map[string]JobSpec)
	data, err := os.ReadFile(filepath.Join(runDir, "commands.json"))
	if err == nil {
		var queue Queue
		if json.Unmarshal(data, &queue) == nil {
			for _, job := range queueToJobs(queue.Commands) {
				specs[job.ID] = job
			}
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
		jobDir, err := latestAttemptJobDir(runDir, entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(jobDir, commandJSONName))
		if err != nil {
			continue
		}
		var job JobSpec
		if json.Unmarshal(data, &job) == nil {
			specs[entry.Name()] = job
		}
	}
	return specs
}

// loadRunOrigin returns the Origin recorded for jobID in runDir's
// commands.json, if any. Carried-forward jobs (see planRerunSelection) are
// not re-executed, so their output only exists under the origin run/job.
func loadRunOrigin(runDir, jobID string) *JobOrigin {
	queue, err := loadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil
	}
	for _, command := range queue.Commands {
		if command.ID == jobID {
			return command.Origin
		}
		if origin, ok := command.TaskOrigins[jobID]; ok {
			return origin
		}
	}
	return nil
}

func showJob(writer io.Writer, paths pathSet, runID, jobID string) int {
	return showJobAttempt(writer, paths, runID, jobID, "")
}

func listAttemptIDs(runDir, jobID string) []string {
	jobDir, err := validatedJobDir(runDir, jobID)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(jobDir, "attempts"))
	if err != nil {
		return nil
	}
	attempts := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && isValidPathElement(entry.Name()) {
			attempts = append(attempts, entry.Name())
		}
	}
	sort.SliceStable(attempts, func(left, right int) bool {
		leftPayload, leftErr := decodeAttemptID(attempts[left])
		rightPayload, rightErr := decodeAttemptID(attempts[right])
		if leftErr == nil && rightErr == nil && leftPayload.Number != rightPayload.Number {
			return leftPayload.Number < rightPayload.Number
		}
		return attempts[left] < attempts[right]
	})
	return attempts
}

func showJobAttempt(writer io.Writer, paths pathSet, runID, jobID, attemptID string) int {
	if !isValidPathElement(runID) {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	if !isValidPathElement(jobID) {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	jobDir, err := latestAttemptJobDir(filepath.Join(paths.runsDir, runID), jobID)
	if attemptID != "" {
		jobDir, err = specificAttemptJobDir(filepath.Join(paths.runsDir, runID), jobID, attemptID)
	}
	if err != nil {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	runDir := filepath.Join(paths.runsDir, runID)
	if info, err := os.Stat(jobDir); err != nil || !info.IsDir() {
		if origin := loadRunOrigin(runDir, jobID); origin != nil {
			fmt.Fprintf(writer, "%s carried forward from run %s (no re-execution)\n\n", cyan("Note:"), origin.RunID)
			return showJobAttempt(writer, paths, origin.RunID, origin.JobID, "")
		}
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	writeShowTargetHeaderWithMode(writer, paths, "run job")
	fmt.Fprintf(writer, "%s %s\n%s %s\n", cyan("Run:"), runID, cyan("Job:"), jobID)
	jobSpecs := loadRunJobSpecs(runDir)
	selectedAttemptID := attemptID
	if selectedAttemptID == "" {
		selectedAttemptID, _ = latestAttemptID(runDir, jobID)
	}
	if selectedAttemptID != "" {
		fmt.Fprintf(writer, "%s %s\n", cyan("Attempt ID:"), selectedAttemptID)
	}
	if attempts := listAttemptIDs(runDir, jobID); len(attempts) > 0 {
		latestAttemptLabel, _ := latestAttemptID(runDir, jobID)
		fmt.Fprintln(writer, cyan("Attempts:"))
		for _, listedAttemptID := range attempts {
			labels := make([]string, 0, 2)
			if listedAttemptID == latestAttemptLabel {
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
	executor, options := jobSpecs[jobID].Executor, jobSpecs[jobID].ExecutorOptions
	if executor == "" {
		executor = "local"
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Executor:"), executor)
	if len(options) > 0 {
		fmt.Fprintf(writer, "%s %s\n", cyan("Executor options:"), strings.Join(options, " "))
	}
	if dependencies := jobSpecs[jobID].DependsOn; len(dependencies) > 0 {
		fmt.Fprintf(writer, "%s %s\n", cyan("Depends on:"), strings.Join(dependencies, ", "))
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Submitted:"), formatDisplayTimestamp(readSubmittedAt(runDir, jobID)))
	fmt.Fprintf(writer, "%s %s\n", cyan("Finished:"), formatDisplayTimestamp(readFinishedAt(runDir, jobID)))
	if summary, err := loadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		for _, result := range summary.Results {
			if result.ID == jobSpecs[jobID].ID {
				hosts := strings.Join(result.Hosts, ",")
				if hosts == "" {
					hosts = "-"
				}
				fmt.Fprintf(writer, "%s %s\n", cyan("Hosts:"), hosts)
				break
			}
		}
	}
	if status, ok := readJobStatus(filepath.Join(jobDir, "status")); ok {
		if status == 0 {
			fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), green(strconv.Itoa(status)))
		} else {
			fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), red(strconv.Itoa(status)))
		}
	} else if slurm, ok := loadSlurmStatus(filepath.Join(jobDir, statusJSONName)); ok {
		status := fmt.Sprintf("Status: %s (exit code %d)", slurm.Phase, slurm.ExitCode)
		if slurm.Phase == "finished" && slurm.ExitCode == 0 {
			fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), green(strings.TrimPrefix(status, "Status: ")))
		} else if slurm.Phase == "finished" {
			fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), red(strings.TrimPrefix(status, "Status: ")))
		} else {
			fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), yellow(strings.TrimPrefix(status, "Status: ")))
		}
	} else {
		summary, err := loadRunSummary(filepath.Join(runDir, "summary.json"))
		if err == nil {
			for _, result := range summary.Results {
				if result.ID != jobSpecs[jobID].ID {
					continue
				}
				switch {
				case strings.HasPrefix(result.Error, "blocked"):
					fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), yellow("blocked (dependency failed)"))
				case result.Error != "":
					// No status/status.json file was ever written for this job (e.g. the
					// scheduler and its accounting were both unreachable), so this is the
					// only place the reason surfaces.
					fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), red(fmt.Sprintf("%d (%s)", result.ExitCode, result.Error)))
				case result.ExitCode == 0:
					fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), green(strconv.Itoa(result.ExitCode)))
				default:
					fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), red(strconv.Itoa(result.ExitCode)))
				}
			}
		}
	}
	command := readJSONCommand(filepath.Join(jobDir, commandJSONName))
	if summary, err := loadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		for _, result := range summary.Results {
			if result.ID == jobSpecs[jobID].ID {
				writeJobDiagnoses(writer, result.Diagnoses)
				break
			}
		}
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Command:"), command)
	fmt.Fprintf(writer, "%s %s\n\n", cyan("Output:"), filepath.Join(jobDir, "output"))
	output, err := os.ReadFile(filepath.Join(jobDir, "output"))
	if err == nil {
		fmt.Fprint(writer, string(output))
	}
	return 0
}

func writeJobDiagnoses(writer io.Writer, diagnoses []ruleDiagnosis) {
	if len(diagnoses) == 0 {
		return
	}
	fmt.Fprintf(writer, "%s\n", cyan("Diagnosis:"))
	for _, diagnosis := range diagnoses {
		fmt.Fprintf(writer, "  %s\n    Evidence: %s\n    Next: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
	}
}

func readJobStatus(path string) (int, bool) {
	data, err := os.ReadFile(path) // NOSONAR: callers pass paths built from validated run/job IDs.
	if err != nil {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return value, err == nil
}

func readJSONCommand(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var job JobSpec
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

func showRunLogs(writer io.Writer, paths pathSet, runID string, failedOnly bool) int {
	runDir := filepath.Join(paths.runsDir, runID)
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
		jobDir, err := latestAttemptJobDir(runDir, jobID)
		if err != nil {
			continue
		}

		status, statusOK := readJobStatus(filepath.Join(jobDir, "status"))
		if !statusOK {
			if slurm, ok := loadSlurmStatus(filepath.Join(jobDir, statusJSONName)); ok && jobStatusTerminal(slurm) {
				status = slurm.ExitCode
				statusOK = true
			}
		}

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
	queue, queueErr := loadQueue(filepath.Join(runDir, "commands.json"))
	if queueErr == nil {
		for _, command := range queue.Commands {
			if command.Array == nil {
				printCarriedForwardOutput(writer, paths, command.ID, command.Name, command.Command, command.WorkingDirectory, command.Origin, seen, failedOnly)
				continue
			}
			for _, task := range arrayTaskIDs(command.Array) {
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
func printCarriedForwardOutput(writer io.Writer, paths pathSet, id, name string, command []string, workingDirectory string, origin *JobOrigin, seen map[string]bool, failedOnly bool) {
	if seen[id] || origin == nil {
		return
	}
	if !isValidPathElement(origin.RunID) || !isValidPathElement(origin.JobID) {
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
	originRunDir, err := validatedRunDir(paths, origin.RunID)
	if err != nil {
		return
	}
	originDir, err := validatedJobDir(originRunDir, origin.JobID)
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

func followJobLog(writer io.Writer, paths pathSet, runID, jobID string) int {
	if !isValidPathElement(runID) {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	if !isValidPathElement(jobID) {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	jobDir, err := validatedJobDir(runDir, jobID)
	if err != nil {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	outputPath := filepath.Join(jobDir, "output")
	output, err := os.ReadFile(outputPath)
	if err != nil {
		if origin := loadRunOrigin(runDir, jobID); origin != nil {
			// Carried forward: it already finished under the origin run, so
			// there is nothing new to follow, just print its output once.
			fmt.Fprintf(writer, "%s carried forward from run %s (no re-execution)\n\n", cyan("Note:"), origin.RunID)
			originRunDir, pathErr := validatedRunDir(paths, origin.RunID)
			originDir, jobErr := validatedJobDir(originRunDir, origin.JobID)
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
		statusPath := filepath.Join(jobDir, "status")
		status, statusOK := readJobStatus(statusPath)
		if !statusOK {
			if slurm, ok := loadSlurmStatus(filepath.Join(jobDir, statusJSONName)); ok && jobStatusTerminal(slurm) {
				status = slurm.ExitCode
				statusOK = true
			}
		}
		if statusOK && status != 0 {
			return 0
		}
		if statusOK && status == 0 {
			return 0
		}
		time.Sleep(250 * time.Millisecond)
	}
}

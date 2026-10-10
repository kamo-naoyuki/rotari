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
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/report"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
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
		baseDir, projectName, err := resolveCLIExistingRun(cliBaseDir, cliProjectName, runID)
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
	filterOptions := cliJobFilterOptions(fs, queueRunJobFilters)
	showLogs := cliBool(fs, "logs", false)
	showFailedLogs := cliBool(fs, "failed-logs", false)
	followLogs := cliBool(fs, "follow", false)
	streamOption := cliString(fs, "stream", "both")
	tailLines := cliInt(fs, "tail", 0)
	noPager := cliBool(fs, "no-pager", false)
	jsonOutput := cliBool(fs, "json", false)
	reportOutput := cliBool(fs, "report", false)
	artifactsOutput := cliBool(fs, "artifacts", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if *streamOption != "both" && *streamOption != "stdout" && *streamOption != "stderr" {
		printError("--stream must be both, stdout, or stderr")
		return 1
	}
	if *tailLines < 0 {
		printError("--tail must be a positive number of lines")
		return 1
	}
	if cliOptionSet(fs, "tail") && (*followLogs || *jsonOutput || *reportOutput || *artifactsOutput) {
		printError("--tail cannot be combined with --follow, --json, --report, or --artifacts")
		return 1
	}
	if *followLogs && *streamOption == "both" && cliOptionSet(fs, "stream") {
		printError("--follow requires --stream stdout or --stream stderr")
		return 1
	}
	scope, err := filterOptions.scope()
	if err != nil {
		printError(err)
		return 1
	}
	filter := filterOptions.filter()
	// resultSelection filters the job table by result, like copy and run;
	// logs and reports only know --failed.
	failedOnly, unfinishedOnly, successOnly := filterOptions.resultFilters()
	resultSelection := filterOptions.resultSelection()
	// Result and execution conditions need a run's results, so they select a
	// run view like a result selection does.
	resultFilter := resultSelection != "" || filter.HasRunConditions()
	if (unfinishedOnly || successOnly) && (*showLogs || *showFailedLogs || *followLogs || *reportOutput || *jsonOutput) {
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
		if *runIDOption != "" || *jobIDOption != "" || *jobNameOption != "" || *showQueueOption {
			printError("a positional selector cannot be combined with run, job, queue, or list options")
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
		baseDir, projectName, resolvedRunID, resolvedJobID, err := resolveCLIAttempt(selector, *basedir, *queueNameOption, "")
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
			baseDir, projectName, err := resolveCLIExistingRun(*basedir, *queueNameOption, selector)
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
	// An attempt ID, run ID, latest, or project has been applied above like
	// its option. A selector left here is searched for as a run name, job
	// ID, or job name, which the output options do not take.
	if selector != "" && (*showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput) {
		printError("a run name, job ID, or job name selector cannot be combined with log, follow, JSON, or report options; use --run-id or --job-id")
		return 1
	}
	if strings.HasPrefix(*jobIDOption, "att_") {
		attemptID = *jobIDOption
		baseDir, projectName, resolvedRunID, resolvedJobID, err := resolveCLIAttempt(*jobIDOption, *basedir, *queueNameOption, *runIDOption)
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
			var missingProject *resolve.ProjectNotFoundError
			if errors.As(err, &missingProject) {
				err = fmt.Errorf("selector %q not found: %w", selector, err)
			}
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
	if *artifactsOutput {
		if *jobIDOption == "" {
			printError("--artifacts requires --job-id, a job selector, or an attempt ID")
			return 1
		}
		if *showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput || *showQueueOption || resultFilter || cliOptionSet(fs, "stream") {
			printError("--artifacts cannot be combined with log, follow, stream, JSON, report, queue, list, or result filter options")
			return 1
		}
	}
	if cliOptionSet(fs, "tail") && !*showLogs && !*showFailedLogs && *jobIDOption == "" {
		printError("--tail requires --logs, --failed-logs, or --job-id")
		return 1
	}
	if cliOptionSet(fs, "stream") && (*reportOutput || (!*showLogs && !*showFailedLogs && !*followLogs && *jobIDOption == "")) {
		printError("--stream requires a job log, --logs, or --failed-logs view")
		return 1
	}
	if scope.Kinds() > 0 && (*jobIDOption != "" || *showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput) {
		printError("--stage and --matrix cannot be combined with job, log, follow, JSON, or report options")
		return 1
	}
	if !filter.Empty() && (*jobIDOption != "" || *showLogs || *showFailedLogs || *followLogs || *jsonOutput || *reportOutput) {
		printError("--filter-* options cannot be combined with job, log, follow, JSON, or report options")
		return 1
	}
	if *reportOutput && (*showQueueOption || *showLogs || *showFailedLogs || *followLogs || *jsonOutput) {
		printError("--report cannot be combined with queue, log, follow, or JSON options")
		return 1
	}
	if *showQueueOption && (*runIDOption != "" || resultFilter || *showLogs || *showFailedLogs || *followLogs) {
		printError("--queue cannot be combined with run, log, or filter options")
		return 1
	}
	baseDir, queueName, err := resolveCLIExistingRun(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		if strings.Contains(err.Error(), "multiple projects exist") {
			printErrorf("%v\nList projects with: rotari projects", err)
			return 1
		}
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	if *showQueueOption {
		if cliOptionSet(fs, "stream") {
			printError("--stream cannot be combined with --queue")
			return 1
		}
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil {
			printErrorf("failed to load queue: %v", err)
			return 1
		}
		if arrayScope, ok := arrayCommandScope(queue.Commands, *jobIDOption); ok {
			return showQueue(paths, queue, arrayScope, jobfilter.Filter{})
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
		return showQueue(paths, queue, scope, filter)
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
			if !*jsonOutput {
				printInterruptedRunNotice(paths, stateRunID)
			}
		} else {
			queue, err := state.LoadQueue(paths.QueueFile)
			if err != nil {
				printErrorf("failed to load queue: %v", err)
				return 1
			}
			// Options that only apply to runs look past a non-empty queue to
			// the latest run; other views show the queue.
			runOnly := *showLogs || *showFailedLogs || *followLogs || resultFilter || *reportOutput || *artifactsOutput || cliOptionSet(fs, "stream")
			if len(queue.Commands) > 0 && !runOnly {
				if arrayScope, ok := arrayCommandScope(queue.Commands, *jobIDOption); ok {
					return showQueue(paths, queue, arrayScope, jobfilter.Filter{})
				}
				if *jobIDOption != "" {
					return showQueueJob(paths, queue, *jobIDOption)
				}
				if *jsonOutput {
					return showQueueJSON(paths, queue)
				}
				return showQueue(paths, queue, scope, filter)
			}
			if project.CountRuns(paths) == 0 && runOnly {
				printErrorf("project %q has no runs; logs, failed filters, and reports need one", paths.ProjectName)
				return 1
			}
			if project.CountRuns(paths) == 0 {
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
			if *artifactsOutput {
				printErrorf("--artifacts requires --job-id of one task, not of the array %q", *jobIDOption)
				return 1
			}
			if *jsonOutput {
				return showRunJobJSON(paths, runID, *jobIDOption, resultSelection)
			}
			return showRun(paths, runID, showJobFilter{selection: resultSelection, scope: arrayScope})
		}
	}
	if *jobIDOption != "" {
		if *artifactsOutput {
			return showWithPager(!*noPager, func(writer io.Writer) int {
				return showJobArtifacts(writer, paths, runID, *jobIDOption, attemptID)
			})
		}
		if *reportOutput {
			report, err := report.Build(jsonStore(), paths, runID, *jobIDOption, false, attemptID, true)
			if err != nil {
				printError(err)
				return 1
			}
			fmt.Print(report)
			return 0
		}
		if *jsonOutput {
			return showRunJobJSON(paths, runID, *jobIDOption, resultSelection)
		}
		running, err := isRunning(paths.LockFile)
		if err != nil {
			printErrorf("failed to check queue state: %v", err)
			return 1
		}
		if shouldFollowLogs(*followLogs, running, isTerminal(os.Stdout)) {
			stream := *streamOption
			if stream == "both" {
				stream = "stdout"
			}
			return followJobLog(os.Stdout, paths, runID, *jobIDOption, attemptID, stream)
		}
		return showWithPager(!*noPager, func(writer io.Writer) int {
			return showJobAttempt(writer, paths, runID, *jobIDOption, attemptID, *tailLines, *streamOption)
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
			return showRunLogs(writer, paths, runID, *showFailedLogs || failedOnly, *tailLines, *streamOption)
		})
	}
	if *reportOutput {
		report, err := report.Build(jsonStore(), paths, runID, "", failedOnly, "", true)
		if err != nil {
			printError(err)
			return 1
		}
		fmt.Print(report)
		return 0
	}
	if *jsonOutput {
		return showRunJSON(paths, runID, resultSelection)
	}
	nextQueue, err := nextQueueForRun(paths, runID)
	if err != nil {
		printErrorf("failed to load next queue: %v", err)
		return 1
	}
	if code := showRun(paths, runID, showJobFilter{selection: resultSelection, scope: scope, filter: filter}); code != 0 {
		return code
	}
	if nextQueue != nil {
		fmt.Println("\n=== Next queue ===")
		return showQueue(paths, *nextQueue, model.CommandSelector{}, jobfilter.Filter{})
	}
	return 0
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
	BaseDir   string                    `json:"base_dir"`
	Project   string                    `json:"project_name"`
	RunID     string                    `json:"run_id"`
	RunDir    string                    `json:"run_dir"`
	Summary   *model.RunSummary         `json:"summary,omitempty"`
	Lifecycle string                    `json:"lifecycle,omitempty"`
	Client    *model.RunClientStatus    `json:"client_status,omitempty"`
	Jobs      []showJSONJob             `json:"jobs,omitempty"`
	Failures  []runlineage.FailureGroup `json:"failures,omitempty"`
	Commands  model.Queue               `json:"commands"`
	NextQueue *model.Queue              `json:"next_queue,omitempty"`
}

type showJobCounts struct {
	success   int
	failed    int
	blocked   int
	cancelled int
	running   int
	waiting   int
	pending   int
	suspended int
	unknown   int
}

// showJobFilter selects the rows of a run job table.
type showJobFilter struct {
	// selection, when set, keeps the jobs whose result matches it, such as
	// "failed" or "failed,unfinished"; see model.ResultSelection.
	selection string
	// scope, when set, keeps only the jobs of one stage or matrix.
	scope model.CommandSelector
	// filter keeps only the jobs that pass its conditions.
	filter jobfilter.Filter
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
// commands that scope selects and filter keeps, or nil when neither is set.
func scopedJobIDs(commands []model.QueuedCommand, scope model.CommandSelector, filter jobfilter.Filter) (map[string]bool, error) {
	if scope.Kinds() == 0 && filter.Empty() {
		return nil, nil
	}
	indexes := make([]int, 0, len(commands))
	if scope.Kinds() > 0 {
		selected, err := model.SelectCommands(commands, scope)
		if err != nil {
			return nil, err
		}
		indexes = selected
	} else {
		for index := range commands {
			indexes = append(indexes, index)
		}
	}
	ids := make(map[string]bool)
	for _, index := range indexes {
		if !filter.MatchesCommand(commands[index]) {
			continue
		}
		for _, job := range model.QueueToJobs(commands[index : index+1]) {
			if filter.MatchesDefinition(job.ID) {
				ids[job.ID] = true
			}
		}
	}
	return ids, nil
}

// showRunJSON prints a run's summary, failure groups, and command snapshot.
// A non-empty selection keeps only the summary results and failure groups of
// the jobs it selects; the command snapshot stays whole.
func showRunJSON(paths state.ProjectPaths, runID, selection string) int {
	result := showJSON{BaseDir: paths.BaseDir, Project: paths.ProjectName, RunID: runID, RunDir: filepath.Join(paths.RunsDir, runID)}
	result.Lifecycle, _ = runview.RunLifecycleLabel(paths, runID)
	clientStatus, _ := runview.ClientStatus(paths, runID)
	if clientStatus.State == "" {
		clientStatus.State = "unknown"
	}
	result.Client = &clientStatus
	var selected map[string]bool
	if summary, err := state.LoadRunSummary(filepath.Join(result.RunDir, "summary.json")); err == nil {
		if selection != "" {
			selected = make(map[string]bool)
			kept := summary.Results[:0]
			for _, jobResult := range summary.Results {
				if selectsShownJob(paths, runID, jobResult.ID, jobResult, true, selection, jobfilter.Filter{}) {
					kept = append(kept, jobResult)
					selected[jobResult.ID] = true
				}
			}
			summary.Results = kept
		}
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
	result.Jobs, err = resolveRunJSONJobs(result.RunDir, model.QueueToJobs(commands.Commands), model.QueueOriginsByJobID(commands), result.Summary)
	if err != nil {
		printErrorf("failed to read job states: %v", err)
		return 1
	}
	result.Jobs = filterRunJSONJobs(paths, runID, result.Jobs, selection)
	nextQueue, err := nextQueueForRun(paths, runID)
	if err != nil {
		printErrorf("failed to load next queue: %v", err)
		return 1
	}
	result.NextQueue = nextQueue
	failures, err := runFailureGroups(paths, runID, selected)
	if err != nil {
		printErrorf("failed to group failures: %v", err)
		return 1
	}
	result.Failures = failures
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		printErrorf("failed to write JSON: %v", err)
		return 1
	}
	return 0
}

func nextQueueForRun(paths state.ProjectPaths, runID string) (*model.Queue, error) {
	inspection, err := project.Inspect(paths, false)
	if err != nil {
		return nil, err
	}
	if inspection.State == project.Idle || inspection.RunID != runID {
		return nil, nil
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return nil, err
	}
	if len(queue.Commands) == 0 {
		return nil, nil
	}
	return &queue, nil
}

// selectsShownJob reports whether a show view of runID keeps the job, given
// its result. jobfilter.Filter.Selects decides; this only gathers the facts.
func selectsShownJob(paths state.ProjectPaths, runID, jobID string, result model.JobResult, finished bool, selection string, filter jobfilter.Filter) bool {
	job := jobstatus.FilterJob(jsonStore(), paths.RunsDir, model.JobOrigin{RunID: runID, JobID: jobID}, jobID, result, finished, time.Now())
	return filter.Selects(selection, job)
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
	if pid, running := serverinternal.Running(paths.ProjectDir); running {
		fmt.Fprintf(writer, "%s running (pid=%d)\n", cyan("Runner server:"), pid)
	} else {
		fmt.Fprintf(writer, "%s stopped\n", cyan("Runner server:"))
	}
	fmt.Fprintf(writer, "%s %d\n", cyan("Runs:"), project.CountRuns(paths))
	if cwd, err := os.Getwd(); err == nil {
		if loaded, err := config.Load(cwd, paths.BaseDir, paths.ProjectName); err == nil {
			for _, source := range loaded.Sources {
				fmt.Fprintf(writer, "%s %s (%s)\n", cyan("Config:"), source.Path, source.Scope)
			}
		}
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

func printInterruptedRunNotice(paths state.ProjectPaths, runID string) {
	fmt.Printf("%s\n", yellow(fmt.Sprintf("Run %s appears to have been interrupted.", runID)))
	fmt.Printf("Recover the queue before modifying or running it:\n  rotari unlock %s --run-id %s\n\n",
		hintLocation(paths), executor.ShellQuote(runID))
}

func showRun(paths state.ProjectPaths, runID string, filter showJobFilter) int {
	if filter.filter.Changed || filter.filter.New {
		printError("--filter-changed and --filter-new apply to the queue view; drop --run-id or use --queue")
		return 1
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printErrorf(runNotFoundMessage, runID)
		return 1
	}
	if err := state.CheckRunVersions(runDir); err != nil {
		printErrorf("failed to read run: %v", err)
		return 1
	}
	summary, summaryErr := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	summaryOK := summaryErr == nil
	if summaryErr != nil && !errors.Is(summaryErr, os.ErrNotExist) {
		printErrorf("failed to read summary: %v", summaryErr)
		return 1
	}
	jobSpecs := loadRunJobSpecs(runDir)
	scoped, err := scopedJobIDs(runScopeCommands(runDir, jobSpecs), filter.scope, filter.filter)
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
	fmt.Printf("%s %s\n", cyan("Run:"), model.RunLabel(runID, summary.RunName))
	if summary.RunName != "" {
		fmt.Printf("%s %s\n", cyan("Run name:"), summary.RunName)
	}
	fmt.Printf("%s %s\n", cyan("Directory:"), runDir)
	lifecycle, lifecycleErr := runview.RunLifecycleLabel(paths, runID)
	if lifecycleErr != nil {
		printErrorf("failed to determine run lifecycle: %v", lifecycleErr)
		return 1
	}
	clientStatus, clientErr := runview.ClientStatus(paths, runID)
	if clientErr != nil {
		clientStatus = model.RunClientStatus{State: "unknown"}
	}
	fmt.Printf("%s %s\n%s %s\n", cyan("Lifecycle:"), colorRunLifecycle(lifecycle), cyan("Client:"), colorClientStatus(runview.ClientStatusLabel(clientStatus)))
	if summaryOK {
		fmt.Printf("%s %s\n%s %s\n%s %s\n%s %d\n", cyan("Status:"), summary.Status, cyan("Started:"), model.FormatDisplayTimestamp(summary.StartedAt), cyan("Finished:"), model.FormatDisplayTimestamp(summary.FinishedAt), cyan("Exit code:"), summary.ExitCode)
	}
	fmt.Printf("%s %s\n", cyan("Output directory:"), runDir)
	if _, failed := model.CountRunResults(summary.Results); failed > 0 {
		// The job table can be long; point to the compact summary before it.
		fmt.Printf("%s rotari lineage %s%s\n", cyan("Failure summary:"), runHintLocation(paths), executor.ShellQuote(runID))
	}
	queue, queueErr := state.LoadQueue(paths.QueueFile)
	if queueErr == nil && len(queue.Commands) > 0 {
		if diff, err := compareQueueWithRun(paths.QueueFile, filepath.Join(runDir, "commands.json")); err == nil && diff.HasChanges() {
			fmt.Printf("\n%s\n", yellow("Queue differs from this run:"))
			fmt.Printf("  Added: %d\n  Removed: %d\n  Changed: %d\n", diff.Added, diff.Removed, diff.Changed)
		}
	}
	runQueue, runQueueErr := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	jobCounts := showJobCounts{}
	var recorded *model.RunSummary
	if summaryOK {
		recorded = &summary
	}
	resultByID := jobstatus.RecordedResults(runDir, recorded)
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
	displayed := make(map[string]bool)
	fmt.Println("\n" + cyan("Jobs:"))
	changeHints := make([]model.JobSpec, 0)
	table := [][]string{{"JOB ID", "LATEST ATTEMPT", "TASK", "NAME", "STAGE", "DEPENDS ON", "STATUS", "EXECUTOR", "SUBMITTED", "FINISHED", "ELAPSED", "HOSTS", "COMMAND"}}
	now := time.Now()
	for _, jobID := range jobIDs {
		jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
		if err != nil {
			continue
		}
		jobSpec := jobSpecs[jobID]
		if scoped != nil && !scoped[jobID] {
			continue
		}
		latestAttemptID, _ := state.LatestAttemptID(runDir, jobID)
		latestAttemptLabel := latestAttemptID
		if latestAttemptLabel == "" {
			latestAttemptLabel = "-"
		}
		if latestAttemptLabel == "-" {
			if origin := originByID[jobID]; origin != nil && origin.AttemptID != "" {
				latestAttemptLabel = origin.AttemptID
			} else if result, ok := resultByID[jobSpec.ID]; ok && result.AttemptID != "" {
				latestAttemptLabel = result.AttemptID
			}
		}
		name := state.ReadJobName(jobDir)
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
		// An attempt records the executor it ran on, which a run-level
		// --executor can make differ from the job's definition.
		executorSpec := jobSpec
		if recorded := state.ReadAttemptExecutor(jobDir); recorded != "" {
			executorSpec.Executor = recorded
		}
		executorText := queueExecutorText(runQueue, executorSpec)
		jobResult, _ := resolved.Result(jobSpec)
		origin := originByID[jobID]
		carried := runlineage.IsCarried(origin, latestAttemptID, blocked, hasSummary)
		displayStatus := resolved.DisplayStatus(jobSpec)
		countStatus := displayStatus
		switch countStatus {
		case "success":
			jobCounts.success++
		case "failed":
			jobCounts.failed++
		case "blocked":
			jobCounts.blocked++
		case "cancelled":
			jobCounts.cancelled++
		case "running (recorded)":
			jobCounts.running++
		case "waiting (recorded)":
			jobCounts.waiting++
		case "pending", "pending (carried)":
			jobCounts.pending++
		case "suspended (recorded)":
			jobCounts.suspended++
		default:
			jobCounts.unknown++
		}
		if carried {
			displayStatus += " (carried)"
		}
		if !selectsShownJob(paths, runID, jobID, jobResult, statusOK, filter.selection, filter.filter) {
			continue
		}
		displayed[jobID] = true
		submittedAt, finishedAt := jobstatus.Timestamps(runDir, jobID, origin, carried)
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
		statusText := jobstatus.DisplayLabel(resolved.DisplayStatus(jobSpec), resolved.Accepted(), carried)
		elapsed := showJobElapsed(submittedAt, finishedAt, statusOK, jobDir, now)
		table = append(table, []string{jobID, latestAttemptLabel, taskText, name, stage, dependsOn, statusText, executorText,
			model.FormatDisplayTimestamp(submittedAt), model.FormatDisplayTimestamp(finishedAt), elapsed, hosts,
			joblist.ShortenText(command, showCommandWidth)})
	}
	printShowJobTable(table)
	fmt.Printf("\n%s success: %d, failed: %d, blocked: %d, cancelled: %d, running (recorded): %d, waiting (recorded): %d, pending: %d, suspended (recorded): %d, unknown: %d\n", cyan("Job status:"), jobCounts.success, jobCounts.failed, jobCounts.blocked, jobCounts.cancelled, jobCounts.running, jobCounts.waiting, jobCounts.pending, jobCounts.suspended, jobCounts.unknown)
	// Grouping needs the job definitions, which a run without a readable
	// command snapshot lacks; its table above lists job IDs only.
	if runQueueErr == nil {
		failures, err := runFailureGroups(paths, runID, displayed)
		if err != nil {
			printErrorf("failed to group failures: %v", err)
			return 1
		}
		if len(failures) > 0 {
			fmt.Println()
			writeFailureGroups(os.Stdout, failures, failureRetryHints(paths, runID))
		}
	}
	fmt.Println("\n" + cyan("To show a job:"))
	fmt.Println("  rotari show -j ATTEMPT_ID")
	printChangeHints(paths, runID, runQueue, changeHints)
	printFailedLogHints(runID, changeHints, resultByID)
	fmt.Printf("\n%s\n  rotari delete -r %s\n", cyan("To delete this run's saved logs:"), runID)
	return 0
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
	fmt.Printf("    rotari retry %s\n", hintLocation(paths))
}

func showQueue(paths state.ProjectPaths, queue model.Queue, scope model.CommandSelector, filter jobfilter.Filter) int {
	jobs := model.QueueToJobs(queue.Commands)
	if filter.Changed || filter.New {
		referenceRunID, err := projectrun.FingerprintReferenceRun(paths, "")
		if err == nil {
			filter, err = projectrun.ClassifyDefinitions(paths, queue, filter, referenceRunID, "")
		}
		if err != nil {
			printErrorf("current queue: %v", err)
			return 1
		}
	}
	scoped, err := scopedJobIDs(queue.Commands, scope, filter)
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
	markedStatusByID := model.QueueMarkedStatusByJobID(queue)
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
		sourceRun := "-"
		origin := originByID[job.ID]
		if origin != nil {
			sourceRun = origin.RunID + "/" + origin.JobID
		}
		sourceStatus := model.QueuedStatusText(origin, markedStatusByID[job.ID])
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
	return len(runlineage.SpecChanges(left, right)) == 0
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
	return showJobAttempt(writer, paths, runID, jobID, "", 0)
}

func showJobAttempt(writer io.Writer, paths state.ProjectPaths, runID, jobID, attemptID string, tail int, selectedStreams ...string) int {
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
	if err := state.CheckRunVersions(runDir); err != nil {
		printErrorf("failed to read run: %v", err)
		return 1
	}
	jobSpecs := loadRunJobSpecs(runDir)
	// A job of the run that never started, such as one blocked by a failed
	// dependency, has no attempt directory; its summary result decides it.
	_, inRun := jobSpecs[jobID]
	neverRan := false
	if info, err := os.Stat(jobDir); err != nil || !info.IsDir() {
		if origin := state.LoadRunOrigin(runDir, jobID); origin != nil {
			if runResultAccepted(runDir, jobID) {
				// The destination result is an accepted success; the source
				// details below keep the original attempt and exit code.
				fmt.Fprintf(writer, "%s %s\n", cyan("Status:"), green("success (accepted)"))
				fmt.Fprintf(writer, "%s manually accepted from run %s attempt %s (no re-execution)\n\n", cyan("Note:"), origin.RunID, origin.AttemptID)
				return showJobAttempt(writer, paths, origin.RunID, origin.JobID, origin.AttemptID, tail, selectedStreams...)
			}
			fmt.Fprintf(writer, "%s carried forward from run %s (no re-execution)\n\n", cyan("Note:"), origin.RunID)
			return showJobAttempt(writer, paths, origin.RunID, origin.JobID, "", tail, selectedStreams...)
		}
		if !inRun || attemptID != "" {
			printErrorf(jobNotFoundMessage, jobID, runID)
			return 1
		}
		neverRan = true
	}
	writeShowTargetHeaderWithMode(writer, paths, "run job")
	fmt.Fprintf(writer, "%s %s\n%s %s\n", cyan("Run:"), runID, cyan("Job:"), jobID)
	lifecycle, lifecycleErr := runview.RunLifecycleLabel(paths, runID)
	if lifecycleErr != nil {
		lifecycle = "unknown"
	}
	clientStatus, clientErr := runview.ClientStatus(paths, runID)
	if clientErr != nil {
		clientStatus = model.RunClientStatus{State: "unknown"}
	}
	fmt.Fprintf(writer, "%s %s\n%s %s\n", cyan("Lifecycle:"), colorRunLifecycle(lifecycle), cyan("Client:"), colorClientStatus(runview.ClientStatusLabel(clientStatus)))
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
	name := state.ReadJobName(jobDir)
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
	if neverRan {
		submittedAt, finishedAt = "-", "-"
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Submitted:"), model.FormatDisplayTimestamp(submittedAt))
	fmt.Fprintf(writer, "%s %s\n", cyan("Finished:"), model.FormatDisplayTimestamp(finishedAt))
	summaryResult, hasSummary := loadRunResult(runDir, jobSpecs[jobID].ID)
	resolved := jobstatus.ResolveAttempt(jobstatus.ReadAttempt(jsonStore(), jobDir), latest, summaryResult, hasSummary)
	fmt.Fprintf(writer, "%s %s\n", cyan("Execution state:"), colorJobStatus(resolved.DisplayStatus(jobSpecs[jobID])))
	if !neverRan {
		fmt.Fprintf(writer, "%s %s\n", cyan("Elapsed:"), showJobElapsed(submittedAt, finishedAt, resolved.Finished(), jobDir, time.Now()))
	}
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
	if neverRan {
		command = strings.Join(jobSpecs[jobID].Command, " ")
	}
	if resolved.HasSummary {
		writeJobDiagnoses(writer, resolved.Summary)
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Command:"), command)
	if neverRan {
		fmt.Fprintf(writer, "%s -\n", cyan("Logs:"))
		return 0
	}
	listing := jobstatus.ListArtifacts(jsonStore(), paths.RunsDir, model.JobOrigin{RunID: runID, JobID: jobID, AttemptID: selectedAttemptID})
	writeArtifactListing(writer, listing, shownArtifacts, "rotari show -j "+selectedAttemptID+" --artifacts")
	printJobStreams(writer, jobDir, selectedStreamNames(selectedStreams...), tail)
	return 0
}

func selectedStreamNames(selected ...string) []string {
	stream := "both"
	if len(selected) > 0 && selected[0] != "" {
		stream = selected[0]
	}
	if stream == "both" {
		return []string{state.StdoutFileName, state.StderrFileName}
	}
	return []string{stream}
}

// printJobStreams prints a job's merged output, or each selected stream, and
// with a positive tail only the last tail lines of each.
func printJobStreams(writer io.Writer, jobDir string, streams []string, tail int) {
	if data, err := os.ReadFile(filepath.Join(jobDir, "output")); err == nil {
		fmt.Fprintf(writer, "%s %s\n", cyan("OUTPUT:"), filepath.Join(jobDir, "output"))
		data = lastLines(data, tail)
		fmt.Fprint(writer, string(data))
		if !strings.HasSuffix(string(data), "\n") {
			fmt.Fprintln(writer)
		}
		return
	}
	for _, stream := range streams {
		path := filepath.Join(jobDir, stream)
		fmt.Fprintf(writer, "%s %s\n", cyan(strings.ToUpper(stream)+":"), path)
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			fmt.Fprintf(writer, "(No %s log)\n", stream)
		} else {
			data = lastLines(data, tail)
			fmt.Fprint(writer, string(data))
			if !strings.HasSuffix(string(data), "\n") {
				fmt.Fprintln(writer)
			}
		}
	}
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

func colorRunLifecycle(status string) string {
	switch status {
	case "finished":
		return green(status)
	case "failed":
		return red(status)
	default:
		return yellow(status)
	}
}

func colorClientStatus(status string) string {
	if strings.HasPrefix(status, "unknown") {
		return yellow(status)
	}
	return status
}

func colorJobStatus(status string) string {
	switch {
	case strings.HasPrefix(status, "success"):
		return green(status)
	case strings.HasPrefix(status, "failed"), status == "cancelled", strings.HasPrefix(status, "blocked"):
		return red(status)
	default:
		return yellow(status)
	}
}

func writeJobDiagnoses(writer io.Writer, result model.JobResult) {
	switch {
	case result.DiagnosisStatus == model.DiagnosisNoMatch:
		fmt.Fprintf(writer, "%s no known rule matched\n  Next: %s\n", cyan("Diagnosis:"), diagnose.NoMatchNext)
	case result.DiagnosisStatus == model.DiagnosisUnavailable:
		fmt.Fprintf(writer, "%s unavailable: %s\n  Next: %s\n", cyan("Diagnosis:"), result.DiagnosisNote, diagnose.UnavailableNext)
	case len(result.Diagnoses) > 0:
		fmt.Fprintf(writer, "%s\n", cyan("Diagnosis:"))
		for _, diagnosis := range result.Diagnoses {
			fmt.Fprintf(writer, "  %s\n    Evidence: %s\n    Next: %s\n", diagnosis.Name, diagnosis.Evidence, diagnosis.Suggestion)
		}
	default:
		return
	}
	if diagnose.Outdated(result) {
		fmt.Fprintf(writer, "  Note: %s\n", diagnose.OutdatedNote)
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

func showRunLogs(writer io.Writer, paths state.ProjectPaths, runID string, failedOnly bool, tail int, selectedStreams ...string) int {
	runDir := filepath.Join(paths.RunsDir, runID)
	entries, err := os.ReadDir(runDir)
	if err != nil {
		printErrorf("failed to read run directory: %v", err)
		return 1
	}
	jobSpecs := loadRunJobSpecs(runDir)
	writeShowTargetHeaderWithMode(writer, paths, "run")
	fmt.Fprintf(writer, "%s %s\n\n", cyan("Run:"), runID)

	executed := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			executed[entry.Name()] = true
		}
	}
	// Jobs follow their definition order, as in the run table, so the tasks
	// of an array and the members of a matrix stay together.
	queue, queueErr := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if queueErr != nil || len(queue.Commands) == 0 {
		// Without the run's definitions, list the directories it executed.
		for _, entry := range entries {
			if entry.IsDir() {
				printExecutedJobLogs(writer, runDir, entry.Name(), jobSpecs[entry.Name()], failedOnly, tail, selectedStreams...)
			}
		}
		return 0
	}
	for _, command := range queue.Commands {
		ids := []string{command.ID}
		origins := map[string]*model.JobOrigin{command.ID: command.Origin}
		if command.Array != nil {
			ids = ids[:0]
			for _, task := range model.ArrayTaskIDs(command.Array) {
				taskID := fmt.Sprintf("%s-%d", command.ID, task)
				ids = append(ids, taskID)
				origins[taskID] = command.TaskOrigins[taskID]
			}
		}
		for _, id := range ids {
			if executed[id] {
				printExecutedJobLogs(writer, runDir, id, jobSpecs[id], failedOnly, tail, selectedStreams...)
				continue
			}
			printCarriedForwardOutput(writer, paths, id, command.Name, command.Command, command.WorkingDirectory, origins[id], executed, failedOnly, tail, selectedStreams...)
		}
	}
	return 0
}

// printExecutedJobLogs prints the header and logs of a job the run executed.
func printExecutedJobLogs(writer io.Writer, runDir, jobID string, spec model.JobSpec, failedOnly bool, tail int, selectedStreams ...string) {
	jobDir, err := state.LatestAttemptJobDir(runDir, jobID)
	if err != nil {
		return
	}
	attempt := jobstatus.ReadAttempt(jsonStore(), jobDir)
	status, statusOK := attempt.ExitCode, attempt.Finished()
	if failedOnly && (!statusOK || status == 0) {
		return
	}
	name := state.ReadJobName(jobDir)
	if name == "" {
		name = spec.Name
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
	if spec.WorkingDirectory != "" {
		fmt.Fprintf(writer, "Working directory: %s\n", spec.WorkingDirectory)
	}
	fmt.Fprintf(writer, "Command: %s\n", command)
	printJobStreams(writer, jobDir, selectedStreamNames(selectedStreams...), tail)
	fmt.Fprintln(writer)
}

// printCarriedForwardOutput prints a carried-forward job's output read from
// its origin run/job, if it was not itself re-executed in this run (i.e. it
// has no directory of its own here).
func printCarriedForwardOutput(writer io.Writer, paths state.ProjectPaths, id, name string, command []string, workingDirectory string, origin *model.JobOrigin, seen map[string]bool, failedOnly bool, tail int, selectedStreams ...string) {
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
	var originDir string
	if origin.AttemptID != "" {
		originDir, err = state.SpecificAttemptJobDir(originRunDir, origin.JobID, origin.AttemptID)
	} else {
		originDir, err = state.LatestAttemptJobDir(originRunDir, origin.JobID)
	}
	if err != nil {
		return
	}
	printJobStreams(writer, originDir, selectedStreamNames(selectedStreams...), tail)
	fmt.Fprintln(writer)
}

func followJobLog(writer io.Writer, paths state.ProjectPaths, runID, jobID, attemptID string, streams ...string) int {
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
	stream := streamForFollow(streams...)
	if stream != state.StdoutFileName && stream != state.StderrFileName {
		printError("--follow requires --stream stdout or --stream stderr")
		return 1
	}
	latestAttemptID, attemptErr := state.LatestAttemptID(runDir, jobID)
	if attemptID == "" && (attemptErr != nil || latestAttemptID == "") {
		if origin := state.LoadRunOrigin(runDir, jobID); origin != nil {
			fmt.Fprintf(writer, "%s carried forward from run %s (no re-execution)\n\n", cyan("Note:"), origin.RunID)
			originRunDir, pathErr := state.SafeJoin(paths.RunsDir, origin.RunID)
			if pathErr != nil {
				return 0
			}
			var originDir string
			if origin.AttemptID != "" {
				originDir, err = state.SpecificAttemptJobDir(originRunDir, origin.JobID, origin.AttemptID)
			} else {
				originDir, err = state.LatestAttemptJobDir(originRunDir, origin.JobID)
			}
			if err == nil {
				originOutput, readErr := os.ReadFile(filepath.Join(originDir, stream))
				if readErr == nil {
					_, _ = writer.Write(originOutput)
				}
			}
			return 0
		}
		if _, exists := loadRunJobSpecs(runDir)[jobID]; !exists {
			printErrorf(jobNotFoundMessage, jobID, runID)
			return 1
		}
		if _, finished := loadRunResult(runDir, jobID); finished {
			return 0
		}
		printErrorf("job %q has not started an attempt in run %q", jobID, runID)
		return 1
	}
	selectedAttemptID := attemptID
	if selectedAttemptID == "" {
		selectedAttemptID = latestAttemptID
	}
	jobDir, err := state.SpecificAttemptJobDir(runDir, jobID, selectedAttemptID)
	if err != nil {
		printErrorf(jobNotFoundMessage, jobID, runID)
		return 1
	}
	outputPath := filepath.Join(jobDir, stream)
	mergedOutputPath := filepath.Join(jobDir, "output")
	if _, err := os.Stat(mergedOutputPath); err == nil {
		// The default log mode stores stdout and stderr together in output.
		// showJobAttempt reads this file regardless of the selected stream;
		// follow must read it too for flushed output to appear while following.
		outputPath = mergedOutputPath
	} else if !os.IsNotExist(err) {
		printErrorf("failed to read job log: %v", err)
		return 1
	}
	output, err := os.ReadFile(outputPath)
	if err != nil && !os.IsNotExist(err) {
		printErrorf("failed to read job %s: %v", stream, err)
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
				printErrorf("failed to read job %s: %v", stream, err)
				return 1
			}
			if jobstatus.ReadAttempt(jsonStore(), jobDir).Finished() {
				return 0
			}
			time.Sleep(250 * time.Millisecond)
			continue
		}
		if len(data) > int(offset) {
			if _, err := writer.Write(data[offset:]); err != nil {
				return 1
			}
			offset = int64(len(data))
		}
		if jobstatus.ReadAttempt(jsonStore(), jobDir).Finished() {
			// The job can write its last output between the read above and
			// its finished marker, so read once more before returning.
			final, readErr := os.ReadFile(outputPath)
			if readErr == nil && len(final) > int(offset) {
				if _, err := writer.Write(final[offset:]); err != nil {
					return 1
				}
			}
			return 0
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func streamForFollow(streams ...string) string {
	if len(streams) > 0 && streams[0] != "" {
		return streams[0]
	}
	return state.StdoutFileName
}

func runResultAccepted(runDir, jobID string) bool {
	result, ok := loadRunResult(runDir, jobID)
	return ok && result.Accepted
}

// loadRunResult returns the job's result from the run's summary.json.
func loadRunResult(runDir, jobID string) (model.JobResult, bool) {
	var recorded *model.RunSummary
	if summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		recorded = &summary
	}
	result, ok := jobstatus.RecordedResults(runDir, recorded)[jobID]
	return result, ok
}

// showCommandWidth caps the COMMAND column of the run table, as `jobs` does;
// `show -j` prints a job's full command.
const showCommandWidth = 60

// showJobElapsed describes how long a job ran and, while it runs, how long
// ago it last wrote output, as `jobs` does. A timestamp that does not parse
// is unknown.
func showJobElapsed(submittedAt, finishedAt string, finished bool, jobDir string, now time.Time) string {
	parse := func(value string) time.Time {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		if err != nil {
			return time.Time{}
		}
		return parsed
	}
	return joblist.FormatRunTime(jobstatus.MeasureRunTime(jobDir, parse(submittedAt), parse(finishedAt), finished, now))
}

// printShowJobTable prints the run table with each column as wide as its
// longest value, coloring the STATUS column after padding so escape codes do
// not count toward the width.
func printShowJobTable(table [][]string) {
	widths := make([]int, len(table[0]))
	for _, row := range table {
		for index, value := range row {
			widths[index] = max(widths[index], len(value))
		}
	}
	const statusColumn = 6
	for rowIndex, row := range table {
		var line strings.Builder
		for index, value := range row {
			if index > 0 {
				line.WriteString("  ")
			}
			padded := value
			if index < len(row)-1 {
				padded += strings.Repeat(" ", widths[index]-len(value))
			}
			if rowIndex > 0 && index == statusColumn {
				switch {
				case strings.HasPrefix(value, "success"):
					padded = green(padded)
				case strings.HasPrefix(value, "failed"), value == "cancelled":
					padded = red(padded)
				default:
					padded = yellow(padded)
				}
			}
			line.WriteString(padded)
		}
		if rowIndex == 0 {
			fmt.Println(cyan(line.String()))
		} else {
			fmt.Println(line.String())
		}
	}
}

// lastLines returns the last n lines of data, or all of it when n is not
// positive. A final line without a newline counts as a line.
func lastLines(data []byte, n int) []byte {
	if n <= 0 {
		return data
	}
	end := len(data)
	if end > 0 && data[end-1] == '\n' {
		end--
	}
	for index := end - 1; index >= 0; index-- {
		if data[index] == '\n' {
			n--
			if n == 0 {
				return data[index+1:]
			}
		}
	}
	return data
}

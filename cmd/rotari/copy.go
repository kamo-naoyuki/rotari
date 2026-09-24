package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func confirmQueueOverwrite(baseDir, queueName string, appendJobs, overwriteJobs bool) (bool, error) {
	overwriteConfirmed := overwriteJobs
	if !appendJobs && !overwriteJobs {
		paths, pathErr := resolvePaths(baseDir, queueName)
		if pathErr != nil {
			return false, pathErr
		}
		queue, loadErr := state.LoadQueue(paths.QueueFile)
		if loadErr != nil {
			return false, loadErr
		}
		if len(queue.Commands) > 0 {
			if !isTerminal(os.Stdin) {
				return false, errors.New("queue is not empty; use --append or --overwrite")
			}
			fmt.Fprintf(os.Stderr, "project %q has %d queued jobs; overwrite them? [y/N] ", queueName, len(queue.Commands))
			answer, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
			if readErr != nil && len(answer) == 0 {
				return false, readErr
			}
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				return false, errors.New("copy cancelled")
			}
			overwriteConfirmed = true
		}
	}
	return overwriteConfirmed, nil
}

// cmdCopy copies queued or historical jobs into the current project queue.
func cmdCopy(args []string) int {
	fs := flag.NewFlagSet("copy", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	jobName := cliString(fs, "job-name", "")
	failed := cliBool(fs, "failed", false)
	unfinished := cliBool(fs, "unfinished", false)
	success := cliBool(fs, "success", false)
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	appendJobs := cliBool(fs, "append", false)
	overwriteJobs := cliBool(fs, "overwrite", false)
	quiet := cliBool(fs, "quiet", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && *runID != "") || (*appendJobs && *overwriteJobs) {
		printError("usage: " + cliUsage("copy"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*runID = fs.Args()[0]
	}
	if *jobName != "" && len(jobIDs) > 0 {
		printError("--job-name cannot be combined with --job-id")
		return 1
	}
	if *jobName != "" {
		if *runID != "" {
			baseDir, projectName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runID)
			if err != nil {
				printError(err)
				return 1
			}
			paths, err := resolvePaths(baseDir, projectName)
			if err != nil {
				printError(err)
				return 1
			}
			target, found, err := findShowJobInRun(paths, *runID, *jobName, true)
			if err != nil || !found {
				printErrorf("job name %q not found in run %q", *jobName, *runID)
				return 1
			}
			jobIDs = stringSliceFlag{target.jobID}
		} else {
			targets, err := resolveJobTargets(*basedir, *queueNameOption, *jobName, true, false)
			if err != nil {
				printError(err)
				return 1
			}
			if len(targets) != 1 {
				printErrorf("job name %q is %s", *jobName, map[bool]string{true: "ambiguous across latest runs", false: "not found"}[len(targets) > 1])
				return 1
			}
			*basedir, *queueNameOption, *runID = targets[0].baseDir, targets[0].projectName, targets[0].runID
			jobIDs = stringSliceFlag{targets[0].jobID}
		}
	}
	if *runID == "" {
		for _, jobID := range jobIDs {
			if payload, err := state.DecodeAttemptID(jobID); err == nil {
				if *runID == "" {
					*runID = payload.RunID
				} else if *runID != payload.RunID {
					printError(fmt.Sprintf("attempt %q belongs to run %q, not %q", jobID, payload.RunID, *runID))
					return 1
				}
			}
		}
		if *runID == "" {
			if len(jobIDs) == 0 {
				printError("copy requires --run-id/-r unless --job-id is specified")
				return 1
			}
			target, err := resolveLatestJobIDSelection(*basedir, *queueNameOption, jobIDs)
			if err != nil {
				printError(err)
				return 1
			}
			*basedir, *queueNameOption, *runID = target.baseDir, target.projectName, target.runID
		}
	}

	selection := model.ResultSelection(*failed, *unfinished, *success)
	if len(jobIDs) > 0 {
		if selection == "" {
			selection = "job-id"
		}
	}
	if selection == "" {
		selection = "all"
	}

	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runID)
	if err != nil {
		printError(err)
		return 1
	}
	if err := ensureProjectIdle(baseDir, queueName, "copy"); err != nil {
		printError(err)
		return 1
	}
	overwriteConfirmed, err := confirmQueueOverwrite(baseDir, queueName, *appendJobs, *overwriteJobs)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := copyRunToQueue(baseDir, queueName, *runID, selection, jobIDs, *appendJobs, overwriteConfirmed)
	if err != nil {
		printError(err)
		return 1
	}
	if !*quiet {
		fmt.Println(colorKeyValueMessage(message, green))
	}
	return 0
}

func copyRunToQueue(baseDir, queueName, runID, selection string, jobIDs []string, appendJobs bool, overwriteJobs ...bool) (string, error) {
	overwrite := len(overwriteJobs) > 0 && overwriteJobs[0]
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "copy"); err != nil {
		return "", err
	}

	sourceRunDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return "", err
	}
	snapshot, err := state.LoadQueue(filepath.Join(sourceRunDir, "commands.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
	if err != nil {
		return "", fmt.Errorf("failed to load command snapshot: %w", err)
	}
	if len(snapshot.Commands) == 0 {
		return "", errors.New("command snapshot has no jobs")
	}
	summary, summaryErr := state.LoadRunSummary(filepath.Join(sourceRunDir, "summary.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
	if summaryErr != nil && selection != "all" {
		return "", fmt.Errorf("failed to load run summary: %w", summaryErr)
	}
	results := model.ResultsByID(summary.Results)
	originCWD := ""
	if context, contextErr := state.LoadContext(jsonStore(), sourceRunDir); contextErr == nil {
		originCWD = context.CWD
	}
	requested := make(map[string]bool, len(jobIDs))
	requestedAttempts := make(map[string]string, len(jobIDs))
	requestedTasks := make(map[string]map[string]bool)
	for _, jobID := range jobIDs {
		if strings.HasPrefix(jobID, "att_") {
			payload, decodeErr := state.DecodeAttemptID(jobID)
			if decodeErr != nil {
				return "", decodeErr
			}
			if payload.RunID != runID {
				return "", fmt.Errorf("attempt %q belongs to run %q, not %q", jobID, payload.RunID, runID)
			}
			attemptDir, pathErr := state.SpecificAttemptJobDir(sourceRunDir, payload.JobID, jobID)
			if pathErr != nil {
				return "", pathErr
			}
			if info, statErr := os.Stat(attemptDir); statErr != nil || !info.IsDir() {
				return "", fmt.Errorf("attempt %q not found in run %s", jobID, runID)
			}
			found := false
			for _, task := range model.QueueToJobs(snapshot.Commands) {
				if task.ID == payload.JobID {
					if task.ArrayGroup != "" {
						requested[task.ArrayGroup] = true
						requestedAttempts[task.ArrayGroup+"/"+task.ID] = jobID
						if requestedTasks[task.ArrayGroup] == nil {
							requestedTasks[task.ArrayGroup] = make(map[string]bool)
						}
						requestedTasks[task.ArrayGroup][task.ID] = true
					} else {
						requested[task.ID] = true
						requestedAttempts[task.ID] = jobID
					}
					found = true
					break
				}
			}
			if !found {
				return "", fmt.Errorf("attempt %q job %q not found in run %s", jobID, payload.JobID, runID)
			}
			continue
		}
		requested[jobID] = true
	}
	selected := make([]QueuedCommand, 0, len(snapshot.Commands))
	selectedNames := make(map[string]bool)
	for _, command := range snapshot.Commands {
		result, finished := model.AggregatedJobResult(command.ID, command.Array, results)
		include := false
		switch selection {
		case "all":
			include = true
		case "job-id":
			include = requested[command.ID]
		default:
			include = model.ResultSelectionMatches(selection, finished, result.ExitCode)
		}
		if requested[command.ID] {
			include = true
			delete(requested, command.ID)
		}
		if include {
			if taskIDs := requestedTasks[command.ID]; len(taskIDs) > 0 {
				narrowArrayCommand(&command, taskIDs)
			}
			selectedNames[command.Name] = true
			selected = append(selected, command)
		}
	}
	if len(requested) > 0 {
		missing := make([]string, 0, len(requested))
		for jobID := range requested {
			missing = append(missing, jobID)
		}
		sort.Strings(missing)
		return "", fmt.Errorf("job IDs not found in run %s: %s", runID, strings.Join(missing, ", "))
	}
	if len(selected) == 0 {
		return "", fmt.Errorf("run %s has no jobs matching selection", runID)
	}
	commandsByName := make(map[string]QueuedCommand, len(snapshot.Commands))
	stageSizes := make(map[string]int)
	for _, command := range snapshot.Commands {
		commandsByName[command.Name] = command
		if command.Stage != "" {
			stageSizes[command.Stage]++
		}
	}
	selectedStageSizes := make(map[string]int)
	for _, command := range selected {
		if command.Stage != "" {
			selectedStageSizes[command.Stage]++
		}
	}
	selectedStages := make(map[string]bool)
	for stage, size := range stageSizes {
		selectedStages[stage] = selectedStageSizes[stage] == size
	}
	for _, command := range selected {
		for _, dependency := range command.DependsOn {
			if selectedNames[dependency] || selectedStages[dependency] {
				continue
			}
			if stageSize, isStage := stageSizes[dependency]; isStage {
				completed := 0
				for _, candidate := range snapshot.Commands {
					if candidate.Stage != dependency {
						continue
					}
					result, finished := model.AggregatedJobResult(candidate.ID, candidate.Array, results)
					if finished && result.ExitCode == 0 {
						completed++
					}
				}
				if completed != stageSize {
					return "", fmt.Errorf("cannot copy job %q: excluded dependency %q did not succeed in run %s", command.Name, dependency, runID)
				}
				continue
			}
			dependencyCommand := commandsByName[dependency]
			result, finished := model.AggregatedJobResult(dependencyCommand.ID, dependencyCommand.Array, results)
			if !finished || result.ExitCode != 0 {
				return "", fmt.Errorf("cannot copy job %q: excluded dependency %q did not succeed in run %s", command.Name, dependency, runID)
			}
		}
	}

	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return "", fmt.Errorf("failed to load queue: %w", err)
	}
	if len(queue.Commands) > 0 && !appendJobs && !overwrite {
		return "", fmt.Errorf("project %q has queued jobs; use --append or --overwrite", queueName)
	}
	existingIDs := make(map[string]bool)
	if appendJobs {
		for _, command := range queue.Commands {
			existingIDs[command.ID] = true
		}
	}
	for index := range selected {
		sourceJobID := selected[index].ID
		dependencies := make([]string, 0, len(selected[index].DependsOn))
		for _, dependency := range selected[index].DependsOn {
			if selectedNames[dependency] || selectedStages[dependency] {
				dependencies = append(dependencies, dependency)
			}
		}
		// Keep the source job ID so the copied job can still be matched
		// against the source run's results; only reassign on collision.
		if existingIDs[sourceJobID] {
			selected[index].ID = makeJobID()
		}
		existingIDs[selected[index].ID] = true
		selected[index].DependsOn = dependencies
		originStatus := "unfinished"
		originAttemptID := ""
		if result, finished := model.AggregatedJobResult(sourceJobID, selected[index].Array, results); finished {
			originAttemptID = result.AttemptID
			originStatus = "failed"
			if result.ExitCode == 0 {
				originStatus = "success"
			}
		}
		if explicitAttemptID, ok := requestedAttempts[sourceJobID]; ok {
			originAttemptID = explicitAttemptID
		}
		submittedAt, finishedAt := originTimestamps(sourceRunDir, sourceJobID, selected[index].Array)
		selected[index].Origin = &JobOrigin{RunID: runID, JobID: sourceJobID, AttemptID: originAttemptID, Status: originStatus, CWD: originCWD, SubmittedAt: submittedAt, FinishedAt: finishedAt}
		if selected[index].Array != nil {
			selected[index].TaskOrigins = copyArrayTaskOrigins(sourceRunDir, runID, sourceJobID, selected[index].Array, results, requestedAttempts, originCWD)
		}
	}
	if !appendJobs {
		queue.Commands = nil
	}
	queue.Commands = append(queue.Commands, selected...)
	if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
		return "", fmt.Errorf("invalid dependencies: %w", err)
	}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		return "", fmt.Errorf("failed to write queue: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return "", fmt.Errorf("failed to update metadata: %w", err)
	}
	return fmt.Sprintf("copied jobs=%d from run=%s to queue=%s", len(selected), runID, queueName), nil
}

func narrowArrayCommand(command *QueuedCommand, taskIDs map[string]bool) {
	if command.Array == nil {
		return
	}
	tasks := make([]int, 0, len(taskIDs))
	for _, task := range model.ArrayTaskIDs(command.Array) {
		if taskIDs[fmt.Sprintf("%s-%d", command.ID, task)] {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		return
	}
	sort.Ints(tasks)
	command.Array = &ArraySpec{First: tasks[0], Last: tasks[len(tasks)-1], Tasks: tasks}
}

func copyArrayTaskOrigins(runDir, runID, commandID string, array *ArraySpec, results map[string]JobResult, requestedAttempts map[string]string, cwd string) map[string]*JobOrigin {
	origins := make(map[string]*JobOrigin)
	for _, task := range model.ArrayTaskIDs(array) {
		taskID := fmt.Sprintf("%s-%d", commandID, task)
		result, finished := results[taskID]
		if !finished {
			continue
		}
		status := "failed"
		if result.ExitCode == 0 {
			status = "success"
		}
		attemptID := result.AttemptID
		if explicitAttemptID, ok := requestedAttempts[commandID+"/"+taskID]; ok {
			attemptID = explicitAttemptID
		}
		origins[taskID] = &JobOrigin{
			RunID: runID, JobID: taskID, AttemptID: attemptID, Status: status, CWD: cwd,
			SubmittedAt: state.ReadJobTimestamp(runDir, taskID, stateFileSubmittedAt),
			FinishedAt:  state.ReadJobTimestamp(runDir, taskID, stateFileFinishedAt),
		}
	}
	return origins
}

// originTimestamps returns the submitted/finished timestamps to record on a
// copied job's Origin. Array jobs are stored per expanded task directory
// (see queueToJobs), so it reports the earliest submission and latest
// completion across all tasks instead of a non-existent "id" directory.
func originTimestamps(runDir, id string, array *ArraySpec) (string, string) {
	if array == nil {
		return state.ReadJobTimestamp(runDir, id, "submitted_at"), state.ReadJobTimestamp(runDir, id, "finished_at")
	}
	var submittedAt, finishedAt string
	for _, task := range model.ArrayTaskIDs(array) {
		taskID := fmt.Sprintf("%s-%d", id, task)
		if value := state.ReadJobTimestamp(runDir, taskID, "submitted_at"); value != "" && (submittedAt == "" || value < submittedAt) {
			submittedAt = value
		}
		if value := state.ReadJobTimestamp(runDir, taskID, "finished_at"); value != "" && value > finishedAt {
			finishedAt = value
		}
	}
	return submittedAt, finishedAt
}

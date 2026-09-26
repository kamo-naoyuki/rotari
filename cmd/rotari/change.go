package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdChange updates queued jobs or prepares a modified follow-up run from a
// completed run snapshot.
func cmdChange(args []string) int {
	fs := flag.NewFlagSet("change", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	jobID := cliString(fs, "job-id", "")
	jobName := cliString(fs, "job-name", "")
	stage := cliString(fs, "stage", "")
	matrixName := cliString(fs, "matrix", "")
	allJobs := cliBool(fs, "all", false)
	executor := cliString(fs, "executor", "")
	workingDirectory := cliString(fs, "working-directory", "")
	clearWorkingDirectory := cliBool(fs, "clear-working-directory", false)
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	clearExecutorOptions := cliBool(fs, "clear-executor-options", false)
	var environment stringSliceFlag
	cliValue(fs, &environment, "env")
	clearEnvironment := cliBool(fs, "clear-env", false)
	setJobName := cliString(fs, "set-job-name", "")
	var dependsOn stringSliceFlag
	cliValue(fs, &dependsOn, "depends-on")
	clearDependsOn := cliBool(fs, "clear-depends-on", false)
	var dependsOnFinished stringSliceFlag
	cliValue(fs, &dependsOnFinished, "depends-on-finished")
	clearDependsOnFinished := cliBool(fs, "clear-depends-on-finished", false)
	timeout := cliString(fs, "timeout", "")
	clearTimeout := cliBool(fs, "clear-timeout", false)
	retry := cliInt(fs, "retry", 0)
	clearRetry := cliBool(fs, "clear-retry", false)
	retryDelay := cliString(fs, "retry-delay", "")
	retryBackoffText := cliString(fs, "retry-backoff", "")
	retryMaxDelay := cliString(fs, "retry-max-delay", "")
	quiet := cliBool(fs, "quiet", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	selector := changeSelector{jobID: *jobID, jobName: *jobName, stage: *stage, matrix: *matrixName, all: *allJobs}
	if selector.count() != 1 ||
		(len(fs.Args()) == 0 && *executor == "" && len(executorOptions) == 0 && !*clearExecutorOptions && *workingDirectory == "" && !*clearWorkingDirectory && len(environment) == 0 && !*clearEnvironment &&
			*setJobName == "" && len(dependsOn) == 0 && !*clearDependsOn && len(dependsOnFinished) == 0 && !*clearDependsOnFinished && *timeout == "" && !*clearTimeout && !cliOptionSet(fs, "retry") && !*clearRetry &&
			*retryDelay == "" && *retryBackoffText == "" && *retryMaxDelay == "") ||
		(*executor != "" && !executorRegistry.Known(*executor)) {
		printError("usage: " + cliUsage("change"))
		return 1
	}
	if selector.multiple() && (len(fs.Args()) > 0 || *setJobName != "") {
		printError("a new command or --set-job-name needs a single job selected with --job-id or --job-name")
		return 1
	}
	if err := model.ValidateEnvironment(environment); err != nil {
		printErrorf("invalid --env: %v", err)
		return 1
	}
	retryBackoff, err := parseRetryBackoff(*retryBackoffText)
	if err == nil {
		err = model.ValidateRetryBackoff(*retryDelay, retryBackoff, *retryMaxDelay)
	}
	if err != nil {
		printError(err)
		return 1
	}

	baseDir, queueName, err := resolve.ExistingRun(*basedir, *queueNameOption, *runID)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := changeQueueJobs(baseDir, queueName, *runID, selector, changeMutation{
		executor: *executor, executorOptions: executorOptions, clearExecutorOptions: *clearExecutorOptions,
		environment: environment, clearEnvironment: *clearEnvironment,
		workingDirectory: *workingDirectory, clearWorkingDirectory: *clearWorkingDirectory, setJobName: *setJobName,
		dependsOn: dependsOn, clearDependsOn: *clearDependsOn,
		dependsOnFinished: dependsOnFinished, clearDependsOnFinished: *clearDependsOnFinished,
		timeout: *timeout, clearTimeout: *clearTimeout, retry: optionalRetry(fs, *retry), clearRetry: *clearRetry,
		retryDelay: *retryDelay, retryBackoff: retryBackoff, retryMaxDelay: *retryMaxDelay,
		command: fs.Args(),
	})
	if err != nil {
		printError(err)
		return 1
	}
	if !*quiet {
		fmt.Println(colorKeyValueMessage(message, green))
	}
	return 0
}

func changeBatch(baseDir, queueName, requestedRunID, requestedJobID, requestedJobName, executor string,
	executorOptions []string, clearExecutorOptions bool, environment []string, clearEnvironment bool, setJobName string, dependsOn []string,
	clearDependsOn bool, command []string) (string, error) {
	return changeQueueJobs(baseDir, queueName, requestedRunID, changeSelector{jobID: requestedJobID, jobName: requestedJobName}, changeMutation{
		executor: executor, executorOptions: executorOptions, clearExecutorOptions: clearExecutorOptions,
		environment: environment, clearEnvironment: clearEnvironment, setJobName: setJobName,
		dependsOn: dependsOn, clearDependsOn: clearDependsOn, command: command,
	})
}

type changeMutation struct {
	executor              string
	executorOptions       []string
	clearExecutorOptions  bool
	environment           []string
	clearEnvironment      bool
	workingDirectory      string
	clearWorkingDirectory bool
	setJobName            string
	dependsOn             []string
	clearDependsOn        bool
	// dependsOnFinished replaces DependsOnFinished when non-empty.
	dependsOnFinished      []string
	clearDependsOnFinished bool
	// timeout replaces Timeout when non-empty.
	timeout      string
	clearTimeout bool
	// retry replaces Retry when set; clearRetry also clears the delay settings.
	retry      *int
	clearRetry bool
	// retryDelay, retryBackoff, and retryMaxDelay replace their fields when
	// set.
	retryDelay    string
	retryBackoff  float64
	retryMaxDelay string
	command       []string
}

// optionalRetry returns the --retry value when it was given.
func optionalRetry(fs *flag.FlagSet, value int) *int {
	if !cliOptionSet(fs, "retry") {
		return nil
	}
	return &value
}

// changeSelector names the jobs a change applies to. Exactly one field is
// set: a job ID or name selects one job; a stage, a matrix base name, or all
// selects every matching job.
type changeSelector struct {
	jobID   string
	jobName string
	stage   string
	matrix  string
	all     bool
}

func (selector changeSelector) count() int {
	count := 0
	for _, set := range []bool{selector.jobID != "", selector.jobName != "", selector.stage != "", selector.matrix != "", selector.all} {
		if set {
			count++
		}
	}
	return count
}

// multiple reports whether the selector may match more than one job.
func (selector changeSelector) multiple() bool {
	return selector.stage != "" || selector.matrix != "" || selector.all
}

// changeQueueJobs applies mutation to the selected jobs of the current queue,
// or of the batch restored from requestedRunID. It returns one line per
// changed job.
func changeQueueJobs(baseDir, queueName, requestedRunID string, selector changeSelector, mutation changeMutation) (string, error) {
	if err := model.ValidateEnvironment(mutation.environment); err != nil {
		return "", fmt.Errorf("invalid environment: %w", err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	var changedIDs []string
	err = project.EditQueue(paths, "change", func(queue *model.Queue) error {
		if requestedRunID != "" {
			snapshot, err := loadChangeSnapshot(paths, requestedRunID)
			if err != nil {
				return err
			}
			*queue = snapshot
		}
		jobs := model.QueueToJobs(queue.Commands)
		indexes, err := selectChangeJobs(*queue, jobs, selector)
		if err != nil {
			return err
		}
		var groupIDs []string
		for _, jobIndex := range indexes {
			if matrix := queue.Commands[jobIndex].Matrix; matrix != nil {
				groupIDs = append(groupIDs, matrix.GroupID)
			}
			if err := applyChangeMutation(*queue, jobIndex, mutation); err != nil {
				return err
			}
		}
		// A group changed the same way throughout still matches its
		// provenance; a partly changed one no longer does.
		model.ClearInconsistentMatrixGroups(queue.Commands, groupIDs)
		if err := validateQueueJobs(*queue); err != nil {
			return err
		}
		if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
			return fmt.Errorf("invalid dependencies: %w", err)
		}
		changedIDs = changedIDs[:0]
		for _, jobIndex := range indexes {
			changedIDs = append(changedIDs, jobs[jobIndex].ID)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	lines := make([]string, len(changedIDs))
	for index, id := range changedIDs {
		lines[index] = fmt.Sprintf("changed queue=%s job=%s", queueName, id)
	}
	return strings.Join(lines, "\n"), nil
}

func selectChangeJobs(queue model.Queue, jobs []model.JobSpec, selector changeSelector) ([]int, error) {
	if !selector.multiple() {
		jobIndex, err := selectChangeJob(jobs, selector.jobID, selector.jobName)
		if err != nil {
			return nil, err
		}
		return []int{jobIndex}, nil
	}
	var indexes []int
	for index, command := range queue.Commands {
		matrix := command.Matrix
		if selector.all || (selector.stage != "" && command.Stage == selector.stage) ||
			(selector.matrix != "" && matrix != nil && matrix.BaseName == selector.matrix) {
			indexes = append(indexes, index)
		}
	}
	if len(indexes) > 0 {
		return indexes, nil
	}
	switch {
	case selector.stage != "":
		return nil, fmt.Errorf("no jobs in stage %q", selector.stage)
	case selector.matrix != "":
		return nil, fmt.Errorf("no matrix named %q", selector.matrix)
	default:
		return nil, errors.New("no jobs to change")
	}
}

func selectChangeJob(jobs []model.JobSpec, requestedJobID, requestedJobName string) (int, error) {
	jobIndex := -1
	for index, job := range jobs {
		if (requestedJobID == "" || job.ID != requestedJobID) &&
			(requestedJobName == "" || job.Name != requestedJobName) {
			continue
		}
		if jobIndex != -1 {
			return -1, fmt.Errorf("job selector matches multiple jobs")
		}
		jobIndex = index
	}
	if jobIndex == -1 {
		return -1, fmt.Errorf("job not found")
	}
	return jobIndex, nil
}

func applyChangeMutation(queue model.Queue, jobIndex int, mutation changeMutation) error {
	changed := &queue.Commands[jobIndex]
	if queue.WorkflowImport {
		changed.Force = true
		changed.Accepted = false
		changed.TaskAccepted = nil
	}
	if mutation.executor != "" {
		changed.Executor = mutation.executor
	}
	if len(mutation.executorOptions) > 0 || mutation.clearExecutorOptions {
		changed.ExecutorOptions = append([]string(nil), mutation.executorOptions...)
	}
	if len(mutation.environment) > 0 || mutation.clearEnvironment {
		changed.Environment = append([]string(nil), mutation.environment...)
	}
	if mutation.workingDirectory != "" || mutation.clearWorkingDirectory {
		changed.WorkingDirectory = mutation.workingDirectory
	}
	if mutation.setJobName != "" && mutation.setJobName != changed.Name {
		if err := validateChangeRename(queue, jobIndex, mutation.setJobName); err != nil {
			return err
		}
		changed.Name = mutation.setJobName
	}
	if len(mutation.command) > 0 {
		changed.Command = append([]string(nil), mutation.command...)
	}
	if len(mutation.dependsOn) > 0 || mutation.clearDependsOn {
		changed.DependsOn = append([]string(nil), mutation.dependsOn...)
	}
	if len(mutation.dependsOnFinished) > 0 || mutation.clearDependsOnFinished {
		changed.DependsOnFinished = append([]string(nil), mutation.dependsOnFinished...)
	}
	if mutation.timeout != "" || mutation.clearTimeout {
		changed.Timeout = mutation.timeout
	}
	if mutation.retry != nil || mutation.clearRetry {
		changed.Retry = mutation.retry
	}
	if mutation.clearRetry {
		changed.RetryDelay, changed.RetryBackoff, changed.RetryMaxDelay = "", 0, ""
	}
	if mutation.retryDelay != "" {
		changed.RetryDelay = mutation.retryDelay
	}
	if mutation.retryBackoff != 0 {
		changed.RetryBackoff = mutation.retryBackoff
	}
	if mutation.retryMaxDelay != "" {
		changed.RetryMaxDelay = mutation.retryMaxDelay
	}
	return nil
}

func validateChangeRename(queue model.Queue, jobIndex int, newName string) error {
	jobs := model.QueueToJobs(queue.Commands)
	for index, job := range jobs {
		if index != jobIndex && job.Name == newName {
			return fmt.Errorf("job name %q is already in use", newName)
		}
	}
	oldName := jobs[jobIndex].Name
	for index, command := range queue.Commands {
		if index == jobIndex {
			continue
		}
		for _, dependency := range command.AllDependencies() {
			if dependency == oldName {
				return fmt.Errorf("job %q is referenced by dependency; rename is not allowed", oldName)
			}
		}
	}
	return nil
}

func loadChangeSnapshot(paths state.ProjectPaths, requestedRunID string) (model.Queue, error) {
	runID, err := resolve.RunID(paths, requestedRunID)
	if err != nil {
		return model.Queue{}, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return model.Queue{}, err
	}
	queue, err := state.ReadQueueFile(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return model.Queue{}, fmt.Errorf("failed to load command snapshot: %w", err)
	}
	if len(queue.Commands) == 0 {
		return model.Queue{}, errors.New("command snapshot has no jobs")
	}
	return queue, nil
}

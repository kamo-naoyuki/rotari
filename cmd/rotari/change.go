package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
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
	quiet := cliBool(fs, "quiet", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if (*jobID == "" && *jobName == "") || (*jobID != "" && *jobName != "") ||
		(len(fs.Args()) == 0 && *executor == "" && len(executorOptions) == 0 && !*clearExecutorOptions && *workingDirectory == "" && !*clearWorkingDirectory && len(environment) == 0 && !*clearEnvironment &&
			*setJobName == "" && len(dependsOn) == 0 && !*clearDependsOn && len(dependsOnFinished) == 0 && !*clearDependsOnFinished) ||
		(*executor != "" && !executorRegistry.Known(*executor)) {
		printError("usage: " + cliUsage("change"))
		return 1
	}
	if err := model.ValidateEnvironment(environment); err != nil {
		printErrorf("invalid --env: %v", err)
		return 1
	}

	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runID)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := changeQueueJob(baseDir, queueName, *runID, *jobID, *jobName, changeMutation{
		executor: *executor, executorOptions: executorOptions, clearExecutorOptions: *clearExecutorOptions,
		environment: environment, clearEnvironment: *clearEnvironment,
		workingDirectory: *workingDirectory, clearWorkingDirectory: *clearWorkingDirectory, setJobName: *setJobName,
		dependsOn: dependsOn, clearDependsOn: *clearDependsOn,
		dependsOnFinished: dependsOnFinished, clearDependsOnFinished: *clearDependsOnFinished, command: fs.Args(),
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
	return changeQueueJob(baseDir, queueName, requestedRunID, requestedJobID, requestedJobName, changeMutation{
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
	command                []string
}

// changeQueueJob applies mutation to one job of the current queue, or of the
// batch restored from requestedRunID.
func changeQueueJob(baseDir, queueName, requestedRunID, requestedJobID, requestedJobName string, mutation changeMutation) (string, error) {
	if err := model.ValidateEnvironment(mutation.environment); err != nil {
		return "", fmt.Errorf("invalid environment: %w", err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "change"); err != nil {
		return "", err
	}

	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return "", fmt.Errorf("failed to load queue: %w", err)
	}
	if requestedRunID != "" {
		queue, err = loadChangeSnapshot(paths, requestedRunID)
		if err != nil {
			return "", err
		}
	}

	jobs := model.QueueToJobs(queue.Commands)
	jobIndex, err := selectChangeJob(jobs, requestedJobID, requestedJobName)
	if err != nil {
		return "", err
	}
	if matrix := queue.Commands[jobIndex].Matrix; matrix != nil {
		model.ClearMatrixGroup(queue.Commands, matrix.GroupID)
	}
	if err := applyChangeMutation(queue, jobIndex, mutation); err != nil {
		return "", err
	}
	if err := validateQueueJobs(queue); err != nil {
		return "", err
	}
	if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
		return "", fmt.Errorf("invalid dependencies: %w", err)
	}
	if err := writeIdleQueue(paths, queue); err != nil {
		return "", err
	}
	return fmt.Sprintf("changed queue=%s job=%s", queueName, jobs[jobIndex].ID), nil
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
	runID, err := selectRunID(paths, requestedRunID)
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

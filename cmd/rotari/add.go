package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdAdd appends a command to the current project queue through the background
// server.
func cmdAdd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	executor := cliString(fs, "executor", "")
	workingDirectory := cliString(fs, "working-directory", "")
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	var environment stringSliceFlag
	cliValue(fs, &environment, "env")
	jobName := cliString(fs, "job-name", "")
	stage := cliString(fs, "stage", "")
	var dependsOn stringSliceFlag
	cliValue(fs, &dependsOn, "depends-on")
	arrayRange := cliString(fs, "array", "")
	var matrixValues stringSliceFlag
	cliValue(fs, &matrixValues, "matrix")
	quiet := cliBool(fs, "quiet", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	left := fs.Args()
	if len(left) < 1 {
		printError("usage: " + cliUsage("add"))
		return 1
	}
	var array *ArraySpec
	if *arrayRange != "" {
		parsed, parseErr := model.ParseArrayRange(*arrayRange)
		if parseErr != nil {
			printErrorf("invalid --array: %v", parseErr)
			return 1
		}
		array = &parsed
	}
	dimensions := make([]model.MatrixDimension, 0, len(matrixValues))
	dimensionNames := make(map[string]bool, len(matrixValues))
	for _, value := range matrixValues {
		dimension, parseErr := model.ParseMatrixDimension(value)
		if parseErr != nil {
			printErrorf("invalid --matrix: %v", parseErr)
			return 1
		}
		if dimensionNames[dimension.Name] {
			printErrorf("invalid --matrix: duplicate key %q", dimension.Name)
			return 1
		}
		dimensionNames[dimension.Name] = true
		dimensions = append(dimensions, dimension)
	}
	baseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := resolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	if err := model.ValidateEnvironment(environment); err != nil {
		printErrorf("invalid --env: %v", err)
		return 1
	}
	commands := expandMatrixCommands(left, *executor, executorOptions, environment, *workingDirectory, *jobName, *stage, dependsOn, dimensions)
	message, err := enqueueCommands(baseDir, queueName, commands, array)
	if err != nil {
		printError(err)
		return 1
	}
	if !*quiet {
		fmt.Println(colorKeyValueMessage(message, green))
	}
	return 0
}

func expandMatrixCommands(command []string, executor string, executorOptions, environment []string, workingDirectory, jobName, stage string, dependsOn []string, dimensions []model.MatrixDimension) []QueuedCommand {
	if len(dimensions) == 0 {
		return []QueuedCommand{{Command: command, Executor: executor, ExecutorOptions: executorOptions, Environment: environment, WorkingDirectory: workingDirectory, Name: jobName, Stage: stage, DependsOn: dependsOn}}
	}
	groupID := makeJobID()
	commands := make([]QueuedCommand, 0)
	for _, combination := range model.ExpandMatrix(dimensions) {
		matrixEnvironment := model.MatrixEnvironment(environment, combination)
		matrixName := model.MatrixJobName(jobName, combination)
		commands = append(commands, QueuedCommand{
			Command: command, Executor: executor, ExecutorOptions: executorOptions, Environment: matrixEnvironment,
			WorkingDirectory: workingDirectory, Name: matrixName, Stage: stage, DependsOn: dependsOn,
			Matrix: &model.MatrixSpec{
				GroupID: groupID, Dimensions: cloneMatrixDimensions(dimensions), Values: append([]model.MatrixValue(nil), combination...),
				BaseName: jobName, BaseEnvironment: append([]string(nil), environment...),
			},
		})
	}
	return commands
}

func cloneMatrixDimensions(dimensions []model.MatrixDimension) []model.MatrixDimension {
	cloned := make([]model.MatrixDimension, len(dimensions))
	for index, dimension := range dimensions {
		cloned[index] = model.MatrixDimension{Name: dimension.Name, Values: append([]string(nil), dimension.Values...)}
	}
	return cloned
}

func sanitizeMatrixName(value string) string {
	return model.SanitizeMatrixName(value)
}

// enqueueCommands appends commands to the project queue. A non-nil array
// makes every command an array job.
func enqueueCommands(baseDir, queueName string, commands []QueuedCommand, array *ArraySpec) (string, error) {
	if queueName == "" || len(commands) == 0 {
		return "", errors.New("project name and command are required")
	}
	for index := range commands {
		if array != nil {
			commands[index].Array = array
		}
		if err := model.ValidateEnvironment(commands[index].Environment); err != nil {
			return "", fmt.Errorf("invalid environment: %w", err)
		}
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.ProjectDir, stateDirMode()); err != nil {
		return "", err
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return "", err
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "add"); err != nil {
		return "", err
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return "", err
	}
	for index := range commands {
		if commands[index].Executor != "" && !isKnownExecutor(commands[index].Executor) {
			return "", fmt.Errorf("unsupported executor: %s", commands[index].Executor)
		}
		commands[index].ID = makeJobID()
		if queue.WorkflowImport {
			commands[index].Force = true
		}
	}
	if err := validateQueueJobs(Queue{Commands: append(append([]QueuedCommand(nil), queue.Commands...), commands...)}); err != nil {
		return "", err
	}
	// Dependencies may refer to jobs added later, so only duplicate names are checked here.
	for _, command := range commands {
		if command.Name != "" {
			for _, existing := range queue.Commands {
				if existing.Name == command.Name {
					return "", fmt.Errorf("invalid dependencies: duplicate job name: %s", command.Name)
				}
			}
		}
	}
	queue.Commands = append(queue.Commands, commands...)
	if err := writeIdleQueue(paths, queue); err != nil {
		return "", err
	}
	message := fmt.Sprintf("added project=%s jobs=%d", queueName, len(commands))
	if len(commands) == 1 {
		message = fmt.Sprintf("added project=%s job_id=%s", queueName, commands[0].ID)
		if commands[0].Name != "" {
			message += fmt.Sprintf(" job_name=%s", commands[0].Name)
		}
	}
	return fmt.Sprintf("%s command=%s", message, joinCommand(commands[0].Command)), nil
}

func joinCommand(command []string) string {
	return fmt.Sprintf("%v", command)
}

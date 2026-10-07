package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdAdd appends a command to the current project queue under the state lock.
func cmdAdd(args []string) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	executor := cliString(fs, "executor", "")
	workingDirectory := cliString(fs, "working-directory", "")
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")
	var outputPaths stringSliceFlag
	cliValue(fs, &outputPaths, "output")
	var errorPaths stringSliceFlag
	cliValue(fs, &errorPaths, "error")
	var artifacts stringSliceFlag
	cliValue(fs, &artifacts, "artifact")
	logMode := cliString(fs, "log-mode", model.LogModeMerge)
	openMode := cliString(fs, "open-mode", model.OpenModeAppend)
	var environment stringSliceFlag
	cliValue(fs, &environment, "env")
	jobName := cliString(fs, "job-name", "")
	stage := cliString(fs, "stage", "")
	var dependsOn stringSliceFlag
	cliValue(fs, &dependsOn, "depends-on")
	var dependsOnFinished stringSliceFlag
	cliValue(fs, &dependsOnFinished, "depends-on-finished")
	timeout := cliString(fs, "timeout", "")
	retry := cliInt(fs, "retry", 0)
	retryDelay := cliString(fs, "retry-delay", "")
	retryBackoffText := cliString(fs, "retry-backoff", "")
	retryMaxDelay := cliString(fs, "retry-max-delay", "")
	arrayRange := cliString(fs, "array", "")
	var matrixValues stringSliceFlag
	cliValue(fs, &matrixValues, "matrix")
	var matrixExclusionValues stringSliceFlag
	cliValue(fs, &matrixExclusionValues, "matrix-exclude")
	quiet := cliBool(fs, "quiet", false)
	guard := cliGuardFlags(fs)
	if err := parseLeadingFlags(fs, args); err != nil {
		return 1
	}
	left := fs.Args()
	if len(left) < 1 {
		printError("usage: " + cliUsage("add"))
		return 1
	}
	var array *model.ArraySpec
	if *arrayRange != "" {
		parsed, parseErr := model.ParseArrayRange(*arrayRange)
		if parseErr != nil {
			printErrorf("invalid --array: %v", parseErr)
			return 1
		}
		array = &parsed
	}
	matrix, err := parseAddMatrix(matrixValues, matrixExclusionValues)
	if err != nil {
		printError(err)
		return 1
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := state.ResolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	if !resolve.ProjectExists(baseDir, queueName) {
		if err := model.ValidateReservedName("project", queueName); err != nil {
			printError(err)
			return 1
		}
	}
	if err := model.ValidateEnvironment(environment); err != nil {
		printErrorf("invalid --env: %v", err)
		return 1
	}
	if *logMode != model.LogModeMerge && *logMode != model.LogModeSeparate {
		printError("invalid --log-mode: want merge or separate")
		return 1
	}
	if *openMode != model.OpenModeAppend && *openMode != model.OpenModeTruncate {
		printError("invalid --open-mode: want append or truncate")
		return 1
	}
	if err := validateOutputPaths(outputPaths, errorPaths); err != nil {
		printError(err)
		return 1
	}
	if err := model.ValidateArtifacts(artifacts); err != nil {
		printErrorf("invalid --artifact: %v", err)
		return 1
	}
	if *timeout != "" {
		if _, err := model.ParseTimeout(*timeout); err != nil {
			printErrorf("invalid --timeout: %v", err)
			return 1
		}
	}
	var jobRetry *int
	if cliOptionSet(fs, "retry") {
		if *retry < 0 {
			printError("invalid --retry: must be 0 or more")
			return 1
		}
		jobRetry = retry
	}
	retryBackoff, err := parseRetryBackoff(*retryBackoffText)
	if err == nil {
		err = model.ValidateRetryBackoff(*retryDelay, retryBackoff, *retryMaxDelay)
	}
	if err != nil {
		printError(err)
		return 1
	}
	commands := expandMatrixCommands(left, addCommandOptions{
		executor: *executor, executorOptions: executorOptions, environment: environment,
		workingDirectory: *workingDirectory, jobName: *jobName, stage: *stage,
		dependsOn: dependsOn, matrix: matrix,
	})
	for index := range commands {
		commands[index].Output = normalizeOutputPaths(outputPaths)
		commands[index].Error = normalizeOutputPaths(errorPaths)
		commands[index].Artifacts = append([]string(nil), artifacts...)
		if cliOptionSet(fs, "log-mode") {
			commands[index].LogMode = *logMode
		}
		if cliOptionSet(fs, "open-mode") {
			commands[index].OpenMode = *openMode
		}
		commands[index].DependsOnFinished = dependsOnFinished
		commands[index].Timeout = *timeout
		commands[index].Retry = jobRetry
		commands[index].RetryDelay = *retryDelay
		commands[index].RetryBackoff = retryBackoff
		commands[index].RetryMaxDelay = *retryMaxDelay
	}
	editor := guard.editor()
	editor.Warn = func(message string) { printWarningf("%s", message) }
	message, err := editor.Add(baseDir, queueName, commands, array)
	if err != nil {
		printError(err)
		return 1
	}
	guard.printResult(message, *quiet)
	return 0
}

func validateOutputPaths(outputPaths, errorPaths []string) error {
	for _, paths := range [][]string{outputPaths, errorPaths} {
		seen := make(map[string]bool, len(paths))
		for _, path := range paths {
			if path == "" || strings.IndexByte(path, 0) >= 0 {
				return fmt.Errorf("output destinations must be non-empty and must not contain NUL")
			}
			cleaned := filepath.Clean(path)
			if seen[cleaned] {
				return fmt.Errorf("output destination %q was specified more than once", path)
			}
			seen[cleaned] = true
		}
	}
	return nil
}

func normalizeOutputPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		result = append(result, filepath.Clean(path))
	}
	return result
}

type addMatrixExpansion struct {
	dimensions   []model.MatrixDimension
	exclusions   []model.MatrixExclusion
	combinations [][]model.MatrixValue
}

func parseAddMatrix(matrixValues, exclusionValues []string) (addMatrixExpansion, error) {
	dimensions := make([]model.MatrixDimension, 0, len(matrixValues))
	dimensionNames := make(map[string]bool, len(matrixValues))
	for _, value := range matrixValues {
		dimension, err := model.ParseMatrixDimension(value)
		if err != nil {
			return addMatrixExpansion{}, fmt.Errorf("invalid --matrix: %w", err)
		}
		if dimensionNames[dimension.Name] {
			return addMatrixExpansion{}, fmt.Errorf("invalid --matrix: duplicate key %q", dimension.Name)
		}
		dimensionNames[dimension.Name] = true
		dimensions = append(dimensions, dimension)
	}
	if len(exclusionValues) > 0 && len(dimensions) == 0 {
		return addMatrixExpansion{}, fmt.Errorf("--matrix-exclude requires --matrix")
	}
	exclusions := make([]model.MatrixExclusion, 0, len(exclusionValues))
	for _, value := range exclusionValues {
		exclusion, err := model.ParseMatrixExclusion(value)
		if err != nil {
			return addMatrixExpansion{}, fmt.Errorf("invalid --matrix-exclude: %w", err)
		}
		exclusions = append(exclusions, exclusion)
	}
	combinations, exclusions, err := model.ExpandMatrixWithExclusions(dimensions, exclusions)
	if err != nil {
		return addMatrixExpansion{}, fmt.Errorf("invalid --matrix-exclude: %w", err)
	}
	return addMatrixExpansion{dimensions: dimensions, exclusions: exclusions, combinations: combinations}, nil
}

type addCommandOptions struct {
	executor         string
	executorOptions  []string
	environment      []string
	workingDirectory string
	jobName          string
	stage            string
	dependsOn        []string
	matrix           addMatrixExpansion
}

func expandMatrixCommands(command []string, options addCommandOptions) []model.QueuedCommand {
	if len(options.matrix.dimensions) == 0 {
		return []model.QueuedCommand{{Command: command, Executor: options.executor, ExecutorOptions: options.executorOptions, Environment: options.environment, WorkingDirectory: options.workingDirectory, Name: options.jobName, Stage: options.stage, DependsOn: options.dependsOn}}
	}
	groupID := makeJobID()
	commands := make([]model.QueuedCommand, 0, len(options.matrix.combinations))
	for _, combination := range options.matrix.combinations {
		matrixEnvironment := model.MatrixEnvironment(options.environment, combination)
		matrixName := model.MatrixJobName(options.jobName, combination)
		commands = append(commands, model.QueuedCommand{
			Command: command, Executor: options.executor, ExecutorOptions: options.executorOptions, Environment: matrixEnvironment,
			WorkingDirectory: options.workingDirectory, Name: matrixName, Stage: options.stage, DependsOn: options.dependsOn,
			Matrix: &model.MatrixSpec{
				GroupID: groupID, Dimensions: cloneMatrixDimensions(options.matrix.dimensions), Values: append([]model.MatrixValue(nil), combination...),
				Exclusions: model.CloneMatrixExclusions(options.matrix.exclusions),
				BaseName:   options.jobName, BaseEnvironment: append([]string(nil), options.environment...),
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

// parseRetryBackoff parses --retry-backoff; empty means no backoff factor.
func parseRetryBackoff(value string) (float64, error) {
	if value == "" {
		return 0, nil
	}
	factor, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid --retry-backoff %q: want a number such as 2", value)
	}
	return factor, nil
}

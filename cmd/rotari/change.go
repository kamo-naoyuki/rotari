package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
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
	selector := model.CommandSelector{Name: *jobName, Stage: *stage, Matrix: *matrixName, All: *allJobs}
	if *jobID != "" {
		selector.IDs = []string{*jobID}
	}
	if selector.Kinds() != 1 ||
		(len(fs.Args()) == 0 && *executor == "" && len(executorOptions) == 0 && !*clearExecutorOptions && *workingDirectory == "" && !*clearWorkingDirectory && len(environment) == 0 && !*clearEnvironment &&
			*setJobName == "" && len(dependsOn) == 0 && !*clearDependsOn && len(dependsOnFinished) == 0 && !*clearDependsOnFinished && *timeout == "" && !*clearTimeout && !cliOptionSet(fs, "retry") && !*clearRetry &&
			*retryDelay == "" && *retryBackoffText == "" && *retryMaxDelay == "") ||
		(*executor != "" && !executorRegistry.Known(*executor)) {
		printError("usage: " + cliUsage("change"))
		return 1
	}
	if selector.Group() && (len(fs.Args()) > 0 || *setJobName != "") {
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

	if *queueNameOption == "" && *runID == "" {
		project, err := locateQueuedJobs(*basedir, selector)
		if err != nil {
			printError(err)
			return 1
		}
		*queueNameOption = project
	}
	baseDir, queueName, err := resolve.ExistingRun(*basedir, *queueNameOption, *runID)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := queueEditor().Change(baseDir, queueName, *runID, selector, queueops.Mutation{
		Executor: *executor, ExecutorOptions: executorOptions, ClearExecutorOptions: *clearExecutorOptions,
		Environment: environment, ClearEnvironment: *clearEnvironment,
		WorkingDirectory: *workingDirectory, ClearWorkingDirectory: *clearWorkingDirectory, SetJobName: *setJobName,
		DependsOn: dependsOn, ClearDependsOn: *clearDependsOn,
		DependsOnFinished: dependsOnFinished, ClearDependsOnFinished: *clearDependsOnFinished,
		Timeout: *timeout, ClearTimeout: *clearTimeout, Retry: optionalRetry(fs, *retry), ClearRetry: *clearRetry,
		RetryDelay: *retryDelay, RetryBackoff: retryBackoff, RetryMaxDelay: *retryMaxDelay,
		Command: fs.Args(),
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

// optionalRetry returns the --retry value when it was given.
func optionalRetry(fs *flag.FlagSet, value int) *int {
	if !cliOptionSet(fs, "retry") {
		return nil
	}
	return &value
}

// locateQueuedJobs finds the project whose queue holds the jobs a job ID or
// name selector names, for queue edits without a project, as other commands
// search every project. It returns "" when selector names a group or no
// queue holds the jobs, leaving the normal project resolution to report it.
func locateQueuedJobs(cliBaseDir string, selector model.CommandSelector) (string, error) {
	if len(selector.IDs) == 0 && selector.Name == "" {
		return "", nil
	}
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return "", err
	}
	projects, err := resolve.ProjectNames(baseDir, "")
	if err != nil {
		return "", err
	}
	var targets []resolve.Job
	for _, project := range projects {
		paths, err := state.ResolveProjectPaths(baseDir, project)
		if err != nil {
			return "", err
		}
		queue, err := state.LoadQueue(paths.QueueFile)
		if err != nil || len(queue.Commands) == 0 {
			continue
		}
		if indexes, err := model.SelectCommands(queue.Commands, selector); err == nil {
			targets = append(targets, resolve.Job{Run: resolve.Run{BaseDir: baseDir, ProjectName: project}, JobID: queue.Commands[indexes[0]].ID, FromQueue: true})
		}
	}
	switch len(targets) {
	case 0:
		return "", nil
	case 1:
		return targets[0].ProjectName, nil
	}
	what := fmt.Sprintf("job %q", strings.Join(selector.IDs, ","))
	if selector.Name != "" {
		what = fmt.Sprintf("job name %q", selector.Name)
	}
	return "", resolve.AmbiguousError(what, targets)
}

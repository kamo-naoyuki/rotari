package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
)

// stringSliceFlag collects repeated occurrences of a CLI flag (e.g. --executor-option).
type stringSliceFlag []string

func (flag *stringSliceFlag) String() string {
	return strings.Join(*flag, ",")
}

func (flag *stringSliceFlag) Set(value string) error {
	*flag = append(*flag, value)
	return nil
}

func (flag *stringSliceFlag) Reset() {
	*flag = nil
}

var executorRunSettingNames = []string{"ssh", "slurm", "pbs", "lsf"}

// cliExecutorRunSettings registers the per-executor run-setting flags on fs
// and returns a function that builds the settings from their values. Call it
// after fs.Parse so command-line values are included.
func cliExecutorRunSettings(fs *flag.FlagSet) func() executor.RunSettingsMap {
	type settingFlags struct {
		concurrency      *int
		options          *stringSliceFlag
		submitInterval   *time.Duration
		submitRetryLimit *int
	}
	registered := make(map[string]settingFlags, len(executorRunSettingNames))
	for _, name := range executorRunSettingNames {
		flags := settingFlags{concurrency: cliInt(fs, name+"-concurrency", 0), options: new(stringSliceFlag)}
		cliValue(fs, flags.options, name+"-options")
		if name != "ssh" {
			flags.submitInterval = cliDuration(fs, name+"-submit-interval", 0)
			flags.submitRetryLimit = cliInt(fs, name+"-submit-retry-limit", 0)
		}
		registered[name] = flags
	}
	return func() executor.RunSettingsMap {
		settings := make(executor.RunSettingsMap, len(registered))
		for name, flags := range registered {
			setting := executor.RunSettings{Concurrency: *flags.concurrency, Options: *flags.options}
			if flags.submitInterval != nil {
				setting.SubmitInterval = *flags.submitInterval
				setting.SubmitRetryLimit = *flags.submitRetryLimit
			}
			settings[name] = setting
		}
		return settings
	}
}

func executorSettingsFor(settings executor.RunSettingsMap, name string) executor.RunSettings {
	if settings == nil {
		return executor.RunSettings{}
	}
	return settings[name]
}

func effectiveExecutorConcurrency(settings executor.RunSettingsMap, name string, fallback int) int {
	if concurrency := executorSettingsFor(settings, name).Concurrency; concurrency > 0 {
		return concurrency
	}
	return fallback
}

func effectiveExecutorOptions(settings executor.RunSettingsMap, name string, fallback []string) []string {
	if options := executorSettingsFor(settings, name).Options; len(options) > 0 {
		return options
	}
	return fallback
}

// executorRegistry is initialized eagerly (rather than in an init func) so
// that other package-level vars, such as cliCommandSpecs, can depend on
// executorRegistry.Names() during their own initialization.
var executorRegistry = executor.NewRegistry(jsonStore(), jobLogf)

func validateQueueForRun(queue model.Queue, requestedExecutor string, executorOptions []string, settings executor.RunSettingsMap) error {
	if err := queueops.ValidateJobs(queue); err != nil {
		return err
	}
	if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
		return fmt.Errorf("invalid dependencies: %w", err)
	}
	defaultExecutor := requestedExecutor
	if defaultExecutor == "" {
		defaultExecutor = queue.DefaultExecutor
	}
	if defaultExecutor == "" {
		defaultExecutor = "local"
	}
	if !executorRegistry.Known(defaultExecutor) {
		return fmt.Errorf("unsupported executor: %s", defaultExecutor)
	}

	for _, queued := range queue.Commands {
		executorName := queued.Executor
		if executorName == "" {
			executorName = defaultExecutor
		}
		if !executorRegistry.Known(executorName) {
			return fmt.Errorf("job %q uses unsupported executor: %s", queued.ID, executorName)
		}
		options := queued.ExecutorOptions
		if len(options) == 0 {
			options = effectiveExecutorOptions(settings, executorName, executorOptions)
		}
		if len(options) == 0 {
			options = queue.DefaultExecutorOptions
		}
		if err := validateExecutorOptions(executorName, options, queued.Array != nil); err != nil {
			return fmt.Errorf("job %q executor options: %w", queued.ID, err)
		}
	}
	return nil
}

func validateExecutorOptions(executorName string, options []string, array bool) error {
	switch executorName {
	case "local":
		return nil
	case "ssh":
		_, _, err := executor.SSHTarget(options)
		return err
	case "slurm":
		if array {
			return executor.RejectArraySchedulerOptions(options, "--array")
		}
	case "pbs":
		if array {
			return executor.RejectArraySchedulerOptions(options, "-J", "-t")
		}
	case "lsf":
		if array {
			return executor.RejectArraySchedulerOptions(options, "-J")
		}
	default:
		return fmt.Errorf("unsupported executor: %s", executorName)
	}
	_, err := executor.ExpandShellOptions(options)
	return err
}

func validateLocalExecutionEnvironment(queue model.Queue) error {
	defaultExecutor := queue.DefaultExecutor
	if defaultExecutor == "" {
		defaultExecutor = "local"
	}
	checkedExecutors := make(map[string]bool)
	for _, queued := range queue.Commands {
		executorName := queued.Executor
		if executorName == "" {
			executorName = defaultExecutor
		}
		if !checkedExecutors[executorName] {
			if err := validateExecutorCommand(executorName); err != nil {
				return err
			}
			checkedExecutors[executorName] = true
		}
		if executorName == "local" {
			if err := validateLocalJobEnvironment(queued); err != nil {
				return fmt.Errorf("job %q: %w", queued.ID, err)
			}
		}
	}
	return nil
}

func validateExecutorCommand(executorName string) error {
	command := map[string]string{
		"local": "/bin/sh",
		"ssh":   executor.SSHCommandPath,
		"slurm": "sbatch",
		"pbs":   "qsub",
		"lsf":   "bsub",
	}[executorName]
	if command == "" {
		return fmt.Errorf("unsupported executor: %s", executorName)
	}
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("%s executor command %q is not available: %w", executorName, command, err)
	}
	return nil
}

func validateLocalJobEnvironment(job model.QueuedCommand) error {
	if job.WorkingDirectory != "" {
		info, err := os.Stat(job.WorkingDirectory)
		if err != nil {
			return fmt.Errorf("working directory %q is not accessible: %w", job.WorkingDirectory, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("working directory %q is not a directory", job.WorkingDirectory)
		}
	}
	command := exec.Command("/bin/sh", "-c", `command -v "$1" >/dev/null 2>&1`, "sh", job.Command[0])
	command.Dir = job.WorkingDirectory
	command.Env = executor.MergeEnvironment(os.Environ(), job.Environment)
	if err := command.Run(); err != nil {
		return fmt.Errorf("command %q is not available", job.Command[0])
	}
	return nil
}

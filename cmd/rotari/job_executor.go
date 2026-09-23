package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
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

type JobHandle = executor.JobHandle
type JobExecutor = executor.JobExecutor
type ArraySubmitter = executor.ArraySubmitter
type Suspender = executor.Suspender
type Canceller = executor.Canceller

// ExecutorRunSettings controls the dispatch defaults for one executor.
// Job-specific options remain higher priority than these values.
type ExecutorRunSettings = executor.RunSettings
type executorRunSettingsMap = executor.RunSettingsMap

var executorRunSettingNames = []string{"ssh", "slurm", "pbs", "lsf"}

func cliExecutorRunSettings(fs *flag.FlagSet) executorRunSettingsMap {
	settings := make(executorRunSettingsMap)
	for _, name := range executorRunSettingNames {
		concurrency := cliInt(fs, name+"-concurrency", 0)
		var options stringSliceFlag
		cliValue(fs, &options, name+"-options")
		settings[name] = ExecutorRunSettings{Concurrency: *concurrency, Options: options}
	}
	return settings
}

func executorSettingsFor(settings executorRunSettingsMap, name string) ExecutorRunSettings {
	if settings == nil {
		return ExecutorRunSettings{}
	}
	return settings[name]
}

func effectiveExecutorConcurrency(settings executorRunSettingsMap, name string, fallback int) int {
	if concurrency := executorSettingsFor(settings, name).Concurrency; concurrency > 0 {
		return concurrency
	}
	return fallback
}

func effectiveExecutorOptions(settings executorRunSettingsMap, name string, fallback []string) []string {
	if options := executorSettingsFor(settings, name).Options; len(options) > 0 {
		return options
	}
	return fallback
}

// jobOwnerExecutor determines which executor owns the job recorded in jobDir,
// based on the metadata files each executor writes on submission. Every
// scheduler-style executor writes job.json with its own "executor" name, so
// that job.json alone (not its mere existence) tells us which one to use.
func jobOwnerExecutor(jobDir string) (JobExecutor, error) {
	if path, err := state.ValidatedStateFile(jobDir, stateFileJobJSON); err == nil { // NOSONAR: jobDir is restricted to validated job-path boundaries
		var meta struct {
			Executor string `json:"executor"`
		}
		if err := jsonStore().ReadJSON(path, &meta); err == nil {
			if executor, ok := lookupExecutor(meta.Executor); ok {
				return executor, nil
			}
		}
	}
	if path, err := state.ValidatedStateFile(jobDir, stateFilePID); err == nil {
		if _, err := os.Stat(path); err == nil {
			executor, _ := lookupExecutor("local")
			return executor, nil
		}
	}
	return nil, fmt.Errorf("job is not running")
}

// localExecutorHostMismatch reports the run's recorded hostname when the
// given job belongs to the "local" executor and this process is running on a
// different host. The local executor signals jobs by PID, which is only
// meaningful on the host that actually spawned the process; over a shared
// base directory, "job is not running" from a failed PID check on the wrong
// host is misleading, so callers should surface this instead before trying.
func localExecutorHostMismatch(executor JobExecutor, runDir string) (recordedHost string, mismatch bool) {
	if executor.Name() != "local" {
		return "", false
	}
	safeRunDir := filepath.Join(filepath.Dir(runDir), filepath.Base(runDir))
	context, err := state.LoadContext(jsonStore(), safeRunDir)
	if err != nil || context.Hostname == "" {
		return "", false
	}
	host, err := os.Hostname()
	if err != nil || strings.EqualFold(host, context.Hostname) {
		return "", false
	}
	return context.Hostname, true
}

// runningWorkerHostMismatch reports the recorded host when a run's
// running.lock belongs to a different host than this process. Whole-run
// cancel (no --job-id) signals the runner's process group by the PID stored
// in that lock; on the wrong host that PID belongs to (at best) nothing, so
// the signal harmlessly returns ESRCH and callers ignore it -- silently
// reporting success without actually cancelling anything. This mirrors the
// host check `isRunning` already applies before trusting a local PID.
func runningWorkerHostMismatch(lock LockInfo) (recordedHost string, mismatch bool) {
	if lock.Host == "" {
		return "", false
	}
	host, err := os.Hostname()
	if err != nil || strings.EqualFold(host, lock.Host) {
		return "", false
	}
	return lock.Host, true
}

// executorRegistry is initialized eagerly (rather than in an init func) so
// that other package-level vars, such as cliCommandSpecs, can depend on
// executorNames() during their own initialization.
var executorRegistry = map[string]JobExecutor{
	"local": executor.NewLocal(jsonStore(), jobLogf),
	"slurm": executor.NewSlurm(jsonStore(), jobLogf),
	"pbs":   executor.NewPBS(jsonStore(), jobLogf),
	"lsf":   executor.NewLSF(jsonStore(), jobLogf),
	"ssh":   executor.NewSSH(jsonStore()),
}

func lookupExecutor(name string) (JobExecutor, bool) {
	b, ok := executorRegistry[name]
	return b, ok
}

func isKnownExecutor(name string) bool {
	_, ok := executorRegistry[name]
	return ok
}

// executorNames returns the registered executor names, sorted for stable output.
func executorNames() []string {
	names := make([]string, 0, len(executorRegistry))
	for name := range executorRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateQueueForRun(queue Queue, requestedExecutor string, executorOptions []string, settings executorRunSettingsMap) error {
	if err := validateQueueJobs(queue); err != nil {
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
	if !isKnownExecutor(defaultExecutor) {
		return fmt.Errorf("unsupported executor: %s", defaultExecutor)
	}

	for _, queued := range queue.Commands {
		executorName := queued.Executor
		if executorName == "" {
			executorName = defaultExecutor
		}
		if !isKnownExecutor(executorName) {
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

func validateLocalExecutionEnvironment(queue Queue) error {
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

func validateLocalJobEnvironment(job QueuedCommand) error {
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

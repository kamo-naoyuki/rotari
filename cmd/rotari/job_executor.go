package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
)

type JobHandle = executor.JobHandle
type JobExecutor = executor.JobExecutor
type ArraySubmitter = executor.ArraySubmitter
type Suspender = executor.Suspender
type Canceller = executor.Canceller

func mergeEnvironment(base, overrides []string) []string {
	return executor.MergeEnvironment(base, overrides)
}

func statusWrapperScript(command []string, jobDir string, environment []string, workingDirectory string) string {
	return executor.StatusWrapperScript(command, jobDir, environment, workingDirectory)
}

type schedulerStatus struct {
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
}

func writeSchedulerStatus(jobDir, state string) {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		return
	}
	path, err := validatedStateFile(jobDir, stateFileSchedulerJSON)
	if err != nil {
		return
	}
	_ = writeJSON(path, schedulerStatus{State: state, UpdatedAt: nowRFC3339()})
}

func loadSchedulerStatus(jobDir string) string {
	path, err := validatedStateFile(jobDir, stateFileSchedulerJSON)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path) // NOSONAR: path is restricted by validatedStateFile to scheduler_status.json.
	if err != nil {
		return ""
	}
	var status schedulerStatus
	if json.Unmarshal(data, &status) != nil {
		return ""
	}
	return status.State
}

// jobOwnerExecutor determines which executor owns the job recorded in jobDir,
// based on the metadata files each executor writes on submission. Every
// scheduler-style executor writes job.json with its own "executor" name, so
// that job.json alone (not its mere existence) tells us which one to use.
func jobOwnerExecutor(jobDir string) (JobExecutor, error) {
	if path, err := validatedStateFile(jobDir, stateFileJobJSON); err == nil { // NOSONAR: jobDir is restricted to validated job-path boundaries
		if data, err := os.ReadFile(path); err == nil { // NOSONAR: path is restricted by validatedStateFile to job.json.
			var meta struct {
				Executor string `json:"executor"`
			}
			if err := json.Unmarshal(data, &meta); err == nil {
				if executor, ok := lookupExecutor(meta.Executor); ok {
					return executor, nil
				}
			}
		}
	}
	if path, err := validatedStateFile(jobDir, stateFilePID); err == nil {
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
	path, err := validatedStateFile(safeRunDir, stateFileContextJSON) // NOSONAR: safeRunDir is restricted to base path construction
	if err != nil {
		return "", false
	}
	// NOSONAR: path is restricted by validatedStateFile to allowed state file names
	data, err := os.ReadFile(path) // NOSONAR: path is restricted by validatedStateFile to context.json.
	if err != nil {
		return "", false
	}
	var context RunContext
	if json.Unmarshal(data, &context) != nil || context.Hostname == "" {
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

// schedulerCommandHint clarifies two common causes of an opaque scheduler
// control command failure: the scheduler's client tools (scontrol/qsig/
// bstop/...) not being installed on this host -- e.g. a web/CLI host outside
// the cluster that only shares the state directory over NFS, where the raw
// "executable file not found in $PATH" is easy to mistake for the job itself
// not running -- and the command running fine but being rejected by the
// scheduler itself (e.g. suspending a job that is still queued/pending
// rather than actually running), where Go's generic "exit status 1" hides
// the scheduler's own explanation unless the command's output is folded in.
func schedulerCommandHint(binary string, output []byte, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%q is not installed on this host; run this command from a host with that scheduler's client tools (%w)", binary, err)
	}
	if text := strings.TrimSpace(string(output)); text != "" {
		return fmt.Errorf("%w: %s", err, text)
	}
	return err
}

// executorRegistry is initialized eagerly (rather than in an init func) so
// that other package-level vars, such as cliCommandSpecs, can depend on
// executorNames() during their own initialization.
var executorRegistry = map[string]JobExecutor{
	"local": executor.NewLocal(jsonStore(), jobLogf),
	"slurm": slurmExecutor{},
	"pbs":   pbsExecutor{},
	"lsf":   lsfExecutor{},
	"ssh":   sshExecutor{},
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
	if err := validateQueueDependencies(queue); err != nil {
		return err
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
		_, _, err := sshTarget(options)
		return err
	case "slurm":
		if array {
			return rejectArraySchedulerOptions(options, "--array")
		}
	case "pbs":
		if array {
			return rejectArraySchedulerOptions(options, "-J", "-t")
		}
	case "lsf":
		if array {
			return rejectArraySchedulerOptions(options, "-J")
		}
	default:
		return fmt.Errorf("unsupported executor: %s", executorName)
	}
	_, err := expandShellOptions(options)
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
		"ssh":   sshCommandPath,
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
	command.Env = mergeEnvironment(os.Environ(), job.Environment)
	if err := command.Run(); err != nil {
		return fmt.Errorf("command %q is not available", job.Command[0])
	}
	return nil
}

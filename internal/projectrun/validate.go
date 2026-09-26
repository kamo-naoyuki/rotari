package projectrun

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// LoadQueue loads the project's queue and checks that it can run; see
// ValidateQueue.
func (runner Runner) LoadQueue(paths state.ProjectPaths, requestedExecutor string, executorOptions []string, settings executor.RunSettingsMap) (model.Queue, error) {
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return model.Queue{}, err
	}
	if err := runner.ValidateQueue(queue, requestedExecutor, executorOptions, settings); err != nil {
		return model.Queue{}, err
	}
	return queue, nil
}

// ValidateQueue checks that queue can run: its jobs and dependencies are
// valid, every job's executor is known, and each job's executor options,
// after run-level defaults are applied, suit its executor.
func (runner Runner) ValidateQueue(queue model.Queue, requestedExecutor string, executorOptions []string, settings executor.RunSettingsMap) error {
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
	if !runner.Executors.Known(defaultExecutor) {
		return fmt.Errorf("unsupported executor: %s", defaultExecutor)
	}

	for _, queued := range queue.Commands {
		executorName := queued.Executor
		if executorName == "" {
			executorName = defaultExecutor
		}
		if !runner.Executors.Known(executorName) {
			return fmt.Errorf("job %q uses unsupported executor: %s", queued.ID, executorName)
		}
		options := queued.ExecutorOptions
		if len(options) == 0 {
			options = settings.Options(executorName, executorOptions)
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

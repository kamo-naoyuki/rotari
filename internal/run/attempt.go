package run

import (
	"fmt"
	"sync"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type AttemptOptions struct {
	LocalConcurrency  int
	BatchMaxActive    int
	RequestedExecutor string
	ExecutorOptions   []string
	Settings          executor.RunSettingsMap
	ResolveExecutor   func(name string) (executor.JobExecutor, bool)
	Callbacks         BatchLaneCallbacks
}

func RunAttempt(runDir string, queue model.Queue, jobs []model.JobSpec, options AttemptOptions, onStart func(model.JobSpec)) []model.JobResult {
	defaultExecutor := options.RequestedExecutor
	if defaultExecutor == "" {
		defaultExecutor = queue.DefaultExecutor
	}
	if defaultExecutor == "" {
		defaultExecutor = "local"
	}
	grouped := make(map[string][]model.JobSpec)
	for _, job := range jobs {
		name := job.Executor
		if name == "" {
			name = defaultExecutor
		}
		grouped[name] = append(grouped[name], job)
	}
	results := make(chan model.JobResult, len(jobs))
	var workers sync.WaitGroup
	for name, executorJobs := range grouped {
		jobExecutor, ok := options.ResolveExecutor(name)
		if !ok {
			for _, job := range executorJobs {
				results <- model.JobResult{ID: job.ID, Command: job.Command, ExitCode: 1, Error: fmt.Sprintf("unsupported executor: %s", name)}
			}
			continue
		}
		workers.Add(1)
		if name == "local" {
			go RunLocalLane(&workers, runDir, jobExecutor, executorJobs, effectiveConcurrency(options.Settings, name, options.LocalConcurrency), results, onStart)
		} else {
			go RunBatchLane(&workers, runDir, queue, jobExecutor, executorJobs, effectiveConcurrency(options.Settings, name, options.BatchMaxActive), effectiveOptions(options.Settings, name, options.ExecutorOptions), results, options.Callbacks, onStart)
		}
	}
	workers.Wait()
	close(results)
	collected := make([]model.JobResult, 0, len(jobs))
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}

func effectiveConcurrency(settings executor.RunSettingsMap, name string, fallback int) int {
	if settings != nil && settings[name].Concurrency > 0 {
		return settings[name].Concurrency
	}
	return fallback
}

func effectiveOptions(settings executor.RunSettingsMap, name string, fallback []string) []string {
	if settings != nil && len(settings[name].Options) > 0 {
		return settings[name].Options
	}
	return fallback
}

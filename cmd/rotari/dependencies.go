package main

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func validateQueueDependencies(queue Queue) error {
	if err := model.ValidateDependencies(model.QueueToJobs(queue.Commands)); err != nil {
		return fmt.Errorf("invalid dependencies: %w", err)
	}
	return nil
}

func validateDependencies(jobs []JobSpec) error {
	return model.ValidateDependencies(jobs)
}

func dependenciesReady(job JobSpec, results map[string]JobResult, jobsByName map[string]JobSpec) (bool, string) {
	return model.DependenciesReady(job, results, jobsByName)
}

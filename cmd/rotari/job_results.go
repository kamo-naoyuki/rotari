package main

import "path/filepath"

func jobResultsByID(results []JobResult) map[string]JobResult {
	byID := make(map[string]JobResult, len(results))
	for _, result := range results {
		byID[result.ID] = result
	}
	return byID
}

func loadTerminalJobStatus(jobDir string) (int, bool) {
	status, ok := readJobStatus(filepath.Join(jobDir, stateFileStatus))
	if ok {
		return status, true
	}
	if scheduler, ok := loadSlurmStatus(filepath.Join(jobDir, stateFileStatusJSON)); ok && jobStatusTerminal(scheduler) {
		return scheduler.ExitCode, true
	}
	return loadTerminalSchedulerState(jobDir)
}

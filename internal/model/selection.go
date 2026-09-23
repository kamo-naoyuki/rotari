package model

import (
	"fmt"
	"strings"
)

func RunStatus(exitCode int) string {
	if exitCode == 0 {
		return "finished"
	}
	return "failed"
}

func CountRunResults(results []JobResult) (successCount, failedCount int) {
	for _, result := range results {
		if result.ExitCode == 0 {
			successCount++
		} else {
			failedCount++
		}
	}
	return successCount, failedCount
}

func ResultSelection(failed, unfinished, success bool) string {
	selections := make([]string, 0, 3)
	if failed {
		selections = append(selections, "failed")
	}
	if unfinished {
		selections = append(selections, "unfinished")
	}
	if success {
		selections = append(selections, "success")
	}
	return strings.Join(selections, ",")
}

func ResultSelectionMatches(selection string, finished bool, exitCode int) bool {
	for _, filter := range strings.Split(selection, ",") {
		switch filter {
		case "failed":
			if finished && exitCode != 0 {
				return true
			}
		case "unfinished":
			if !finished {
				return true
			}
		case "success":
			if finished && exitCode == 0 {
				return true
			}
		}
	}
	return false
}

func AggregatedJobResult(id string, array *ArraySpec, results map[string]JobResult) (JobResult, bool) {
	if array == nil {
		result, finished := results[id]
		return result, finished
	}
	aggregate := JobResult{ID: id}
	for _, task := range ArrayTaskIDs(array) {
		result, ok := results[fmt.Sprintf("%s-%d", id, task)]
		if !ok {
			return JobResult{}, false
		}
		if result.ExitCode != 0 && aggregate.ExitCode == 0 {
			aggregate.ExitCode = result.ExitCode
			aggregate.Error = result.Error
		}
	}
	return aggregate, true
}

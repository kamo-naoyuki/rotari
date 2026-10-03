package model

import (
	"fmt"
	"strings"
)

// Job statuses, as `show`, workflow manifests, and change --status name them.
const (
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
	StatusUnfinished = "unfinished"
)

// Errors that MarkResult records on a result marked failed or cancelled. The
// cancelled one keeps the "cancelled " prefix that IsCancelledError reads.
const (
	MarkedFailedError    = "marked failed"
	MarkedCancelledError = "cancelled (marked)"
)

// ValidMarkedStatus reports whether status can mark a queued job.
func ValidMarkedStatus(status string) bool {
	switch status {
	case StatusSuccess, StatusFailed, StatusCancelled, StatusUnfinished:
		return true
	}
	return false
}

// CancelledError returns the error a cancelled job's result records:
// "cancelled", keeping an earlier error in parentheses. An error that
// already records a cancellation is kept as it is.
func CancelledError(previous string) string {
	switch {
	case IsCancelledError(previous):
		return previous
	case strings.TrimSpace(previous) == "":
		return "cancelled"
	}
	return "cancelled (" + previous + ")"
}

// IsCancelledError reports whether a result's error text records a
// cancellation.
func IsCancelledError(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	return text == "cancelled" || text == "canceled" || strings.HasPrefix(text, "cancelled ") || strings.HasPrefix(text, "canceled ")
}

// ResultStatus names a job's result: unfinished without one, success for exit
// code zero, cancelled for a cancelled job, and failed otherwise.
func ResultStatus(result JobResult, finished bool) string {
	switch {
	case !finished:
		return StatusUnfinished
	case result.ExitCode == 0:
		return StatusSuccess
	case IsCancelledError(result.Error):
		return StatusCancelled
	default:
		return StatusFailed
	}
}

// MarkedStatusOf returns the status jobID, the command or one of its array
// tasks, is marked with: the task's own mark, else the command's, else "".
func (command QueuedCommand) MarkedStatusOf(jobID string) string {
	if status, ok := command.TaskMarkedStatus[jobID]; ok {
		return status
	}
	return command.MarkedStatus
}

// MarkResult returns the result a job has once marked with status, in place
// of its recorded result. An empty status, or the one the result already has,
// keeps it; unfinished drops it; success accepts it with exit code zero; and
// failed or cancelled record that status with a non-zero exit code. Marking a
// job without a result as anything but unfinished is an error, since there
// is no attempt to link the result to.
func MarkResult(id string, result JobResult, finished bool, status string) (JobResult, bool, error) {
	if status == "" || status == ResultStatus(result, finished) {
		return result, finished, nil
	}
	if status == StatusUnfinished {
		return JobResult{}, false, nil
	}
	if !finished {
		return JobResult{}, false, fmt.Errorf("job %q is marked %s but has no recorded result", id, status)
	}
	result.ClearDiagnosis()
	result.Accepted = false
	switch status {
	case StatusSuccess:
		result.ExitCode = 0
		result.Accepted = true
		result.Error = ""
	case StatusFailed, StatusCancelled:
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
		result.Error = MarkedFailedError
		if status == StatusCancelled {
			result.Error = MarkedCancelledError
		}
	default:
		return JobResult{}, false, fmt.Errorf("job %q has invalid marked status %q", id, status)
	}
	return result, true, nil
}

// QueuedStatusText describes a queued job's status for display: its origin's
// status, or its mark when that differs, as "success (accepted)" or
// "STATUS (marked)". It is "-" for a job with neither.
func QueuedStatusText(origin *JobOrigin, marked string) string {
	recorded := ""
	if origin != nil {
		recorded = origin.Status
	}
	switch {
	case marked == "" || marked == recorded:
		if recorded == "" {
			return "-"
		}
		return recorded
	case marked == StatusSuccess:
		return "success (accepted)"
	default:
		return marked + " (marked)"
	}
}

// QueueMarkedStatusByJobID maps each marked command and array task of queue
// to its marked status; see QueuedCommand.MarkedStatusOf.
func QueueMarkedStatusByJobID(queue Queue) map[string]string {
	marked := make(map[string]string)
	for _, command := range queue.Commands {
		if command.MarkedStatus != "" {
			marked[command.ID] = command.MarkedStatus
		}
		if command.Array == nil {
			continue
		}
		for _, task := range ArrayTaskIDs(command.Array) {
			taskID := fmt.Sprintf("%s-%d", command.ID, task)
			if status := command.MarkedStatusOf(taskID); status != "" {
				marked[taskID] = status
			}
		}
	}
	return marked
}

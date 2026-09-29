package model

import "strings"

// TimeoutExitCode is the exit code recorded for a job stopped by its timeout,
// matching GNU timeout.
const TimeoutExitCode = 124

// Failure kinds that --filter-failure-kind selects.
const (
	FailureKindTimeout   = "timeout"
	FailureKindCancelled = "cancelled"
	FailureKindBlocked   = "blocked"
	FailureKindOOM       = "oom"
	FailureKindSignal    = "signal"
	FailureKindError     = "error"
)

var failureKinds = []string{FailureKindTimeout, FailureKindCancelled, FailureKindBlocked, FailureKindOOM, FailureKindSignal, FailureKindError}

// FailureKindValues returns the failure kinds in the order the CLI lists them.
func FailureKindValues() []string {
	return append([]string(nil), failureKinds...)
}

// FailureKinds returns the kinds a failed result falls into, error when no
// other applies, and none for a success.
func FailureKinds(result JobResult) []string {
	if result.ExitCode == 0 {
		return nil
	}
	kinds := make([]string, 0, 3)
	text := strings.ToLower(strings.TrimSpace(result.Error))
	if result.ExitCode == TimeoutExitCode || strings.Contains(text, "timed out") || strings.Contains(text, "timeout") {
		kinds = append(kinds, FailureKindTimeout)
	}
	if IsCancelledError(result.Error) {
		kinds = append(kinds, FailureKindCancelled)
	}
	if strings.HasPrefix(text, "blocked") {
		kinds = append(kinds, FailureKindBlocked)
	}
	if strings.Contains(text, "out of memory") || strings.Contains(text, "oom") || strings.Contains(text, "memory limit") {
		kinds = append(kinds, FailureKindOOM)
	}
	if result.ExitCode > 128 {
		kinds = append(kinds, FailureKindSignal)
	}
	if len(kinds) == 0 {
		kinds = append(kinds, FailureKindError)
	}
	return kinds
}

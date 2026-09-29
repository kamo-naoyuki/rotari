package jobstatus

import (
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

const (
	FailureKindTimeout   = "timeout"
	FailureKindCancelled = "cancelled"
	FailureKindBlocked   = "blocked"
	FailureKindOOM       = "oom"
	FailureKindSignal    = "signal"
	FailureKindError     = "error"
)

var failureKinds = []string{FailureKindTimeout, FailureKindCancelled, FailureKindBlocked, FailureKindOOM, FailureKindSignal, FailureKindError}

// FailureKindValues returns the supported failure kinds, ordered by the CLI help.
func FailureKindValues() []string {
	return append([]string(nil), failureKinds...)
}

// FailureKinds classifies a finished failed job by the kinds that apply.
func FailureKinds(result model.JobResult) []string {
	if result.ExitCode == 0 {
		return nil
	}
	kinds := make([]string, 0, 3)
	text := strings.ToLower(strings.TrimSpace(result.Error))
	if result.ExitCode == executor.TimeoutExitCode || strings.Contains(text, "timed out") || strings.Contains(text, "timeout") {
		kinds = append(kinds, FailureKindTimeout)
	}
	if model.IsCancelledError(result.Error) {
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

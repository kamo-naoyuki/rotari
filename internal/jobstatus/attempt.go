package jobstatus

import (
	"os"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Source names the step of the fallback chain that supplied an exit code.
type Source int

const (
	// SourceNone means no step has a terminal outcome yet.
	SourceNone Source = iota
	// SourceStatus is the attempt's plain `status` exit-code file.
	SourceStatus
	// SourceWrapper is the attempt's wrapper `status.json` in a terminal phase.
	SourceWrapper
	// SourceScheduler is a terminal state in the attempt's
	// `scheduler_status.json`.
	SourceScheduler
	// SourceSummary is the job's result in the run's `summary.json`.
	SourceSummary
)

// Attempt is what one attempt directory records about its outcome.
type Attempt struct {
	ExitCode int
	Source   Source
	// Wrapper is the attempt's `status.json`, valid when HasWrapper is set,
	// whether or not its phase is terminal.
	Wrapper        executor.WrapperStatus
	HasWrapper     bool
	SchedulerState string
}

// Finished reports whether the attempt has a terminal exit code.
func (attempt Attempt) Finished() bool {
	return attempt.Source != SourceNone
}

// Result returns the attempt's outcome as a job result, or false while the
// attempt has not finished.
func (attempt Attempt) Result(job model.JobSpec) (model.JobResult, bool) {
	if !attempt.Finished() {
		return model.JobResult{}, false
	}
	result := model.JobResult{ID: job.ID, Command: job.Command, ExitCode: attempt.ExitCode}
	if attempt.HasWrapper {
		result.Hosts = attempt.Wrapper.Hosts
		if attempt.Source == SourceWrapper {
			result.Error = attempt.Wrapper.Error
		}
	}
	return result, true
}

// ReadAttempt resolves an attempt directory's outcome: `status`, then a
// terminal wrapper `status.json`, then a terminal `scheduler_status.json`.
func ReadAttempt(store state.Store, jobDir string) Attempt {
	attempt := Attempt{SchedulerState: executor.LoadSchedulerStatus(store, jobDir)}
	if path, err := state.ValidatedStateFile(jobDir, "status.json"); err == nil {
		attempt.Wrapper, attempt.HasWrapper = executor.LoadWrapperStatus(store, path)
	}
	if exitCode, ok := ReadStatusFile(jobDir); ok {
		attempt.ExitCode, attempt.Source = exitCode, SourceStatus
	} else if attempt.HasWrapper && WrapperTerminal(attempt.Wrapper) {
		attempt.ExitCode, attempt.Source = attempt.Wrapper.ExitCode, SourceWrapper
	} else if exitCode, ok := executor.SchedulerStateExitCode(attempt.SchedulerState); ok {
		attempt.ExitCode, attempt.Source = exitCode, SourceScheduler
	}
	return attempt
}

// ReadStatusFile reads the attempt's plain `status` exit-code file.
func ReadStatusFile(jobDir string) (int, bool) {
	path, err := state.ValidatedStateFile(jobDir, "status")
	if err != nil {
		return 0, false
	}
	// codeql[go/path-injection]: path is restricted by ValidatedStateFile to the status file.
	data, err := os.ReadFile(path) // NOSONAR: path is restricted by ValidatedStateFile.
	if err != nil {
		return 0, false
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return exitCode, err == nil
}

// WrapperTerminal reports whether a wrapper `status.json` records a finished
// job.
func WrapperTerminal(status executor.WrapperStatus) bool {
	return status.FinishedAt != "" || executor.SchedulerStateTerminal(status.Phase)
}

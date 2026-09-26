package projectrun

import (
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Runner runs project queues. The zero value is not usable; every field
// except the optional hooks must be set.
type Runner struct {
	Store     state.Store
	Executors executor.Registry
	// Logf receives job submission, completion, and failure lines.
	Logf func(string, ...any)
	// Errorf reports a run that failed to prepare or record its jobs; the run
	// still finishes with a failing exit code. Optional.
	Errorf func(string, ...any)

	// Environment names the variables exported to every job.
	Environment run.EnvironmentNames
	// AttemptIDName names the variable that carries a job's attempt ID.
	AttemptIDName string
	// PropagatedVariables are copied from the runner's environment into jobs.
	PropagatedVariables []string

	// ConfigPaths lists the config files active for a project; Begin snapshots
	// them into the run directory. Optional.
	ConfigPaths func(paths state.ProjectPaths) []string
	// RegisterRun records where runID lives so other commands can find it.
	// Optional.
	RegisterRun func(paths state.ProjectPaths, runID string) error
	// Diagnose attaches failure diagnoses to a result before the summary is
	// written. Optional.
	Diagnose func(runDir string, result model.JobResult) model.JobResult
	// RunFinished is called after the project is finalized. Optional.
	RunFinished func(paths state.ProjectPaths, runID string, exitCode int)

	// Now returns the current time; nil means time.Now.
	Now func() time.Time
}

func (runner Runner) now() time.Time {
	if runner.Now != nil {
		return runner.Now()
	}
	return time.Now()
}

func (runner Runner) timestamp() string {
	return runner.now().UTC().Format(time.RFC3339)
}

func (runner Runner) timestampNano() string {
	return runner.now().UTC().Format(time.RFC3339Nano)
}

func (runner Runner) logf(format string, args ...any) {
	if runner.Logf != nil {
		runner.Logf(format, args...)
	}
}

func (runner Runner) errorf(format string, args ...any) {
	if runner.Errorf != nil {
		runner.Errorf(format, args...)
	}
}

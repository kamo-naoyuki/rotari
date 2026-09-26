package projectrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ErrNoPreviousRun reports that a selection needs the project's last run but
// the project has none.
var ErrNoPreviousRun = errors.New("no previous run")

// PlanSelection decides which of the queue's jobs a run executes, reading
// earlier results from the project's runs; see run.PlanRerun.
func (runner Runner) PlanSelection(paths state.ProjectPaths, queue model.Queue, selection string, jobIDs []string, scope model.CommandSelector, referenceRunID string, partialArray bool) (run.Plan, error) {
	return run.PlanRerun(queue, selection, jobIDs, scope, referenceRunID, partialArray, originResults{paths: paths, store: runner.Store})
}

// originResults reads origin results from a project's runs.
type originResults struct {
	paths state.ProjectPaths
	store state.Store
}

func (source originResults) LastRunID() (string, error) {
	meta, err := state.LoadMeta(source.paths.MetaFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project '%s' has no previous run: %w", source.paths.ProjectName, ErrNoPreviousRun)
		}
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	if meta.LastRunID == "" {
		return "", fmt.Errorf("project '%s' has no previous run: %w", source.paths.ProjectName, ErrNoPreviousRun)
	}
	return meta.LastRunID, nil
}

func (source originResults) RunResults(runID string) (map[string]model.JobResult, error) {
	runDir, err := state.SafeJoin(source.paths.RunsDir, runID)
	if err != nil {
		return nil, err
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return nil, err
	}
	return model.ResultsByID(summary.Results), nil
}

func (source originResults) AttemptResult(origin model.JobOrigin) (model.JobResult, bool, error) {
	return LoadOriginAttemptResult(source.store, source.paths, origin)
}

func (source originResults) Origin(runID, jobID string, result model.JobResult) *model.JobOrigin {
	runDir := filepath.Join(source.paths.RunsDir, runID)
	cwd := ""
	if context, err := state.LoadContext(source.store, runDir); err == nil {
		cwd = context.CWD
	}
	status := "failed"
	if result.ExitCode == 0 {
		status = "success"
	}
	return &model.JobOrigin{
		RunID: runID, JobID: jobID, AttemptID: result.AttemptID, Status: status, CWD: cwd,
		SubmittedAt: state.ReadJobTimestamp(runDir, jobID, "submitted_at"),
		FinishedAt:  state.ReadJobTimestamp(runDir, jobID, "finished_at"),
	}
}

// LoadOriginAttemptResult reads the result of the exact attempt an origin
// names. It reports false when the attempt has not finished.
func LoadOriginAttemptResult(store state.Store, paths state.ProjectPaths, origin model.JobOrigin) (model.JobResult, bool, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, origin.RunID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, origin.JobID, origin.AttemptID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	var job model.JobSpec
	if err := store.ReadJSON(filepath.Join(attemptDir, "command.json"), &job); err != nil {
		return model.JobResult{}, false, fmt.Errorf("failed to load origin attempt %q command: %w", origin.AttemptID, err)
	}
	if result, ok := state.LoadLocalJobResult(attemptDir, job); ok {
		result.AttemptID = origin.AttemptID
		return result, true, nil
	}
	if status, ok := executor.LoadWrapperStatus(store, filepath.Join(attemptDir, "status.json")); ok {
		result := model.JobResult{ID: origin.JobID, AttemptID: origin.AttemptID, Command: job.Command, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}
		return result, true, nil
	}
	return model.JobResult{}, false, nil
}

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

var errNoPreviousRun = errors.New("no previous run")

// planRerunSelection decides which of the queue's jobs a run executes; see
// runcontract.PlanRerun.
func planRerunSelection(paths state.ProjectPaths, queue model.Queue, selection string, jobIDs []string, referenceRunID string, partialArray bool) (runcontract.Plan, error) {
	return runcontract.PlanRerun(queue, selection, jobIDs, referenceRunID, partialArray, projectOriginResults{paths: paths})
}

// projectOriginResults reads origin results from a project's runs.
type projectOriginResults struct {
	paths state.ProjectPaths
}

func (source projectOriginResults) LastRunID() (string, error) {
	meta, err := state.LoadMeta(source.paths.MetaFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("project '%s' has no previous run: %w", source.paths.ProjectName, errNoPreviousRun)
		}
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	if meta.LastRunID == "" {
		return "", fmt.Errorf("project '%s' has no previous run: %w", source.paths.ProjectName, errNoPreviousRun)
	}
	return meta.LastRunID, nil
}

func (source projectOriginResults) RunResults(runID string) (map[string]model.JobResult, error) {
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

func (source projectOriginResults) AttemptResult(origin model.JobOrigin) (model.JobResult, bool, error) {
	return loadOriginAttemptResult(source.paths, &origin)
}

func (source projectOriginResults) Origin(runID, jobID string, result model.JobResult) *model.JobOrigin {
	runDir := filepath.Join(source.paths.RunsDir, runID)
	cwd := ""
	if context, err := state.LoadContext(jsonStore(), runDir); err == nil {
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

func loadOriginAttemptResult(paths state.ProjectPaths, origin *model.JobOrigin) (model.JobResult, bool, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, origin.RunID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, origin.JobID, origin.AttemptID)
	if err != nil {
		return model.JobResult{}, false, err
	}
	var job model.JobSpec
	if err := jsonStore().ReadJSON(filepath.Join(attemptDir, commandJSONName), &job); err != nil {
		return model.JobResult{}, false, fmt.Errorf("failed to load origin attempt %q command: %w", origin.AttemptID, err)
	}
	if result, ok := state.LoadLocalJobResult(attemptDir, job); ok {
		result.AttemptID = origin.AttemptID
		return result, true, nil
	}
	if status, ok := loadWrapperStatus(filepath.Join(attemptDir, statusJSONName)); ok {
		result := model.JobResult{ID: origin.JobID, AttemptID: origin.AttemptID, Command: job.Command, ExitCode: status.ExitCode, Error: status.Error, Hosts: status.Hosts}
		return result, true, nil
	}
	return model.JobResult{}, false, nil
}

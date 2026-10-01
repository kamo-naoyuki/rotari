// Package runview loads a run's persisted snapshot and resolves its displayed
// job statuses for read-side consumers such as the CLI and Web UI.
package runview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// LoadRun loads a run's command snapshot and resolves every job through the
// shared jobstatus fallback chain.
func LoadRun(paths state.ProjectPaths, runID string, store state.Store) (runlineage.Run, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return runlineage.Run{}, fmt.Errorf("run %q not found", runID)
	}
	commands, err := state.ReadQueueFile(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s commands: %w", runID, err)
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s summary: %w", runID, err)
	}
	results := model.ResultsByID(summary.Results)
	origins := model.QueueOriginsByJobID(commands)
	run := runlineage.Run{ID: runID, Name: summary.RunName, StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt}
	for _, spec := range model.QueueToJobs(commands.Commands) {
		jobDir, err := state.LatestAttemptJobDir(runDir, spec.ID)
		if err != nil {
			return runlineage.Run{}, fmt.Errorf("invalid job ID %q: %w", spec.ID, err)
		}
		summaryResult, hasSummary := results[spec.ID]
		resolved := jobstatus.ReadJob(store, jobDir, summaryResult, hasSummary)
		status := runlineage.StatusUnfinished
		switch {
		case !resolved.Finished():
		case resolved.Accepted() || resolved.ExitCode == 0:
			status = runlineage.StatusSuccess
		case resolved.Blocked():
			status = runlineage.StatusBlocked
		default:
			status = runlineage.StatusFailed
		}
		attemptID, _ := state.LatestAttemptID(runDir, spec.ID)
		origin := origins[spec.ID]
		carried := origin != nil && (attemptID == "" || attemptID == origin.AttemptID)
		diagnoses := make([]string, 0, len(summaryResult.Diagnoses))
		for _, diagnosis := range summaryResult.Diagnoses {
			diagnoses = append(diagnoses, diagnosis.Name)
		}
		run.Jobs = append(run.Jobs, runlineage.Job{
			Spec: spec, Status: status, Origin: origin, Carried: carried,
			DiagnosisStatus: summaryResult.DiagnosisStatus, Diagnoses: diagnoses,
		})
	}
	return run, nil
}

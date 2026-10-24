// Package runview loads a run's persisted snapshot and resolves its displayed
// job statuses for read-side consumers such as the CLI and Web UI.
package runview

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rotari-dev/rotari/internal/jobstatus"
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/project"
	"github.com/rotari-dev/rotari/internal/runlineage"
	"github.com/rotari-dev/rotari/internal/state"
)

// LoadRun loads a run's command snapshot and resolves every job through the
// shared jobstatus fallback chain. An active run without a snapshot, which
// only an earlier rotari could leave, has no jobs yet; for any other run a
// missing snapshot is an error.
func LoadRun(paths state.ProjectPaths, runID string, store state.Store) (runlineage.Run, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return runlineage.Run{}, fmt.Errorf("run %q not found", runID)
	}
	files, err := state.LoadRunSnapshot(runDir)
	if err != nil {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s state versions: %w", runID, err)
	}
	commands, err := files.Commands, files.CommandsErr
	if errors.Is(err, os.ErrNotExist) {
		if phase, phaseErr := project.RunPhaseOf(paths, runID); phaseErr == nil && phase == project.RunPhaseRunning {
			return runlineage.Run{ID: runID}, nil
		}
	}
	if err != nil {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s commands: %w", runID, err)
	}
	summary, err := files.Summary, files.SummaryErr
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s summary: %w", runID, err)
	}
	results := jobstatus.RecordedResults(files)
	origins := model.QueueOriginsByJobID(commands)
	run := runlineage.Run{ID: runID, Name: summary.RunName, StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt}
	sources, recordedSources, err := state.LoadRunSources(runDir)
	if err != nil {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s sources: %w", runID, err)
	}
	if recordedSources {
		run.Sources = &sources
	}
	if run.Notes, err = state.LoadRunNotes(runDir); err != nil {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s notes: %w", runID, err)
	}
	if run.StartedBy, err = state.LoadRunStartedBy(store, runDir); err != nil {
		return runlineage.Run{}, fmt.Errorf("failed to load run %s context: %w", runID, err)
	}
	specs := model.QueueToJobs(commands.Commands)
	jobs := make([]runlineage.Job, len(specs))
	errs := make([]error, len(specs))
	state.ReadEach(len(specs), func(index int) {
		jobs[index], errs[index] = loadRunJob(store, runDir, specs[index], results, origins)
	})
	for _, err := range errs {
		if err != nil {
			return runlineage.Run{}, err
		}
	}
	run.Jobs = append(run.Jobs, jobs...)
	return run, nil
}

// loadRunJob resolves one job of LoadRun. It only reads results and origins.
func loadRunJob(store state.Store, runDir string, spec model.JobSpec, results map[string]model.JobResult, origins map[string]*model.JobOrigin) (runlineage.Job, error) {
	attemptID, jobDir, err := state.LatestAttempt(runDir, spec.ID)
	if err != nil {
		return runlineage.Job{}, fmt.Errorf("invalid job ID %q: %w", spec.ID, err)
	}
	summaryResult, hasSummary := results[spec.ID]
	resolved := jobstatus.ReadJob(store, jobDir, summaryResult, hasSummary)
	status := LineageStatus(resolved)
	origin := origins[spec.ID]
	carried := runlineage.IsCarried(origin, attemptID, resolved.Blocked(), hasSummary)
	diagnoses := make([]string, 0, len(summaryResult.Diagnoses))
	for _, diagnosis := range summaryResult.Diagnoses {
		diagnoses = append(diagnoses, diagnosis.Name)
	}
	result, _ := resolved.Result(spec)
	// While the run is active, a job that reached its final result has
	// recorded it, diagnosis included, beside its attempt.
	var finalResult model.JobResult
	final := hasSummary || store.ReadJSON(filepath.Join(jobDir, state.FinalResultFileName), &finalResult) == nil
	if !hasSummary && final {
		finalResult.ExitCode = result.ExitCode
		result = finalResult
	}
	return runlineage.Job{
		Spec: spec, Status: status, Origin: origin, Carried: carried,
		DiagnosisStatus: summaryResult.DiagnosisStatus, Diagnoses: diagnoses, Result: result, Final: final,
	}, nil
}

// LineageStatus classifies a resolved job for run lineage summaries, which
// count cancellations as failures.
func LineageStatus(job jobstatus.Job) string {
	switch {
	case !job.Finished():
		return runlineage.StatusUnfinished
	case job.Accepted() || job.ExitCode == 0:
		return runlineage.StatusSuccess
	case job.Blocked():
		return runlineage.StatusBlocked
	default:
		return runlineage.StatusFailed
	}
}

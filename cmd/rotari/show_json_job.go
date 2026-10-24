package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/rotari-dev/rotari/internal/jobfilter"
	"github.com/rotari-dev/rotari/internal/jobstatus"
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/runlineage"
	"github.com/rotari-dev/rotari/internal/runview"
	"github.com/rotari-dev/rotari/internal/state"
)

type showJSONJob struct {
	Job      model.JobSpec    `json:"job"`
	Finished bool             `json:"finished"`
	Status   string           `json:"status"`
	Carried  bool             `json:"carried,omitempty"`
	Result   *model.JobResult `json:"result,omitempty"`
	// Artifacts are the candidates of the attempt that produced the job's
	// result, as `show -j JOB --artifacts` lists them.
	Artifacts jobstatus.ArtifactListing `json:"artifacts"`
}

// showRunJobJSON resolves each expanded job through the same status fallback
// used by the human-readable run and job views. An array command includes
// its tasks, rather than pretending that the command has one attempt. A
// non-empty attemptID resolves that attempt of the job instead of its latest.
// A non-empty selection keeps only the jobs it selects.
func showRunJobJSON(paths state.ProjectPaths, runID, jobID, attemptID, selection string) int {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printError(err)
		return 1
	}
	files, err := state.LoadRunSnapshot(runDir)
	if err != nil {
		printErrorf("failed to read run: %v", err)
		return 1
	}
	queue, err := files.Queue()
	if err != nil {
		printErrorf("failed to read commands: %v", err)
		return 1
	}
	selected := selectRunJSONJobs(queue, jobID)
	if len(selected) == 0 {
		printErrorf("job %q not found in run %q", jobID, runID)
		return 1
	}
	if files.SummaryErr != nil && !os.IsNotExist(files.SummaryErr) {
		printErrorf("failed to read summary: %v", files.SummaryErr)
		return 1
	}
	output := struct {
		BaseDir      string                `json:"base_dir"`
		Project      string                `json:"project_name"`
		RunID        string                `json:"run_id"`
		JobID        string                `json:"job_id"`
		AttemptID    string                `json:"attempt_id,omitempty"`
		Lifecycle    string                `json:"lifecycle"`
		ClientStatus model.RunClientStatus `json:"client_status"`
		Jobs         []showJSONJob         `json:"jobs"`
	}{BaseDir: paths.BaseDir, Project: paths.ProjectName, RunID: runID, JobID: jobID, AttemptID: attemptID}
	output.Lifecycle, _ = runview.RunLifecycleLabel(paths, runID)
	output.ClientStatus, _ = runview.ClientStatus(paths, runID)
	if output.ClientStatus.State == "" {
		output.ClientStatus.State = "unknown"
	}
	output.Jobs, err = resolveRunJSONJobs(runDir, selected, model.QueueOriginsByJobID(queue), files, attemptID)
	if err != nil {
		printError(err)
		return 1
	}
	output.Jobs = filterRunJSONJobs(paths, runID, output.Jobs, selection)
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		printErrorf("failed to write JSON: %v", err)
		return 1
	}
	return 0
}

func filterRunJSONJobs(paths state.ProjectPaths, runID string, jobs []showJSONJob, selection string) []showJSONJob {
	if selection == "" {
		return jobs
	}
	kept := make([]showJSONJob, 0, len(jobs))
	for _, entry := range jobs {
		var result model.JobResult
		if entry.Result != nil {
			result = *entry.Result
		}
		if selectsShownJob(paths, runID, entry.Job.ID, result, entry.Finished, selection, jobfilter.Filter{}) {
			kept = append(kept, entry)
		}
	}
	return kept
}

func selectRunJSONJobs(queue model.Queue, jobID string) []model.JobSpec {
	for _, command := range queue.Commands {
		if command.ID == jobID {
			return model.QueueToJobs([]model.QueuedCommand{command})
		}
	}
	for _, job := range model.QueueToJobs(queue.Commands) {
		if job.ID == jobID {
			return []model.JobSpec{job}
		}
	}
	return nil
}

func resolveRunJSONJobs(runDir string, selected []model.JobSpec, origins map[string]*model.JobOrigin, files state.RunSnapshot, attemptID string) ([]showJSONJob, error) {
	results := jobstatus.RecordedResults(files)
	entries := make([]showJSONJob, 0, len(selected))
	for _, spec := range selected {
		latestAttemptID, jobDir, err := state.LatestAttempt(runDir, spec.ID)
		if attemptID != "" {
			jobDir, err = state.SpecificAttemptJobDir(runDir, spec.ID, attemptID)
		}
		if err != nil {
			return nil, err
		}
		latest := attemptID == "" || attemptID == latestAttemptID
		saved, hasSummary := results[spec.ID]
		resolved := jobstatus.ResolveAttempt(jobstatus.ReadAttempt(jsonStore(), jobDir), latest, saved, hasSummary)
		carried := latest && runlineage.IsCarried(origins[spec.ID], latestAttemptID, resolved.Blocked(), hasSummary)
		status := resolved.DisplayStatus(spec)
		if carried {
			status += " (carried)"
		}
		item := showJSONJob{Job: spec, Finished: resolved.Finished(), Status: status, Carried: carried,
			Artifacts: jobstatus.ListArtifacts(jsonStore(), filepath.Dir(runDir), model.JobOrigin{RunID: filepath.Base(runDir), JobID: spec.ID, AttemptID: attemptID})}
		if result, ok := resolved.Result(spec); ok {
			item.Result = &result
		}
		entries = append(entries, item)
	}
	return entries, nil
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
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
// non-empty selection keeps only the jobs it selects.
func showRunJobJSON(paths state.ProjectPaths, runID, jobID, selection string) int {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		printError(err)
		return 1
	}
	if err := state.CheckRunVersions(runDir); err != nil {
		printErrorf("failed to read run: %v", err)
		return 1
	}
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		printErrorf("failed to read commands: %v", err)
		return 1
	}
	selected := selectRunJSONJobs(queue, jobID)
	if len(selected) == 0 {
		printErrorf("job %q not found in run %q", jobID, runID)
		return 1
	}
	var summary *model.RunSummary
	if value, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json")); err == nil {
		summary = &value
	} else if !os.IsNotExist(err) {
		printErrorf("failed to read summary: %v", err)
		return 1
	}
	output := struct {
		BaseDir      string                `json:"base_dir"`
		Project      string                `json:"project_name"`
		RunID        string                `json:"run_id"`
		JobID        string                `json:"job_id"`
		Lifecycle    string                `json:"lifecycle"`
		ClientStatus model.RunClientStatus `json:"client_status"`
		Jobs         []showJSONJob         `json:"jobs"`
	}{BaseDir: paths.BaseDir, Project: paths.ProjectName, RunID: runID, JobID: jobID}
	output.Lifecycle, _ = runview.RunLifecycleLabel(paths, runID)
	output.ClientStatus, _ = runview.ClientStatus(paths, runID)
	if output.ClientStatus.State == "" {
		output.ClientStatus.State = "unknown"
	}
	output.Jobs, err = resolveRunJSONJobs(runDir, selected, model.QueueOriginsByJobID(queue), summary)
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

func resolveRunJSONJobs(runDir string, selected []model.JobSpec, origins map[string]*model.JobOrigin, summary *model.RunSummary) ([]showJSONJob, error) {
	results := jobstatus.RecordedResults(runDir, summary)
	entries := make([]showJSONJob, 0, len(selected))
	for _, spec := range selected {
		jobDir, err := state.LatestAttemptJobDir(runDir, spec.ID)
		if err != nil {
			return nil, err
		}
		saved, hasSummary := results[spec.ID]
		resolved := jobstatus.ReadJob(jsonStore(), jobDir, saved, hasSummary)
		latestAttemptID, _ := state.LatestAttemptID(runDir, spec.ID)
		carried := runlineage.IsCarried(origins[spec.ID], latestAttemptID, resolved.Blocked(), hasSummary)
		status := resolved.DisplayStatus(spec)
		if carried {
			status += " (carried)"
		}
		item := showJSONJob{Job: spec, Finished: resolved.Finished(), Status: status, Carried: carried,
			Artifacts: jobstatus.ListArtifacts(jsonStore(), filepath.Dir(runDir), model.JobOrigin{RunID: filepath.Base(runDir), JobID: spec.ID})}
		if result, ok := resolved.Result(spec); ok {
			item.Result = &result
		}
		entries = append(entries, item)
	}
	return entries, nil
}

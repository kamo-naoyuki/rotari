package webui

import (
	"fmt"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

// webArtifacts returns the artifact listing of a job in a run for
// /api/artifacts: its latest attempt, or attemptID when given. It is the
// listing `show -j JOB --artifacts` prints, built by jobstatus.ListArtifacts.
func webArtifacts(store stateinternal.Store, baseDir, projectName, runID, jobID, attemptID string) (jobstatus.ArtifactListing, error) {
	if !stateinternal.IsValidPathElement(projectName) || !stateinternal.IsValidPathElement(runID) || !stateinternal.IsValidPathElement(jobID) {
		return jobstatus.ArtifactListing{}, fmt.Errorf("project_name, run_id and job_id are required")
	}
	if attemptID != "" {
		payload, err := stateinternal.DecodeAttemptID(attemptID)
		if err != nil || payload.JobID != jobID {
			return jobstatus.ArtifactListing{}, fmt.Errorf("attempt_id %q is not an attempt of job %q", attemptID, jobID)
		}
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return jobstatus.ArtifactListing{}, err
	}
	runDir, err := stateinternal.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return jobstatus.ArtifactListing{}, err
	}
	queue, err := stateinternal.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return jobstatus.ArtifactListing{}, fmt.Errorf("run %q not found", runID)
	}
	for _, job := range model.QueueToJobs(queue.Commands) {
		if job.ID == jobID {
			return jobstatus.ListArtifacts(store, paths.RunsDir, model.JobOrigin{RunID: runID, JobID: jobID, AttemptID: attemptID}), nil
		}
	}
	return jobstatus.ArtifactListing{}, fmt.Errorf("job %q not found in run %q", jobID, runID)
}

// staticArtifactsKey identifies an artifact listing in a static export,
// matching staticArtifactsKey in web_static_bootstrap.js.
func staticArtifactsKey(projectName, runID, jobID, attemptID string) string {
	return projectName + "/" + runID + "/" + jobID + "/" + attemptID
}

// staticArtifactAttempts lists the attempts a static export keeps a listing
// for: the latest ("") and each of the job's attempts.
func staticArtifactAttempts(job webprojection.Job) []string {
	attempts := []string{""}
	for _, attempt := range job.Attempts {
		attempts = append(attempts, attempt.ID)
	}
	return attempts
}

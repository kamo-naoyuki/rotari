package mcp

import (
	"errors"
	"fmt"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
)

// The tools here cancel, suspend, and resume the jobs of a running run, as
// `rotari cancel`, `suspend`, and `resume` do with --run-id. A preview lists
// the jobs an operation reaches; each write names the run, and acts only
// while that run is still the project's active run.

type JobControlInput struct {
	RunID     string   `json:"run_id" jsonschema:"exact ID of the running run"`
	Operation string   `json:"operation" jsonschema:"cancel, suspend, or resume"`
	JobNames  []string `json:"job_names,omitempty" jsonschema:"only the unfinished jobs with these names; an array's or matrix's name selects its members. By default every job the operation reaches"`
}

type JobControlPreview struct {
	Project   string   `json:"project"`
	RunID     string   `json:"run_id"`
	Operation string   `json:"operation"`
	JobIDs    []string `json:"job_ids" jsonschema:"the unfinished jobs the operation reaches: running and pending ones for cancel, running ones for suspend and resume"`
}

type ControlJobsInput struct {
	RunID  string   `json:"run_id" jsonschema:"exact ID of the running run; the operation fails if it is no longer running"`
	JobIDs []string `json:"job_ids,omitempty" jsonschema:"jobs from rotari_preview_job_control; without them, cancel stops the whole run and suspend or resume every running job"`
}

type ControlJobsOutput struct {
	Project string `json:"project"`
	RunID   string `json:"run_id"`
	Message string `json:"message"`
}

func (tools writeTools) controller() jobcontrol.Controller {
	runner := tools.runner()
	return jobcontrol.Controller{Store: runner.Store, Executors: runner.Executors}
}

// previewJobControl lists the jobs of a running run that operation reaches,
// as `rotari cancel` and the others list them before asking to continue.
func (tools writeTools) previewJobControl(input JobControlInput) (JobControlPreview, error) {
	states, ok := jobcontrol.States[input.Operation]
	if !ok {
		return JobControlPreview{}, fmt.Errorf("operation must be cancel, suspend, or resume, not %q", input.Operation)
	}
	location, _, err := registeredRun(tools.masterDir, input.RunID)
	if err != nil {
		return JobControlPreview{}, err
	}
	runID, jobIDs, err := tools.controller().Select(location.BaseDir, location.ProjectName, location.RunID,
		jobcontrol.Selection{Names: input.JobNames, States: states}, time.Now())
	if err != nil {
		return JobControlPreview{}, err
	}
	return JobControlPreview{Project: location.ProjectName, RunID: runID, Operation: input.Operation, JobIDs: jobIDs}, nil
}

// controlJobs applies operation to input's jobs of the running run, as
// `rotari OPERATION --run-id RUN_ID [JOB_ID...]` does.
func (tools writeTools) controlJobs(input ControlJobsInput, operation string) (ControlJobsOutput, error) {
	if input.RunID == "" {
		return ControlJobsOutput{}, errors.New("run_id is required")
	}
	location, _, err := registeredRun(tools.masterDir, input.RunID)
	if err != nil {
		return ControlJobsOutput{}, err
	}
	controller := tools.controller()
	var message string
	if operation == "cancel" {
		message, err = controller.Cancel(location.BaseDir, location.ProjectName, location.RunID, input.JobIDs, false)
	} else {
		message, err = controller.Control(location.BaseDir, location.ProjectName, location.RunID, input.JobIDs, operation)
	}
	if err != nil {
		return ControlJobsOutput{}, err
	}
	return ControlJobsOutput{Project: location.ProjectName, RunID: location.RunID, Message: message}, nil
}

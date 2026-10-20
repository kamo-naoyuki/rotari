package mcp

import (
	"errors"

	"github.com/kamo-naoyuki/rotari/internal/project"
)

type UnlockInput struct {
	BaseDirRef    string `json:"basedir_ref" jsonschema:"basedir_ref of the project, from rotari_list_projects"`
	Project       string `json:"project" jsonschema:"project name"`
	RunID         string `json:"run_id,omitempty" jsonschema:"the interrupted run to recover; by default the one the project's lock or metadata names"`
	AssumeStopped bool   `json:"assume_stopped,omitempty" jsonschema:"explicit attestation that an unverifiable coordinator stopped; never overrides live ownership; jobs must be checked separately"`
}

type ApplyUnlockInput struct {
	UnlockInput
	IfRevision string `json:"if_revision" jsonschema:"revision from rotari_preview_unlock; the unlock applies only if the project is still at it. Confirm first that the run's jobs have stopped"`
}

type UnlockOutput struct {
	ExecutionChecked bool                          `json:"execution_checked" jsonschema:"false in this phase: executor termination is not checked; terminal status is not proof all jobs stopped"`
	Project          string                        `json:"project"`
	InterruptedRunID string                        `json:"interrupted_run_id,omitempty" jsonschema:"the interrupted run the unlock recovers; empty when the project has none and nothing changes"`
	InterruptedRun   string                        `json:"interrupted_run,omitempty" jsonschema:"what the interrupted run's jobs last reported"`
	JobsMayBeRunning bool                          `json:"jobs_may_be_running,omitempty" jsonschema:"some jobs of the interrupted run appear to still be running; confirm they stopped before recovering"`
	Revision         string                        `json:"revision" jsonschema:"for a preview, the revision to pass to rotari_unlock; after an unlock, the new revision"`
	Coordinator      project.CoordinatorAssessment `json:"coordinator"`
	Allowed          bool                          `json:"allowed"`
	Applied          bool                          `json:"applied"`
	RefusalCode      string                        `json:"refusal_code,omitempty"`
	AssumeStopped    bool                          `json:"assume_stopped"`
}

// unlock previews or applies `rotari unlock` on a project through
// project.Unlock.
func (tools writeTools) unlock(input UnlockInput, guard project.Guard) (UnlockOutput, error) {
	_, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return UnlockOutput{}, err
	}
	result, err := project.Unlock(paths, input.RunID, guard, input.AssumeStopped)
	if err != nil {
		var refusal *project.UnlockRefusedError
		if !errors.As(err, &refusal) {
			return UnlockOutput{}, err
		}
	}
	output := UnlockOutput{Project: paths.ProjectName, InterruptedRunID: result.RunID, InterruptedRun: result.Detail, JobsMayBeRunning: result.JobsMayBeRunning, ExecutionChecked: result.ExecutionChecked, Revision: result.Revision,
		Coordinator: result.Coordinator, Allowed: result.Allowed, Applied: result.Applied, RefusalCode: result.RefusalCode, AssumeStopped: result.AssumeStopped}
	return output, err
}

func (tools writeTools) applyUnlock(input ApplyUnlockInput) (UnlockOutput, error) {
	if input.IfRevision == "" {
		return UnlockOutput{}, errors.New("if_revision is required; take it from rotari_preview_unlock")
	}
	return tools.unlock(input.UnlockInput, project.Guard{IfRevision: input.IfRevision})
}

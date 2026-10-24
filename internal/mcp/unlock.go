package mcp

import (
	"errors"

	"github.com/rotari-dev/rotari/internal/project"
)

type UnlockInput struct {
	BaseDirRef    string `json:"basedir_ref" jsonschema:"basedir_ref of the project, from rotari_list_projects"`
	Project       string `json:"project" jsonschema:"project name"`
	RunID         string `json:"run_id,omitempty" jsonschema:"the interrupted run to recover; by default the one the project's lock or metadata names"`
	AssumeStopped bool   `json:"assume_stopped,omitempty" jsonschema:"explicit attestation that unverifiable coordinator and managed executions stopped; required for unknown execution evidence; never overrides verified live ownership or execution"`
}

type ApplyUnlockInput struct {
	UnlockInput
	IfRevision string `json:"if_revision" jsonschema:"revision from rotari_preview_unlock; the unlock applies only if the project and execution evidence are still at it. Confirm first that the coordinator and all managed executions have stopped"`
}

type UnlockOutput struct {
	ExecutionChecked bool                          `json:"execution_checked" jsonschema:"true only for a nonempty inventory whose executions all have active/stopped probe evidence within their stated scope; true does not imply stopped or recovery allowed; unknown evidence remains false"`
	Project          string                        `json:"project"`
	InterruptedRunID string                        `json:"interrupted_run_id,omitempty" jsonschema:"the interrupted run the unlock recovers; empty when the project has none and nothing changes"`
	InterruptedRun   string                        `json:"interrupted_run,omitempty" jsonschema:"what the interrupted run's jobs last reported"`
	JobsMayBeRunning bool                          `json:"jobs_may_be_running,omitempty" jsonschema:"some jobs of the interrupted run are active or unknown and may still be running; confirm they stopped before recovering"`
	Revision         string                        `json:"revision" jsonschema:"for a preview, the revision to pass to rotari_unlock; after an unlock, the new revision"`
	Coordinator      project.CoordinatorAssessment `json:"coordinator"`
	Allowed          bool                          `json:"allowed"`
	Applied          bool                          `json:"applied"`
	RefusalCode      string                        `json:"refusal_code,omitempty"`
	AssumeStopped    bool                          `json:"assume_stopped"`
	Action           string                        `json:"action,omitempty" jsonschema:"finalize a complete recorded run or release it to collecting without synthesizing results"`
	Repair           project.RepairAssessment      `json:"repair"`
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
		Coordinator: result.Coordinator, Allowed: result.Allowed, Applied: result.Applied, RefusalCode: result.RefusalCode, AssumeStopped: result.AssumeStopped, Action: result.Action, Repair: result.Repair}
	return output, err
}

func (tools writeTools) applyUnlock(input ApplyUnlockInput) (UnlockOutput, error) {
	if input.IfRevision == "" {
		return UnlockOutput{}, errors.New("if_revision is required; take it from rotari_preview_unlock")
	}
	return tools.unlock(input.UnlockInput, project.Guard{IfRevision: input.IfRevision})
}

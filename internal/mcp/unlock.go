package mcp

import (
	"errors"

	"github.com/kamo-naoyuki/rotari/internal/project"
)

type UnlockInput struct {
	BaseDirRef string `json:"basedir_ref" jsonschema:"basedir_ref of the project, from rotari_list_projects"`
	Project    string `json:"project" jsonschema:"project name"`
	RunID      string `json:"run_id,omitempty" jsonschema:"the interrupted run to recover; by default the one the project's lock or metadata names"`
}

type ApplyUnlockInput struct {
	UnlockInput
	IfRevision string `json:"if_revision" jsonschema:"revision from rotari_preview_unlock; the unlock applies only if the project is still at it. Confirm first that the run's jobs have stopped"`
}

type UnlockOutput struct {
	Project          string `json:"project"`
	InterruptedRunID string `json:"interrupted_run_id,omitempty" jsonschema:"the interrupted run the unlock recovers; empty when the project has none and nothing changes"`
	InterruptedRun   string `json:"interrupted_run,omitempty" jsonschema:"what the interrupted run's jobs last reported"`
	JobsMayBeRunning bool   `json:"jobs_may_be_running,omitempty" jsonschema:"some jobs of the interrupted run appear to still be running; confirm they stopped before recovering"`
	Revision         string `json:"revision" jsonschema:"for a preview, the revision to pass to rotari_unlock; after an unlock, the new revision"`
}

// unlock previews or applies `rotari unlock` on a project through
// project.Unlock.
func (tools writeTools) unlock(input UnlockInput, guard project.Guard) (UnlockOutput, error) {
	_, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return UnlockOutput{}, err
	}
	var outcome project.Outcome
	guard.Report = func(result project.Outcome) { outcome = result }
	result, err := project.Unlock(paths, input.RunID, guard)
	if err != nil {
		return UnlockOutput{}, err
	}
	output := UnlockOutput{Project: paths.ProjectName, InterruptedRunID: result.RunID, InterruptedRun: result.Detail, JobsMayBeRunning: result.JobsMayBeRunning, Revision: outcome.Revision}
	if outcome.Applied {
		output.Revision = outcome.NewRevision
	}
	return output, nil
}

func (tools writeTools) applyUnlock(input ApplyUnlockInput) (UnlockOutput, error) {
	if input.IfRevision == "" {
		return UnlockOutput{}, errors.New("if_revision is required; take it from rotari_preview_unlock")
	}
	return tools.unlock(input.UnlockInput, project.Guard{IfRevision: input.IfRevision})
}

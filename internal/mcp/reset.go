package mcp

import (
	"errors"

	"github.com/kamo-naoyuki/rotari/internal/project"
)

type ResetInput struct {
	BaseDirRef string `json:"basedir_ref" jsonschema:"basedir_ref of the project, from rotari_list_projects"`
	Project    string `json:"project" jsonschema:"project name"`
}

type ApplyResetInput struct {
	ResetInput
	IfRevision         string `json:"if_revision" jsonschema:"revision from rotari_preview_reset; the reset applies only if the project is still at it"`
	RecoverInterrupted *bool  `json:"recover_interrupted,omitempty" jsonschema:"Deprecated and rejected; use rotari_preview_unlock and rotari_unlock to recover an interrupted run"`
}

type ResetOutput struct {
	Project  string `json:"project"`
	Cleared  int    `json:"cleared" jsonschema:"queued jobs the reset removes"`
	Revision string `json:"revision" jsonschema:"for a preview, the revision to pass to rotari_reset; after a reset, the new revision"`
}

// reset previews or applies `rotari reset` on a project through project.Reset.
func (tools writeTools) reset(input ResetInput, guard project.Guard) (ResetOutput, error) {
	_, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return ResetOutput{}, err
	}
	var outcome project.Outcome
	guard.Report = func(result project.Outcome) { outcome = result }
	result, err := project.Reset(paths, guard)
	if err != nil {
		return ResetOutput{}, err
	}
	output := ResetOutput{Project: paths.ProjectName, Cleared: result.Cleared, Revision: outcome.Revision}
	if outcome.Applied {
		output.Revision = outcome.NewRevision
	}
	return output, nil
}

func (tools writeTools) applyReset(input ApplyResetInput) (ResetOutput, error) {
	if input.RecoverInterrupted != nil {
		return ResetOutput{}, errors.New("recover_interrupted is no longer supported; use rotari_preview_unlock and rotari_unlock")
	}
	if input.IfRevision == "" {
		return ResetOutput{}, errors.New("if_revision is required; take it from rotari_preview_reset")
	}
	return tools.reset(input.ResetInput, project.Guard{IfRevision: input.IfRevision})
}

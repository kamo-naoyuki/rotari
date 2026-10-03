package mcp

import (
	"errors"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/project"
)

type ResetInput struct {
	BaseDirRef string `json:"basedir_ref" jsonschema:"basedir_ref of the project, from rotari_list_projects"`
	Project    string `json:"project" jsonschema:"project name"`
}

type ApplyResetInput struct {
	ResetInput
	IfRevision         string `json:"if_revision" jsonschema:"revision from rotari_preview_reset; the reset applies only if the project is still at it"`
	RecoverInterrupted bool   `json:"recover_interrupted,omitempty" jsonschema:"confirm recovering the project's interrupted run; required when the preview names one. Confirm first that its jobs have stopped"`
}

type ResetOutput struct {
	Project string `json:"project"`
	Cleared int    `json:"cleared" jsonschema:"queued jobs the reset removes"`
	// InterruptedRunID names the interrupted run the reset recovers.
	InterruptedRunID string `json:"interrupted_run_id,omitempty"`
	InterruptedRun   string `json:"interrupted_run,omitempty" jsonschema:"what the interrupted run's jobs last reported"`
	JobsMayBeRunning bool   `json:"jobs_may_be_running,omitempty" jsonschema:"some jobs of the interrupted run appear to still be running; confirm they stopped before recovering"`
	Revision         string `json:"revision" jsonschema:"for a preview, the revision to pass to rotari_reset; after a reset, the new revision"`
}

// reset previews or applies `rotari reset` on a project through
// project.Reset. A preview always plans the recovery of an interrupted run,
// and reports it so the caller can confirm it.
func (tools writeTools) reset(input ResetInput, recoverInterrupted bool, guard project.Guard) (ResetOutput, error) {
	_, paths, err := projectPaths(tools.masterDir, input.BaseDirRef, input.Project)
	if err != nil {
		return ResetOutput{}, err
	}
	// What an interrupted run's jobs last reported is read before a recovery
	// rewrites the project's metadata.
	var detail string
	var stillRunning bool
	if inspection, err := project.Inspect(paths, false); err == nil && inspection.State == project.Interrupted {
		detail, stillRunning = project.InterruptedRunDetail(paths, inspection.RunID)
	}
	var outcome project.Outcome
	guard.Report = func(result project.Outcome) { outcome = result }
	result, err := project.Reset(paths, recoverInterrupted, guard)
	if err != nil {
		return ResetOutput{}, err
	}
	output := ResetOutput{Project: paths.ProjectName, Cleared: result.Cleared, InterruptedRunID: result.RecoveredRunID, Revision: outcome.Revision}
	if outcome.Applied {
		output.Revision = outcome.NewRevision
	}
	if result.RecoveredRunID != "" {
		output.InterruptedRun, output.JobsMayBeRunning = strings.TrimPrefix(detail, ": "), stillRunning
	}
	return output, nil
}

func (tools writeTools) applyReset(input ApplyResetInput) (ResetOutput, error) {
	if input.IfRevision == "" {
		return ResetOutput{}, errors.New("if_revision is required; take it from rotari_preview_reset")
	}
	return tools.reset(input.ResetInput, input.RecoverInterrupted, project.Guard{IfRevision: input.IfRevision})
}

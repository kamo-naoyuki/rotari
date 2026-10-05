package workflowstate

import (
	"fmt"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

// RequireSettledRun refuses an active or interrupted run of the project:
// runID, or any when runID is empty. Their results are not final, so a
// manifest of them would re-execute jobs that may yet succeed. See
// project.EnsureRunSettled.
func RequireSettledRun(paths state.ProjectPaths, runID string, cleanupStale bool) error {
	return project.EnsureRunSettled(paths, runID, "export", cleanupStale)
}

// LoadSettledRun reads a settled run as a source for export, after checking
// with validate that its command snapshot can run.
func LoadSettledRun(paths state.ProjectPaths, runID string, validate func(model.Queue) error, cleanupStale bool) (workflow.SourceRun, error) {
	if err := RequireSettledRun(paths, runID, cleanupStale); err != nil {
		return workflow.SourceRun{}, err
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return workflow.SourceRun{}, err
	}
	queue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load run %s commands: %w", runID, err)
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		return workflow.SourceRun{}, fmt.Errorf("failed to load run %s summary: %w", runID, err)
	}
	queue = workflow.FlattenQueueDefaults(queue)
	if err := validate(queue); err != nil {
		return workflow.SourceRun{}, fmt.Errorf("invalid run %s commands: %w", runID, err)
	}
	return SourceRun(runID, runDir, queue, summary), nil
}

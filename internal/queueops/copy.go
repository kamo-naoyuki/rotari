package queueops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Copy copies the jobs that request selects from runID into the queue.
// Attempt IDs in request.JobIDs select those attempts. The caller confirms
// an overwrite before setting request.Overwrite.
func (editor Editor) Copy(baseDir, projectName, runID string, request queueedit.CopyRequest) (string, error) {
	selection := request.Selection
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", err
	}
	var copied int
	err = project.EditQueue(paths, "copy", func(queue *model.Queue) error {
		sourceRunDir, err := state.SafeJoin(paths.RunsDir, runID)
		if err != nil {
			return err
		}
		snapshot, err := state.LoadQueue(filepath.Join(sourceRunDir, "commands.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
		if err != nil {
			return fmt.Errorf("failed to load command snapshot: %w", err)
		}
		if len(snapshot.Commands) == 0 {
			return errors.New("command snapshot has no jobs")
		}
		summary, summaryErr := state.LoadRunSummary(filepath.Join(sourceRunDir, "summary.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
		if summaryErr != nil && selection != "all" {
			return fmt.Errorf("failed to load run summary: %w", summaryErr)
		}
		source := queueedit.Run{
			ID: runID, Snapshot: snapshot, Results: model.ResultsByID(summary.Results),
			Timestamps: func(jobID string) (string, string) {
				return state.ReadJobTimestamp(sourceRunDir, jobID, "submitted_at"), state.ReadJobTimestamp(sourceRunDir, jobID, "finished_at")
			},
		}
		if context, contextErr := state.LoadContext(editor.Store, sourceRunDir); contextErr == nil {
			source.CWD = context.CWD
		}
		sourceRequest := request
		sourceRequest.JobIDs = nil
		for _, jobID := range request.JobIDs {
			if !strings.HasPrefix(jobID, "att_") {
				sourceRequest.JobIDs = append(sourceRequest.JobIDs, jobID)
				continue
			}
			attempt, err := copyAttempt(sourceRunDir, runID, jobID)
			if err != nil {
				return err
			}
			sourceRequest.Attempts = append(sourceRequest.Attempts, attempt)
		}
		edited, count, err := queueedit.Copy(*queue, projectName, source, sourceRequest, editor.NewJobID)
		if err != nil {
			return err
		}
		*queue, copied = edited, count
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("copied jobs=%d from run=%s to queue=%s", copied, runID, projectName), nil
}

// copyAttempt checks that attemptID is an existing attempt of runID.
func copyAttempt(runDir, runID, attemptID string) (queueedit.Attempt, error) {
	payload, err := state.DecodeAttemptID(attemptID)
	if err != nil {
		return queueedit.Attempt{}, err
	}
	if payload.RunID != runID {
		return queueedit.Attempt{}, fmt.Errorf("attempt %q belongs to run %q, not %q", attemptID, payload.RunID, runID)
	}
	attemptDir, err := state.SpecificAttemptJobDir(runDir, payload.JobID, attemptID)
	if err != nil {
		return queueedit.Attempt{}, err
	}
	// codeql[go/path-injection]: attemptDir is produced by validated attempt path helpers.
	if info, statErr := os.Stat(attemptDir); statErr != nil || !info.IsDir() {
		return queueedit.Attempt{}, fmt.Errorf("attempt %q not found in run %s", attemptID, runID)
	}
	return queueedit.Attempt{ID: attemptID, JobID: payload.JobID}, nil
}

package queueops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Copy copies the jobs that request selects from runID into the queue.
// Attempt IDs in request.JobIDs select those attempts. The caller confirms
// an overwrite before setting request.Overwrite.
func (editor Editor) Copy(baseDir, projectName, runID string, request queueedit.CopyRequest) (string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", err
	}
	var copied int
	err = project.EditQueueGuarded(paths, editor.Guard, func(queue *model.Queue) error {
		edited, count, err := editor.CopySnapshot(baseDir, projectName, runID, *queue, request)
		if err != nil {
			return err
		}
		*queue, copied = edited, count
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := editor.registerBaseDir(paths.BaseDir); err != nil {
		return "", err
	}
	return fmt.Sprintf("copied jobs=%d from run=%s to queue=%s", copied, runID, projectName), nil
}

// CopySnapshot builds the queue that Copy would produce without writing it.
// The caller must hold the destination project's state lock when it needs the
// source and destination to describe one revision. Run startup uses this to
// build an immutable snapshot from a settled run while leaving queue.json for
// the next run untouched.
func (editor Editor) CopySnapshot(baseDir, projectName, runID string, destination model.Queue, request queueedit.CopyRequest) (model.Queue, int, error) {
	selection := request.Selection
	if request.Selection == "" {
		request.Selection = "all"
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return model.Queue{}, 0, err
	}
	if err := project.EnsureRunSettled(paths, runID, "copy", false); err != nil {
		return model.Queue{}, 0, err
	}
	sourceRunDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return model.Queue{}, 0, err
	}
	if err := state.CheckRunVersions(sourceRunDir); err != nil {
		return model.Queue{}, 0, err
	}
	snapshot, err := state.LoadQueue(filepath.Join(sourceRunDir, "commands.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
	if err != nil {
		return model.Queue{}, 0, fmt.Errorf("failed to load command snapshot: %w", err)
	}
	if len(snapshot.Commands) == 0 {
		return model.Queue{}, 0, errors.New("command snapshot has no jobs")
	}
	summary, summaryErr := state.LoadRunSummary(filepath.Join(sourceRunDir, "summary.json")) // NOSONAR: sourceRunDir is produced by validatedRunDir.
	if summaryErr != nil && selection != "all" {
		return model.Queue{}, 0, fmt.Errorf("failed to load run summary: %w", summaryErr)
	}
	source := queueedit.Run{
		ID: runID, Snapshot: snapshot, Results: model.ResultsByID(summary.Results),
		Timestamps: func(jobID string) (string, string) {
			return state.ReadJobTimestamp(sourceRunDir, jobID, "submitted_at"), state.ReadJobTimestamp(sourceRunDir, jobID, "finished_at")
		},
		FilterJob: func(jobID string, result model.JobResult, finished bool) jobfilter.Job {
			return jobstatus.FilterJob(editor.Store, filepath.Dir(sourceRunDir), model.JobOrigin{RunID: runID, JobID: jobID}, jobID, result, finished, time.Now())
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
			return model.Queue{}, 0, err
		}
		sourceRequest.Attempts = append(sourceRequest.Attempts, attempt)
	}
	return queueedit.Copy(destination, projectName, source, sourceRequest, editor.NewJobID)
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

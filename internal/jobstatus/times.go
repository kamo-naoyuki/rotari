package jobstatus

import (
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Timestamps returns a job's submitted and finished times. Missing values
// fall back to the origin record of a carried job, then to the origin job's
// own timestamps in its source run.
func Timestamps(runDir, jobID string, origin *model.JobOrigin) (submittedAt, finishedAt string) {
	submittedAt = state.ReadJobTimestamp(runDir, jobID, "submitted_at")
	finishedAt = state.ReadJobTimestamp(runDir, jobID, "finished_at")
	return resolveOriginTimestamps(submittedAt, finishedAt, origin, func(runID, sourceJobID string) (string, string) {
		sourceRunDir, err := state.SafeJoin(filepath.Dir(runDir), runID)
		if err != nil {
			return "", ""
		}
		return state.ReadJobTimestamp(sourceRunDir, sourceJobID, "submitted_at"), state.ReadJobTimestamp(sourceRunDir, sourceJobID, "finished_at")
	})
}

type originTimestampSource func(runID, jobID string) (submittedAt, finishedAt string)

func resolveOriginTimestamps(submittedAt, finishedAt string, origin *model.JobOrigin, source originTimestampSource) (string, string) {
	if origin == nil || (submittedAt != "" && finishedAt != "") {
		return submittedAt, finishedAt
	}
	if submittedAt == "" {
		submittedAt = origin.SubmittedAt
	}
	if finishedAt == "" {
		finishedAt = origin.FinishedAt
	}
	if submittedAt == "" || finishedAt == "" {
		sourceSubmitted, sourceFinished := source(origin.RunID, origin.JobID)
		if submittedAt == "" {
			submittedAt = sourceSubmitted
		}
		if finishedAt == "" {
			finishedAt = sourceFinished
		}
	}
	return submittedAt, finishedAt
}

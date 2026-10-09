package jobstatus

import (
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Timestamps returns a job's submitted and finished times. A carried result
// uses timestamps from its origin; a job executed in this run uses only its
// own attempt timestamps.
func Timestamps(runDir, jobID string, origin *model.JobOrigin, carried bool) (submittedAt, finishedAt string) {
	if !carried || origin == nil {
		return state.ReadJobTimestamp(runDir, jobID, "submitted_at"), state.ReadJobTimestamp(runDir, jobID, "finished_at")
	}
	return resolveOriginTimestamps(origin.SubmittedAt, origin.FinishedAt, origin, func(runID, sourceJobID string) (string, string) {
		sourceRunDir, err := state.SafeJoin(filepath.Dir(runDir), runID)
		if err != nil {
			return "", ""
		}
		return state.ReadJobTimestamp(sourceRunDir, sourceJobID, "submitted_at"), state.ReadJobTimestamp(sourceRunDir, sourceJobID, "finished_at")
	})
}

// LastOutputAt returns when the attempt in jobDir last wrote to its output
// logs, the latest modification time of its merged or separate stdout and
// stderr files, and false when it has written none. A running job whose last
// output is long past may be stuck. Logs an executor keeps on another host
// are not seen.
func LastOutputAt(jobDir string) (time.Time, bool) {
	var latest time.Time
	for _, name := range []string{"output", state.StdoutFileName, state.StderrFileName} {
		path, err := state.ValidatedStateFile(jobDir, name)
		if err != nil {
			continue
		}
		// codeql[go/path-injection]: path is returned by ValidatedStateFile for a fixed state file.
		info, err := os.Stat(path)
		if err != nil || info.Size() == 0 {
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest, !latest.IsZero()
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

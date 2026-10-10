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

// RunTime is how long a job has run and, while it runs, how long ago it last
// wrote output.
type RunTime struct {
	// Elapsed runs from submission to the finish, or to now while the job
	// runs. It is negative when either end is unknown.
	Elapsed time.Duration
	// Running is true for a submitted job that has not finished.
	Running bool
	// Quiet is, for a running job, how long ago it last wrote output. It is
	// negative when the job has written none, or is not running.
	Quiet time.Duration
}

// MeasureRunTime measures the attempt in jobDir from its submission and
// finish times; a zero time is unknown. Every view of a job's run time,
// `show` and `jobs` alike, measures through it.
func MeasureRunTime(jobDir string, submitted, finished time.Time, done bool, now time.Time) RunTime {
	unknown := RunTime{Elapsed: -1, Quiet: -1}
	if submitted.IsZero() {
		return unknown
	}
	if done {
		if finished.IsZero() {
			return unknown
		}
		return RunTime{Elapsed: finished.Sub(submitted), Quiet: -1}
	}
	runTime := RunTime{Elapsed: now.Sub(submitted), Running: true, Quiet: -1}
	if lastOutput, ok := LastOutputAt(jobDir); ok {
		runTime.Quiet = max(now.Sub(lastOutput), 0)
	}
	return runTime
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

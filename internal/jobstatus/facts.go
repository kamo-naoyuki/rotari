package jobstatus

import (
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// maxCarryDepth bounds how many carried origins FilterJob follows.
const maxCarryDepth = 16

// FilterJob returns what job filters need to know about the job that origin
// names under runsDir: result and finished as the caller resolved them, and
// the hosts, times, and log of the attempt that produced them. A job carried
// into origin's run is read from the run it was carried from. id is the job's
// ID for definition conditions. Hosts recorded on result take precedence over
// the attempt's, and times the carry records fill in ones the attempt lacks.
func FilterJob(store state.Store, runsDir string, origin model.JobOrigin, id string, result model.JobResult, finished bool, now time.Time) jobfilter.Job {
	attemptDir, carries := locateAttempt(runsDir, origin, 0)
	attributes := AttemptAttributes(store, attemptDir, now)
	if len(result.Hosts) > 0 {
		attributes.Hosts = result.Hosts
	}
	for _, carry := range append([]model.JobOrigin{origin}, carries...) {
		if attributes.StartedAt.IsZero() {
			attributes.StartedAt, _ = jobfilter.ParseTimestamp(carry.SubmittedAt)
		}
		if attributes.FinishedAt.IsZero() {
			attributes.FinishedAt, _ = jobfilter.ParseTimestamp(carry.FinishedAt)
		}
	}
	return jobfilter.Job{
		ID: id, Result: result, Finished: finished, Attributes: attributes,
		Diagnosis: func(selectors []string) bool {
			log, err := ReadLog(attemptDir)
			return err == nil && diagnose.MatchesResultSelectors(selectors, result, log)
		},
	}
}

// AttemptAttributes reads the hosts and times an attempt directory records:
// the wrapper's start, or the submission when it has not started.
func AttemptAttributes(store state.Store, attemptDir string, now time.Time) jobfilter.Attributes {
	attributes := jobfilter.Attributes{Now: now}
	if attemptDir == "" {
		return attributes
	}
	if path, err := state.ValidatedStateFile(attemptDir, "status.json"); err == nil {
		if status, ok := executor.LoadWrapperStatus(store, path); ok {
			attributes.Hosts = status.Hosts
			attributes.StartedAt, _ = jobfilter.ParseTimestamp(status.StartedAt)
		}
	}
	if attributes.StartedAt.IsZero() {
		attributes.StartedAt, _ = jobfilter.ParseTimestamp(state.ResolveAttemptTimestamp(attemptDir, "submitted_at"))
	}
	attributes.FinishedAt, _ = jobfilter.ParseTimestamp(state.ResolveAttemptTimestamp(attemptDir, "finished_at"))
	return attributes
}

// ReadLog returns the log of an attempt for diagnosis: its merged output, or
// else its stdout, or else its stderr.
func ReadLog(attemptDir string) (string, error) {
	if attemptDir == "" {
		return "", os.ErrNotExist
	}
	for _, name := range []string{"output", state.StdoutFileName, state.StderrFileName} {
		data, err := os.ReadFile(filepath.Join(attemptDir, name))
		if err == nil {
			return string(data), nil
		}
	}
	return "", os.ErrNotExist
}

// locateAttempt returns the directory of the attempt origin names, following
// the origins of carried jobs, and the carries it followed; "" when none is
// found.
func locateAttempt(runsDir string, origin model.JobOrigin, depth int) (string, []model.JobOrigin) {
	if origin.AttemptID != "" {
		if payload, err := state.DecodeAttemptID(origin.AttemptID); err == nil {
			if runDir, err := state.SafeJoin(runsDir, payload.RunID); err == nil {
				if dir, err := state.SpecificAttemptJobDir(runDir, payload.JobID, origin.AttemptID); err == nil && isDir(dir) {
					return dir, nil
				}
			}
		}
	}
	runDir, err := state.SafeJoin(runsDir, origin.RunID)
	if err != nil {
		return "", nil
	}
	if dir, err := state.LatestAttemptJobDir(runDir, origin.JobID); err == nil && isDir(dir) {
		return dir, nil
	}
	carried := state.LoadRunOrigin(runDir, origin.JobID)
	if carried == nil || depth >= maxCarryDepth {
		return "", nil
	}
	dir, carries := locateAttempt(runsDir, *carried, depth+1)
	return dir, append([]model.JobOrigin{*carried}, carries...)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

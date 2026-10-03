package state

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func FinalizeRun(queue model.Queue, meta model.Meta, runID string, exitCode int, now time.Time) (model.Queue, model.Meta, error) {
	if runID == "" {
		return model.Queue{}, model.Meta{}, fmt.Errorf("run ID must not be empty")
	}
	queue.Commands = nil
	meta.Phase = "finished"
	meta.LastRunID = runID
	meta.LastRunExitCode = exitCode
	meta.UpdatedAt = now.UTC().Format(time.RFC3339)
	return queue, meta, nil
}

// LoadRunOrigin returns jobID's origin from the run's commands.json snapshot,
// or nil when the job has none or the snapshot cannot be read; see
// model.Queue.OriginOf.
func LoadRunOrigin(runDir, jobID string) *model.JobOrigin {
	queue, err := LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil
	}
	return queue.OriginOf(jobID)
}

// CheckRunVersions returns ErrNewerStateVersion when the run's commands.json
// or summary.json was written by a newer rotari, and nil otherwise, leaving a
// missing or unreadable file to the caller. Readers that treat those files
// as optional call it first, so a newer file is refused instead of being read
// as absent.
func CheckRunVersions(runDir string) error {
	if _, err := LoadQueue(filepath.Join(runDir, "commands.json")); errors.Is(err, ErrNewerStateVersion) {
		return err
	}
	for _, name := range []string{"summary.json", CarriedResultsFileName} {
		if _, err := LoadRunSummary(filepath.Join(runDir, name)); errors.Is(err, ErrNewerStateVersion) {
			return err
		}
	}
	return nil
}

// CarriedResultsFileName is the run file in which a run records, before it
// dispatches any job, the results it carries forward from earlier runs
// instead of executing. It holds a model.RunSummary with only RunID and
// Results, and is read through LoadRunSummary until the run writes
// summary.json; see jobstatus.RecordedResults.
const CarriedResultsFileName = "carried.json"

func LoadRunSummary(path string) (model.RunSummary, error) {
	var summary model.RunSummary
	if err := NewStore(0o700, 0o600).ReadJSON(path, &summary); err != nil {
		return model.RunSummary{}, err
	}
	if err := checkStateVersion(path, summary.StateVersion); err != nil {
		return model.RunSummary{}, err
	}
	return summary, nil
}

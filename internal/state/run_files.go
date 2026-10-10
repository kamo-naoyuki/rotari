package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// FinalizeRun returns meta marked finished with runID's exit code.
func FinalizeRun(meta model.Meta, runID string, exitCode int, now time.Time) (model.Meta, error) {
	if runID == "" {
		return model.Meta{}, fmt.Errorf("run ID must not be empty")
	}
	meta.Phase = "finished"
	meta.LastRunID = runID
	meta.LastRunExitCode = exitCode
	meta.UpdatedAt = now.UTC().Format(time.RFC3339)
	return meta, nil
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

// RunSourcesFileName is the run file in which a run records, before it
// dispatches any job, the version-control revisions its executed jobs run
// from. A run started before rotari recorded sources has none.
const RunSourcesFileName = "sources.json"

// SaveRunSources writes a run's sources.
func SaveRunSources(runDir string, sources model.RunSources) error {
	return WriteJSON(filepath.Join(runDir, RunSourcesFileName), sources)
}

// LoadRunSources reads a run's sources, and false when the run recorded
// none.
func LoadRunSources(runDir string) (model.RunSources, bool, error) {
	var sources model.RunSources
	path := filepath.Join(runDir, RunSourcesFileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return model.RunSources{}, false, nil
	}
	if err != nil {
		return model.RunSources{}, false, err
	}
	if err := json.Unmarshal(data, &sources); err != nil {
		return model.RunSources{}, false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	return sources, true, nil
}

// AttemptSource returns the recorded revision of the repository the attempt
// in attemptDir ran from: its working directory, from its command.json,
// matched against its run's sources. It returns false when either is not
// recorded.
func AttemptSource(runDir, attemptDir string) (model.SourceRevision, bool) {
	var spec model.JobSpec
	data, err := os.ReadFile(filepath.Join(attemptDir, "command.json"))
	if err != nil || json.Unmarshal(data, &spec) != nil || spec.WorkingDirectory == "" {
		return model.SourceRevision{}, false
	}
	sources, ok, err := LoadRunSources(runDir)
	if err != nil || !ok {
		return model.SourceRevision{}, false
	}
	return sources.SourceFor(spec.WorkingDirectory)
}

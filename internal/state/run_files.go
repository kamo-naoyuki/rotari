package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		path := filepath.Join(runDir, entry.Name())
		if entry.IsDir() {
			path = filepath.Join(path, ManualRetryPendingFileName)
		} else if !isManualRetryRootFile(entry.Name()) {
			continue
		}
		if err := checkRetryStateVersion(path); err != nil {
			return err
		}
	}
	return nil
}

// isManualRetryRootFile reports whether name is active-retry protocol state
// stored in a run's root directory.
func isManualRetryRootFile(name string) bool {
	return name == ManualRetryAcceptingFileName || strings.HasPrefix(name, ManualRetryRequestPrefix) && strings.HasSuffix(name, ".json")
}

// checkRetryStateVersion checks one retry protocol file. Such files are
// written and removed while the run is active, so a vanished file is fine.
func checkRetryStateVersion(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var versioned struct {
		StateVersion int `json:"state_version"`
	}
	if err := json.Unmarshal(data, &versioned); err != nil {
		return nil
	}
	if versioned.StateVersion == 0 {
		return fmt.Errorf("%s is missing state_version", path)
	}
	return checkStateVersion(path, versioned.StateVersion)
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

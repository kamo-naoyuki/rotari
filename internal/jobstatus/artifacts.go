package jobstatus

import (
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Artifacts returns the artifact candidates recorded for the attempt that
// origin names under runsDir. A job carried into origin's run is read from
// the attempt that produced its result, as FilterJob reads it. ok is false
// when that attempt has no record, as in state written before discovery
// existed or for an attempt that never started: discovery information is
// unavailable, which does not mean the job had no associated files.
func Artifacts(store state.Store, runsDir string, origin model.JobOrigin) (artifact.Record, bool) {
	attemptDir, _ := locateAttempt(runsDir, origin, 0)
	if attemptDir == "" {
		return artifact.Record{}, false
	}
	var record artifact.Record
	if err := store.ReadJSON(filepath.Join(attemptDir, state.ArtifactsFileName), &record); err != nil {
		return artifact.Record{}, false
	}
	return record, true
}

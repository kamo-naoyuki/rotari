package projectrun

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// SourcePolicy says whether a run uses the current queue or constructs its
// snapshot from a saved run. CopyIfEmpty is resolved against the queue under
// the state lock, so a concurrent queue edit cannot change the decision.
type SourcePolicy string

const (
	SourceKeepQueue   SourcePolicy = "queue"
	SourceCopyIfEmpty SourcePolicy = "copy-if-empty"
	SourceCopyRun     SourcePolicy = "copy-run"
)

// RunSource describes the input requested by `rotari run` or `retry` without
// deciding from a potentially stale queue read. An explicit run is copied;
// a selection copies the latest run only if the queue is empty when the
// supervisor takes its state lock; a plain run keeps the queue.
func RunSource(paths state.ProjectPaths, selection, requestedRunID string) (sourceRunID string, policy SourcePolicy, err error) {
	if requestedRunID != "" {
		return requestedRunID, SourceCopyRun, nil
	}
	if selection == "" {
		return "", SourceKeepQueue, nil
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return "", SourceKeepQueue, fmt.Errorf("failed to load metadata: %w", err)
	}
	if meta.LastRunID == "" {
		return "", SourceKeepQueue, fmt.Errorf("queue %q has no previous run", paths.ProjectName)
	}
	return meta.LastRunID, SourceCopyIfEmpty, nil
}

package projectrun

import (
	"fmt"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// RunSource decides where a run request's queue comes from, as `rotari run`
// and `retry` do. An explicit requestedRunID is copied into the queue first.
// Without one, a selection (a result filter or job selection) refers to the
// project's last run, which is copied only into an empty queue, so a queue
// restored and edited earlier is kept. A plain run uses the queue as it is.
func RunSource(paths state.ProjectPaths, selection, requestedRunID string) (sourceRunID string, copyFirst bool, err error) {
	if requestedRunID != "" {
		return requestedRunID, true, nil
	}
	if selection == "" {
		return "", false, nil
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return "", false, fmt.Errorf("failed to load metadata: %w", err)
	}
	if meta.LastRunID == "" {
		return "", false, fmt.Errorf("queue %q has no previous run", paths.ProjectName)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		return "", false, fmt.Errorf("failed to load queue: %w", err)
	}
	return meta.LastRunID, len(queue.Commands) == 0, nil
}

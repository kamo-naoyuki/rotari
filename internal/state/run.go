package state

import (
	"fmt"
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

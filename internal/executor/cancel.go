package executor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rotari-dev/rotari/internal/state"
)

// RecordCancellation writes the marker that a job's wrapper and its final
// result read as an explicit cancel.
func RecordCancellation(jobDir string) error {
	// codeql[go/path-injection]: jobDir is validated and cancelled is a fixed file name.
	return os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte(nowRFC3339()+"\n"), state.FileMode()) // NOSONAR: jobDir comes from validated job path helpers.
}

// CancelJob cancels the submitted job in jobDir through canceller after
// recording the cancel, since a scheduler may keep no result for a job
// deleted while queued.
func CancelJob(jobDir string, canceller Canceller) error {
	if err := RecordCancellation(jobDir); err != nil {
		return fmt.Errorf("record cancellation: %w", err)
	}
	return canceller.Cancel(jobDir)
}

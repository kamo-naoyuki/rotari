package attachment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ID returns the stable identifier sent in the run startup request.
func (session *Session) ID() string {
	if session == nil {
		return ""
	}
	return session.id
}

// EnableRun marks a newly started run as using session-derived attachment
// state. Older runs without this marker retain their lock-boolean fallback.
func EnableRun(paths state.ProjectPaths, runID string) error {
	if !state.IsValidPathElement(runID) {
		return fmt.Errorf("invalid run ID %q", runID)
	}
	dir := filepath.Join(paths.ProjectDir, directoryName)
	if err := os.MkdirAll(dir, state.DirectoryMode()); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, runID+".enabled"), os.O_CREATE|os.O_WRONLY, state.FileMode())
	if err != nil {
		return err
	}
	return file.Close()
}

// IsAttached derives current attachment state, retaining compatibility with
// active runs created before session records were introduced.
func IsAttached(paths state.ProjectPaths, runID string) (bool, error) {
	attached, err := Attached(paths, runID)
	if err != nil || attached || Enabled(paths, runID) {
		return attached, err
	}
	lock, err := state.LoadLock(paths.LockFile)
	if errors.Is(err, os.ErrNotExist) || err == nil && lock.RunID != runID {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return lock.ClientAttached, nil
}

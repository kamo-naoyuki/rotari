package queueops

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// DeleteHistory deletes one run, or every run of the project when runID is
// empty, and returns the message the CLI reports.
func (editor Editor) DeleteHistory(baseDir, projectName, runID string) (string, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to resolve paths: %w", err)
	}
	if err := os.MkdirAll(paths.ProjectDir, state.DirectoryMode()); err != nil {
		return "", fmt.Errorf("failed to create queue directory: %w", err)
	}
	var message string
	err = project.Edit(paths, "delete", func() error {
		var err error
		message, err = editor.deleteHistory(paths, runID)
		return err
	})
	if err != nil {
		return "", err
	}
	return message, nil
}

// DeleteRun deletes one run of an idle project.
func (editor Editor) DeleteRun(baseDir, projectName, runID string) error {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return err
	}
	return project.Edit(paths, "delete", func() error { return editor.deleteRun(paths, runID) })
}

// deleteHistory deletes one run, or every run when runID is empty, and
// returns the message to report. The caller holds the state lock of an idle
// project.
func (editor Editor) deleteHistory(paths state.ProjectPaths, runID string) (string, error) {
	if runID != "" {
		if err := editor.deleteRun(paths, runID); err != nil {
			return "", fmt.Errorf("failed to clear run %q: %w", runID, err)
		}
		return fmt.Sprintf("cleared logs project=%s run=%s", paths.ProjectName, runID), nil
	}
	deletedRunIDs := runIDsInDirectory(paths.RunsDir)
	if err := os.RemoveAll(paths.RunsDir); err != nil {
		return "", fmt.Errorf("failed to clear run history: %w", err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return "", fmt.Errorf("failed to load metadata: %w", err)
	}
	meta.LastRunID = ""
	meta.LastRunExitCode = 0
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return "", fmt.Errorf("failed to update metadata: %w", err)
	}
	for _, deleted := range deletedRunIDs {
		if err := editor.UnregisterRun(deleted); err != nil {
			return "", fmt.Errorf("failed to remove run registry entry %q: %w", deleted, err)
		}
	}
	return fmt.Sprintf("cleared logs project=%s directory=%s", paths.ProjectName, filepath.Join(paths.ProjectDir, "runs")), nil
}

func runIDsInDirectory(runsDir string) []string {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return nil
	}
	runIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			runIDs = append(runIDs, entry.Name())
		}
	}
	return runIDs
}

func (editor Editor) deleteRun(paths state.ProjectPaths, runID string) error {
	if !state.IsValidPathElement(runID) {
		return fmt.Errorf("run %q not found", runID)
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	// codeql[go/path-injection]: runDir is produced by validatedRunDir.
	info, err := os.Stat(runDir) // NOSONAR: runDir is produced by validatedRunDir.
	if err != nil || !info.IsDir() {
		return fmt.Errorf("run %q not found", runID)
	}
	// codeql[go/path-injection]: runDir is produced by validatedRunDir.
	if err := os.RemoveAll(runDir); err != nil { // NOSONAR: runDir is produced by validatedRunDir.
		return fmt.Errorf("failed to clear run %q: %w", runID, err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		return err
	}
	if meta.LastRunID == runID {
		meta.LastRunID = latestRunID(paths.RunsDir)
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		return err
	}
	return editor.UnregisterRun(runID)
}

func latestRunID(runsDir string) string {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return ""
	}
	latestID := ""
	var latestTime time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || (latestID != "" && !info.ModTime().After(latestTime)) {
			continue
		}
		latestID = entry.Name()
		latestTime = info.ModTime()
	}
	return latestID
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

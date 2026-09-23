package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func cmdDelete(args []string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && *runIDOption != "") {
		printError("usage: " + cliUsage("delete"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*runIDOption = fs.Args()[0]
	}

	baseDir, queueName, err := resolveExistingRunTarget(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	if err := os.MkdirAll(paths.ProjectDir, stateDirMode()); err != nil {
		printErrorf("failed to create queue directory: %v", err)
		return 1
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		printErrorf("failed to lock queue: %v", err)
		return 1
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "delete"); err != nil {
		printError(err)
		return 1
	}

	if *runIDOption != "" {
		if err := deleteRun(paths, *runIDOption); err != nil {
			printErrorf("failed to clear run %q: %v", *runIDOption, err)
			return 1
		}
		fmt.Printf("%s\n", colorKeyValueMessage(fmt.Sprintf("cleared logs project=%s run=%s", queueName, *runIDOption), green))
		return 0
	}

	deletedRunIDs := runIDsInDirectory(paths.RunsDir)
	if err := os.RemoveAll(paths.RunsDir); err != nil {
		printErrorf("failed to clear run history: %v", err)
		return 1
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		printErrorf("failed to load metadata: %v", err)
		return 1
	}
	if *runIDOption == "" || meta.LastRunID == *runIDOption {
		meta.LastRunID = latestRunID(paths.RunsDir)
		if meta.LastRunID == "" {
			meta.LastRunExitCode = 0
		}
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		printErrorf("failed to update metadata: %v", err)
		return 1
	}
	for _, runID := range deletedRunIDs {
		if err := unregisterRun(runID); err != nil {
			printErrorf("failed to remove run registry entry %q: %v", runID, err)
			return 1
		}
	}
	fmt.Printf("%s\n", colorKeyValueMessage(fmt.Sprintf("cleared logs project=%s directory=%s", queueName, filepath.Join(paths.ProjectDir, "runs")), green))
	return 0
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

func deleteRun(paths pathSet, runID string) error {
	if !state.IsValidPathElement(runID) {
		return fmt.Errorf("run %q not found", runID)
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return err
	}
	info, err := os.Stat(runDir) // NOSONAR: runDir is produced by validatedRunDir.
	if err != nil || !info.IsDir() {
		return fmt.Errorf("run %q not found", runID)
	}
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
	return unregisterRun(runID)
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

func clearRunHistory(baseDir, queueName, runID string) error {
	paths, err := resolvePaths(baseDir, queueName)
	if err != nil {
		return err
	}
	release, err := acquireStateLock(paths.StateLockFile)
	if err != nil {
		return fmt.Errorf("failed to lock queue: %w", err)
	}
	defer release()
	if err := ensureProjectIdleForPaths(paths, "delete"); err != nil {
		return err
	}
	return deleteRun(paths, runID)
}

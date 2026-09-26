package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdUnlock removes a stale running lock after validating that the recorded
// runner process is no longer active.
func cmdUnlock(args []string) int {
	fs := flag.NewFlagSet("unlock", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 {
		printError("usage: " + cliUsage("unlock"))
		return 1
	}
	if len(fs.Args()) == 1 && cliOptionSet(fs, "project-name") {
		// The positional argument used to be the run ID in this case.
		printErrorf("the positional argument names the project, so it cannot be combined with --project-name; pass the run ID as --run-id %s", fs.Args()[0])
		return 1
	}
	if len(fs.Args()) == 1 {
		*queueNameOption = fs.Args()[0]
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := state.ResolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	release, err := state.AcquireStateLock(paths.StateLockFile)
	if err != nil {
		printErrorf("failed to lock queue: %v", err)
		return 1
	}
	defer release()
	lock, err := state.LoadLock(paths.LockFile)
	removedLock := false
	lockExists := err == nil
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			printErrorf("failed to read run lock: %v", err)
			return 1
		}
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil {
		printErrorf("failed to load metadata: %v", err)
		return 1
	}
	if *runID == "" {
		if lockExists {
			*runID = lock.RunID
		} else {
			*runID = meta.LastRunID
		}
	}
	if *runID == "" {
		printError("no matching interrupted run exists")
		return 1
	}
	if lockExists {
		if lock.RunID != *runID {
			printErrorf("run lock belongs to %q, not %q", lock.RunID, *runID)
			return 1
		}
		if err := os.Remove(paths.LockFile); err != nil {
			printErrorf("failed to remove run lock: %v", err)
			return 1
		}
		removedLock = true
	}
	if !removedLock && (meta.Phase != "running" && meta.Phase != "cancelling" || meta.LastRunID != *runID) {
		printError("no matching interrupted run exists")
		return 1
	}
	meta.Phase = "collecting"
	meta.UpdatedAt = nowRFC3339()
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		printErrorf("failed to update metadata: %v", err)
		return 1
	}
	message := fmt.Sprintf("recovered queue project=%s run_id=%s", queueName, *runID)
	fmt.Println(colorKeyValueMessage(message, green))
	return 0
}

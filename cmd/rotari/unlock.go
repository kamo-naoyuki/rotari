package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
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
	// An explicit run ID must still locate and verify an existing run. Without
	// one, unlocking a project that has never been created is a no-op.
	var baseDir, queueName string
	var err error
	if *runID != "" {
		baseDir, queueName, err = resolveCLIExistingRun(*basedir, *queueNameOption, *runID)
	} else {
		baseDir, _, err = state.ResolveBaseDir(*basedir)
		if err == nil {
			queueName, err = state.ResolveProjectName(baseDir, *queueNameOption)
		}
	}
	if err != nil {
		printError(err)
		return 1
	}
	if !resolve.ProjectExists(baseDir, queueName) {
		fmt.Printf("project=%s already unlocked\n", queueName)
		return 0
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	result, err := project.Unlock(paths, *runID, project.Guard{})
	switch {
	case errors.Is(err, project.ErrRunAlive):
		lock, _ := state.LoadLock(paths.LockFile)
		fmt.Fprint(os.Stderr, formatProjectRunningError(paths, lock.RunID))
		return 1
	case err != nil:
		printError(err)
		return 1
	case result.RunID == "":
		fmt.Printf("project=%s already unlocked\n", queueName)
		return 0
	}
	if result.JobsMayBeRunning {
		printWarningf("%s; make sure they have stopped before rerunning them", result.Detail)
	}
	message := fmt.Sprintf("recovered queue project=%s run_id=%s", queueName, result.RunID)
	fmt.Println(colorKeyValueMessage(message, green))
	fmt.Println("Rerun its failed and unfinished jobs: " + project.RerunCommand(paths, result.RunID))
	return 0
}

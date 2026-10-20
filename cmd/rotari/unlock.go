package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdUnlock previews or applies recovery using the shared ownership policy.
func cmdUnlock(args []string) int {
	fs := flag.NewFlagSet("unlock", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	assumeStopped := cliBool(fs, "assume-stopped", false)
	dryRun := cliBool(fs, "dry-run", false)
	jsonOutput := cliBool(fs, "json", false)
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
		if *jsonOutput {
			if err := json.NewEncoder(os.Stdout).Encode(project.UnlockResult{Allowed: true}); err != nil {
				printError(err)
				return 1
			}
			return 0
		}
		fmt.Printf("project=%s already unlocked\n", queueName)
		return 0
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	result, err := project.Unlock(paths, *runID, project.Guard{DryRun: *dryRun}, *assumeStopped)
	if *jsonOutput {
		if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
			printError(encodeErr)
			return 1
		}
		if err != nil {
			printError(err)
			return 1
		}
		return 0
	}
	switch {
	case err != nil:
		printError(err)
		return 1
	case result.RunID == "":
		fmt.Printf("project=%s already unlocked\n", queueName)
		return 0
	}
	if *dryRun {
		fmt.Printf("dry run: project=%s run_id=%s allowed=%t refusal_code=%s coordinator=%s reason=%q revision=%s\n", queueName, result.RunID, result.Allowed, result.RefusalCode, result.Coordinator.State, result.Coordinator.Reason, result.Revision)
		fmt.Printf("Recovery action: %s; %s\n", result.Action, result.Repair.Reason)
		if !result.Allowed {
			fmt.Println((&project.UnlockRefusedError{Result: result}).Error())
		}
		if result.Detail != "" {
			fmt.Println("Recorded jobs: " + result.Detail)
		}
		fmt.Println("Job termination is not verified; confirm jobs stopped before applying recovery.")
		return 0
	}
	if result.JobsMayBeRunning {
		printWarningf("%s; make sure they have stopped before rerunning them", result.Detail)
	}
	if result.Coordinator.State == project.CoordinatorUnknown {
		printWarningf("coordinator ownership was not verified; recovery used --assume-stopped: %s", result.Coordinator.Reason)
	}
	message := fmt.Sprintf("recovered queue project=%s run_id=%s", queueName, result.RunID)
	if result.Action == "finalize" {
		message = fmt.Sprintf("repaired finalization project=%s run_id=%s", queueName, result.RunID)
	}
	fmt.Println(colorKeyValueMessage(message, green))
	fmt.Println("Job termination was not verified; recovery does not stop jobs.")
	if result.Action == "finalize" {
		fmt.Println("Recorded results and finish time were preserved; completion notifications were not replayed.")
	} else {
		fmt.Println("Recovery action: release; " + result.Repair.Reason)
		fmt.Println("Rerun its failed and unfinished jobs: " + project.RerunCommand(paths, result.RunID))
	}
	return 0
}

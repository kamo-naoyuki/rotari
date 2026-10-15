package main

import (
	"flag"
	"os"
)

// cmdDelete removes one run, all historical runs, or the whole selected project.
func cmdDelete(args []string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	allRuns := cliBool(fs, "all", false)
	wholeProject := cliBool(fs, "project", false)
	guard := cliGuardFlags(fs)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if *wholeProject {
		return deleteProject(args, fs.Args(), *basedir, *queueNameOption, *runIDOption, *allRuns, guard)
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && *runIDOption != "") {
		printError("usage: " + cliUsage("delete"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*runIDOption = fs.Args()[0]
	}
	// Deleting every run is never the default.
	if (*runIDOption == "") == !*allRuns {
		printError("pass a run ID to delete one run, or --all to delete every run of the project")
		return 1
	}

	baseDir, queueName, resolvedRunID, err := resolveCLIExistingRunID(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	*runIDOption = resolvedRunID
	message, err := guard.editor().DeleteHistory(baseDir, queueName, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	guard.printResult(message, false)
	return 0
}

// deleteProject is delete --project. The project must be named on the command
// line: deleting a whole project is never taken from a default.
func deleteProject(args, positional []string, basedir, projectName, runID string, allRuns bool, guard guardFlags) int {
	if len(positional) > 0 || runID != "" || allRuns {
		printError("--project deletes the whole project and cannot be combined with a run ID or --all; name the project with --project-name/-p")
		return 1
	}
	if _, given := configLocationArgs(args); given == "" {
		printError("--project needs the project named with --project-name/-p on the command line")
		return 1
	}
	editor := guard.editor()
	editor.Warn = func(message string) { printWarningf("%s", message) }
	message, err := editor.DeleteProject(basedir, projectName)
	if err != nil {
		printError(err)
		return 1
	}
	guard.printResult(message, false)
	return 0
}

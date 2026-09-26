package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/resolve"
)

// cmdDelete removes one run or all historical runs from the selected project.
func cmdDelete(args []string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runIDOption := cliString(fs, "run-id", "")
	allRuns := cliBool(fs, "all", false)
	if err := cliParse(fs, args); err != nil {
		return 1
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

	baseDir, queueName, resolvedRunID, err := resolve.ExistingRunID(*basedir, *queueNameOption, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	*runIDOption = resolvedRunID
	message, err := queueEditor().DeleteHistory(baseDir, queueName, *runIDOption)
	if err != nil {
		printError(err)
		return 1
	}
	fmt.Printf("%s\n", colorKeyValueMessage(message, green))
	return 0
}

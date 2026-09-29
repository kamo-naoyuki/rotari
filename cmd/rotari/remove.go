package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
)

// cmdRemove removes jobs from the current queue or prepares a filtered
// follow-up run from historical commands.
func cmdRemove(args []string) int {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	filterOptions := cliJobFilterOptions(fs, definitionJobFilters)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	jobName := cliString(fs, "job-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	allJobs := cliBool(fs, "all", false)
	quiet := cliBool(fs, "quiet", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 && len(jobIDs) > 0 {
		printError("usage: " + cliUsage("remove"))
		return 1
	}
	jobIDs = append(jobIDs, fs.Args()...)
	scope, err := filterOptions.scope()
	if err != nil {
		printError(err)
		return 1
	}
	selector := model.CommandSelector{IDs: jobIDs, Name: *jobName, Stage: *filterOptions.stage, Matrix: *filterOptions.matrix, All: *allJobs}
	if scope.Stage != "" {
		selector.Stage = scope.Stage
	}
	if scope.Matrix != "" {
		selector.Matrix = scope.Matrix
	}
	if selector.Kinds() != 1 {
		printError("usage: " + cliUsage("remove"))
		return 1
	}

	if *queueNameOption == "" && *runID == "" {
		project, err := locateQueuedJobs(*basedir, selector)
		if err != nil {
			printError(err)
			return 1
		}
		*queueNameOption = project
	}
	baseDir, queueName, err := resolve.ExistingRun(*basedir, *queueNameOption, *runID)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := queueEditor().RemoveWithFilter(baseDir, queueName, *runID, selector, filterOptions.filter())
	if err != nil {
		printError(err)
		return 1
	}
	if !*quiet {
		fmt.Println(colorKeyValueMessage(message, green))
	}
	return 0
}

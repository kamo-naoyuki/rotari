package main

import (
	"errors"
	"flag"
	"os"
	"strings"

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
	guard := cliGuardFlags(fs)
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
	if err := validateRemoveSelector(fs, selector); err != nil {
		printError(err)
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
	message, err := guard.editor().RemoveWithFilter(baseDir, queueName, *runID, selector, filterOptions.filter())
	if err != nil {
		printError(err)
		return 1
	}
	guard.printResult(message, *quiet)
	return 0
}

func validateRemoveSelector(fs *flag.FlagSet, selector model.CommandSelector) error {
	switch selector.Kinds() {
	case 0:
		return errors.New("usage: " + cliUsage("remove"))
	case 1:
		return nil
	default:
		return errors.New(removeSelectorConflict(fs, selector))
	}
}

func removeSelectorConflict(fs *flag.FlagSet, selector model.CommandSelector) string {
	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	options := make([]string, 0, selector.Kinds())
	if len(selector.IDs) > 0 {
		switch {
		case visited["job-id"]:
			options = append(options, "--job-id")
		case visited["j"]:
			options = append(options, "-j")
		case len(fs.Args()) > 0:
			options = append(options, "positional job IDs")
		default:
			options = append(options, "--job-id")
		}
	}
	if selector.Name != "" {
		options = append(options, "--job-name")
	}
	if selector.Stage != "" {
		options = append(options, visitedSelectorOption(visited, "stage", "filter-stage"))
	}
	if selector.Matrix != "" {
		options = append(options, visitedSelectorOption(visited, "matrix", "filter-matrix"))
	}
	if selector.All {
		options = append(options, "--all")
	}
	return "remove selectors cannot be combined: " + strings.Join(options, " and ")
}

func visitedSelectorOption(visited map[string]bool, short, long string) string {
	switch {
	case visited[short] && visited[long]:
		return "--" + short + "/--" + long
	case visited[long]:
		return "--" + long
	case visited[short]:
		return "--" + short
	default:
		return "--" + short
	}
}

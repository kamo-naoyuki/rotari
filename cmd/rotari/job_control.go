package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
)

// jobControlOptions holds the options cancel, suspend, and resume share
// beyond job IDs: --job-name, the filters, --filter-state, and --yes.
type jobControlOptions struct {
	names   stringSliceFlag
	states  cliChoiceList
	yes     *bool
	filters *jobFilterOptions
}

// cliJobControlOptions registers the shared options on fs; states lists the
// values --filter-state accepts.
func cliJobControlOptions(fs *flag.FlagSet, states []string) *jobControlOptions {
	options := &jobControlOptions{states: cliChoiceList{choices: states}}
	cliValue(fs, &options.names, "job-name")
	cliValue(fs, &options.states, "filter-state")
	options.yes = cliBool(fs, "yes", false)
	options.filters = cliJobFilterOptions(fs, jobControlFilters)
	return options
}

// jobControlFilterSpecs returns the filter options of a job-control command
// whose --filter-state accepts states.
func jobControlFilterSpecs(states []string) []cliFlagSpec {
	return append([]cliFlagSpec{
		{Name: "filter-state", Description: "select jobs in this state; may be repeated", ValueName: "STATE", Values: states, Repeated: true, CommandLineOnly: true},
	}, jobFilterFlagSpecs(jobControlFilters)...)
}

// selection returns the jobcontrol selection the options describe, and
// whether it uses filters rather than job names alone.
func (options *jobControlOptions) selection() (jobcontrol.Selection, bool, error) {
	scope, err := options.filters.scope()
	if err != nil {
		return jobcontrol.Selection{}, false, err
	}
	filter := options.filters.filter()
	filtered := scope.Kinds() > 0 || !filter.Empty() || len(options.states.values) > 0
	if filtered && len(options.names) > 0 {
		return jobcontrol.Selection{}, false, errors.New("--job-name cannot be combined with --stage, --matrix, or --filter-* options")
	}
	return jobcontrol.Selection{Names: options.names, Scope: scope, Filter: filter, States: options.states.values}, filtered, nil
}

// selected reports whether the options choose jobs by name or filter.
func (options *jobControlOptions) selected() (bool, error) {
	selection, filtered, err := options.selection()
	return filtered || len(selection.Names) > 0, err
}

// resolveJobControl resolves the jobs that operation acts on. Without a job
// name or filter it returns target unchanged. Otherwise target must name no
// jobs, and the unfinished jobs of the active run that the options choose
// replace them; a filtered selection is confirmed first, and the confirmed
// jobs are the ones acted on.
func resolveJobControl(target resolve.JobControl, options *jobControlOptions, defaultStates []string, operation string) (resolve.JobControl, error) {
	selection, filtered, err := options.selection()
	if err != nil {
		return target, err
	}
	if !filtered && len(selection.Names) == 0 {
		return target, nil
	}
	if len(target.JobIDs) > 0 {
		return target, errors.New("job IDs cannot be combined with --job-name, --stage, --matrix, or --filter-* options")
	}
	if len(selection.States) == 0 {
		selection.States = defaultStates
	}
	runID, jobIDs, err := jobController().Select(target.BaseDir, target.ProjectName, target.RunID, selection, time.Now())
	if err != nil {
		return target, err
	}
	if filtered {
		if err := confirmJobControl(os.Stdin, os.Stderr, stdinIsTerminal(), *options.yes, operation, runID, jobIDs); err != nil {
			return target, err
		}
	}
	target.RunID, target.JobIDs = runID, jobIDs
	return target, nil
}

// confirmJobControl lists the jobs a filtered selection chose and asks before
// acting on them, unless yes is set. Without a terminal it requires yes.
func confirmJobControl(input io.Reader, output io.Writer, terminal, yes bool, operation, runID string, jobIDs []string) error {
	if yes {
		return nil
	}
	if !terminal {
		return fmt.Errorf("%s with filters needs --yes when stdin is not a terminal", operation)
	}
	fmt.Fprintf(output, "%s %d job(s) of run %s:\n", operation, len(jobIDs), runID)
	for _, jobID := range jobIDs {
		fmt.Fprintf(output, "  %s\n", jobID)
	}
	fmt.Fprint(output, "Continue? [y/N] ")
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && answer == "" {
		return err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("%s cancelled", operation)
	}
	return nil
}

// cmdCancel cancels the active run or selected running jobs.
func cmdCancel(args []string) int {
	fs := flag.NewFlagSet("cancel", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	wait := cliBool(fs, "wait", false)
	control := cliJobControlOptions(fs, cancelStates)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 && len(jobIDs) > 0 {
		printError("usage: " + cliUsage("cancel"))
		return 1
	}
	if len(fs.Args()) > 0 {
		jobIDs = append(jobIDs, fs.Args()...)
	}
	selected, err := control.selected()
	if err != nil {
		printError(err)
		return 1
	}
	if (len(jobIDs) > 0 || selected) && *wait {
		printError("--wait may not be used with a job selection")
		return 1
	}
	target, err := resolveCLIJobSelection(*basedir, *queueNameOption, jobIDs)
	if err != nil {
		printError(err)
		return 1
	}
	target, err = resolveJobControl(target, control, cancelStates, "cancel")
	if err != nil {
		printError(err)
		return 1
	}
	message, err := jobController().Cancel(target.BaseDir, target.ProjectName, target.RunID, target.JobIDs, *wait)
	if err != nil {
		printError(err)
		return 1
	}
	fmt.Println(message)
	return 0
}

// cancelStates are the job states cancel selects; suspend and resume act on
// running jobs only. See jobcontrol.States.
var (
	cancelStates = jobcontrol.States["cancel"]
	signalStates = jobcontrol.States["suspend"]
)

// cmdJobSignal suspends or resumes selected running jobs.
func cmdJobSignal(args []string, operation string) int {
	fs := flag.NewFlagSet(operation, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	control := cliJobControlOptions(fs, signalStates)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 && len(jobIDs) > 0 {
		printError("usage: " + cliUsage(operation))
		return 1
	}
	if len(fs.Args()) > 0 {
		jobIDs = append(jobIDs, fs.Args()...)
	}
	target, err := resolveCLIJobSelection(*basedir, *queueNameOption, jobIDs)
	if err != nil {
		printError(err)
		return 1
	}
	target, err = resolveJobControl(target, control, signalStates, operation)
	if err != nil {
		printError(err)
		return 1
	}
	message, err := jobController().Control(target.BaseDir, target.ProjectName, target.RunID, target.JobIDs, operation)
	if err != nil {
		printError(err)
		return 1
	}
	fmt.Println(message)
	return 0
}

package main

import (
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// filterFlagPrefix starts the name of every option that filters jobs; help
// lists these options under their own heading.
const filterFlagPrefix = "filter-"

var resultFilterValues = []string{"failed", "unfinished", "success"}

// jobFilterFlagSpecs returns the --filter-* options of a command that
// selects jobs by definition, and by result when results is set.
func jobFilterFlagSpecs(results bool) []cliFlagSpec {
	var specs []cliFlagSpec
	if results {
		specs = append(specs, cliFlagSpec{Name: "filter-result", Description: "select jobs with this result; may be repeated; --failed, --unfinished, and --success are short forms", ValueName: "RESULT", Values: resultFilterValues, Repeated: true, CommandLineOnly: true})
	}
	return append(specs,
		cliFlagSpec{Name: "filter-stage", Description: "select jobs in this stage; same as --stage", ValueName: "NAME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-not-stage", Description: "exclude jobs in this stage; may be repeated", ValueName: "NAME", Repeated: true, CommandLineOnly: true},
		cliFlagSpec{Name: "filter-matrix", Description: "select jobs of this matrix, named by its base job name; same as --matrix", ValueName: "NAME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-not-matrix", Description: "exclude jobs of this matrix, named by its base job name; may be repeated", ValueName: "NAME", Repeated: true, CommandLineOnly: true},
	)
}

// jobFilterOptions holds the parsed result filters, scope, and --filter-*
// options of a command.
type jobFilterOptions struct {
	failed, unfinished, success *bool
	results                     cliChoiceList
	stage, filterStage          *string
	matrix, filterMatrix        *string
	notStages, notMatrices      stringSliceFlag
}

// cliJobFilterOptions registers the job filter options on fs: the result
// filters and --filter-result when results is set, and the scope and
// definition filters always.
func cliJobFilterOptions(fs *flag.FlagSet, results bool) *jobFilterOptions {
	options := &jobFilterOptions{}
	if results {
		options.failed = cliBool(fs, "failed", false)
		options.unfinished = cliBool(fs, "unfinished", false)
		options.success = cliBool(fs, "success", false)
		options.results.choices = resultFilterValues
		cliValue(fs, &options.results, "filter-result")
	}
	options.stage = cliString(fs, "stage", "")
	options.filterStage = cliString(fs, "filter-stage", "")
	cliValue(fs, &options.notStages, "filter-not-stage")
	options.matrix = cliString(fs, "matrix", "")
	options.filterMatrix = cliString(fs, "filter-matrix", "")
	cliValue(fs, &options.notMatrices, "filter-not-matrix")
	return options
}

// resultFilters reports which results the result filters select.
func (options *jobFilterOptions) resultFilters() (failed, unfinished, success bool) {
	if options.failed == nil {
		return false, false, false
	}
	return *options.failed || slices.Contains(options.results.values, "failed"),
		*options.unfinished || slices.Contains(options.results.values, "unfinished"),
		*options.success || slices.Contains(options.results.values, "success")
}

// resultSelection returns the result selection; see model.ResultSelection.
func (options *jobFilterOptions) resultSelection() string {
	return model.ResultSelection(options.resultFilters())
}

// scope returns the stage or matrix given by --stage, --matrix, or their
// --filter-* forms.
func (options *jobFilterOptions) scope() (model.CommandSelector, error) {
	stage, err := sameOption("stage", *options.stage, *options.filterStage)
	if err != nil {
		return model.CommandSelector{}, err
	}
	matrix, err := sameOption("matrix", *options.matrix, *options.filterMatrix)
	if err != nil {
		return model.CommandSelector{}, err
	}
	return model.CommandSelector{Stage: stage, Matrix: matrix}, nil
}

// sameOption returns the value of --name or --filter-name, which name one
// value and may both be given only with the same one.
func sameOption(name, short, long string) (string, error) {
	if short != "" && long != "" && short != long {
		return "", fmt.Errorf("--%s %q and --%s%s %q differ", name, short, filterFlagPrefix, name, long)
	}
	if short != "" {
		return short, nil
	}
	return long, nil
}

// filter returns the conditions that narrow the selection further.
func (options *jobFilterOptions) filter() jobfilter.Filter {
	return jobfilter.Filter{NotStages: options.notStages, NotMatrices: options.notMatrices}
}

// cliChoiceList collects repeated values of an option that takes one of
// choices.
type cliChoiceList struct {
	values  []string
	choices []string
}

func (list *cliChoiceList) String() string {
	if list == nil {
		return ""
	}
	return strings.Join(list.values, ",")
}

func (list *cliChoiceList) Set(value string) error {
	if !slices.Contains(list.choices, value) {
		return fmt.Errorf("invalid choice %q (choose from %s)", value, strings.Join(list.choices, ", "))
	}
	list.values = append(list.values, value)
	return nil
}

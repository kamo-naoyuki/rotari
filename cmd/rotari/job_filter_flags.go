package main

import (
	"flag"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
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
		cliFlagSpec{Name: "filter-exit-code", Description: "select jobs with this exit code; may be repeated", ValueName: "N", Repeated: true, CommandLineOnly: true},
		cliFlagSpec{Name: "filter-failure-kind", Description: "select jobs of this failure kind; may be repeated; valid values: timeout, cancelled, blocked, oom, signal, error", ValueName: "KIND", Values: jobstatus.FailureKindValues(), Repeated: true, CommandLineOnly: true},
		cliFlagSpec{Name: "filter-diagnosis", Description: "select failed jobs matching a current diagnosis rule; may be repeated", ValueName: "VALUE", Repeated: true, CommandLineOnly: true},
		cliFlagSpec{Name: "filter-host", Description: "select jobs run on a matching host; may be repeated", ValueName: "PATTERN", Repeated: true, CommandLineOnly: true},
		cliFlagSpec{Name: "filter-started-after", Description: "select jobs started at or after this time", ValueName: "TIME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-started-before", Description: "select jobs started before this time", ValueName: "TIME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-finished-after", Description: "select jobs finished at or after this time", ValueName: "TIME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-finished-before", Description: "select jobs finished before this time", ValueName: "TIME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-longer-than", Description: "select jobs running at least this long", ValueName: "DURATION", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-shorter-than", Description: "select jobs running less than this long", ValueName: "DURATION", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-command", Description: "select jobs whose argv matches this Go regexp", ValueName: "RE", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-stage", Description: "select jobs in this stage; same as --stage", ValueName: "NAME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-not-stage", Description: "exclude jobs in this stage; may be repeated", ValueName: "NAME", Repeated: true, CommandLineOnly: true},
		cliFlagSpec{Name: "filter-matrix", Description: "select jobs of this matrix, named by its base job name; same as --matrix", ValueName: "NAME", CommandLineOnly: true},
		cliFlagSpec{Name: "filter-not-matrix", Description: "exclude jobs of this matrix, named by its base job name; may be repeated", ValueName: "NAME", Repeated: true, CommandLineOnly: true},
	)
}

// jobFilterOptions holds the parsed result filters, scope, and --filter-*
// options of a command.
type jobFilterOptions struct {
	failed, unfinished, success   *bool
	results                       cliChoiceList
	stage, filterStage            *string
	matrix, filterMatrix          *string
	command                       *string
	exitCodes                     *intSliceFlag
	failureKinds                  *failureKindFlag
	diagnoses                     diagnosisFlag
	hosts                         stringSliceFlag
	startedAfter, startedBefore   *timeFlag
	finishedAfter, finishedBefore *timeFlag
	longerThan, shorterThan       *durationFlag
	notStages, notMatrices        stringSliceFlag
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
	options.command = new(string)
	fs.Var(&regexFlag{value: options.command}, "filter-command", "select jobs whose argv matches this Go regexp")
	options.exitCodes = new(intSliceFlag)
	fs.Var(options.exitCodes, "filter-exit-code", "select jobs with this exit code; may be repeated")
	options.failureKinds = new(failureKindFlag)
	fs.Var(options.failureKinds, "filter-failure-kind", "select jobs of this failure kind; may be repeated")
	cliValue(fs, &options.diagnoses, "filter-diagnosis")
	cliValue(fs, &options.hosts, "filter-host")
	options.startedAfter = new(timeFlag)
	fs.Var(options.startedAfter, "filter-started-after", "select jobs started at or after this time")
	options.startedBefore = new(timeFlag)
	fs.Var(options.startedBefore, "filter-started-before", "select jobs started before this time")
	options.finishedAfter = new(timeFlag)
	fs.Var(options.finishedAfter, "filter-finished-after", "select jobs finished at or after this time")
	options.finishedBefore = new(timeFlag)
	fs.Var(options.finishedBefore, "filter-finished-before", "select jobs finished before this time")
	options.longerThan = new(durationFlag)
	fs.Var(options.longerThan, "filter-longer-than", "select jobs running at least this long")
	options.shorterThan = new(durationFlag)
	fs.Var(options.shorterThan, "filter-shorter-than", "select jobs running less than this long")
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
	pattern := ""
	if options.command != nil {
		pattern = *options.command
	}
	var exitCodes []int
	if options.exitCodes != nil {
		exitCodes = append([]int(nil), (*options.exitCodes)...)
	}
	var failureKinds []string
	if options.failureKinds != nil {
		failureKinds = append([]string(nil), (*options.failureKinds)...)
	}
	filter := jobfilter.Filter{NotStages: options.notStages, NotMatrices: options.notMatrices, Command: pattern, ExitCodes: exitCodes, FailureKinds: failureKinds, Diagnoses: append([]string(nil), options.diagnoses...), Hosts: append([]string(nil), options.hosts...)}
	if options.startedAfter.set {
		filter.StartedAfter = &options.startedAfter.value
	}
	if options.startedBefore.set {
		filter.StartedBefore = &options.startedBefore.value
	}
	if options.finishedAfter.set {
		filter.FinishedAfter = &options.finishedAfter.value
	}
	if options.finishedBefore.set {
		filter.FinishedBefore = &options.finishedBefore.value
	}
	if options.longerThan.set {
		filter.LongerThan = options.longerThan.value
	}
	if options.shorterThan.set {
		filter.ShorterThan = options.shorterThan.value
	}
	return filter
}

type intSliceFlag []int

func (flag *intSliceFlag) String() string {
	if flag == nil {
		return ""
	}
	values := make([]string, len(*flag))
	for i, value := range *flag {
		values[i] = strconv.Itoa(value)
	}
	return strings.Join(values, ",")
}

func (flag *intSliceFlag) Set(value string) error {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid integer %q: %w", value, err)
	}
	*flag = append(*flag, parsed)
	return nil
}

type failureKindFlag []string

func (flag *failureKindFlag) String() string {
	if flag == nil {
		return ""
	}
	return strings.Join(*flag, ",")
}

type diagnosisFlag []string

func (flag *diagnosisFlag) String() string {
	if flag == nil {
		return ""
	}
	return strings.Join(*flag, ",")
}

func (flag *diagnosisFlag) Set(value string) error {
	if err := diagnose.ResolveRuleSelectors([]string{value}); err != nil {
		return err
	}
	*flag = append(*flag, value)
	return nil
}

type timeFlag struct {
	value time.Time
	set   bool
}

func (flag *timeFlag) String() string {
	if flag == nil || !flag.set {
		return ""
	}
	return flag.value.Format(time.RFC3339)
}

func (flag *timeFlag) Set(value string) error {
	parsed, err := parseFilterTime(value, time.Now())
	if err != nil {
		return err
	}
	flag.value = parsed
	flag.set = true
	return nil
}

type durationFlag struct {
	value time.Duration
	set   bool
}

func (flag *durationFlag) String() string {
	if flag == nil || !flag.set {
		return ""
	}
	return flag.value.String()
}

func (flag *durationFlag) Set(value string) error {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		if err == nil {
			err = fmt.Errorf("duration must be positive")
		}
		return fmt.Errorf("invalid duration %q: %w", value, err)
	}
	flag.value = parsed
	flag.set = true
	return nil
}

func parseFilterTime(value string, now time.Time) (time.Time, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.ParseFloat(strings.TrimSuffix(value, "d"), 64)
		if err != nil || days < 0 {
			return time.Time{}, fmt.Errorf("invalid relative time %q", value)
		}
		return now.Add(-time.Duration(days * float64(24*time.Hour))), nil
	}
	for _, layout := range []string{time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", value)
}

func (flag *failureKindFlag) Set(value string) error {
	valid := jobstatus.FailureKindValues()
	if !slices.Contains(valid, value) {
		return fmt.Errorf("invalid choice %q (choose from %s)", value, strings.Join(valid, ", "))
	}
	*flag = append(*flag, value)
	return nil
}

type regexFlag struct {
	value *string
}

func (flag *regexFlag) String() string {
	if flag == nil || flag.value == nil {
		return ""
	}
	return *flag.value
}

func (flag *regexFlag) Set(value string) error {
	if _, err := regexp.Compile(value); err != nil {
		return err
	}
	if flag.value != nil {
		*flag.value = value
	}
	return nil
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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlist"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdBasedirs lists state directories registered with rotari.
func cmdBasedirs(args []string) int {
	fs := flag.NewFlagSet("basedirs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	masterdir := cliString(fs, "masterdir", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("basedirs"))
		return 1
	}
	resolvedMasterDir, err := state.ResolveMasterDir(*masterdir)
	if err != nil {
		printErrorf("failed to resolve master directory: %v", err)
		return 1
	}
	return showBaseDirs(resolvedMasterDir)
}

// cmdProjects lists project overviews across the known state directories, or
// in one explicitly selected directory.
func cmdProjects(args []string) int {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	masterdir := cliString(fs, "masterdir", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("projects"))
		return 1
	}
	requestedBaseDir := ""
	if cliOptionSet(fs, "basedir") {
		requestedBaseDir = *basedir
	}
	return showAllProjects(requestedBaseDir, *masterdir)
}

// runListRow is one line of the `runs` table.
type runListRow struct {
	BaseDir   string
	Project   string
	RunID     string
	Name      string
	Status    string
	Client    string
	ExitCode  string
	StartedAt string
	Finished  string
}

func newRunListRow(run runlist.Run) runListRow {
	row := runListRow{
		BaseDir: run.BaseDir, Project: run.Project, RunID: run.RunID,
		Name: firstNonEmpty(run.RunName, "-"), Status: run.Lifecycle, Client: run.ClientLabel,
		ExitCode: "-", StartedAt: displayRunTime(run.StartedAt), Finished: displayRunTime(run.FinishedAt),
	}
	if run.ExitCode != nil {
		row.ExitCode = strconv.Itoa(*run.ExitCode)
	}
	return row
}

func displayRunTime(value string) string {
	if value == "" {
		return "-"
	}
	return model.FormatDisplayTimestamp(value)
}

var runLifecycleFilterValues = []string{"running", "interrupted", "incomplete", "finished", "failed", "cancelled"}

func runLifecycleFilterSpecs() []cliFlagSpec {
	specs := make([]cliFlagSpec, 0, len(runLifecycleFilterValues)*2)
	for _, lifecycle := range runLifecycleFilterValues {
		specs = append(specs,
			cliFlagSpec{Name: "filter-" + lifecycle, Description: "select runs in the " + lifecycle + " lifecycle", CommandLineOnly: true},
			cliFlagSpec{Name: lifecycle, Description: "short form of --filter-" + lifecycle, CommandLineOnly: true},
		)
	}
	return specs
}

type runLifecycleOptions struct {
	flags map[string]runLifecycleFlagPair
}

type runLifecycleFlagPair struct {
	filter *bool
	short  *bool
}

func cliRunLifecycleOptions(fs *flag.FlagSet) runLifecycleOptions {
	options := runLifecycleOptions{flags: make(map[string]runLifecycleFlagPair, len(runLifecycleFilterValues))}
	for _, lifecycle := range runLifecycleFilterValues {
		options.flags[lifecycle] = runLifecycleFlagPair{
			filter: cliBool(fs, "filter-"+lifecycle, false),
			short:  cliBool(fs, lifecycle, false),
		}
	}
	return options
}

func (options runLifecycleOptions) selected(lifecycle string) bool {
	flags := options.flags[lifecycle]
	return flags.filter != nil && *flags.filter || flags.short != nil && *flags.short
}

func (options runLifecycleOptions) anySelected() bool {
	for _, lifecycle := range runLifecycleFilterValues {
		if options.selected(lifecycle) {
			return true
		}
	}
	return false
}

func filterRunsByLifecycle(runs []runlist.Run, options runLifecycleOptions) []runlist.Run {
	if !options.anySelected() {
		return runs
	}
	filtered := make([]runlist.Run, 0, len(runs))
	for _, run := range runs {
		if options.selected(run.Lifecycle) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

// runListJSON is one run of `runs --json`, named like `show --json`.
type runListJSON struct {
	BaseDir      string                `json:"base_dir"`
	Project      string                `json:"project_name"`
	RunID        string                `json:"run_id"`
	RunName      string                `json:"run_name,omitempty"`
	Lifecycle    string                `json:"lifecycle"`
	ClientStatus model.RunClientStatus `json:"client_status"`
	ClientLabel  string                `json:"client_label"`
	ExitCode     *int                  `json:"exit_code"`
	StartedAt    string                `json:"started_at,omitempty"`
	FinishedAt   string                `json:"finished_at,omitempty"`
	// Where the run ran, which code it ran, and its own notes.
	CWD       string   `json:"cwd,omitempty"`
	Host      string   `json:"host,omitempty"`
	Sources   []string `json:"sources,omitempty"`
	NoteCount int      `json:"note_count"`
	// StartedBy is how the run was started, absent for older runs.
	StartedBy *model.StartedBy `json:"started_by,omitempty"`
}

// cmdRuns lists active and interrupted runs plus recently finished runs across
// all known state directories. --basedir limits the listing to one directory.
func cmdRuns(args []string) int {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	masterdir := cliString(fs, "masterdir", "")
	since := cliString(fs, "since", joblist.DefaultSinceText)
	jsonOutput := cliBool(fs, "json", false)
	lifecycles := cliRunLifecycleOptions(fs)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "project-name")) {
		printError("usage: " + cliUsage("runs"))
		return 1
	}
	window, err := joblist.ParseSince(*since)
	if err != nil {
		printErrorf("invalid --since duration %q", *since)
		return 1
	}
	projectFilter := ""
	if cliOptionSet(fs, "project-name") {
		projectFilter = *projectName
	}
	if len(fs.Args()) == 1 {
		projectFilter = fs.Args()[0]
	}
	if (cliOptionSet(fs, "project-name") || len(fs.Args()) == 1) && !state.IsValidPathElement(projectFilter) {
		printErrorf("invalid project name %q", projectFilter)
		return 1
	}
	baseDirs, err := jobsBaseDirs(valueIfSet(fs, "basedir", *basedir), *masterdir)
	if err != nil {
		printError(err)
		return 1
	}
	runs, err := runlist.Collect(baseDirs, projectFilter)
	if err != nil {
		printError(err)
		return 1
	}
	runs = filterRunsByLifecycle(runlist.Recent(runs, time.Now().Add(-window)), lifecycles)
	if *jsonOutput {
		return printRunRowsJSON(runs)
	}
	if len(runs) == 0 {
		if lifecycles.anySelected() {
			fmt.Printf("No runs matched the requested lifecycle filters in %s (using --since %s).\n", runlist.DescribeScope(baseDirs, projectFilter), *since)
		} else {
			fmt.Printf("No active or interrupted runs, or runs finished within %s, found in %s.\n", *since, runlist.DescribeScope(baseDirs, projectFilter))
		}
		return 0
	}
	rows := make([]runListRow, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, newRunListRow(run))
	}
	printRunRows(rows, len(baseDirs) > 1)
	return 0
}

func valueIfSet(fs *flag.FlagSet, name, value string) string {
	if cliOptionSet(fs, name) {
		return value
	}
	return ""
}

func printRunRowsJSON(runs []runlist.Run) int {
	rows := make([]runListJSON, 0, len(runs))
	for _, run := range runs {
		details, err := runlist.ReadDetails(run)
		if err != nil {
			printError(err)
			return 1
		}
		rows = append(rows, runListJSON{
			BaseDir: run.BaseDir, Project: run.Project, RunID: run.RunID, RunName: run.RunName,
			Lifecycle: run.Lifecycle, ClientStatus: run.ClientStatus, ClientLabel: run.ClientLabel,
			ExitCode: run.ExitCode, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
			CWD: details.WorkingDirectory, Host: details.Host, Sources: details.Sources, NoteCount: details.NoteCount,
			StartedBy: details.StartedBy,
		})
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(rows); err != nil {
		printError(err)
		return 1
	}
	return 0
}

func printRunRows(rows []runListRow, showBaseDir bool) {
	table := textTable{}
	if showBaseDir {
		table.columns = append(table.columns, tableColumn{header: "BASEDIR"})
	}
	for _, header := range []string{"PROJECT", "RUN ID", "NAME", "STATUS", "CLIENT", "EXIT CODE", "STARTED", "FINISHED"} {
		column := tableColumn{header: header}
		if header == "STATUS" {
			column.color = func(_ int, value string) string {
				switch value {
				case "finished", "success":
					return green(value)
				case "failed":
					return red(value)
				}
				return yellow(value)
			}
		}
		table.columns = append(table.columns, column)
	}
	for _, row := range rows {
		values := []string{row.Project, row.RunID, row.Name, row.Status, row.Client, row.ExitCode, row.StartedAt, row.Finished}
		if showBaseDir {
			values = append([]string{row.BaseDir}, values...)
		}
		table.rows = append(table.rows, values)
	}
	table.write(os.Stdout)
	fmt.Println("\nTo inspect a run:")
	fmt.Println("  rotari show -r RUN_ID")
}

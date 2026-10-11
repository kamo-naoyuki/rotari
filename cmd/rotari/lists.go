package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	runs = runlist.Recent(runs, time.Now().Add(-window))
	if *jsonOutput {
		return printRunRowsJSON(runs)
	}
	if len(runs) == 0 {
		fmt.Printf("No active or interrupted runs, or runs finished within %s, found in %s.\n", *since, runlist.DescribeScope(baseDirs, projectFilter))
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
		rows = append(rows, runListJSON{
			BaseDir: run.BaseDir, Project: run.Project, RunID: run.RunID, RunName: run.RunName,
			Lifecycle: run.Lifecycle, ClientStatus: run.ClientStatus, ClientLabel: run.ClientLabel,
			ExitCode: run.ExitCode, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
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
	header := []string{"PROJECT", "RUN ID", "NAME", "STATUS", "CLIENT", "EXIT CODE", "STARTED", "FINISHED"}
	if showBaseDir {
		header = append([]string{"BASEDIR"}, header...)
	}
	table := [][]string{header}
	widths := make([]int, len(header))
	for _, row := range rows {
		values := []string{row.Project, row.RunID, row.Name, row.Status, row.Client, row.ExitCode, row.StartedAt, row.Finished}
		if showBaseDir {
			values = append([]string{row.BaseDir}, values...)
		}
		table = append(table, values)
	}
	for _, values := range table {
		for index, value := range values {
			widths[index] = max(widths[index], len(value))
		}
	}
	statusColumn := 3
	if showBaseDir {
		statusColumn++
	}
	for rowIndex, values := range table {
		if rowIndex == 0 {
			fmt.Println(cyan(formatJobsRow(values, widths, make([]byte, len(values)))))
			continue
		}
		// Pad the plain text first: ANSI sequences must not affect widths.
		padded := make([]string, len(values))
		for index, value := range values {
			padded[index] = value
			if index < len(values)-1 {
				padded[index] += strings.Repeat(" ", widths[index]-len(value))
			}
		}
		switch values[statusColumn] {
		case "finished", "success":
			padded[statusColumn] = green(padded[statusColumn])
		case "failed":
			padded[statusColumn] = red(padded[statusColumn])
		default:
			padded[statusColumn] = yellow(padded[statusColumn])
		}
		fmt.Println(strings.Join(padded, "  "))
	}
	fmt.Println("\nTo inspect a run:")
	fmt.Println("  rotari show -r RUN_ID")
}

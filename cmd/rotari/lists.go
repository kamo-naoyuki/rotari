package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runview"
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

type runListRow struct {
	BaseDir    string
	Project    string
	RunID      string
	Name       string
	Status     string
	Client     string
	ExitCode   string
	StartedAt  string
	Finished   string
	FinishedAt time.Time
	Order      int64
	// The recorded values behind the display columns, for --json.
	runName      string
	exitCode     *int
	startedAt    string
	finishedAt   string
	clientStatus model.RunClientStatus
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
	rows, err := collectRunRows(baseDirs, projectFilter)
	if err != nil {
		printError(err)
		return 1
	}
	rows = filterRunRows(rows, time.Now().Add(-window))
	if *jsonOutput {
		return printRunRowsJSON(rows)
	}
	if len(rows) == 0 {
		fmt.Printf("No active or interrupted runs, or runs finished within %s, found in %s.\n", *since, describeRunScope(baseDirs, projectFilter))
		return 0
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

func collectRunRows(baseDirs []string, projectFilter string) ([]runListRow, error) {
	if projectFilter != "" && !state.IsValidPathElement(projectFilter) {
		return nil, fmt.Errorf("invalid project name %q", projectFilter)
	}
	rows := make([]runListRow, 0)
	hasProjects, foundProject := false, false
	for _, baseDir := range baseDirs {
		entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list projects in %q: %w", baseDir, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || !state.IsValidPathElement(entry.Name()) {
				continue
			}
			hasProjects = true
			if projectFilter != "" && entry.Name() != projectFilter {
				continue
			}
			foundProject = true
			projectRows, err := collectProjectRuns(baseDir, entry.Name())
			if err != nil {
				return nil, err
			}
			rows = append(rows, projectRows...)
		}
	}
	// Keep the empty-state-directory result, but distinguish a misspelled
	// selector from an existing project that simply has no runs yet.
	if projectFilter != "" && hasProjects && !foundProject {
		return nil, fmt.Errorf("project %q not found in %s", projectFilter, describeRunScope(baseDirs, ""))
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Order != rows[j].Order {
			return rows[i].Order > rows[j].Order
		}
		if rows[i].BaseDir != rows[j].BaseDir {
			return rows[i].BaseDir < rows[j].BaseDir
		}
		if rows[i].Project != rows[j].Project {
			return rows[i].Project < rows[j].Project
		}
		return rows[i].RunID < rows[j].RunID
	})
	return rows, nil
}

func collectProjectRuns(baseDir, projectName string) ([]runListRow, error) {
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project %q: %w", projectName, err)
	}
	entries, err := os.ReadDir(paths.RunsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read runs for project %q: %w", projectName, err)
	}
	rows := make([]runListRow, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		row, err := readRunRow(paths, projectName, entry)
		if err != nil {
			return nil, fmt.Errorf("failed to read run %q for project %q: %w", entry.Name(), projectName, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func readRunRow(paths state.ProjectPaths, projectName string, entry os.DirEntry) (runListRow, error) {
	runID := entry.Name()
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return runListRow{}, err
	}
	info, _ := entry.Info()
	order := int64(0)
	if info != nil {
		order = info.ModTime().UnixNano()
	}
	row := runListRow{BaseDir: paths.BaseDir, Project: projectName, RunID: runID, Name: "-", Status: "incomplete", ExitCode: "-", StartedAt: "-", Finished: "-", Order: order}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil && !os.IsNotExist(err) {
		return runListRow{}, err
	}
	if err == nil {
		row.Name = firstNonEmpty(summary.RunName, "-")
		row.Status = firstNonEmpty(summary.Status, model.RunStatus(summary.ExitCode))
		row.ExitCode = strconv.Itoa(summary.ExitCode)
		row.runName, row.exitCode = summary.RunName, &summary.ExitCode
		row.startedAt, row.finishedAt = summary.StartedAt, summary.FinishedAt
		row.StartedAt = model.FormatDisplayTimestamp(summary.StartedAt)
		row.Finished = model.FormatDisplayTimestamp(summary.FinishedAt)
		row.FinishedAt, _ = time.Parse(time.RFC3339Nano, summary.FinishedAt)
		if started, err := time.Parse(time.RFC3339Nano, summary.StartedAt); err == nil {
			row.Order = started.UnixNano()
		}
	} else if lock, lockErr := state.LoadLock(paths.LockFile); lockErr == nil && lock.RunID == runID {
		// An active or interrupted run has no summary yet; its lock records
		// the name and start time.
		row.Name = firstNonEmpty(lock.RunName, "-")
		row.runName = lock.RunName
		row.startedAt = lock.StartedAt
		if started, err := time.Parse(time.RFC3339Nano, lock.StartedAt); err == nil {
			row.StartedAt = model.FormatDisplayTimestamp(lock.StartedAt)
			row.Order = started.UnixNano()
		}
	}
	lifecycle, err := runview.RunLifecycleLabel(paths, runID)
	if err != nil {
		return runListRow{}, err
	}
	row.Status = lifecycle
	clientStatus, err := runview.ClientStatus(paths, runID)
	if err != nil {
		clientStatus = model.RunClientStatus{State: "unknown"}
	}
	row.clientStatus = clientStatus
	row.Client = runview.ClientStatusLabel(clientStatus)
	return row, nil
}

func printRunRowsJSON(rows []runListRow) int {
	runs := make([]runListJSON, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, runListJSON{
			BaseDir: row.BaseDir, Project: row.Project, RunID: row.RunID, RunName: row.runName,
			Lifecycle: row.Status, ClientStatus: row.clientStatus, ClientLabel: row.Client,
			ExitCode: row.exitCode, StartedAt: row.startedAt, FinishedAt: row.finishedAt,
		})
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(runs); err != nil {
		printError(err)
		return 1
	}
	return 0
}

func describeRunScope(baseDirs []string, projectFilter string) string {
	scope := fmt.Sprintf("%d state directories", len(baseDirs))
	if len(baseDirs) == 1 {
		scope = "state directory " + baseDirs[0]
	}
	if projectFilter != "" {
		scope = fmt.Sprintf("project %q in %s", projectFilter, scope)
	}
	return scope
}

func filterRunRows(rows []runListRow, cutoff time.Time) []runListRow {
	filtered := make([]runListRow, 0, len(rows))
	for _, row := range rows {
		if row.Status == "running" || row.Status == "interrupted" || row.Status == "incomplete" || row.FinishedAt.IsZero() || !row.FinishedAt.Before(cutoff) {
			filtered = append(filtered, row)
		}
	}
	return filtered
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

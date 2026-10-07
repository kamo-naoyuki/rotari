package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
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
	BaseDir   string
	Project   string
	RunID     string
	Name      string
	Status    string
	ExitCode  string
	StartedAt string
	Finished  string
	Order     int64
}

// cmdRuns lists saved and active runs in the default state directory. Use
// --all-basedirs to include every state directory known to the master registry.
func cmdRuns(args []string) int {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	masterdir := cliString(fs, "masterdir", "")
	allBaseDirs := cliBool(fs, "all-basedirs", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "project-name")) {
		printError("usage: " + cliUsage("runs"))
		return 1
	}
	if *allBaseDirs && cliOptionSet(fs, "basedir") {
		printError("--all-basedirs cannot be combined with --basedir")
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
	baseDirs, err := jobsBaseDirs(valueIfSet(fs, "basedir", *basedir), *masterdir, *allBaseDirs)
	if err != nil {
		printError(err)
		return 1
	}
	rows, err := collectRunRows(baseDirs, projectFilter)
	if err != nil {
		printError(err)
		return 1
	}
	if len(rows) == 0 {
		fmt.Printf("No runs found in %s.\n", describeRunScope(baseDirs, projectFilter))
		if !*allBaseDirs {
			fmt.Println("To include every registered state directory:")
			fmt.Println("  rotari runs --all-basedirs")
		}
		return 0
	}
	printRunRows(rows)
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
		row.StartedAt = model.FormatDisplayTimestamp(summary.StartedAt)
		row.Finished = model.FormatDisplayTimestamp(summary.FinishedAt)
		if started, err := time.Parse(time.RFC3339Nano, summary.StartedAt); err == nil {
			row.Order = started.UnixNano()
		}
	}
	phase, err := project.RunPhaseOf(paths, runID)
	if err != nil {
		return runListRow{}, err
	}
	if phase == project.RunPhaseRunning || phase == project.RunPhaseInterrupted {
		row.Status = string(phase)
	}
	return row, nil
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

func printRunRows(rows []runListRow) {
	showBaseDir := false
	for _, row := range rows {
		showBaseDir = showBaseDir || row.BaseDir != rows[0].BaseDir
	}
	header := []string{"PROJECT", "RUN ID", "NAME", "STATUS", "EXIT CODE", "STARTED", "FINISHED"}
	if showBaseDir {
		header = append([]string{"BASEDIR"}, header...)
	}
	table := [][]string{header}
	widths := make([]int, len(header))
	for _, row := range rows {
		values := []string{row.Project, row.RunID, row.Name, row.Status, row.ExitCode, row.StartedAt, row.Finished}
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
	fmt.Println("  rotari show --basedir BASEDIR --project-name PROJECT --run-id RUN_ID")
	fmt.Println("With a registered run, you can also use: rotari show RUN_ID")
}

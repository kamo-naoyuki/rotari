package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/jobfilter"
	"github.com/rotari-dev/rotari/internal/joblist"
	"github.com/rotari-dev/rotari/internal/jobstatus"
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/resolve"
	"github.com/rotari-dev/rotari/internal/state"
)

const defaultJobsFormat = "%s %p %a %n %c %f %e"
const basedirJobsFormat = "%s %b %p %a %n %c %f %e"

type jobsColumn struct {
	header string
	code   byte
	width  int
}

// cmdJobs lists historical jobs across runs for the selected project, with
// filters for status, command metadata, and time windows.
func cmdJobs(args []string) int {
	fs := flag.NewFlagSet("jobs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	masterdir := cliString(fs, "masterdir", "")
	format := cliString(fs, "format", defaultJobsFormat)
	since := cliString(fs, "since", joblist.DefaultSinceText)
	results := cliJobResultOptions(fs)
	states := cliJobsStateOptions(fs)
	jsonOutput := cliBool(fs, "json", false)
	wide := cliBool(fs, "wide", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if *jsonOutput && cliOptionSet(fs, "format") {
		printError("--format cannot be combined with --json")
		return 1
	}
	customFormat := cliOptionSet(fs, "format") || configString("format", "") != ""
	if *wide && (*jsonOutput || customFormat) {
		printError("--wide prints the default fields; it cannot be combined with --json, --format, or a configured format")
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "project-name")) {
		printError("usage: " + cliUsage("jobs"))
		return 1
	}
	projectFilter := ""
	if cliOptionSet(fs, "project-name") {
		projectFilter = *projectName
	}
	if len(fs.Args()) == 1 {
		projectFilter = fs.Args()[0]
	}
	columns, err := parseJobsFormat(*format)
	if err != nil {
		printErrorf("invalid --format: %v", err)
		return 1
	}
	window, err := joblist.ParseSince(*since)
	if err != nil {
		printErrorf("invalid --since duration %q", *since)
		return 1
	}
	requestedBaseDir := valueIfSet(fs, "basedir", *basedir)
	baseDirs, err := jobsBaseDirs(requestedBaseDir, *masterdir)
	if err != nil {
		printError(err)
		return 1
	}
	if projectFilter != "" {
		found := false
		for _, baseDir := range baseDirs {
			found = found || resolve.ProjectExists(baseDir, projectFilter)
		}
		if !found {
			message := fmt.Sprintf("project %q does not exist in the listed state directories", projectFilter)
			if elsewhere := resolve.RegisteredProjectBaseDirs(projectFilter, ""); len(elsewhere) > 0 {
				verb := "have"
				if len(elsewhere) == 1 {
					verb = "has"
				}
				message += fmt.Sprintf("; %d registered state director%s %s it (select one with --basedir, or inspect the registry with rotari basedirs)", len(elsewhere), pluralSuffix(len(elsewhere)), verb)
			}
			printError(message)
			return 1
		}
	}
	rows, err := collectJobsAcrossBaseDirs(baseDirs, projectFilter, time.Now(), window)
	if err != nil {
		printError(err)
		return 1
	}
	rows = filterJobs(rows, results.selection(), states)
	joblist.Sort(rows)
	if *jsonOutput {
		return printJobsJSON(rows)
	}
	if len(rows) == 0 {
		// Name where rotari looked, so an empty result is not mistaken for
		// no activity in other state directories.
		scope := fmt.Sprintf("%d state directories", len(baseDirs))
		if len(baseDirs) == 1 {
			scope = "state directory " + baseDirs[0]
		}
		if projectFilter != "" {
			scope = fmt.Sprintf("project %q in %s", projectFilter, scope)
		}
		if results.selection() != "" || states.anySelected() {
			fmt.Printf("No jobs matched the requested filters in %s (finished within %s).\n", scope, *since)
		} else {
			fmt.Printf("No unfinished or recently finished jobs found in %s (finished within %s).\n", scope, *since)
		}
		return 0
	}
	if !customFormat && len(baseDirs) > 1 {
		columns, _ = parseJobsFormat(basedirJobsFormat)
	}
	printJobsTableFormat(rows, columns, !customFormat, *wide)
	return 0
}

func filterJobs(rows []joblist.Row, resultSelection string, states jobsStateOptions) []joblist.Row {
	filtered := make([]joblist.Row, 0, len(rows))
	filter := jobfilter.Filter{}
	filterStates := states.anySelected()
	for _, row := range rows {
		state := normalizedJobsState(row.State)
		if filterStates && !states.selected(state) {
			continue
		}
		if resultSelection != "" {
			result := model.JobResult{}
			finished := false
			switch state {
			case model.StatusSuccess:
				finished = true
			case model.StatusFailed, model.StatusCancelled, "blocked":
				finished, result.ExitCode = true, 1
			}
			if !filter.Selects(resultSelection, jobfilter.Job{Result: result, Finished: finished}) {
				continue
			}
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func normalizedJobsState(value string) string {
	for _, annotation := range []string{" (recorded)", " (carried)", " (accepted)"} {
		value = strings.TrimSuffix(value, annotation)
	}
	return value
}

// jobsJSON is one job of `jobs --json`, named like `runs --json`.
type jobsJSON struct {
	BaseDir        string   `json:"base_dir"`
	Project        string   `json:"project_name"`
	RunID          string   `json:"run_id"`
	JobID          string   `json:"job_id"`
	AttemptID      string   `json:"attempt_id,omitempty"`
	JobName        string   `json:"job_name,omitempty"`
	State          string   `json:"state"`
	Command        string   `json:"command"`
	StartedAt      string   `json:"started_at,omitempty"`
	FinishedAt     string   `json:"finished_at,omitempty"`
	ElapsedSeconds *float64 `json:"elapsed_seconds"`
	QuietSeconds   *float64 `json:"quiet_seconds,omitempty"`
}

func printJobsJSON(rows []joblist.Row) int {
	jobs := make([]jobsJSON, 0, len(rows))
	for _, row := range rows {
		job := jobsJSON{BaseDir: row.BaseDir, Project: row.Project, RunID: row.RunID, JobID: row.JobID, AttemptID: row.AttemptID, State: row.State, Command: row.FullCommand}
		if row.JobName != "-" {
			job.JobName = row.JobName
		}
		if !row.StartedAt.IsZero() {
			job.StartedAt = row.StartedAt.Format(time.RFC3339Nano)
		}
		if !row.FinishedAt.IsZero() {
			job.FinishedAt = row.FinishedAt.Format(time.RFC3339Nano)
		}
		if row.Elapsed >= 0 {
			seconds := row.Elapsed.Seconds()
			job.ElapsedSeconds = &seconds
		}
		if row.Running && row.Quiet >= 0 {
			seconds := row.Quiet.Seconds()
			job.QuietSeconds = &seconds
		}
		jobs = append(jobs, job)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(jobs); err != nil {
		printError(err)
		return 1
	}
	return 0
}

func printJobsTable(rows []joblist.Row, showBaseDir bool) {
	format := defaultJobsFormat
	if showBaseDir {
		format = "%s %b %p %a %n %c %f %e"
	}
	columns, _ := parseJobsFormat(format)
	printJobsTableFormat(rows, columns, true, false)
}

// printJobsTableFormat prints rows with formatColumns. Unless wide, the
// default fields fold a project or state directory that every job shares,
// leave out job names, elapsed times, and the time zone, and drop the
// "(recorded)" that every unfinished state carries.
func printJobsTableFormat(rows []joblist.Row, formatColumns []jobsColumn, defaultFields, wide bool) {
	table := textTable{wide: wide, rowNoun: "listed jobs"}
	for _, column := range formatColumns {
		tableColumn := tableColumn{header: column.header}
		switch column.code {
		case 's':
			tableColumn.color = func(_ int, value string) string { return colorJobsState(value) }
		case 'b':
			if defaultFields {
				tableColumn.fold = "State directory"
			}
		case 'p':
			if defaultFields {
				tableColumn.fold = "Project"
			}
		case 'n', 'e':
			tableColumn.wideOnly = defaultFields
		}
		table.columns = append(table.columns, tableColumn)
	}
	compact := defaultFields && !wide
	for _, row := range rows {
		values := make([]string, len(formatColumns))
		for index, column := range formatColumns {
			values[index] = jobsColumnValue(column.code, row, wide, compact)
			if column.width > 0 {
				values[index] = joblist.ShortenText(values[index], column.width)
			}
		}
		table.rows = append(table.rows, values)
	}
	table.write(os.Stdout)
}

func parseJobsFormat(format string) ([]jobsColumn, error) {
	fields := strings.Fields(format)
	if len(fields) == 0 {
		return nil, fmt.Errorf("format must contain at least one field")
	}
	columns := make([]jobsColumn, 0, len(fields))
	for _, field := range fields {
		if len(field) < 2 || field[0] != '%' {
			return nil, fmt.Errorf("invalid field %q", field)
		}
		position := 1
		width := 0
		if position < len(field) && field[position] == '.' {
			position++
			start := position
			for position < len(field) && field[position] >= '0' && field[position] <= '9' {
				width = width*10 + int(field[position]-'0')
				position++
			}
			if start == position || width == 0 {
				return nil, fmt.Errorf("invalid width in field %q", field)
			}
		}
		if position+1 != len(field) {
			return nil, fmt.Errorf("invalid field %q", field)
		}
		code := field[position]
		header := map[byte]string{'s': "STATE", 'b': "BASEDIR", 'p': "PROJECT", 'a': "ATTEMPT_ID", 'n': "JOB_NAME", 'c': "COMMAND", 't': "STARTED", 'f': "FINISHED", 'e': "ELAPSED"}[code]
		if header == "" {
			return nil, fmt.Errorf("unknown field %q", field)
		}
		columns = append(columns, jobsColumn{header: header, code: code, width: width})
	}
	return columns, nil
}

// jobsColumnValue returns a row's field; wide keeps the state directory and
// command whole, and compact drops the recorded mark and the time zone.
func jobsColumnValue(code byte, row joblist.Row, wide, compact bool) string {
	switch code {
	case 's':
		if compact {
			return jobstatus.WithoutRecordedMark(row.State)
		}
		return row.State
	case 'b':
		if wide {
			return row.BaseDir
		}
		return shortenJobsPath(row.BaseDir)
	case 'p':
		return row.Project
	case 'a':
		if row.AttemptID == "" {
			return "-"
		}
		return row.AttemptID
	case 'n':
		return row.JobName
	case 'c':
		if wide {
			return row.FullCommand
		}
		return row.Command
	case 't':
		return jobsTimestamp(row.StartedAt, compact)
	case 'f':
		return jobsTimestamp(row.FinishedAt, compact)
	case 'e':
		return joblist.FormatRunTime(row.RunTime)
	default:
		return ""
	}
}

func jobsTimestamp(value time.Time, compact bool) string {
	if value.IsZero() {
		return "-"
	}
	if compact {
		return joblist.FormatLocalTimestamp(value)
	}
	return joblist.FormatTimestamp(value)
}

func colorJobsState(value string) string {
	switch value {
	case "success", "success (accepted)":
		return green(value)
	case "failed", "cancelled", "blocked":
		return red(value)
	default:
		if value == "running" || value == "waiting" || value == "suspended" || value == "interrupted" || value == "unknown" || value == "incomplete" || value == "pending" || strings.HasSuffix(value, " (recorded)") {
			return yellow(value)
		}
		return value
	}
}

func jobsBaseDirs(requested, masterdir string) ([]string, error) {
	if requested != "" {
		baseDir, _, err := state.ResolveBaseDir(requested)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve state directory: %w", err)
		}
		baseDir, err = filepath.Abs(baseDir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve state directory: %w", err)
		}
		return []string{baseDir}, nil
	}
	masterDir, err := state.ResolveMasterDir(masterdir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve master directory: %w", err)
	}
	servers, err := listServers(masterDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}
	known, err := listKnownBaseDirs(masterDir, servers)
	if err != nil {
		return nil, fmt.Errorf("failed to list known state directories: %w", err)
	}
	baseDirs := make([]string, 0, len(known)+1)
	seen := make(map[string]bool, len(known)+1)
	for _, item := range known {
		baseDir, err := filepath.Abs(item.BaseDir)
		if err == nil {
			seen[filepath.Clean(baseDir)] = true
			baseDirs = append(baseDirs, filepath.Clean(baseDir))
		}
	}
	if current, err := state.ResolveBaseDirDefault(); err == nil {
		current, absErr := filepath.Abs(current)
		if absErr == nil && !seen[filepath.Clean(current)] {
			baseDirs = append(baseDirs, filepath.Clean(current))
		}
	}
	sort.Strings(baseDirs)
	return baseDirs, nil
}

func collectJobsAcrossBaseDirs(baseDirs []string, project string, now time.Time, window time.Duration) ([]joblist.Row, error) {
	rows := make([]joblist.Row, 0)
	for _, baseDir := range baseDirs {
		projects, err := joblist.Projects(baseDir, project)
		if err != nil {
			return nil, err
		}
		baseRows, err := joblist.Collect(jsonStore(), baseDir, projects, now, window)
		if err != nil {
			return nil, err
		}
		rows = append(rows, baseRows...)
	}
	return rows, nil
}

func shortenJobsPath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		if relative, relErr := filepath.Rel(home, path); relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			path = "~" + string(filepath.Separator) + relative
		}
	}
	const maxLength = 24
	if len(path) <= maxLength {
		return path
	}
	return ".../" + filepath.Base(filepath.Dir(path)) + "/" + filepath.Base(path)
}

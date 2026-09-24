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

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	defaultJobsSince     = 24 * time.Hour
	defaultJobsSinceText = "24h"
)
const defaultJobsFormat = "%s %p %a %n %c %f %e"

type jobsRow struct {
	state       string
	baseDir     string
	project     string
	runID       string
	attemptID   string
	jobName     string
	command     string
	fullCommand string
	startedAt   time.Time
	finishedAt  time.Time
	elapsed     time.Duration
}

type jobsColumn struct {
	header string
	code   byte
	width  int
}

func loadTerminalJobStatus(jobDir string) (int, bool) {
	return executor.ResolveTerminalExitCode(jsonStore(), jobDir)
}

// cmdJobs lists historical jobs across runs for the selected project, with
// filters for status, command metadata, and time windows.
func cmdJobs(args []string) int {
	fs := flag.NewFlagSet("jobs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	masterdir := cliString(fs, "masterdir", "")
	allBaseDirs := cliBool(fs, "all", false)
	format := cliString(fs, "format", defaultJobsFormat)
	since := cliString(fs, "since", defaultJobsSinceText)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("jobs"))
		return 1
	}
	columns, err := parseJobsFormat(*format)
	if err != nil {
		printErrorf("invalid --format: %v", err)
		return 1
	}
	window, err := parseJobsSince(*since)
	if err != nil {
		printErrorf("invalid --since duration %q", *since)
		return 1
	}
	baseDirs, err := jobsBaseDirs(*basedir, *masterdir, *allBaseDirs)
	if err != nil {
		printError(err)
		return 1
	}
	rows, err := collectJobsAcrossBaseDirs(baseDirs, *projectName, time.Now(), window)
	if err != nil {
		printError(err)
		return 1
	}
	if len(rows) == 0 {
		fmt.Println("No running or recently finished jobs found.")
		return 0
	}
	sortJobsRows(rows)

	printJobsTableFormat(rows, columns)
	return 0
}

func parseJobsSince(value string) (time.Duration, error) {
	if value == "" {
		return defaultJobsSince, nil
	}
	window, err := time.ParseDuration(value)
	if err != nil || window < 0 {
		return 0, fmt.Errorf("invalid duration")
	}
	return window, nil
}

func jobsRowSortTime(row jobsRow) time.Time {
	if !row.finishedAt.IsZero() {
		return row.finishedAt
	}
	return row.startedAt
}

func sortJobsRows(rows []jobsRow) {
	sort.SliceStable(rows, func(left, right int) bool {
		return jobsRowSortTime(rows[left]).After(jobsRowSortTime(rows[right]))
	})
}

func printJobsTable(rows []jobsRow, showBaseDir bool) {
	format := defaultJobsFormat
	if showBaseDir {
		format = "%s %b %p %a %n %c %f %e"
	}
	columns, _ := parseJobsFormat(format)
	printJobsTableFormat(rows, columns)
}

func printJobsTableFormat(rows []jobsRow, formatColumns []jobsColumn) {
	columns := make([][]string, len(formatColumns))
	for index, column := range formatColumns {
		columns[index] = []string{column.header}
	}
	for _, row := range rows {
		for index, column := range formatColumns {
			value := jobsColumnValue(column.code, row)
			if column.width > 0 {
				value = shortenJobsText(value, column.width)
			}
			columns[index] = append(columns[index], value)
		}
	}
	widths := make([]int, len(columns))
	for index, column := range columns {
		for _, value := range column {
			if len(value) > widths[index] {
				widths[index] = len(value)
			}
		}
	}
	for rowIndex := range columns[0] {
		values := make([]string, len(columns))
		for columnIndex := range columns {
			values[columnIndex] = columns[columnIndex][rowIndex]
		}
		line := strings.TrimRight(formatJobsRow(values, widths), " ")
		fmt.Println(line)
	}
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

func jobsColumnValue(code byte, row jobsRow) string {
	switch code {
	case 's':
		return row.state
	case 'b':
		return shortenJobsPath(row.baseDir)
	case 'p':
		return row.project
	case 'a':
		return row.attemptID
	case 'n':
		return row.jobName
	case 'c':
		return row.command
	case 't':
		return formatJobsTimestamp(row.startedAt)
	case 'f':
		if row.finishedAt.IsZero() {
			return "-"
		}
		return formatJobsTimestamp(row.finishedAt)
	case 'e':
		return formatJobElapsed(row.elapsed)
	default:
		return ""
	}
}

func formatJobsRow(values []string, widths []int) string {
	var builder strings.Builder
	for index, value := range values {
		if index > 0 {
			builder.WriteString("  ")
		}
		builder.WriteString(value)
		if index < len(values)-1 {
			builder.WriteString(strings.Repeat(" ", widths[index]-len(value)))
		}
	}
	return builder.String()
}

func jobsBaseDirs(requested, masterdir string, all bool) ([]string, error) {
	if !all {
		baseDir, _, err := resolveBaseDir(requested)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve state directory: %w", err)
		}
		baseDir, err = filepath.Abs(baseDir)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve state directory: %w", err)
		}
		return []string{baseDir}, nil
	}
	masterDir, err := resolveMasterDir(masterdir)
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
	baseDirs := make([]string, 0, len(known))
	for _, item := range known {
		baseDir, err := filepath.Abs(item.BaseDir)
		if err == nil {
			baseDirs = append(baseDirs, baseDir)
		}
	}
	return baseDirs, nil
}

func collectJobsAcrossBaseDirs(baseDirs []string, project string, now time.Time, window time.Duration) ([]jobsRow, error) {
	rows := make([]jobsRow, 0)
	for _, baseDir := range baseDirs {
		projects, err := jobsProjects(baseDir, project)
		if err != nil {
			return nil, err
		}
		baseRows, err := collectJobs(baseDir, projects, now, window)
		if err != nil {
			return nil, err
		}
		rows = append(rows, baseRows...)
	}
	return rows, nil
}

func jobsProjects(baseDir, requested string) ([]string, error) {
	if requested != "" {
		if !state.IsValidPathElement(requested) {
			return nil, fmt.Errorf("invalid project name %q", requested)
		}
		return []string{requested}, nil
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{defaultProjectName}, nil
		}
		return nil, fmt.Errorf("failed to read projects: %w", err)
	}
	projects := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && state.IsValidPathElement(entry.Name()) {
			projects = append(projects, entry.Name())
		}
	}
	sort.Strings(projects)
	return projects, nil
}

func collectJobs(baseDir string, projects []string, now time.Time, window time.Duration) ([]jobsRow, error) {
	cutoff := now.Add(-window)
	rows := make([]jobsRow, 0)
	for _, project := range projects {
		paths, err := resolvePaths(baseDir, project)
		if err != nil {
			return nil, err
		}
		runEntries, err := os.ReadDir(paths.RunsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read runs for project %q: %w", project, err)
		}
		runs := make([]os.DirEntry, 0, len(runEntries))
		for _, entry := range runEntries {
			if entry.IsDir() && state.IsValidPathElement(entry.Name()) {
				runs = append(runs, entry)
			}
		}
		sort.SliceStable(runs, func(left, right int) bool { return runs[left].Name() > runs[right].Name() })
		for _, run := range runs {
			runRows, include, stop, err := collectRunJobs(paths, run.Name(), now, cutoff)
			if err != nil {
				return nil, err
			}
			if include {
				rows = append(rows, runRows...)
			}
			// Run IDs are generated from UTC timestamps, and a project cannot
			// start its next run until the previous run has finished. Therefore,
			// after an ordinary completed run is older than the cutoff, every
			// remaining run in this order is also outside the search window.
			// This does not apply to active/interrupted runs or runs with missing
			// or invalid summaries: those cases are deliberately not a signal to
			// stop, because their completion order is unknown.
			if stop {
				break
			}
		}
	}
	return rows, nil
}

func collectRunJobs(paths pathSet, runID string, now, cutoff time.Time) ([]jobsRow, bool, bool, error) {
	runDir := filepath.Join(paths.RunsDir, runID)
	summary, summaryErr := state.LoadRunSummary(filepath.Join(runDir, stateFileSummaryJSON))
	active := runIsActive(paths, runID)
	if summaryErr == nil && !active {
		finishedAt, err := parseJobsTimestamp(summary.FinishedAt)
		if err == nil && finishedAt.Before(cutoff) {
			return nil, false, true, nil
		}
	}
	runQueue, err := state.LoadQueue(filepath.Join(runDir, stateFileCommandsJSON))
	if err != nil {
		return nil, false, false, nil
	}
	resultByID := make(map[string]JobResult, len(summary.Results))
	for _, result := range summary.Results {
		resultByID[result.ID] = result
	}
	rows := make([]jobsRow, 0)
	for _, job := range model.QueueToJobs(runQueue.Commands) {
		jobDir, err := state.LatestAttemptJobDir(runDir, job.ID)
		if err != nil {
			continue
		}
		status, statusOK := loadTerminalJobStatus(jobDir)
		if !statusOK {
			if result, ok := resultByID[job.ID]; ok {
				status, statusOK = result.ExitCode, true
			}
		}
		jobState := ""
		if statusOK {
			if status == 0 {
				jobState = "success"
			} else {
				jobState = "failed"
			}
		} else if active {
			jobState = "running"
		} else {
			continue
		}
		submittedText, finishedText := readShowJobTimestamps(runDir, job.ID, nil)
		startedAt, err := parseJobsTimestamp(submittedText)
		if err != nil && summary.StartedAt != "" {
			startedAt, err = parseJobsTimestamp(summary.StartedAt)
		}
		if err != nil {
			continue
		}
		finishedAt, finishedErr := parseJobsTimestamp(finishedText)
		if finishedErr != nil && summary.FinishedAt != "" && statusOK {
			finishedAt, finishedErr = parseJobsTimestamp(summary.FinishedAt)
		}
		if jobState != "running" && (finishedErr != nil || finishedAt.Before(cutoff)) {
			continue
		}
		attemptID, _ := state.LatestAttemptID(runDir, job.ID)
		if attemptID == "" {
			if result, ok := resultByID[job.ID]; ok {
				attemptID = result.AttemptID
			}
		}
		if attemptID == "" {
			continue
		}
		jobName := readJobName(jobDir)
		if jobName == "" {
			jobName = job.Name
		}
		if jobName == "" {
			jobName = "-"
		}
		fullCommand := strings.Join(job.Command, " ")
		command := shortenJobsText(fullCommand, 40)
		end := now
		if jobState != "running" {
			end = finishedAt
		}
		rows = append(rows, jobsRow{state: jobState, baseDir: paths.BaseDir, project: paths.ProjectName, runID: runID, attemptID: attemptID, jobName: jobName, command: command, fullCommand: fullCommand, startedAt: startedAt, finishedAt: finishedAt, elapsed: end.Sub(startedAt)})
	}
	return rows, true, false, nil
}

func shortenJobsText(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	if maxLength <= 3 {
		return value[:maxLength]
	}
	return value[:maxLength-3] + "..."
}

func parseJobsTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
}

func formatJobsTimestamp(value time.Time) string {
	return value.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02 15:04")
}

func formatJobElapsed(value time.Duration) string {
	if value < 0 {
		return "-"
	}
	seconds := int64(value / time.Second)
	if seconds < 60 {
		return strconv.FormatInt(seconds, 10) + "s"
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %02ds", minutes, seconds%60)
	}
	hours := minutes / 60
	return fmt.Sprintf("%dh %02dm", hours, minutes%60)
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

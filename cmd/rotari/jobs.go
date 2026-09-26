package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const defaultJobsFormat = "%s %p %a %n %c %f %e"

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
	allBaseDirs := cliBool(fs, "all-basedirs", false)
	format := cliString(fs, "format", defaultJobsFormat)
	since := cliString(fs, "since", joblist.DefaultSinceText)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "project-name")) {
		printError("usage: " + cliUsage("jobs"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*projectName = fs.Args()[0]
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
	baseDirs, err := jobsBaseDirs(*basedir, *masterdir, *allBaseDirs)
	if err != nil {
		printError(err)
		return 1
	}
	if *projectName != "" {
		found := false
		for _, baseDir := range baseDirs {
			found = found || resolve.ProjectExists(baseDir, *projectName)
		}
		if !found {
			printErrorf("project %q does not exist in the listed state directories", *projectName)
			return 1
		}
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
	joblist.Sort(rows)

	printJobsTableFormat(rows, columns)
	return 0
}

func printJobsTable(rows []joblist.Row, showBaseDir bool) {
	format := defaultJobsFormat
	if showBaseDir {
		format = "%s %b %p %a %n %c %f %e"
	}
	columns, _ := parseJobsFormat(format)
	printJobsTableFormat(rows, columns)
}

func printJobsTableFormat(rows []joblist.Row, formatColumns []jobsColumn) {
	columns := make([][]string, len(formatColumns))
	for index, column := range formatColumns {
		columns[index] = []string{column.header}
	}
	for _, row := range rows {
		for index, column := range formatColumns {
			value := jobsColumnValue(column.code, row)
			if column.width > 0 {
				value = joblist.ShortenText(value, column.width)
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

func jobsColumnValue(code byte, row joblist.Row) string {
	switch code {
	case 's':
		return row.State
	case 'b':
		return shortenJobsPath(row.BaseDir)
	case 'p':
		return row.Project
	case 'a':
		return row.AttemptID
	case 'n':
		return row.JobName
	case 'c':
		return row.Command
	case 't':
		return joblist.FormatTimestamp(row.StartedAt)
	case 'f':
		if row.FinishedAt.IsZero() {
			return "-"
		}
		return joblist.FormatTimestamp(row.FinishedAt)
	case 'e':
		return joblist.FormatElapsed(row.Elapsed)
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
	baseDirs := make([]string, 0, len(known))
	for _, item := range known {
		baseDir, err := filepath.Abs(item.BaseDir)
		if err == nil {
			baseDirs = append(baseDirs, baseDir)
		}
	}
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

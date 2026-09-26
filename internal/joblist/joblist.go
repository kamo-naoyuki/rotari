package joblist

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	// DefaultSince is the listing window when none is given.
	DefaultSince = 24 * time.Hour
	// DefaultSinceText is DefaultSince as users write it.
	DefaultSinceText = "24h"
)

// Row is one job attempt in the listing: a running job of an active run or a
// job that finished inside the window.
type Row struct {
	State       string
	BaseDir     string
	Project     string
	RunID       string
	AttemptID   string
	JobName     string
	Command     string
	FullCommand string
	StartedAt   time.Time
	FinishedAt  time.Time
	Elapsed     time.Duration
}

// ParseSince parses a listing window such as "24h"; empty means
// DefaultSince.
func ParseSince(value string) (time.Duration, error) {
	if value == "" {
		return DefaultSince, nil
	}
	window, err := time.ParseDuration(value)
	if err != nil || window < 0 {
		return 0, fmt.Errorf("invalid duration")
	}
	return window, nil
}

func sortTime(row Row) time.Time {
	if !row.FinishedAt.IsZero() {
		return row.FinishedAt
	}
	return row.StartedAt
}

// Sort orders rows by finish time, or start time for unfinished jobs,
// newest first.
func Sort(rows []Row) {
	sort.SliceStable(rows, func(left, right int) bool {
		return sortTime(rows[left]).After(sortTime(rows[right]))
	})
}

// Projects returns requested, or every project in baseDir sorted by name, or
// the default project when baseDir has none.
func Projects(baseDir, requested string) ([]string, error) {
	if requested != "" {
		if !state.IsValidPathElement(requested) {
			return nil, fmt.Errorf("invalid project name %q", requested)
		}
		return []string{requested}, nil
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{state.DefaultProjectName}, nil
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

// Collect lists the running jobs and the jobs of projects in baseDir that
// finished within window before now. Each job's status follows the
// internal/jobstatus fallback chain.
func Collect(store state.Store, baseDir string, projects []string, now time.Time, window time.Duration) ([]Row, error) {
	cutoff := now.Add(-window)
	rows := make([]Row, 0)
	for _, project := range projects {
		paths, err := state.ResolveProjectPaths(baseDir, project)
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
			runRows, include, stop, err := collectRun(store, paths, run.Name(), now, cutoff)
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

func collectRun(store state.Store, paths state.ProjectPaths, runID string, now, cutoff time.Time) ([]Row, bool, bool, error) {
	runDir := filepath.Join(paths.RunsDir, runID)
	summary, summaryErr := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	active := project.RunActive(paths, runID)
	if summaryErr == nil && !active {
		finishedAt, err := parseTimestamp(summary.FinishedAt)
		if err == nil && finishedAt.Before(cutoff) {
			return nil, false, true, nil
		}
	}
	runQueue, err := state.LoadQueue(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return nil, false, false, nil
	}
	resultByID := make(map[string]model.JobResult, len(summary.Results))
	for _, result := range summary.Results {
		resultByID[result.ID] = result
	}
	rows := make([]Row, 0)
	for _, job := range model.QueueToJobs(runQueue.Commands) {
		jobDir, err := state.LatestAttemptJobDir(runDir, job.ID)
		if err != nil {
			continue
		}
		summaryResult, hasSummary := resultByID[job.ID]
		resolved := jobstatus.ReadJob(store, jobDir, summaryResult, hasSummary)
		status, statusOK := resolved.ExitCode, resolved.Finished()
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
		submittedText, finishedText := jobstatus.Timestamps(runDir, job.ID, nil)
		startedAt, err := parseTimestamp(submittedText)
		if err != nil && summary.StartedAt != "" {
			startedAt, err = parseTimestamp(summary.StartedAt)
		}
		if err != nil {
			continue
		}
		finishedAt, finishedErr := parseTimestamp(finishedText)
		if finishedErr != nil && summary.FinishedAt != "" && statusOK {
			finishedAt, finishedErr = parseTimestamp(summary.FinishedAt)
		}
		if jobState != "running" {
			if finishedErr == nil {
				if finishedAt.Before(cutoff) {
					continue
				}
			} else if startedAt.Before(cutoff) {
				continue
			}
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
		jobName := state.ReadJobName(jobDir)
		if jobName == "" {
			jobName = job.Name
		}
		if jobName == "" {
			jobName = "-"
		}
		fullCommand := strings.Join(job.Command, " ")
		command := ShortenText(fullCommand, 40)
		end := now
		if jobState != "running" {
			end = finishedAt
		}
		if jobState != "running" && finishedErr != nil {
			end = time.Time{}
		}
		rows = append(rows, Row{State: jobState, BaseDir: paths.BaseDir, Project: paths.ProjectName, RunID: runID, AttemptID: attemptID, JobName: jobName, Command: command, FullCommand: fullCommand, StartedAt: startedAt, FinishedAt: finishedAt, Elapsed: end.Sub(startedAt)})
	}
	return rows, true, false, nil
}

// ShortenText cuts value to maxLength bytes, ending with "..." when
// maxLength leaves room for it.
func ShortenText(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	if maxLength <= 3 {
		return value[:maxLength]
	}
	return value[:maxLength-3] + "..."
}

func parseTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
}

// FormatTimestamp formats a listing time in JST to the minute.
func FormatTimestamp(value time.Time) string {
	return value.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02 15:04")
}

// FormatElapsed formats a duration as "42s", "3m 05s", or "2h 07m", and a
// negative one as "-".
func FormatElapsed(value time.Duration) string {
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

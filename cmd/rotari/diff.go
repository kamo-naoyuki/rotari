package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/rundiff"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdDiff compares two runs of a project: with no run IDs the latest run and
// the one before it, with one run ID that run and the one before it.
func cmdDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	jsonOutput := cliBool(fs, "json", false)
	showAll := cliBool(fs, "all", false)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 2 {
		printError("usage: " + cliUsage("diff"))
		return 1
	}
	fromID, toID := "", ""
	switch len(fs.Args()) {
	case 1:
		toID = fs.Args()[0]
	case 2:
		fromID, toID = fs.Args()[0], fs.Args()[1]
	}
	baseDir, project, err := resolveExistingRunTarget(*basedir, *projectName, firstNonEmpty(toID, fromID))
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	if toID, err = selectRunID(paths, toID); err != nil {
		printError(err)
		return 1
	}
	if fromID == "" {
		if fromID, err = previousRunID(paths, toID); err != nil {
			printError(err)
			return 1
		}
	} else if fromID, err = selectRunID(paths, fromID); err != nil {
		printError(err)
		return 1
	}
	from, err := loadDiffRun(paths, fromID)
	if err != nil {
		printError(err)
		return 1
	}
	to, err := loadDiffRun(paths, toID)
	if err != nil {
		printError(err)
		return 1
	}
	result := rundiff.Compare(from, to)
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			printErrorf("failed to encode diff: %v", err)
			return 1
		}
		return 0
	}
	writeRunDiff(os.Stdout, paths, result, *showAll)
	return 0
}

// previousRunID returns the run of the project that started just before
// runID. Runs of one project never overlap, so start order is run order. Run
// IDs only have one-second resolution, so runs are ordered by their first
// load sample, which has nanoseconds, then by the summary's start time, then
// by run ID.
func previousRunID(paths state.ProjectPaths, runID string) (string, error) {
	entries, err := os.ReadDir(paths.RunsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read runs: %w", err)
	}
	type startedRun struct {
		id      string
		started time.Time
	}
	runs := make([]startedRun, 0, len(entries))
	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runs = append(runs, startedRun{id: entry.Name(), started: runStartTime(paths, entry.Name())})
		found = found || entry.Name() == runID
	}
	if !found {
		return "", fmt.Errorf(runNotFoundMessage, runID)
	}
	sort.Slice(runs, func(i, j int) bool {
		if !runs[i].started.Equal(runs[j].started) {
			return runs[i].started.Before(runs[j].started)
		}
		return runs[i].id < runs[j].id
	})
	for index, run := range runs {
		if run.id == runID {
			if index == 0 {
				return "", fmt.Errorf("run %s has no earlier run in project %q to compare with", runID, paths.ProjectName)
			}
			return runs[index-1].id, nil
		}
	}
	return "", fmt.Errorf(runNotFoundMessage, runID)
}

// runStartTime returns when a run started, or the zero time when unknown.
func runStartTime(paths state.ProjectPaths, runID string) time.Time {
	if samples := state.ReadLoadSamples(loadSamplesPath(paths, runID)); len(samples) > 0 {
		if started, err := time.Parse(time.RFC3339Nano, samples[0].At); err == nil {
			return started
		}
	}
	if summary, err := state.LoadRunSummary(filepath.Join(paths.RunsDir, runID, "summary.json")); err == nil {
		if started, err := time.Parse(time.RFC3339, summary.StartedAt); err == nil {
			return started
		}
	}
	return time.Time{}
}

// loadDiffRun loads a run's command snapshot and resolves each job's result
// through the shared jobstatus fallback chain, as show does.
func loadDiffRun(paths state.ProjectPaths, runID string) (rundiff.Run, error) {
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		return rundiff.Run{}, fmt.Errorf(runNotFoundMessage, runID)
	}
	commands, err := state.ReadQueueFile(filepath.Join(runDir, "commands.json"))
	if err != nil {
		return rundiff.Run{}, fmt.Errorf("failed to load run %s commands: %w", runID, err)
	}
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return rundiff.Run{}, fmt.Errorf("failed to load run %s summary: %w", runID, err)
	}
	results := model.ResultsByID(summary.Results)
	origins := model.QueueOriginsByJobID(commands)
	run := rundiff.Run{ID: runID, Name: summary.RunName, StartedAt: summary.StartedAt, FinishedAt: summary.FinishedAt}
	for _, spec := range model.QueueToJobs(commands.Commands) {
		jobDir, err := state.LatestAttemptJobDir(runDir, spec.ID)
		if err != nil {
			return rundiff.Run{}, fmt.Errorf("invalid job ID %q: %w", spec.ID, err)
		}
		summaryResult, hasSummary := results[spec.ID]
		resolved := jobstatus.ReadJob(jsonStore(), jobDir, summaryResult, hasSummary)
		status := rundiff.StatusUnfinished
		switch {
		case !resolved.Finished():
		case resolved.Accepted() || resolved.ExitCode == 0:
			status = rundiff.StatusSuccess
		case resolved.Blocked():
			status = rundiff.StatusBlocked
		default:
			status = rundiff.StatusFailed
		}
		attemptID, _ := state.LatestAttemptID(runDir, spec.ID)
		origin := origins[spec.ID]
		carried := origin != nil && (attemptID == "" || attemptID == origin.AttemptID)
		run.Jobs = append(run.Jobs, rundiff.Job{Spec: spec, Status: status, Carried: carried})
	}
	return run, nil
}

func writeRunDiff(writer io.Writer, paths state.ProjectPaths, result rundiff.Result, showAll bool) {
	label := func(info rundiff.RunInfo) string { return formatRunLabel(info.ID, info.Name) }
	fmt.Fprintf(writer, "%s %s\n", cyan("Project:"), paths.ProjectName)
	fmt.Fprintf(writer, "%s %s -> %s\n", cyan("Runs:"), label(result.From), label(result.To))
	if result.From.Elapsed != "" || result.To.Elapsed != "" {
		fmt.Fprintf(writer, "%s %s -> %s\n", cyan("Elapsed:"), firstNonEmpty(result.From.Elapsed, "-"), firstNonEmpty(result.To.Elapsed, "-"))
	}
	summary := result.Summary
	fmt.Fprintf(writer, "%s fixed %d, still failing %d, newly failing %d, added %d, removed %d, changed %d, carried %d\n",
		cyan("Summary:"), summary.Fixed, summary.StillFailing, summary.NewlyFailing, summary.Added, summary.Removed, summary.Changed, summary.Carried)

	shown := make([]rundiff.JobDiff, 0, len(result.Jobs))
	for _, job := range result.Jobs {
		if showAll || job.Transition != rundiff.TransitionUnchanged || len(job.Changes) > 0 {
			shown = append(shown, job)
		}
	}
	if len(shown) > 0 {
		width := len("JOB")
		for _, job := range shown {
			width = max(width, len(job.Name))
		}
		fmt.Fprintf(writer, "\n%s\n", cyan(fmt.Sprintf("%-*s  %-10s  %-10s  %-14s  %s", width, "JOB", "FROM", "TO", "RESULT", "CHANGES")))
		for _, job := range shown {
			fields := make([]string, 0, len(job.Changes))
			for _, change := range job.Changes {
				fields = append(fields, change.Field)
			}
			changes := firstNonEmpty(strings.Join(fields, ","), "-")
			if job.Carried {
				changes += " (carried)"
			}
			fmt.Fprintf(writer, "%-*s  %-10s  %-10s  %s  %s\n", width, job.Name, firstNonEmpty(job.FromStatus, "-"), firstNonEmpty(job.ToStatus, "-"),
				colorTransition(fmt.Sprintf("%-14s", strings.ReplaceAll(job.Transition, "_", " ")), job.Transition), changes)
		}
	}
	if hidden := len(result.Jobs) - len(shown); hidden > 0 {
		fmt.Fprintf(writer, "\n%d unchanged job(s) hidden; use --all to list them.\n", hidden)
	}
	wroteHeader := false
	for _, job := range shown {
		if len(job.Changes) == 0 {
			continue
		}
		if !wroteHeader {
			fmt.Fprintf(writer, "\n%s\n", cyan("Definition changes:"))
			wroteHeader = true
		}
		fmt.Fprintf(writer, "  %s\n", job.Name)
		for _, change := range job.Changes {
			if len(change.Added) > 0 || len(change.Removed) > 0 {
				parts := make([]string, 0, len(change.Added)+len(change.Removed))
				for _, value := range change.Removed {
					parts = append(parts, red("-"+value))
				}
				for _, value := range change.Added {
					parts = append(parts, green("+"+value))
				}
				fmt.Fprintf(writer, "    %s: %s\n", change.Field, strings.Join(parts, " "))
				continue
			}
			fmt.Fprintf(writer, "    %s: %s -> %s\n", change.Field, firstNonEmpty(change.From, "-"), firstNonEmpty(change.To, "-"))
		}
	}
}

func colorTransition(text, transition string) string {
	switch transition {
	case rundiff.TransitionFixed:
		return green(text)
	case rundiff.TransitionNewlyFailing:
		return red(text)
	case rundiff.TransitionStillFailing, rundiff.TransitionAdded, rundiff.TransitionRemoved:
		return yellow(text)
	}
	return text
}

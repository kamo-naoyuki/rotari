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

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/rundiff"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdLineage lists runs, summarizes one run, or compares two runs.
func cmdLineage(args []string) int {
	fs := flag.NewFlagSet("lineage", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	jsonOutput := cliBool(fs, "json", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	baseDir, project, err := resolve.ExistingRun(*basedir, *projectName, firstNonEmpty(fs.Args()...))
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	return showLineage(paths, fs.Args(), *jsonOutput)
}

// previousRunID returns the run of the project that started just before
// runID.
func previousRunID(paths state.ProjectPaths, runID string) (string, error) {
	runIDs, err := projectRunsByStart(paths)
	if err != nil {
		return "", err
	}
	for index, candidate := range runIDs {
		if candidate == runID {
			if index == 0 {
				return "", fmt.Errorf("run %s has no earlier run in project %q to compare with", runID, paths.ProjectName)
			}
			return runIDs[index-1], nil
		}
	}
	return "", fmt.Errorf(runNotFoundMessage, runID)
}

// projectRunsByStart lists a project's run IDs oldest first. Runs of one
// project never overlap, so start order is run order. Run IDs only have
// one-second resolution, so runs are ordered by their first load sample,
// which has nanoseconds, then by the summary's start time, then by run ID.
func projectRunsByStart(paths state.ProjectPaths) ([]string, error) {
	entries, err := os.ReadDir(paths.RunsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read runs: %w", err)
	}
	type startedRun struct {
		id      string
		started time.Time
	}
	runs := make([]startedRun, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			runs = append(runs, startedRun{id: entry.Name(), started: runStartTime(paths, entry.Name())})
		}
	}
	sort.Slice(runs, func(i, j int) bool {
		if !runs[i].started.Equal(runs[j].started) {
			return runs[i].started.Before(runs[j].started)
		}
		return runs[i].id < runs[j].id
	})
	runIDs := make([]string, len(runs))
	for index, run := range runs {
		runIDs[index] = run.id
	}
	return runIDs, nil
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

func writeRunDiff(writer io.Writer, paths state.ProjectPaths, result rundiff.Result, showAll bool) {
	label := func(info rundiff.RunInfo) string { return model.RunLabel(info.ID, info.Name) }
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
		fmt.Fprintf(writer, "\n%d unchanged job(s) hidden.\n", hidden)
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

// showLineage lists a project's runs oldest first, summarizes one selected run,
// or compares two selected runs.
func showLineage(paths state.ProjectPaths, runIDs []string, jsonOutput bool) int {
	if len(runIDs) > 0 {
		resolvedIDs := make([]string, len(runIDs))
		for index, runID := range runIDs {
			resolved, err := resolve.RunID(paths, runID)
			if err != nil {
				printError(err)
				return 1
			}
			resolvedIDs[index] = resolved
		}
		if len(resolvedIDs) == 1 {
			run, err := runview.LoadRun(paths, resolvedIDs[0], jsonStore())
			if err != nil {
				printError(err)
				return 1
			}
			summary := rundiff.RunSummary{
				Run: rundiff.RunInfo{ID: run.ID, Name: run.Name}, Counts: rundiff.Summarize(run),
				Diagnoses: rundiff.SummarizeDiagnoses(run),
				Origins:   rundiff.SummarizeOrigins(run),
			}
			if jsonOutput {
				return encodeJSON(summary, "run summary")
			}
			writeRunSummary(os.Stdout, paths, summary)
			return 0
		}
		from, err := runview.LoadRun(paths, resolvedIDs[0], jsonStore())
		if err != nil {
			printError(err)
			return 1
		}
		to, err := runview.LoadRun(paths, resolvedIDs[1], jsonStore())
		if err != nil {
			printError(err)
			return 1
		}
		if len(resolvedIDs) > 2 {
			runs := make([]rundiff.Run, 0, len(resolvedIDs))
			for _, runID := range resolvedIDs {
				run, loadErr := runview.LoadRun(paths, runID, jsonStore())
				if loadErr != nil {
					printError(loadErr)
					return 1
				}
				runs = append(runs, run)
			}
			grid := rundiff.CompareGrid(runs)
			if jsonOutput {
				return encodeJSON(grid, "comparison grid")
			}
			writeRunGrid(os.Stdout, paths, grid)
			return 0
		}
		result := rundiff.Compare(from, to)
		if jsonOutput {
			return encodeJSON(result, "comparison")
		}
		writeRunDiff(os.Stdout, paths, result, false)
		return 0
	}
	runIDs, err := projectRunsByStart(paths)
	if err != nil {
		printError(err)
		return 1
	}
	runs := make([]rundiff.Run, 0, len(runIDs))
	for _, runID := range runIDs {
		run, err := runview.LoadRun(paths, runID, jsonStore())
		if err != nil {
			// An active run may not have written commands.json yet.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			printError(err)
			return 1
		}
		runs = append(runs, run)
	}
	entries := rundiff.Lineage(runs)
	if jsonOutput {
		return encodeJSON(entries, "lineage")
	}
	writeShowTargetHeaderWithMode(os.Stdout, paths, "lineage")
	fmt.Println("\n" + cyan("Runs (oldest first):"))
	if len(entries) == 0 {
		fmt.Println("No runs found.")
		return 0
	}
	labels := make([]string, len(entries))
	width := len("RUN")
	for index, entry := range entries {
		labels[index] = model.RunLabel(entry.Run.ID, entry.Run.Name)
		width = max(width, len(labels[index]))
	}
	fmt.Println(cyan(fmt.Sprintf("%-*s  %5s  %5s  %6s  %7s  %5s  %8s  %5s  %7s  %7s  %7s  %s",
		width, "RUN", "JOBS", "OK", "FAILED", "BLOCKED", "FIXED", "NEW FAIL", "ADDED", "REMOVED", "CHANGED", "CARRIED", "ELAPSED")))
	for index, entry := range entries {
		counts := entry.Counts
		changeColumns := []string{"-", "-", "-", "-", "-", "-"}
		if changes := entry.Changes; changes != nil {
			changeColumns = []string{
				fmt.Sprint(changes.Fixed), fmt.Sprint(changes.NewlyFailing), fmt.Sprint(changes.Added),
				fmt.Sprint(changes.Removed), fmt.Sprint(changes.Changed), fmt.Sprint(changes.Carried),
			}
		}
		fmt.Printf("%-*s  %5d  %5d  %6d  %7d  %5s  %8s  %5s  %7s  %7s  %7s  %s\n",
			width, labels[index], counts.Jobs, counts.Succeeded, counts.Failed, counts.Blocked,
			changeColumns[0], changeColumns[1], changeColumns[2], changeColumns[3], changeColumns[4], changeColumns[5],
			firstNonEmpty(entry.Run.Elapsed, "-"))
	}
	if len(entries) >= 2 {
		previous := entries[len(entries)-2].Run.ID
		current := entries[len(entries)-1].Run.ID
		fmt.Printf("\n%s\n  rotari lineage %s %s\n", cyan("To compare the latest two runs:"), previous, current)
	}
	return 0
}

func writeRunGrid(writer io.Writer, paths state.ProjectPaths, grid rundiff.GridResult) {
	fmt.Fprintf(writer, "%s %s\n", cyan("Project:"), paths.ProjectName)
	labels := make([]string, len(grid.Runs))
	for index, run := range grid.Runs {
		labels[index] = model.RunLabel(run.ID, run.Name)
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Runs:"), strings.Join(labels, " | "))
	width := len("JOB")
	for _, job := range grid.Jobs {
		width = max(width, len(job.Name))
	}
	header := fmt.Sprintf("%-*s", width, "JOB")
	for index := range grid.Runs {
		header += fmt.Sprintf("  %-12s", fmt.Sprintf("RUN_%d", index+1))
	}
	fmt.Fprintln(writer, cyan(header))
	for _, job := range grid.Jobs {
		line := fmt.Sprintf("%-*s", width, job.Name)
		for index, status := range job.Statuses {
			cell := firstNonEmpty(status, "-")
			if index < len(job.DefinitionChanged) && job.DefinitionChanged[index] {
				cell += " *"
			}
			line += fmt.Sprintf("  %-12s", cell)
		}
		fmt.Fprintln(writer, line)
	}
}

func writeRunSummary(writer io.Writer, paths state.ProjectPaths, summary rundiff.RunSummary) {
	fmt.Fprintf(writer, "%s %s\n", cyan("Project:"), paths.ProjectName)
	fmt.Fprintf(writer, "%s %s\n", cyan("Run:"), model.RunLabel(summary.Run.ID, summary.Run.Name))
	counts := summary.Counts
	fmt.Fprintf(writer, "%s jobs %d, succeeded %d, failed %d, blocked %d, unfinished %d\n", cyan("Summary:"),
		counts.Jobs, counts.Succeeded, counts.Failed, counts.Blocked, counts.Unfinished)
	if len(summary.Diagnoses) > 0 {
		fmt.Fprintln(writer, cyan("Diagnoses:"))
		for _, diagnosis := range summary.Diagnoses {
			fmt.Fprintf(writer, "  %s %d\n", diagnosis.Name, diagnosis.Count)
		}
	}
	if len(summary.Origins) > 0 {
		fmt.Fprintln(writer, cyan("Origins:"))
		for _, origin := range summary.Origins {
			label := firstNonEmpty(origin.RunID, "new")
			fmt.Fprintf(writer, "  %s %d\n", label, origin.Count)
		}
	}
}

func encodeJSON(value any, description string) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		printErrorf("failed to encode %s: %v", description, err)
		return 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

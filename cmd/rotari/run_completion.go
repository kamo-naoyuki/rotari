package main

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// formatRunCompletion describes a finished run for `run` and `wait`: a title
// line, labeled details with the run's counts, and for a failed run its
// failures grouped by cause as `lineage` prints them, each with the commands
// to show and rerun it. It stays short however many jobs fail.
func formatRunCompletion(paths state.ProjectPaths, runID string, summary model.RunSummary) string {
	title := "=== Run finished ==="
	switch {
	case summary.Status == model.StatusCancelled:
		title = "=== Run cancelled ==="
	case summary.ExitCode != 0:
		title = "=== Run failed ==="
	}
	var header bytes.Buffer
	fmt.Fprintf(&header, "%s\n  Project: %s\n  Run: %s\n  Status: %s\n  Exit code: %d\n  Directory: %s\n",
		title, paths.ProjectName, model.RunLabel(runID, summary.RunName), summary.Status, summary.ExitCode, filepath.Join(paths.RunsDir, runID))
	run, err := runview.LoadRun(paths, runID, jsonStore())
	if err != nil {
		return colorMessage(header.String())
	}
	counts := runlineage.Summarize(run)
	fmt.Fprintf(&header, "  Summary: jobs %d, succeeded %d, failed %d, blocked %d, unfinished %d\n",
		counts.Jobs, counts.Succeeded, counts.Failed, counts.Blocked, counts.Unfinished)
	carried := 0
	for _, job := range run.Jobs {
		if job.Carried {
			carried++
		}
	}
	if carried > 0 {
		fmt.Fprintf(&header, "  Carried: %d\n", carried)
	}
	for _, origin := range runlineage.SummarizeOrigins(run) {
		label := origin.RunID
		if label == "" {
			label = "new"
		}
		fmt.Fprintf(&header, "  Origin: %s %d\n", label, origin.Count)
	}
	message := colorMessage(header.String())
	groups := runlineage.FailureGroups(run)
	if len(groups) == 0 {
		return message
	}
	var failures bytes.Buffer
	failures.WriteString("\n")
	writeFailureGroups(&failures, groups, failureRetryHints(paths, runID))
	fmt.Fprintf(&failures, "\n%s\n  rotari show %s--run-id %s\n", cyan("Inspect run:"), runHintLocation(paths), executor.ShellQuote(runID))
	return message + failures.String()
}

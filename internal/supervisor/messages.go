package supervisor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// CompletionMessage describes a finished run: a title line, then labeled
// details, and for failed jobs the commands to inspect and retry them.
// `run` sends it to the client, and `wait` prints it.
func CompletionMessage(paths state.ProjectPaths, runID string, summary model.RunSummary) string {
	successCount := 0
	failedCount := 0
	for _, result := range summary.Results {
		if result.ExitCode == 0 {
			successCount++
		} else {
			failedCount++
		}
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	title := "=== Run finished ==="
	if summary.ExitCode != 0 {
		title = "=== Run failed ==="
	}
	message := title + "\n" + fmt.Sprintf("  Project: %s\n  Run: %s\n  Status: %s\n  Exit code: %d\n  Success: %d\n  Failed: %d\n  Directory: %s\n",
		paths.ProjectName, model.RunLabel(runID, summary.RunName), summary.Status, summary.ExitCode, successCount, failedCount, runDir)
	if run, err := runview.LoadRun(paths, runID, state.NewStore(state.DirectoryMode(), state.FileMode())); err == nil {
		counts := runlineage.Summarize(run)
		message += fmt.Sprintf("  Summary: jobs %d, succeeded %d, failed %d, blocked %d, unfinished %d\n",
			counts.Jobs, counts.Succeeded, counts.Failed, counts.Blocked, counts.Unfinished)
		for _, diagnosis := range runlineage.SummarizeDiagnoses(run) {
			message += fmt.Sprintf("  Diagnosis: %s %d\n", diagnosis.Name, diagnosis.Count)
		}
		for _, origin := range runlineage.SummarizeOrigins(run) {
			label := origin.RunID
			if label == "" {
				label = "new"
			}
			message += fmt.Sprintf("  Origin: %s %d\n", label, origin.Count)
		}
	}
	if failedCount > 0 {
		message += fmt.Sprintf("\nInspect run:\n  rotari show --run-id %s\n\nSee failed job output below.\n\nFailed job output:\n%s\nRerun failed jobs:\n  rotari retry --basedir %s --project-name %s\n",
			runID, failedJobHints(runID, summary.Results), paths.BaseDir, paths.ProjectName)
	}
	return message
}

func failedJobHints(runID string, results []model.JobResult) string {
	var hints strings.Builder
	seen := make(map[string]bool)
	for _, result := range results {
		if result.ExitCode == 0 || seen[result.ID] {
			continue
		}
		seen[result.ID] = true
		hosts := strings.Join(result.Hosts, ",")
		if hosts == "" {
			hosts = "-"
		}
		if hints.Len() > 0 {
			hints.WriteString("  ----\n")
		}
		attemptID := result.AttemptID
		if attemptID == "" {
			attemptID = result.ID
		}
		fmt.Fprintf(&hints, "  Job: %s\n  Attempt ID: %s\n  Hosts: %s\n  Command: %s\n  Show output:\n    rotari show --run-id %s --job-id %s\n",
			result.ID, result.AttemptID, hosts, strings.Join(result.Command, " "), runID, attemptID)
	}
	return hints.String()
}

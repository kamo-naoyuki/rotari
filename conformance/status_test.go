package conformance

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// Contract DUR-5: the CLI and the Web UI resolve a job's result and times
// through the same fallback chain, so they never disagree about a job. See
// contracts/04-coordination-and-safety.md and the status invariant in
// AGENTS.md. This test checks a finished run, where the summary decides; the
// attempt-file fallbacks are not covered yet.

type webJob struct {
	ID        string `json:"id"`
	AttemptID string `json:"attempt_id"`
	Result    *struct {
		ExitCode int    `json:"exit_code"`
		Error    string `json:"error"`
	} `json:"result"`
	FinishedAt string `json:"finished_at"`
}

// loadWebJobs returns the jobs of project's run runID in the Web API state,
// keyed by job ID.
func loadWebJobs(t *testing.T, e *env, base, project, runID string) map[string]webJob {
	t.Helper()
	got := e.httpGet(base + "/api/state")
	if got.status != 200 {
		t.Fatalf("GET /api/state: status %d: %s", got.status, got.body)
	}
	var state struct {
		Projects []struct {
			ProjectName string `json:"project_name"`
			Runs        []struct {
				RunID string   `json:"run_id"`
				Jobs  []webJob `json:"jobs"`
			} `json:"runs"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(got.body), &state); err != nil {
		t.Fatal(err)
	}
	jobs := map[string]webJob{}
	for _, listed := range state.Projects {
		if listed.ProjectName != project {
			continue
		}
		for _, webRun := range listed.Runs {
			if webRun.RunID != runID {
				continue
			}
			for _, job := range webRun.Jobs {
				jobs[job.ID] = job
			}
		}
	}
	if len(jobs) == 0 {
		t.Fatalf("Web API state has no jobs for run %s", runID)
	}
	return jobs
}

var columnGap = regexp.MustCompile(`\s{2,}`)

// parseTable reads a table printed with a header row and columns separated
// by two or more spaces, keyed by its first column.
func parseTable(t *testing.T, output string) map[string]map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	header := columnGap.Split(strings.TrimSpace(lines[0]), -1)
	rows := map[string]map[string]string{}
	for _, line := range lines[1:] {
		cells := columnGap.Split(strings.TrimSpace(line), -1)
		if len(cells) != len(header) {
			t.Fatalf("row %q does not match header %q", line, lines[0])
		}
		row := map[string]string{}
		for i, name := range header {
			row[name] = cells[i]
		}
		rows[cells[0]] = row
	}
	return rows
}

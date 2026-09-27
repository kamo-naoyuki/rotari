package conformance

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Contract DUR-5: the CLI and the Web UI resolve a job's result and times
// through the same fallback chain, so they never disagree about a job. See
// contracts/04-coordination-and-safety.md and the status invariant in
// AGENTS.md. This test checks a finished run, where the summary decides; the
// attempt-file fallbacks are not covered yet.

func TestCLIAndWebAgreeOnJobResults(t *testing.T) {
	covers(t, "DUR-5")
	e := newEnv(t)
	run := e.createFinishedRun()

	// show --json: the run's summary.
	var shown struct {
		Summary struct {
			Results []struct {
				ID        string `json:"id"`
				AttemptID string `json:"attempt_id"`
				ExitCode  int    `json:"exit_code"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", run.project, "--json").stdout), &shown); err != nil {
		t.Fatal(err)
	}

	// jobs: one row per attempt with its state and finish time.
	jobRows := parseTable(t, e.mustRotari("jobs", run.project, "--format", "%a %s %f").stdout)

	// Web API: the same run's jobs.
	webJobs := loadWebJobs(t, e, e.startWeb(), run.project, run.runID)

	if len(shown.Summary.Results) != 2 {
		t.Fatalf("show --json lists %d results, want 2", len(shown.Summary.Results))
	}
	for _, cliResult := range shown.Summary.Results {
		web, ok := webJobs[cliResult.ID]
		if !ok {
			t.Errorf("job %s: in show --json but not in the Web API", cliResult.ID)
			continue
		}
		if web.AttemptID != cliResult.AttemptID {
			t.Errorf("job %s: attempt %q in show --json, %q in the Web API", cliResult.ID, cliResult.AttemptID, web.AttemptID)
		}
		if web.Result == nil || web.Result.ExitCode != cliResult.ExitCode {
			t.Errorf("job %s: exit code %d in show --json, Web API result %+v", cliResult.ID, cliResult.ExitCode, web.Result)
		}

		row, ok := jobRows[cliResult.AttemptID]
		if !ok {
			t.Errorf("job %s: attempt %s missing from jobs", cliResult.ID, cliResult.AttemptID)
			continue
		}
		wantState := "failed"
		if cliResult.ExitCode == 0 {
			wantState = "success"
		}
		if row["STATE"] != wantState {
			t.Errorf("job %s: jobs state %q, want %q for exit code %d", cliResult.ID, row["STATE"], wantState, cliResult.ExitCode)
		}
		// jobs shows minutes; the Web API shows seconds in the same form.
		if web.FinishedAt == "" || !strings.HasPrefix(web.FinishedAt, row["FINISHED"]) {
			t.Errorf("job %s: finished %q in jobs, %q in the Web API", cliResult.ID, row["FINISHED"], web.FinishedAt)
		}
	}
}

// Contract RES-11: persisted timestamps are UTC RFC3339; human-readable CLI
// and Web views use the IANA zone from TZ when valid, otherwise the local
// zone. See contracts/01-resolution-and-config.md. An invalid or unset
// TZ is not covered yet.

func TestDisplayTimesFollowTZ(t *testing.T) {
	covers(t, "RES-11")
	e := newEnv(t)
	run := e.createFinishedRun()

	var shown struct {
		Summary struct {
			StartedAt string `json:"started_at"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", run.project, "--json").stdout), &shown); err != nil {
		t.Fatal(err)
	}
	started, err := time.Parse(time.RFC3339, shown.Summary.StartedAt)
	if err != nil {
		t.Fatalf("show --json started_at %q is not RFC3339: %v", shown.Summary.StartedAt, err)
	}
	if !strings.HasSuffix(shown.Summary.StartedAt, "Z") {
		t.Errorf("show --json started_at %q is not stored in UTC", shown.Summary.StartedAt)
	}

	for _, zone := range []string{"UTC", "Asia/Tokyo", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			location, err := time.LoadLocation(zone)
			if err != nil {
				t.Skipf("time zone data unavailable: %v", err)
			}
			want := started.In(location).Format("2006-01-02 15:04:05 MST")
			zoned := e.in(t).withVar("TZ", zone)

			if output := zoned.mustRotari("show", "-p", run.project).stdout; !strings.Contains(output, want) {
				t.Errorf("show does not print the start time as %q:\n%s", want, output)
			}
			if got := webRunStartedAt(t, zoned, zoned.startWeb(), run); got != want {
				t.Errorf("Web API started_at = %q, want %q", got, want)
			}
		})
	}
}

func webRunStartedAt(t *testing.T, e *env, base string, run finishedRun) string {
	t.Helper()
	got := e.httpGet(base + "/api/state")
	var state struct {
		Projects []struct {
			ProjectName string `json:"project_name"`
			Runs        []struct {
				RunID     string `json:"run_id"`
				StartedAt string `json:"started_at"`
			} `json:"runs"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(got.body), &state); err != nil {
		t.Fatalf("GET /api/state: status %d: %v", got.status, err)
	}
	for _, project := range state.Projects {
		for _, webRun := range project.Runs {
			if project.ProjectName == run.project && webRun.RunID == run.runID {
				return webRun.StartedAt
			}
		}
	}
	t.Fatalf("Web API state has no run %s", run.runID)
	return ""
}

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

package coordination

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCLIAndWebAgreeOnJobResults(t *testing.T) {
	covers(t, "DUR-5")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	var shown struct {
		Summary struct {
			Results []struct {
				ID        string `json:"id"`
				AttemptID string `json:"attempt_id"`
				ExitCode  int    `json:"exit_code"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", run.Project, "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	jobRows := parseJobsTable(t, e.MustRotari("jobs", run.Project, "--format", "%a %s %f").Stdout)
	webJobs := loadWebJobs(t, e.HTTPGet(webRunURL(e.StartWeb(), run.Project, run.RunID)).Body, run.Project, run.RunID)
	if len(shown.Summary.Results) != 2 {
		t.Fatalf("show --json lists %d results, want 2", len(shown.Summary.Results))
	}
	for _, result := range shown.Summary.Results {
		web, ok := webJobs[result.ID]
		if !ok || web.AttemptID != result.AttemptID || web.Result == nil || web.Result.ExitCode != result.ExitCode {
			t.Errorf("job %s differs between CLI and Web: cli=%+v web=%+v", result.ID, result, web)
		}
		var selected struct {
			RunID string `json:"run_id"`
			JobID string `json:"job_id"`
			Jobs  []struct {
				Finished bool `json:"finished"`
				Result   *struct {
					ID       string `json:"id"`
					ExitCode int    `json:"exit_code"`
				} `json:"result"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", run.Project, "--run-id", run.RunID, "--job-id", result.ID, "--json").Stdout), &selected); err != nil {
			t.Fatal(err)
		}
		if selected.RunID != run.RunID || selected.JobID != result.ID || len(selected.Jobs) != 1 || !selected.Jobs[0].Finished || selected.Jobs[0].Result == nil || selected.Jobs[0].Result.ID != result.ID || selected.Jobs[0].Result.ExitCode != result.ExitCode {
			t.Errorf("job JSON view differs from run summary: %+v", selected)
		}
		wantState := "failed"
		if result.ExitCode == 0 {
			wantState = "success"
		}
		if jobRows[result.AttemptID]["STATE"] != wantState {
			t.Errorf("job %s state = %q, want %q", result.ID, jobRows[result.AttemptID]["STATE"], wantState)
		}
	}
}

func webRunURL(base, project, runID string) string {
	return base + "/api/run?project_name=" + url.QueryEscape(project) + "&run_id=" + url.QueryEscape(runID)
}

type statusWebJob struct {
	ID        string `json:"id"`
	AttemptID string `json:"attempt_id"`
	Result    *struct {
		ExitCode int `json:"exit_code"`
	} `json:"result"`
}

var jobsColumnGap = regexp.MustCompile(`\s{2,}`)

func loadWebJobs(t *testing.T, body, project, runID string) map[string]statusWebJob {
	t.Helper()
	var run struct {
		RunID string         `json:"run_id"`
		Jobs  []statusWebJob `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(body), &run); err != nil {
		t.Fatal(err)
	}
	if run.RunID != runID {
		t.Fatalf("Web API detail has run %q, want %q for project %q", run.RunID, runID, project)
	}
	jobs := map[string]statusWebJob{}
	for _, job := range run.Jobs {
		jobs[job.ID] = job
	}
	return jobs
}

func parseJobsTable(t *testing.T, output string) map[string]map[string]string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("job table has no rows: %q", output)
	}
	headers := jobsColumnGap.Split(strings.TrimSpace(lines[0]), -1)
	rows := map[string]map[string]string{}
	for _, line := range lines[1:] {
		fields := jobsColumnGap.Split(strings.TrimSpace(line), -1)
		if len(fields) != len(headers) {
			continue
		}
		row := map[string]string{}
		for i, header := range headers {
			row[header] = fields[i]
		}
		rows[fields[0]] = row
	}
	return rows
}

type fallbackCase struct {
	name     string
	command  []string
	edit     func(*testing.T, string)
	exitCode int
	blocked  bool
}

func TestCLIShowSelectedOlderAttemptIgnoresLatestSummary(t *testing.T) {
	covers(t, "DUR-5")
	e := support.NewEnv(t)
	project := "older-select"
	runID, _ := e.FinishedJobRun(project)
	runDir := filepath.Join(e.Base, "projects", project, "runs", runID)
	path := filepath.Join(runDir, "summary.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var summary model.RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("summary has %d rows, want 1", len(summary.Results))
	}
	jobID := summary.Results[0].ID
	olderAttempt := state.MakeAttemptID(runID, jobID, 0)
	latestAttempt := state.MakeAttemptID(runID, jobID, 1)
	olderDir := filepath.Join(runDir, jobID, "attempts", olderAttempt)
	newerDir := filepath.Join(runDir, jobID, "attempts", latestAttempt)
	if err := os.MkdirAll(olderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	olderTimestamp := "2026-09-24T01:02:03Z"
	newerTimestamp := "2026-09-25T01:02:03Z"
	if err := os.WriteFile(filepath.Join(olderDir, "status.json"), []byte(`{"phase":"running","finished_at":"`+olderTimestamp+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(olderDir, "submitted_at"), []byte(olderTimestamp+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newerDir, "status"), []byte("0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newerDir, "submitted_at"), []byte(newerTimestamp+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	summary.Results[0].AttemptID = latestAttempt
	summary.Results[0].Hosts = []string{"latest-host"}
	summary.Results[0].Diagnoses = []model.RuleDiagnosis{{Name: "latest-diagnosis"}}
	if err := state.WriteJSON(path, summary); err != nil {
		t.Fatal(err)
	}
	out := e.MustRotari("show", olderAttempt).Stdout
	if !strings.Contains(out, "Attempt ID: "+olderAttempt) {
		t.Fatalf("show %s omitted the selected attempt ID:\n%s", olderAttempt, out)
	}
	if !strings.Contains(out, "Submitted: ") || !strings.Contains(out, "Finished: ") {
		t.Fatalf("show %s omitted its own timestamp fields:\n%s", olderAttempt, out)
	}
	if strings.Contains(out, "latest-host") || strings.Contains(out, "latest-diagnosis") {
		t.Fatalf("show %s leaked latest summary metadata:\n%s", olderAttempt, out)
	}
	if !strings.Contains(e.MustRotari("show", "--run-id", runID, "--job-id", jobID, "--json").Stdout, "latest-host") {
		t.Fatalf("latest summary metadata was not visible in the current job view")
	}
}

func TestStatusFallbackChainAgreesAcrossViews(t *testing.T) {
	covers(t, "DUR-5")
	e := support.NewEnv(t)
	project := "chain"
	cases := []fallbackCase{
		{name: "status", command: []string{"true"}, exitCode: 3, edit: func(t *testing.T, dir string) { writeFile(t, filepath.Join(dir, "status"), "3\n") }},
		{name: "wrapper", command: []string{"true"}, exitCode: 5, edit: func(t *testing.T, dir string) {
			removeFile(t, filepath.Join(dir, "status"))
			writeFile(t, filepath.Join(dir, "status.json"), `{"phase":"finished","exit_code":5,"finished_at":"2026-09-27T00:00:00Z"}`)
		}},
		{name: "scheduler", command: []string{"true"}, exitCode: 1, edit: func(t *testing.T, dir string) {
			removeFile(t, filepath.Join(dir, "status"))
			writeFile(t, filepath.Join(dir, "status.json"), `{"phase":"running"}`)
			writeFile(t, filepath.Join(dir, "scheduler_status.json"), `{"state":"failed","updated_at":"2026-09-27T00:00:00Z"}`)
		}},
		{name: "fail", command: []string{"false"}, exitCode: 1},
		{name: "blocked", command: []string{"true"}, exitCode: 1, blocked: true},
	}
	for _, c := range cases {
		args := []string{"add", "-p", project, "--job-name", c.name}
		if c.blocked {
			args = append(args, "--depends-on", "fail")
		}
		e.MustRotari(append(append(args, "--"), c.command...)...)
	}
	e.Rotari("run", "-p", project, "--quiet")
	runID, ids := runJobIDs(t, e, project)
	for _, c := range cases {
		if c.edit == nil {
			continue
		}
		dirs, err := filepath.Glob(filepath.Join(e.Base, "projects", project, "runs", runID, ids[c.name], "attempts", "*"))
		if err != nil || len(dirs) != 1 {
			t.Fatalf("job %s attempts %q: %v", c.name, dirs, err)
		}
		c.edit(t, dirs[0])
	}
	if out := e.MustRotari("show", "-p", project, "--run-id", runID).Stdout; !strings.Contains(out, "Job status: success: 0, failed: 4, blocked: 1") {
		t.Errorf("show summary missing fallback counts:\n%s", out)
	}
	jobRows := parseJobsTable(t, e.MustRotari("jobs", project, "--format", "%n %s").Stdout)
	base := e.StartWeb()
	webJobs := loadWebJobs(t, e.HTTPGet(webRunURL(base, project, runID)).Body, project, runID)
	for _, c := range cases {
		id := ids[c.name]
		web := webJobs[id]
		if web.Result == nil || web.Result.ExitCode != c.exitCode {
			t.Errorf("Web job %s result=%+v, want exit %d", c.name, web.Result, c.exitCode)
		}
		if c.blocked {
			report := e.HTTPGet(base + "/api/report?" + url.Values{"project_name": {project}, "run_id": {runID}, "job_id": {id}}.Encode())
			if report.Status != 200 || !strings.Contains(report.Body, "Status: blocked") {
				t.Errorf("blocked report: %d %s", report.Status, report.Body)
			}
		}
		shown := e.Rotari("show", "-p", project, "--run-id", runID, "--job-id", id)
		if shown.Code != 0 {
			t.Fatalf("show --job-id: %s", shown)
		}
		if c.blocked && !strings.Contains(shown.Stdout, "blocked") {
			t.Errorf("blocked result missing: %s", shown.Stdout)
		}
		if !c.blocked && !regexp.MustCompile(`Status:.*\b`+strconv.Itoa(c.exitCode)+`\b`).MatchString(shown.Stdout) {
			t.Errorf("exit code %d missing: %s", c.exitCode, shown.Stdout)
		}
		if c.blocked {
			if _, listed := jobRows[c.name]; listed {
				t.Errorf("jobs lists blocked job")
			}
		} else if jobRows[c.name]["STATE"] != "failed" {
			t.Errorf("jobs row=%v", jobRows[c.name])
		}
	}
}

func runJobIDs(t *testing.T, e *support.Env, project string) (string, map[string]string) {
	t.Helper()
	var shown struct {
		RunID    string `json:"run_id"`
		Commands struct {
			Commands []struct{ ID, Name string } `json:"commands"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", project, "--json").Stdout), &shown); err != nil || shown.RunID == "" {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, command := range shown.Commands.Commands {
		ids[command.Name] = command.ID
	}
	return shown.RunID, ids
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// TestReportNamesTheExecutorTheAttemptRanOn runs a job that names no
// executor and checks that its report names the executor its attempt ran
// on, as the run's job table does, instead of "default".
func TestReportNamesTheExecutorTheAttemptRanOn(t *testing.T) {
	covers(t, "DUR-5")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "exec", "--", "true")
	e.MustRotari("run", "-p", "exec", "--quiet")
	var shown struct {
		Summary struct {
			Results []struct {
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "exec", "--json").Stdout), &shown); err != nil || len(shown.Summary.Results) != 1 {
		t.Fatalf("show --json: %v", err)
	}
	report := e.MustRotari("show", "-j", shown.Summary.Results[0].AttemptID, "--report").Stdout
	if !strings.Contains(report, "- Executor: local\n") {
		t.Fatalf("the report does not name the attempt's executor:\n%s", report)
	}
}

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

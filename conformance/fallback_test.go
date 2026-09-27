package conformance

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Contract DUR-5: a job's result comes from one fallback chain, the same for
// every view: the attempt's `status`, then a terminal `status.json`, then a
// terminal `scheduler_status.json`, and finally the run's summary. See "Job
// execution durability" in contracts/04-coordination-and-safety.md.
//
// The test runs jobs through the binary, then rewrites their attempt files
// so each job is decided by a different step of the chain, and checks that
// `show`, `jobs`, and the Web API report the same outcome for each.

type fallbackCase struct {
	name string
	// command runs the job; edit, if set, then rewrites its attempt files.
	command []string
	edit    func(t *testing.T, attemptDir string)
	// exitCode is the result the chain resolves; blocked marks a job that
	// never ran, whose summary result decides.
	exitCode int
	blocked  bool
}

var fallbackCases = []fallbackCase{
	{name: "status", command: []string{"true"}, exitCode: 3, edit: func(t *testing.T, dir string) {
		writeFile(t, filepath.Join(dir, "status"), "3\n")
	}},
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

func TestStatusFallbackChainAgreesAcrossViews(t *testing.T) {
	covers(t, "DUR-5")
	e := newEnv(t)
	requireUnixSockets(t)
	project := "chain"
	for _, c := range fallbackCases {
		args := []string{"add", "-p", project, "--job-name", c.name}
		if c.blocked {
			args = append(args, "--depends-on", "fail")
		}
		e.mustRotari(append(append(args, "--"), c.command...)...)
	}
	e.rotari("run", "-p", project, "--quiet")
	runID, ids := runJobIDs(t, e, project)
	for _, c := range fallbackCases {
		if c.edit == nil {
			continue
		}
		dirs, err := filepath.Glob(filepath.Join(e.base, "projects", project, "runs", runID, ids[c.name], "attempts", "*"))
		if err != nil || len(dirs) != 1 {
			t.Fatalf("job %s: attempt directories %q, %v", c.name, dirs, err)
		}
		c.edit(t, dirs[0])
	}

	// show RUN counts every job, blocked ones apart.
	summaryLine := "Job status: success: 0, failed: 4, blocked: 1"
	if out := e.mustRotari("show", "-p", project, "--run-id", runID).stdout; !strings.Contains(out, summaryLine) {
		t.Errorf("show of the run does not report %q:\n%s", summaryLine, out)
	}
	jobRows := parseTable(t, e.mustRotari("jobs", project, "--format", "%n %s").stdout)
	base := e.startWeb()
	webJobs := loadWebJobs(t, e, base, project, runID)

	for _, c := range fallbackCases {
		t.Run(c.name, func(t *testing.T) {
			e := e.in(t)
			id := ids[c.name]
			web, ok := webJobs[id]
			switch {
			case !ok || web.Result == nil:
				t.Errorf("Web API has no result for job %s", c.name)
			case web.Result.ExitCode != c.exitCode:
				t.Errorf("Web API exit code %d, want %d", web.Result.ExitCode, c.exitCode)
			case c.blocked && !strings.HasPrefix(web.Result.Error, "blocked"):
				t.Errorf("Web API result error %q, want the summary's blocked result", web.Result.Error)
			}

			if c.blocked {
				report := e.httpGet(base + "/api/report?" + url.Values{"project_name": {project}, "run_id": {runID}, "job_id": {id}}.Encode())
				if report.status != 200 || !strings.Contains(report.body, "Status: blocked") {
					t.Errorf("Web report of the blocked job: status %d:\n%s", report.status, report.body)
				}
			}
			shown := e.rotari("show", "-p", project, "--run-id", runID, "--job-id", id)
			if shown.code != 0 {
				t.Fatalf("show --job-id: %s", shown)
			}
			if c.blocked {
				if !strings.Contains(shown.stdout, "blocked") {
					t.Errorf("show --job-id does not report the blocked result:\n%s", shown.stdout)
				}
			} else if !regexp.MustCompile(`Status:.*\b` + strconv.Itoa(c.exitCode) + `\b`).MatchString(shown.stdout) {
				t.Errorf("show --job-id does not report exit code %d:\n%s", c.exitCode, shown.stdout)
			}

			// jobs lists attempts, so a job that never ran has no row.
			row, listed := jobRows[c.name]
			if c.blocked {
				if listed {
					t.Errorf("jobs lists the blocked job, which never ran: %v", row)
				}
			} else if !listed || row["STATE"] != "failed" {
				t.Errorf("jobs row %v, want state failed", row)
			}
		})
	}
}

// runJobIDs returns project's latest run ID and its job IDs by job name.
func runJobIDs(t *testing.T, e *env, project string) (string, map[string]string) {
	t.Helper()
	var shown struct {
		RunID    string `json:"run_id"`
		Commands struct {
			Commands []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"commands"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", project, "--json").stdout), &shown); err != nil || shown.RunID == "" {
		t.Fatalf("show --json did not describe the run of %s: %v", project, err)
	}
	ids := map[string]string{}
	for _, command := range shown.Commands.Commands {
		ids[command.Name] = command.ID
	}
	return shown.RunID, ids
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

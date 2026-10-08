package interfaces

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestCarriedJobsReadAsCarriedDuringTheRun retries a project whose first job
// succeeded, so the retry carries it while the second job runs, and checks
// every view of the active run: the carried job reads as its success, not
// as running, and job control does not offer it.
func TestCarriedJobsReadAsCarriedDuringTheRun(t *testing.T) {
	covers(t, "DUR-7")
	e := support.NewEnv(t)
	flag := filepath.Join(e.Root, "flag")
	e.MustRotari("add", "-p", "p1", "--job-name", "carried", "--", "true")
	e.MustRotari("add", "-p", "p1", "--job-name", "again", "--", "sh", "-c", "test -f "+flag+" && sleep 30")
	e.Rotari("run", "-p", "p1", "--quiet")
	if err := os.WriteFile(flag, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("retry", "-p", "p1", "--async", "--quiet")
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the retried job did not start"
	})
	t.Cleanup(func() { e.Rotari("cancel", "-p", "p1", "--wait") })
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "p1", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}

	for name, check := range map[string]func() (string, bool){
		"show": func() (string, bool) {
			out := e.MustRotari("show", "-p", "p1", "-r", shown.RunID).Stdout
			return out, strings.Contains(out, "Job status: success: 1, failed: 0, blocked: 0, cancelled: 0, running (recorded): 1") && strings.Contains(out, "success (carried)")
		},
		"lineage": func() (string, bool) {
			out := e.MustRotari("lineage", "-p", "p1", shown.RunID).Stdout
			return out, strings.Contains(out, "succeeded 1") && strings.Contains(out, "unfinished 1")
		},
		"jobs": func() (string, bool) {
			out := e.MustRotari("jobs", "--basedir", e.Base, "p1", "--format", "%n %s").Stdout
			return out, regexp.MustCompile(`(?m)^carried\s+success$`).MatchString(out) && !regexp.MustCompile(`(?m)^carried\s+running$`).MatchString(out)
		},
	} {
		if out, ok := check(); !ok {
			t.Errorf("%s of the active run does not read the carried job as its success:\n%s", name, out)
		}
	}

	response := e.HTTPGet(e.StartWeb() + "/api/run?project_name=p1&run_id=" + url.QueryEscape(shown.RunID))
	var detail struct {
		Jobs []struct {
			Name   string `json:"name"`
			Final  bool   `json:"final"`
			Result *struct {
				ExitCode int `json:"exit_code"`
			} `json:"result"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(response.Body), &detail); err != nil || response.Status != 200 {
		t.Fatalf("GET /api/run: status %d: %s", response.Status, response.Body)
	}
	for _, job := range detail.Jobs {
		if job.Name == "carried" && (!job.Final || job.Result == nil || job.Result.ExitCode != 0) {
			t.Errorf("the Web API of the active run does not read the carried job as its success: %+v", job)
		}
	}

	session := startMCP(t, e)
	var summary mcpRunSummary
	if message := session.call("rotari_run_summary", map[string]any{"run_id": shown.RunID}, &summary); message != "" || summary.State != "running" {
		t.Fatalf("rotari_run_summary: %q %+v", message, summary)
	}
	var counts struct {
		Summary struct {
			Counts struct {
				Succeeded  int `json:"succeeded"`
				Unfinished int `json:"unfinished"`
			} `json:"counts"`
		} `json:"summary"`
	}
	session.call("rotari_run_summary", map[string]any{"run_id": shown.RunID}, &counts)
	if counts.Summary.Counts.Succeeded != 1 || counts.Summary.Counts.Unfinished != 1 {
		t.Errorf("rotari_run_summary counts = %+v, want 1 succeeded and 1 unfinished", counts.Summary.Counts)
	}
	var preview struct {
		JobIDs []string `json:"job_ids"`
	}
	if message := session.call("rotari_preview_job_control", map[string]any{"run_id": shown.RunID, "operation": "cancel"}, &preview); message != "" || len(preview.JobIDs) != 1 {
		t.Errorf("cancel preview = %q %+v, want only the running job", message, preview)
	}
}

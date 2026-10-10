package web

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestRunReportReadsCarriedJobLogs retries a run with one failed job, so the
// retry carries the other job's result. The retry's report, from the CLI and
// from the Web API alike, shows each job's last log line from the attempt
// that produced its result: the carried job's from the first run.
func TestRunReportReadsCarriedJobLogs(t *testing.T) {
	covers(t, "WEB-8")
	e := support.NewEnv(t)
	fixed := filepath.Join(e.Root, "fixed")
	e.MustRotari("add", "-p", "rep", "--job-name", "keep", "--", "sh", "-c", "echo config keep; echo val=0.78")
	e.MustRotari("add", "-p", "rep", "--job-name", "flaky", "--", "sh", "-c", "if [ -f '"+fixed+"' ]; then echo val=0.86; else echo boom; exit 2; fi")
	if r := e.Rotari("run", "-p", "rep", "--quiet"); r.Code == 0 {
		t.Fatalf("the first run should fail: %s", r)
	}
	if err := os.WriteFile(fixed, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("retry", "-p", "rep", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "rep", "--json").Stdout), &shown); err != nil || shown.RunID == "" {
		t.Fatalf("show --json: %v", err)
	}
	cli := e.MustRotari("show", shown.RunID, "--report", "--no-pager").Stdout
	response := e.HTTPGet(e.StartWeb() + "/api/report?project_name=rep&run_id=" + url.QueryEscape(shown.RunID))
	if response.Status != 200 {
		t.Fatalf("GET report: status %d: %s", response.Status, response.Body)
	}
	for name, report := range map[string]string{"show --report": cli, "/api/report": response.Body} {
		for _, want := range []string{"| keep | success (carried) | 0 | `val=0.78` |", "| flaky | success | 0 | `val=0.86` |"} {
			if !strings.Contains(report, want) {
				t.Errorf("%s lacks %q:\n%s", name, want, report)
			}
		}
	}
}

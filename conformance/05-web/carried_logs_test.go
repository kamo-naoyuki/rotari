package web

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

// A job carried through several runs shows the log of the attempt that
// produced its result, however many carries back: in show's job view,
// --logs, and --follow, and in the Web API's log and output word cloud.
func TestCarriedJobLogsFollowEveryCarry(t *testing.T) {
	covers(t, "RES-27")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "chain", "--job-name", "kept", "--", "sh", "-c", "printf 'zebra%sfish\\n' 42")
	e.MustRotari("add", "-p", "chain", "--job-name", "flaky", "--", "sh", "-c", "echo flaky; exit 1")
	// The log line, zebra42fish, appears in no command line.
	// The first run executes kept; each retry carries it from the one
	// before, so the last run's kept is three carries from its attempt.
	e.Rotari("run", "-p", "chain", "--quiet")
	for i := 0; i < 3; i++ {
		e.Rotari("retry", "-p", "chain", "--quiet")
	}
	var shown struct {
		RunID    string `json:"run_id"`
		Commands struct {
			Commands []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"commands"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "chain", "--json").Stdout), &shown); err != nil || shown.RunID == "" {
		t.Fatalf("show --json: %v", err)
	}
	jobID := ""
	for _, command := range shown.Commands.Commands {
		if command.Name == "kept" {
			jobID = command.ID
		}
	}
	if jobID == "" {
		t.Fatalf("no kept job in %+v", shown)
	}
	for name, args := range map[string][]string{
		"show job":    {"show", "-p", "chain", "-r", shown.RunID, "-j", jobID, "--no-pager"},
		"show --logs": {"show", "-p", "chain", "-r", shown.RunID, "--logs", "--no-pager"},
		"--follow":    {"show", "-p", "chain", "-r", shown.RunID, "-j", jobID, "--follow", "--stream", "stdout"},
	} {
		if output := e.MustRotari(args...).Stdout; !strings.Contains(output, "zebra42fish") {
			t.Errorf("%s does not show the carried job's log:\n%s", name, output)
		}
	}
	base := e.StartWeb()
	query := url.Values{"project_name": {"chain"}, "run_id": {shown.RunID}, "job_id": {jobID}, "stream": {"stdout"}}
	if response := e.HTTPGet(base + "/api/log?" + query.Encode()); response.Status != 200 || !strings.Contains(response.Body, "zebra42fish") {
		t.Errorf("/api/log: status %d: %s", response.Status, response.Body)
	}
	cloud := url.Values{"project_name": {"chain"}, "run_id": {shown.RunID}}
	if response := e.HTTPGet(base + "/api/output-word-cloud?" + cloud.Encode()); response.Status != 200 || !strings.Contains(response.Body, "zebra42fish") {
		t.Errorf("/api/output-word-cloud: status %d: %s", response.Status, response.Body)
	}
}

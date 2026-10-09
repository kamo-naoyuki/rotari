package interfaces

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestShowLogsTailInDefinitionOrder runs a matrix whose members print a
// progress line and then a result. show --logs --tail 1 prints only each
// member's result, in the order the matrix defines them, so a sweep's
// results can be read without searching the logs; show -j --tail does the
// same for one job, and --tail without a log view is rejected.
func TestShowLogsTailInDefinitionOrder(t *testing.T) {
	covers(t, "CLI-24")
	e := support.NewEnv(t)
	project := "tail"
	e.MustRotari("add", "-p", project, "--job-name", "train", "--matrix", "LR=0.3,0.1,0.2", "--", "sh", "-c", `echo "step 1"; echo "acc=$LR"`)
	e.MustRotari("run", "-p", project, "--quiet")

	logs := e.MustRotari("show", "-p", project, "--logs", "--tail", "1", "--no-pager").Stdout
	if strings.Contains(logs, "\nstep 1\n") {
		t.Fatalf("--tail 1 printed more than the last line:\n%s", logs)
	}
	var results []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.HasPrefix(line, "acc=") {
			results = append(results, line)
		}
	}
	if got := strings.Join(results, " "); got != "acc=0.3 acc=0.1 acc=0.2" {
		t.Fatalf("results = %q, want the matrix's definition order:\n%s", got, logs)
	}
	if strings.Count(logs, "=== Job: ") != 3 {
		t.Fatalf("--logs lists something other than the run's three jobs:\n%s", logs)
	}

	one := e.MustRotari("show", "-p", project, "--job-name", "train-LR0.1", "--tail", "1", "--no-pager").Stdout
	if strings.Contains(one, "\nstep 1\n") || !strings.Contains(one, "\nacc=0.1\n") {
		t.Fatalf("show -j --tail 1 does not print only the last line:\n%s", one)
	}
	if r := e.Rotari("show", "-p", project, "--tail", "1"); r.Code != 1 || !strings.Contains(r.Stderr, "--tail requires --logs, --failed-logs, or --job-id") {
		t.Fatalf("show --tail without a log view was not rejected: %s", r)
	}
}

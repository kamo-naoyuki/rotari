package coordination

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestJobOutlivesKilledSupervisor(t *testing.T) {
	covers(t, "DUR-1", "DUR-2", "DUR-3", "DUR-4", "DUR-8")
	e := support.NewEnv(t)
	jobID := e.OrphanRun("p", "sleep 2; exit 7")
	interrupted := e.MustRotari("show", "-p", "p").Stdout
	if !strings.Contains(interrupted, "Lifecycle: interrupted") || !strings.Contains(interrupted, "running (recorded)") || !strings.Contains(interrupted, "Client: unknown") {
		t.Errorf("interrupted run does not distinguish lifecycle, recorded job phase, and unknown client state:\n%s", interrupted)
	}
	e.JobExitStatus("p", jobID)
	updated := e.MustRotari("show", "-p", "p").Stdout
	if !strings.Contains(updated, "Lifecycle: interrupted") || !strings.Contains(updated, "failed") {
		t.Errorf("interrupted run did not reflect the wrapper's later terminal status:\n%s", updated)
	}
	if rows := e.MustRotari("jobs", "--basedir", e.Base, "p", "--format", "%s").Stdout; !strings.Contains(rows, "failed") {
		t.Errorf("jobs does not report failure: %s", rows)
	}
	if state := e.CheckState("p"); state != "interrupted" {
		t.Errorf("state %q, want interrupted", state)
	}
}

func TestRecoveryLeavesJobsRunning(t *testing.T) {
	covers(t, "DUR-6")
	e := support.NewEnv(t)
	jobID := e.OrphanRun("p", "sleep 3; exit 7")
	e.MustRotari("unlock", "p")
	if alive := support.JobProcesses(t, e.Root, jobID); alive != 1 {
		t.Fatalf("recovery stopped job: %d processes", alive)
	}
	e.JobExitStatus("p", jobID)
}

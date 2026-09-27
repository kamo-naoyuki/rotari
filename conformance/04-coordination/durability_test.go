package coordination

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestJobOutlivesKilledSupervisor(t *testing.T) {
	covers(t, "DUR-3", "DUR-4")
	e := support.NewEnv(t)
	jobID := e.OrphanRun("p", "sleep 2; exit 7")
	e.JobExitStatus("p", jobID)
	if rows := e.MustRotari("jobs", "p", "--format", "%s").Stdout; !strings.Contains(rows, "failed") {
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
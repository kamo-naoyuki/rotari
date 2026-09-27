package interfaces

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestWaitReturnsCompletedRunExitCode(t *testing.T) {
	covers(t, "RES-16")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	r := e.Rotari("wait", "-p", run.Project, "--run-id", run.RunID)
	// wait returns the run's overall exit code, not the failed job's.
	if r.Code != 1 {
		t.Fatalf("wait exit code = %d, want 1: %s", r.Code, r)
	}
	for _, want := range []string{"=== Run failed ===", "Success: 1", "Failed: 1"} {
		if !strings.Contains(r.Stdout, want) {
			t.Fatalf("wait output does not contain %q:\n%s", want, r.Stdout)
		}
	}
}

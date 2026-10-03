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

func TestWaitPrintsSameCompletionMessageAsRun(t *testing.T) {
	covers(t, "CLI-14")
	e := support.NewEnv(t)
	project := "completion-message"
	e.MustRotari("add", "-p", project, "--", "true")
	run := e.Rotari("run", "-p", project)
	if run.Code != 0 {
		t.Fatalf("run exit code = %d, want 0: %s", run.Code, run)
	}
	start := strings.LastIndex(run.Stdout, "=== Run finished ===")
	if start < 0 {
		t.Fatalf("run output has no completion message:\n%s", run.Stdout)
	}
	completion := run.Stdout[start:]
	wait := e.Rotari("wait", "-p", project)
	if wait.Code != 0 {
		t.Fatalf("wait exit code = %d, want 0: %s", wait.Code, wait)
	}
	if wait.Stdout != completion {
		t.Fatalf("wait completion message differs from run\nrun:  %q\nwait: %q", completion, wait.Stdout)
	}
}

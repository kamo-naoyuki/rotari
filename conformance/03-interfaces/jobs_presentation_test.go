package interfaces

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestJobsTableKeepsVisibleColumnsAligned(t *testing.T) {
	covers(t, "CLI-2")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	output := e.MustRotari("jobs", run.Project, "--format", "%s %a").Stdout
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		t.Fatalf("jobs table has too few rows: %q", output)
	}

	columnStart := strings.Index(lines[0], "ATTEMPT_ID")
	if columnStart < 0 {
		t.Fatalf("jobs table header has no ATTEMPT_ID column: %q", lines[0])
	}
	for _, line := range lines[1:] {
		attemptStart := strings.Index(line, "att_")
		if attemptStart < 0 {
			t.Fatalf("jobs table row has no attempt ID: %q", line)
		}
		if attemptStart != columnStart {
			t.Errorf("jobs table row starts ATTEMPT_ID at %d, want %d: %q", attemptStart, columnStart, line)
		}
	}
}

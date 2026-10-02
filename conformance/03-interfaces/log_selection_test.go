package interfaces

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestShowLogResultSelection(t *testing.T) {
	covers(t, "SEL-11")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	// The retry carries the successful job. Both executed and carried logs
	// must obey the same selection, including the older run explicitly.
	if result := e.Rotari("retry", "-p", run.Project, "--quiet"); result.Code != 1 {
		t.Fatalf("expected a failing retry: %s", result)
	}
	for _, view := range []string{run.RunID, "latest"} {
		for _, selection := range [][]string{{"--failed"}, {"--filter-result", "failed"}} {
			for _, mode := range []string{"--logs", "--failed-logs"} {
				t.Run(view+"/"+mode+"/"+strings.Join(selection, " "), func(t *testing.T) {
					args := []string{"show", "-p", run.Project, "--run-id", view, "--no-pager", mode}
					result := e.MustRotari(append(args, selection...)...)
					if !strings.Contains(result.Stdout, "=== Job: "+run.BadJob) || strings.Contains(result.Stdout, "=== Job: "+run.OKJob) {
						t.Fatalf("log selection kept the wrong jobs: %s", result)
					}
				})
			}
		}
	}
}

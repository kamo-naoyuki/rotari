package interfaces

import (
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

func TestShowNoLogs(t *testing.T) {
	covers(t, "CLI-29")
	for _, mode := range []string{"merge", "separate"} {
		t.Run(mode, func(t *testing.T) {
			e := support.NewEnv(t)
			project := "details"
			plain := support.AddedJobID(t, e.MustRotari("add", "-p", project, "--job-name", "plain", "--log-mode", mode, "--", "sh", "-c", "echo hidden-stdout; echo hidden-stderr >&2"))
			array := support.AddedJobID(t, e.MustRotari("add", "-p", project, "--job-name", "array", "--array", "1-2", "--log-mode", mode, "--", "echo", "hidden-task"))
			e.MustRotari("add", "-p", project, "--job-name", "matrix", "--matrix", "VALUE=a,b", "--log-mode", mode, "--", "echo", "hidden-matrix")
			e.MustRotari("run", "-p", project, "--quiet")
			attempt := lastAttemptID(t, e, project, plain)
			for _, selector := range [][]string{
				{"-j", plain}, {"--job-name", "plain"}, {plain}, {attempt},
				{"-j", array + "-1"}, {"--job-name", "matrix-VALUEa"},
			} {
				args := append([]string{"show", "-p", project, "--no-pager"}, selector...)
				normal := e.MustRotari(args...).Stdout
				details := e.MustRotari(append(args, "--no-logs")...).Stdout
				logStart := strings.Index(normal, "OUTPUT:")
				if mode == "separate" {
					logStart = strings.Index(normal, "STDOUT:")
				}
				if logStart < 0 || !strings.Contains(normal[logStart:], "\nhidden-") {
					t.Fatalf("fixture has no log output: %s", normal)
				}
				if strings.TrimSpace(details) != strings.TrimSpace(normal[:logStart]) {
					t.Fatalf("--no-logs changed details for %v:\nnormal:\n%s\nwithout logs:\n%s", selector, normal, details)
				}
			}
			for _, selector := range [][]string{{}, {"-j", array}} {
				args := append([]string{"show", "-p", project, "--no-logs"}, selector...)
				if result := e.Rotari(args...); result.Code != 1 || !strings.Contains(result.Stderr, "--no-logs requires --job-id") {
					t.Fatalf("unsupported target accepted: %s", result)
				}
			}
		})
	}
	t.Run("running", func(t *testing.T) {
		e := support.NewEnv(t)
		run := e.StartRun("running", 1, true)
		details := e.MustRotari("show", "-p", run.Project, "-j", run.Jobs[0], "--no-logs", "--no-pager").Stdout
		if !strings.Contains(details, "Execution state: running") || !strings.Contains(details, "Command: sleep 300") || strings.Contains(details, "OUTPUT:") {
			t.Fatalf("running job's details: %s", details)
		}
	})
}

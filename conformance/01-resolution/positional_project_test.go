package resolution

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestPositionalProject(t *testing.T) {
	covers(t, "RES-4", "RES-5")
	e := support.NewEnv(t)
	e.FinishedJobRun("a")
	e.FinishedJobRun("b")
	if r := e.Rotari("check", "b"); !strings.Contains(r.Stdout, "project=b") {
		t.Errorf("check b did not check project b: %s", r)
	}
	e.MustRotari("reset", "b", "--quiet")
	for _, command := range []string{"check", "reset"} {
		if r := e.Rotari(command, "-p", "a", "b"); r.Code == 0 || !strings.Contains(r.Stderr, "usage") {
			t.Errorf("%s with both a positional project and --project-name should be a usage error: %s", command, r)
		}
	}
	listed := e.WithVar("ROTARI_PROJECT_NAME", "a").MustRotari("jobs", "--basedir", e.Base, "b", "--format", "%p").Stdout
	if !strings.Contains(listed, "b") || strings.Contains(strings.ReplaceAll(listed, "PROJECT", ""), "a") {
		t.Errorf("jobs b with ROTARI_PROJECT_NAME=a listed:\n%s\nwant only project b", listed)
	}
	if r := e.Rotari("jobs", "-p", "a", "b"); r.Code == 0 || !strings.Contains(r.Stderr, "usage") {
		t.Errorf("jobs with both a positional project and --project-name should be a usage error: %s", r)
	}
}

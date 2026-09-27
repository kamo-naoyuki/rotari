package resolution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMissingProjectIsAnError(t *testing.T) {
	covers(t, "RES-3")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "exists", "--", "true")
	for _, args := range [][]string{{"show", "-p", "nope"}, {"check", "nope"}, {"reset", "nope"}, {"jobs", "nope"}, {"remove", "-p", "nope", "--all"}} {
		r := e.Rotari(args...)
		if r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, `project "nope" does not exist`) {
			t.Errorf("want a missing-project error: %s", r)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "nope")); err == nil {
		t.Error("a command that reads a missing project created it")
	}
}

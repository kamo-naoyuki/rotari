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
	for _, args := range [][]string{{"show", "-p", "nope"}, {"jobs", "--basedir", e.Base, "nope"}, {"remove", "-p", "nope", "--all"}} {
		r := e.Rotari(args...)
		if r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, `project "nope" does not exist`) {
			t.Errorf("want a missing-project error: %s", r)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "nope")); err == nil {
		t.Error("a command that reads a missing project created it")
	}
}

func TestCheckMissingProjectIsEmptyWithoutCreatingIt(t *testing.T) {
	covers(t, "RES-3")
	e := support.NewEnv(t)
	r := e.Rotari("check", "--project-name", "nope", "--json")
	if r.Code != 1 || !strings.Contains(r.Stdout, `"state":"empty"`) || !strings.Contains(r.Stdout, `"queued":0`) || !strings.Contains(r.Stdout, `"runnable":false`) || !strings.Contains(r.Stdout, `"project":"nope"`) {
		t.Fatalf("want empty-project check: %s", r)
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "nope")); !os.IsNotExist(err) {
		t.Fatalf("check created a project: %v", err)
	}
}

func TestResetMissingProjectCreatesEmptyQueue(t *testing.T) {
	covers(t, "RES-3")
	e := support.NewEnv(t)
	r := e.Rotari("reset", "--project-name", "nope")
	if r.Code != 0 || !strings.Contains(r.Stdout, "reset project=nope cleared=0 job(s)") {
		t.Fatalf("want successful reset: %s", r)
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "nope", "meta.json")); err != nil {
		t.Fatalf("reset did not initialize the project: %v", err)
	}
	e.MustRotari("add", "--project-name", "nope", "--", "true")
}

func TestUnlockMissingProjectIsNoOp(t *testing.T) {
	covers(t, "RES-3")
	e := support.NewEnv(t)
	r := e.Rotari("unlock", "--project-name", "nope")
	if r.Code != 0 || !strings.Contains(r.Stdout, "project=nope already unlocked") {
		t.Fatalf("want successful unlock of missing project: %s", r)
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "nope")); !os.IsNotExist(err) {
		t.Fatalf("unlock created a project: %v", err)
	}
	if r := e.Rotari("unlock", "--project-name", "nope", "--run-id", "not-a-run"); r.Code == 0 {
		t.Fatalf("unlock of an explicit missing run succeeded: %s", r)
	}
}

func TestWaitMissingProjectIsNoOp(t *testing.T) {
	covers(t, "RES-3", "RES-16")
	e := support.NewEnv(t)
	for _, args := range [][]string{{"wait", "--project-name", "nope"}, {"wait", "nope"}} {
		if r := e.Rotari(args...); r.Code != 0 {
			t.Errorf("wait for missing project %v failed: %s", args, r)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "nope")); !os.IsNotExist(err) {
		t.Fatalf("wait created project: %v", err)
	}
	for _, args := range [][]string{{"wait", "--project-name", "nope", "--run-id", "missing-run"}, {"wait", "--project-name", "nope", "latest"}} {
		if r := e.Rotari(args...); r.Code == 0 {
			t.Errorf("wait for explicit missing run %v succeeded: %s", args, r)
		}
	}
}

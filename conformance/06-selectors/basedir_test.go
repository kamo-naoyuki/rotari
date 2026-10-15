package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Contract SEL-13: without --basedir, read-only commands look for a project,
// job, or run name first where ROTARI_BASEDIR, ROTARI_PROJECT_NAME, or a
// config file points, and then in the local default and registered base
// directories alike; commands that run or change jobs stay in the resolved
// base directory.
func TestCrossBaseDirSelectors(t *testing.T) {
	covers(t, "SEL-13")
	e := newEnv(t)
	local := filepath.Join(e.root, "state", "rotari")
	other := filepath.Join(e.root, "other")
	l := e.withVar("ROTARI_BASEDIR", local)
	o := e.withVar("ROTARI_BASEDIR", other)
	for _, in := range []*env{e, l, o} {
		in.mustRotari("add", "-p", "home", "--", "true")
		in.mustRotari("run", "-p", "home", "--quiet")
	}
	l.mustRotari("add", "-p", "solo", "--", "true")
	farJob := addedJobID(t, o.mustRotari("add", "-p", "far", "--job-name", "train", "--", "true"))
	o.mustRotari("run", "-p", "far", "--quiet", "--run-name", "weekly")
	queued := addedJobID(t, o.mustRotari("add", "-p", "far", "--job-name", "queued", "--", "true"))

	otherDir := "Base directory: " + other + "\n"
	for _, test := range []struct {
		name string
		in   *env
		args []string
		want string
	}{
		{"positional project", e, []string{"show", "far"}, otherDir},
		{"project option", e, []string{"show", "-p", "far"}, otherDir},
		{"job in a project from the environment", e.withVar("ROTARI_PROJECT_NAME", "far"), []string{"show", farJob}, otherDir},
		{"project from the environment", e.withVar("ROTARI_PROJECT_NAME", "home"), []string{"show"}, "Base directory: " + e.base + "\n"},
		{"job name in a project", e, []string{"show", "-p", "far", "--job-name", "train"}, otherDir},
		{"positional job ID", e, []string{"show", farJob}, otherDir},
		{"job ID option", e, []string{"show", "-j", farJob}, otherDir},
		{"job name", e, []string{"show", "--job-name", "train"}, otherDir},
		{"run name", e, []string{"show", "weekly"}, otherDir},
		{"ROTARI_BASEDIR first", e, []string{"show", "home"}, "Base directory: " + e.base + "\n"},
		{"default base directory", e, []string{"show", "solo"}, "Base directory: " + local + "\n"},
		{"wait for a project", e, []string{"wait", "far"}, "Project: far"},
		{"check a project", e, []string{"check", "far"}, "project=far"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := test.in.in(t).rotari(test.args...)
			if r.code != 0 || !strings.Contains(r.stdout, test.want) {
				t.Fatalf("want success with %q: %s", test.want, r)
			}
		})
	}

	for _, test := range []struct {
		name string
		args []string
	}{
		{"--basedir keeps a project local", []string{"show", "-b", e.base, "far"}},
		{"--basedir keeps a project option local", []string{"show", "-b", e.base, "-p", "far"}},
		{"--basedir keeps a job local", []string{"show", "-b", e.base, "-j", farJob}},
		{"a project keeps a job in its base directory", []string{"show", "-p", "solo", "--job-name", "train"}},
		{"run stays in ROTARI_BASEDIR", []string{"run", "-j", farJob, "--quiet"}},
		{"run of a project stays in ROTARI_BASEDIR", []string{"run", "-p", "far", "--quiet"}},
		{"copy stays in ROTARI_BASEDIR", []string{"copy", "-j", farJob}},
		{"change stays in ROTARI_BASEDIR", []string{"change", "-j", queued, "--retry", "2"}},
		{"remove stays in ROTARI_BASEDIR", []string{"remove", queued}},
		{"cancel stays in ROTARI_BASEDIR", []string{"cancel", farJob}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if r := e.in(t).rotari(test.args...); r.code == 0 {
				t.Fatalf("want an error: %s", r)
			}
		})
	}
	if runs, err := os.ReadDir(filepath.Join(other, "projects", "far", "runs")); err != nil || len(runs) != 1 {
		t.Errorf("far in %s has %d runs, want 1: %v", other, len(runs), err)
	}
	if queue := o.rotari("show", "-p", "far", "--queue").stdout; !strings.Contains(queue, queued) {
		t.Errorf("far's queue in %s lost %s:\n%s", other, queued, queue)
	}

	third := filepath.Join(e.root, "third")
	e.withVar("ROTARI_BASEDIR", third).mustRotari("add", "-p", "far", "--job-name", "train", "--", "true")
	e.withVar("ROTARI_BASEDIR", third).mustRotari("run", "-p", "far", "--quiet")
	for _, args := range [][]string{{"show", "far"}, {"show", "-p", "far"}, {"wait", "far"}, {"show", "--job-name", "train"}} {
		r := e.rotari(args...)
		if r.code == 0 || !strings.Contains(r.stderr, "--basedir") || !strings.Contains(r.stderr, other) || !strings.Contains(r.stderr, third) {
			t.Errorf("want an ambiguity naming both base directories: %s", r)
		}
	}
	// Without ROTARI_BASEDIR, no base directory comes first.
	if r := e.without("ROTARI_BASEDIR").rotari("show", "-p", "home"); r.code == 0 || !strings.Contains(r.stderr, e.base) || !strings.Contains(r.stderr, local) || !strings.Contains(r.stderr, other) {
		t.Errorf("want an ambiguity naming every base directory with home: %s", r)
	}
}

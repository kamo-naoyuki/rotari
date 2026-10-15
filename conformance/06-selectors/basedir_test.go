package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Contract SEL-13: without --basedir, a project, job, or run name that the
// base directory does not have is looked for in the other registered base
// directories.
func TestCrossBaseDirSelectors(t *testing.T) {
	covers(t, "SEL-13")
	e := newEnv(t)
	other := filepath.Join(e.root, "other")
	o := e.withVar("ROTARI_BASEDIR", other)
	e.mustRotari("add", "-p", "home", "--", "true")
	e.mustRotari("run", "-p", "home", "--quiet")
	o.mustRotari("add", "-p", "home", "--", "true")
	o.mustRotari("run", "-p", "home", "--quiet")
	farJob := addedJobID(t, o.mustRotari("add", "-p", "far", "--job-name", "train", "--", "true"))
	o.mustRotari("run", "-p", "far", "--quiet", "--run-name", "weekly")
	queued := addedJobID(t, o.mustRotari("add", "-p", "far", "--job-name", "queued", "--", "true"))

	otherDir := "Base directory: " + other + "\n"
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"positional project", []string{"show", "far"}, otherDir},
		{"positional job ID", []string{"show", farJob}, otherDir},
		{"job ID option", []string{"show", "-j", farJob}, otherDir},
		{"job name", []string{"show", "--job-name", "train"}, otherDir},
		{"run name", []string{"show", "weekly"}, otherDir},
		{"local project first", []string{"show", "home"}, "Base directory: " + e.base + "\n"},
		{"wait for a project", []string{"wait", "far"}, "Project: far"},
		{"change", []string{"change", "-j", queued, "--retry", "2"}, "queue=far"},
		{"remove", []string{"remove", queued}, "queue=far"},
		{"copy", []string{"copy", "-j", farJob}, "to queue=far"},
		{"run", []string{"run", "-j", farJob, "--quiet"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := e.in(t).rotari(test.args...)
			if r.code != 0 || !strings.Contains(r.stdout, test.want) {
				t.Fatalf("want success with %q: %s", test.want, r)
			}
		})
	}
	if runs, err := os.ReadDir(filepath.Join(other, "projects", "far", "runs")); err != nil || len(runs) != 2 {
		t.Errorf("run -j %s ran %d runs of far in %s, want 2: %v", farJob, len(runs), other, err)
	}

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"--basedir keeps a project local", []string{"show", "-b", e.base, "far"}, `selector "far" not found`},
		{"--basedir keeps a job local", []string{"show", "-b", e.base, "-j", farJob}, "not found"},
		{"a project keeps a job local", []string{"show", "-p", "home", "--job-name", "train"}, "not found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if r := e.in(t).rotari(test.args...); r.code == 0 || !strings.Contains(r.stderr, test.want) {
				t.Fatalf("want an error with %q: %s", test.want, r)
			}
		})
	}

	third := filepath.Join(e.root, "third")
	e.withVar("ROTARI_BASEDIR", third).mustRotari("add", "-p", "far", "--job-name", "train", "--", "true")
	e.withVar("ROTARI_BASEDIR", third).mustRotari("run", "-p", "far", "--quiet")
	for _, args := range [][]string{{"show", "far"}, {"wait", "far"}, {"show", "--job-name", "train"}} {
		r := e.rotari(args...)
		if r.code == 0 || !strings.Contains(r.stderr, "--basedir") || !strings.Contains(r.stderr, other) || !strings.Contains(r.stderr, third) {
			t.Errorf("want an ambiguity naming both base directories: %s", r)
		}
	}
}

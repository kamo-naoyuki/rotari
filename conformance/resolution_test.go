package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Contracts RES-1 to RES-19: how commands find the base directory, project,
// and run they act on. See "Resolution rules" in
// contracts/01-resolution-and-config.md.

// projectCreated reports whether project exists in the base directory dir.
func projectCreated(dir, project string) bool {
	_, err := os.Stat(filepath.Join(dir, "projects", project, "queue.json"))
	return err == nil
}

// finishedJobRun queues one successful job in project, runs it, and returns
// the run ID and the job's attempt ID.
func (e *env) finishedJobRun(project string) (runID, attemptID string) {
	e.t.Helper()
	requireUnixSockets(e.t)
	e.mustRotari("add", "-p", project, "--", "true")
	e.mustRotari("run", "-p", project, "--quiet")
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", project, "--json").stdout), &shown); err != nil || shown.RunID == "" || len(shown.Summary.Results) == 0 {
		e.t.Fatalf("show --json did not describe the run of %s: %v", project, err)
	}
	return shown.RunID, shown.Summary.Results[0].AttemptID
}

func TestBaseDirResolutionOrder(t *testing.T) {
	covers(t, "RES-1")
	type layout struct{ flag, env, local, xdg, home string }
	dirs := func(e *env) layout {
		return layout{
			flag:  filepath.Join(e.root, "flag"),
			env:   e.base,
			local: filepath.Join(e.root, ".rotari-state"),
			xdg:   filepath.Join(e.root, "state", "rotari"),
			home:  filepath.Join(e.root, "home", ".local", "state", "rotari"),
		}
	}
	cases := []struct {
		name  string
		setup func(e *env) (*env, []string)
		want  func(l layout) string
	}{
		{"--basedir over ROTARI_BASEDIR", func(e *env) (*env, []string) {
			return e, []string{"-b", dirs(e).flag}
		}, func(l layout) string { return l.flag }},
		{"ROTARI_BASEDIR over ./.rotari-state", func(e *env) (*env, []string) {
			if err := os.MkdirAll(dirs(e).local, 0o755); err != nil {
				t.Fatal(err)
			}
			return e, nil
		}, func(l layout) string { return l.env }},
		{"./.rotari-state over XDG_STATE_HOME", func(e *env) (*env, []string) {
			if err := os.MkdirAll(dirs(e).local, 0o755); err != nil {
				t.Fatal(err)
			}
			return e.without("ROTARI_BASEDIR"), nil
		}, func(l layout) string { return l.local }},
		{"XDG_STATE_HOME over the home default", func(e *env) (*env, []string) {
			return e.without("ROTARI_BASEDIR"), nil
		}, func(l layout) string { return l.xdg }},
		{"the home default last", func(e *env) (*env, []string) {
			return e.without("ROTARI_BASEDIR").without("XDG_STATE_HOME"), nil
		}, func(l layout) string { return l.home }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t).in(t)
			l := dirs(e)
			resolved, flags := c.setup(e)
			resolved.mustRotari(append(append([]string{"add"}, flags...), "-p", "where", "--", "true")...)
			want := c.want(l)
			for _, dir := range []string{l.flag, l.env, l.local, l.xdg, l.home} {
				if got := projectCreated(dir, "where"); got != (dir == want) {
					t.Errorf("project in %s: %t, want it only in %s", dir, got, want)
				}
			}
		})
	}
}

func TestProjectResolutionOrder(t *testing.T) {
	covers(t, "RES-2")
	e := newEnv(t)

	// No project yet: the name is "default".
	e.mustRotari("add", "--", "true")
	if !projectCreated(e.base, "default") {
		t.Fatal(`add without a project did not create "default"`)
	}
	// The only project is chosen.
	var shown struct {
		ProjectName string `json:"project_name"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "--json").stdout), &shown); err != nil || shown.ProjectName != "default" {
		t.Fatalf("show --json chose project %q, want the only project: %v", shown.ProjectName, err)
	}
	// ROTARI_PROJECT_NAME wins over the only project, and --project-name
	// over ROTARI_PROJECT_NAME.
	fromEnv := e.withVar("ROTARI_PROJECT_NAME", "fromenv")
	fromEnv.mustRotari("add", "--", "true")
	fromEnv.mustRotari("add", "-p", "fromflag", "--", "true")
	if !projectCreated(e.base, "fromenv") || !projectCreated(e.base, "fromflag") {
		t.Fatal("ROTARI_PROJECT_NAME or --project-name did not name the project")
	}
	// Several projects need an explicit choice.
	if r := e.rotari("check"); r.code == 0 || !strings.Contains(r.stderr+r.stdout, "fromenv") {
		t.Errorf("check with several projects should fail and list them: %s", r)
	}
}

func TestMissingProjectIsAnError(t *testing.T) {
	covers(t, "RES-3")
	e := newEnv(t)
	e.mustRotari("add", "-p", "exists", "--", "true")
	for _, args := range [][]string{
		{"show", "-p", "nope"},
		{"check", "nope"},
		{"reset", "nope"},
		{"jobs", "nope"},
		{"remove", "-p", "nope", "--all"},
	} {
		r := e.rotari(args...)
		if r.code == 0 || !strings.Contains(r.stderr+r.stdout, `project "nope" does not exist`) {
			t.Errorf("want a missing-project error: %s", r)
		}
	}
	if projectCreated(e.base, "nope") {
		t.Error("a command that reads a missing project created it")
	}
}

func TestPositionalProject(t *testing.T) {
	covers(t, "RES-4", "RES-5")
	e := newEnv(t)
	e.finishedJobRun("a")
	e.finishedJobRun("b")

	// check exits 1 for a project with nothing to run, so its output shows
	// which project it resolved.
	if r := e.rotari("check", "b"); !strings.Contains(r.stdout, "project=b") {
		t.Errorf("check b did not check project b: %s", r)
	}
	e.mustRotari("reset", "b", "--quiet")
	for _, command := range []string{"check", "reset"} {
		if r := e.rotari(command, "-p", "a", "b"); r.code == 0 || !strings.Contains(r.stderr, "usage") {
			t.Errorf("%s with both a positional project and --project-name should be a usage error: %s", command, r)
		}
	}
	listed := e.withVar("ROTARI_PROJECT_NAME", "a").mustRotari("jobs", "b", "--format", "%p").stdout
	if !strings.Contains(listed, "b") || strings.Contains(strings.ReplaceAll(listed, "PROJECT", ""), "a") {
		t.Errorf("jobs b with ROTARI_PROJECT_NAME=a listed:\n%s\nwant only project b", listed)
	}
	if r := e.rotari("jobs", "-p", "a", "b"); r.code == 0 || !strings.Contains(r.stderr, "usage") {
		t.Errorf("jobs with both a positional project and --project-name should be a usage error: %s", r)
	}
}

func TestLatestRunID(t *testing.T) {
	covers(t, "RES-12")
	e := newEnv(t)
	e.finishedJobRun("a")
	second, _ := e.finishedJobRun("a")

	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", "a", "--run-id", "latest", "--json").stdout), &shown); err != nil || shown.RunID != second {
		t.Errorf("--run-id latest chose %q, want the latest run %q: %v", shown.RunID, second, err)
	}
	for _, args := range [][]string{
		{"add", "-p", "latest", "--", "true"},
		{"add", "-p", "a", "--job-name", "latest", "--", "true"},
		{"add", "-p", "a", "--stage", "latest", "--", "true"},
		{"run", "-p", "a", "--run-name", "latest"},
	} {
		if r := e.rotari(args...); r.code == 0 || !strings.Contains(r.stderr, "reserved") {
			t.Errorf("the reserved name latest was accepted: %s", r)
		}
	}
	if projectCreated(e.base, "latest") {
		t.Error(`a project named "latest" was created`)
	}
}

func TestRunIDAloneResolvesLocation(t *testing.T) {
	covers(t, "RES-13")
	e := newEnv(t)
	runID, attemptID := e.finishedJobRun("a")
	// Without ROTARI_BASEDIR the default base directory is elsewhere, so
	// only the run registry can locate the run.
	elsewhere := e.without("ROTARI_BASEDIR")
	for _, id := range []string{runID, attemptID} {
		r := elsewhere.rotari("show", id)
		if r.code != 0 || !strings.Contains(r.stdout, runID) {
			t.Errorf("show %s outside its base directory: %s", id, r)
		}
	}
	var shown struct {
		BaseDir     string `json:"base_dir"`
		ProjectName string `json:"project_name"`
	}
	if err := json.Unmarshal([]byte(elsewhere.mustRotari("show", "--run-id", runID, "--json").stdout), &shown); err != nil || shown.BaseDir != e.base || shown.ProjectName != "a" {
		t.Errorf("show --run-id resolved %q/%q, want %q/a: %v", shown.BaseDir, shown.ProjectName, e.base, err)
	}
}

func TestExplicitLocationMustMatchRegistry(t *testing.T) {
	covers(t, "RES-14")
	e := newEnv(t)
	runID, _ := e.finishedJobRun("a")
	e.mustRotari("add", "-p", "b", "--", "true")

	for _, command := range []string{"show", "wait"} {
		t.Run(command, func(t *testing.T) {
			e := e.in(t)
			for _, args := range [][]string{
				{command, "-b", filepath.Join(e.root, "other"), runID},
				{command, "-p", "b", runID},
			} {
				if r := e.rotari(args...); r.code == 0 || !strings.Contains(r.stderr, "registered under") {
					t.Errorf("a location that disagrees with the registry was accepted: %s", r)
				}
			}
		})
	}
	t.Run("missing explicit run", func(t *testing.T) {
		e := e.in(t)
		if r := e.rotari("show", "-p", "a", "--run-id", "20990101-000000-deadbeef"); r.code == 0 {
			t.Errorf("a missing --run-id fell back instead of failing: %s", r)
		}
	})
}

func TestWaitResolvesRunIDsIndependently(t *testing.T) {
	covers(t, "RES-19")
	e := newEnv(t)
	first, _ := e.finishedJobRun("a")
	second, _ := e.finishedJobRun("b")
	out := e.without("ROTARI_BASEDIR").mustRotari("wait", first, second, "--json").stdout
	var runs []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var summary struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal([]byte(line), &summary); err != nil {
			t.Fatalf("wait --json line %q: %v", line, err)
		}
		runs = append(runs, summary.RunID)
	}
	if strings.Join(runs, ",") != first+","+second {
		t.Errorf("wait --json reported runs %q, want %q and %q", runs, first, second)
	}
}

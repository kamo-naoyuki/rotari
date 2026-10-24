package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/internal/basedirregistry"
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
)

// scopeFixture registers three state directories: local, the default state
// directory under XDG_STATE_HOME, and two others. Each holds projects and
// jobs that tell apart a match in one of them, in two including local, and
// in the two others.
//
//	local:  project "shared" (queue: shared-job "prep"), run "nightly" of "shared"
//	other:  project "shared" (queue: shared-job), project "far" (queue: far-job "train",
//	        both-job), active run of "far" holding active-job, run "weekly" of "far"
//	third:  project "far" (queue: both-job), run "weekly" of "far"
func scopeFixture(t *testing.T) (local, other, third string) {
	t.Helper()
	master := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", master)
	t.Setenv("ROTARI_PROJECT_NAME", "")
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	local, other, third = filepath.Join(xdg, "rotari"), t.TempDir(), t.TempDir()
	t.Setenv("ROTARI_BASEDIR", "")
	for _, baseDir := range []string{local, other, third} {
		if err := basedirregistry.Open(master).Register(baseDir); err != nil {
			t.Fatal(err)
		}
	}
	queue := func(baseDir, project string, commands ...model.QueuedCommand) state.ProjectPaths {
		paths, err := state.ResolveProjectPaths(baseDir, project)
		if err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: commands}); err != nil {
			t.Fatal(err)
		}
		return paths
	}
	savedRun := func(paths state.ProjectPaths, runID, runName string) {
		runDir := filepath.Join(paths.RunsDir, runID)
		if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{}); err != nil {
			t.Fatal(err)
		}
		if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunName: runName}); err != nil {
			t.Fatal(err)
		}
	}
	command := func(id, name string) model.QueuedCommand {
		return model.QueuedCommand{ID: id, Name: name, Command: []string{"true"}}
	}
	savedRun(queue(local, "shared", command("shared-job", "prep")), "20261001-000000-11111111", "nightly")
	queue(other, "shared", command("shared-job", "prep"))
	far := queue(other, "far", command("far-job", "train"), command("both-job", ""))
	savedRun(far, "20261001-000000-22222222", "weekly")
	activeRunID := "20261002-000000-33333333"
	if err := state.WriteJSON(filepath.Join(far.RunsDir, activeRunID, "commands.json"), model.Queue{Commands: []model.QueuedCommand{command("active-job", "")}}); err != nil {
		t.Fatal(err)
	}
	host, _ := os.Hostname()
	if err := state.WriteJSON(far.LockFile, model.LockInfo{PID: os.Getpid(), RunID: activeRunID, Host: host}); err != nil {
		t.Fatal(err)
	}
	savedRun(queue(third, "far", command("both-job", "")), "20261001-000000-44444444", "weekly")
	return local, other, third
}

func TestProjectBaseDirLooksInRegisteredStateDirectories(t *testing.T) {
	local, other, third := scopeFixture(t)
	for _, test := range []struct {
		name, project, preferred string
		pinned                   bool
		want                     string
		wantError                []string
	}{
		{name: "local and other", project: "shared", wantError: []string{local, other}},
		{name: "two others", project: "far", wantError: []string{other, third}},
		{name: "preferred has it", project: "shared", preferred: other, want: other},
		{name: "preferred lacks it", project: "far", preferred: local, wantError: []string{other, third}},
		{name: "missing", project: "nowhere"},
		{name: "pinned", project: "shared", preferred: local, pinned: true, want: local},
		{name: "pinned elsewhere", project: "far", preferred: local, pinned: true},
		{name: "path", project: "../far"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, found, err := ProjectBaseDir(Scope{BaseDir: test.preferred, Pinned: test.pinned}, test.project)
			if len(test.wantError) > 0 {
				for _, want := range append(test.wantError, "--basedir") {
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("ProjectBaseDir(%q) error = %v, want it to name %q", test.project, err, want)
					}
				}
				return
			}
			if err != nil || got != test.want || found != (test.want != "") {
				t.Fatalf("ProjectBaseDir(%q) = %q, %v, %v; want %q", test.project, got, found, err, test.want)
			}
		})
	}
	if err := os.RemoveAll(filepath.Join(third, "projects", "far")); err != nil {
		t.Fatal(err)
	}
	if got, found, err := ProjectBaseDir(Scope{}, "far"); err != nil || !found || got != other {
		t.Fatalf("ProjectBaseDir(far) = %q, %v, %v; want %q", got, found, err, other)
	}
}

// TestSelectorsLookInRegisteredStateDirectories gives every selector search
// the same fixture: a preferred state directory or implicit project that
// has a match wins, a match in one state directory is found there, matches
// in two are ambiguous, and --basedir or a project named on the command line
// keeps the search in one state directory or project.
func TestSelectorsLookInRegisteredStateDirectories(t *testing.T) {
	local, other, third := scopeFixture(t)
	type target struct{ baseDir, project string }
	searches := map[string]func(scope Scope, project, selector string) ([]target, error){
		"Jobs": func(scope Scope, project, selector string) ([]target, error) {
			found, err := Jobs(scope, project, selector, false, true)
			return jobTargets(found, func(job Job) target { return target{job.BaseDir, job.ProjectName} }), err
		},
		"RunsOrJobs": func(scope Scope, project, selector string) ([]target, error) {
			found, err := RunsOrJobs(scope, project, selector)
			return jobTargets(found, func(job Job) target { return target{job.BaseDir, job.ProjectName} }), err
		},
		"QueueWithJobs": func(scope Scope, _, selector string) ([]target, error) {
			run, found, err := QueueWithJobs(scope, model.CommandSelector{IDs: []string{selector}})
			if !found {
				return nil, err
			}
			return []target{{run.BaseDir, run.ProjectName}}, err
		},
	}
	for _, test := range []struct {
		name, selector, preferred, project string
		pinned, explicit                   bool
		want                               []target
		ambiguous                          []string
		err                                string
	}{
		{name: "local and other", selector: "shared-job", ambiguous: []string{local, other}},
		{name: "one other", selector: "far-job", want: []target{{other, "far"}}},
		{name: "two others", selector: "both-job", ambiguous: []string{other, third}},
		{name: "preferred has it", selector: "shared-job", preferred: local, want: []target{{local, "shared"}}},
		{name: "preferred lacks it", selector: "both-job", preferred: local, ambiguous: []string{other, third}},
		{name: "pinned", selector: "shared-job", preferred: local, pinned: true, want: []target{{local, "shared"}}},
		{name: "pinned elsewhere", selector: "far-job", preferred: local, pinned: true},
		{name: "explicit project", selector: "shared-job", preferred: other, project: "shared", explicit: true, want: []target{{other, "shared"}}},
		{name: "explicit project lacks it", selector: "far-job", preferred: local, project: "shared", explicit: true},
		{name: "explicit project in two", selector: "far-job", project: "far", explicit: true, err: "exists in more than one state directory"},
		{name: "implicit project has it", selector: "shared-job", preferred: local, project: "shared", want: []target{{local, "shared"}}},
		{name: "implicit project lacks it", selector: "far-job", preferred: local, project: "shared", want: []target{{other, "far"}}},
	} {
		for searchName, search := range searches {
			if searchName == "QueueWithJobs" && test.project != "" {
				continue
			}
			t.Run(test.name+"/"+searchName, func(t *testing.T) {
				scope := Scope{BaseDir: test.preferred, Pinned: test.pinned, ExplicitProject: test.explicit}
				got, err := search(scope, test.project, test.selector)
				if test.err != "" {
					if err == nil || !strings.Contains(err.Error(), test.err) {
						t.Fatalf("error = %v, want %q", err, test.err)
					}
					return
				}
				if len(test.ambiguous) > 0 {
					if searchName == "QueueWithJobs" {
						if err == nil || !strings.Contains(err.Error(), "basedir="+test.ambiguous[0]) || !strings.Contains(err.Error(), "basedir="+test.ambiguous[1]) {
							t.Fatalf("error = %v, want both state directories", err)
						}
						return
					}
					if err != nil || len(got) != 2 || got[0].baseDir == got[1].baseDir {
						t.Fatalf("got %v, %v; want a candidate in each of %v", got, err, test.ambiguous)
					}
					return
				}
				if err != nil || len(got) != len(test.want) || len(got) == 1 && got[0] != test.want[0] {
					t.Fatalf("got %v, %v; want %v", got, err, test.want)
				}
			})
		}
	}

	runs, err := RunsByName(Scope{BaseDir: local}, "", "weekly", false)
	if err != nil || len(runs) != 2 || runs[0].BaseDir == runs[1].BaseDir {
		t.Fatalf("RunsByName(weekly) = %v, %v; want a run in each other state directory", runs, err)
	}
	runs, err = RunsByName(Scope{BaseDir: local}, "", "nightly", false)
	if err != nil || len(runs) != 1 || runs[0].BaseDir != local {
		t.Fatalf("RunsByName(nightly) = %v, %v; want the local run", runs, err)
	}

	control, err := JobSelection(Scope{BaseDir: local}, "", []string{"active-job"})
	if err != nil || control.BaseDir != other || control.ProjectName != "far" || control.RunID != "20261002-000000-33333333" {
		t.Fatalf("JobSelection(active-job) = %+v, %v; want far's active run in %s", control, err, other)
	}
	if control, err := JobSelection(Scope{BaseDir: local, Pinned: true}, "", []string{"active-job"}); err != nil || control.BaseDir != local {
		t.Fatalf("JobSelection with --basedir = %+v, %v; want the local project", control, err)
	}

	ambiguous := AmbiguousError(`job "both-job"`, []Job{{Run: Run{BaseDir: other, ProjectName: "far"}}, {Run: Run{BaseDir: third, ProjectName: "far"}}})
	if !strings.Contains(ambiguous.Error(), "pass --basedir") || !strings.Contains(ambiguous.Error(), "basedir="+other+" project=far") {
		t.Fatalf("AmbiguousError = %v, want each candidate's state directory", ambiguous)
	}
}

func jobTargets[T any](jobs []Job, convert func(Job) T) []T {
	targets := make([]T, 0, len(jobs))
	for _, job := range jobs {
		targets = append(targets, convert(job))
	}
	return targets
}

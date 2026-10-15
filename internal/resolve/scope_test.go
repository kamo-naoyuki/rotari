package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// scopeFixture registers three state directories: local, which the scope
// resolves to, and two others. Each holds projects and jobs that tell apart
// a local match, a match in one other, and a match in both others.
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
	local, other, third = t.TempDir(), t.TempDir(), t.TempDir()
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
		name, project string
		pinned        bool
		want          string
		wantError     []string
	}{
		{name: "local first", project: "shared", want: local},
		{name: "two others", project: "far", wantError: []string{other, third}},
		{name: "missing", project: "nowhere"},
		{name: "pinned", project: "far", pinned: true},
		{name: "path", project: "../far"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, found, err := ProjectBaseDir(Scope{BaseDir: local, Pinned: test.pinned}, test.project)
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
	if got, found, err := ProjectBaseDir(Scope{BaseDir: local}, "far"); err != nil || !found || got != other {
		t.Fatalf("ProjectBaseDir(far) = %q, %v, %v; want %q", got, found, err, other)
	}
}

// TestSelectorsLookInRegisteredStateDirectories gives every selector search
// the same fixture: a match in the local state directory wins, a match only
// elsewhere is found there, matches in two others are ambiguous, and
// --basedir or a project keeps the search local.
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
		"QueueWithJobs": func(scope Scope, project, selector string) ([]target, error) {
			if project != "" {
				return nil, nil
			}
			run, found, err := QueueWithJobs(scope, model.CommandSelector{IDs: []string{selector}})
			if !found {
				return nil, err
			}
			return []target{{run.BaseDir, run.ProjectName}}, err
		},
	}
	for _, test := range []struct {
		name, selector, project string
		pinned                  bool
		want                    []target
		ambiguous               bool
	}{
		{name: "local first", selector: "shared-job", want: []target{{local, "shared"}}},
		{name: "one other", selector: "far-job", want: []target{{other, "far"}}},
		{name: "two others", selector: "both-job", ambiguous: true},
		{name: "pinned", selector: "far-job", pinned: true},
		{name: "project", selector: "far-job", project: "shared"},
	} {
		for searchName, search := range searches {
			t.Run(test.name+"/"+searchName, func(t *testing.T) {
				got, err := search(Scope{BaseDir: local, Pinned: test.pinned}, test.project, test.selector)
				if test.ambiguous {
					if searchName == "QueueWithJobs" {
						if err == nil || !strings.Contains(err.Error(), "basedir="+other) || !strings.Contains(err.Error(), "basedir="+third) {
							t.Fatalf("error = %v, want both state directories", err)
						}
						return
					}
					if err != nil || len(got) != 2 {
						t.Fatalf("got %v, %v; want a candidate in each other state directory", got, err)
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

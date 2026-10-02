package resolution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestExportResolvesProjectAndRunTargets(t *testing.T) {
	covers(t, "RES-6")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "project", "--", "true")
	projectManifest := filepath.Join(e.Root, "project.yaml")
	e.MustRotari("export", "project", projectManifest)
	assertManifest(t, projectManifest)

	runID, _ := e.FinishedJobRun("run")
	runManifest := filepath.Join(e.Root, "run.yaml")
	e.MustRotari("export", runID, runManifest)
	assertManifest(t, runManifest)

	explicitManifest := filepath.Join(e.Root, "explicit.yaml")
	e.MustRotari("export", "--project-name", "run", runID, explicitManifest)
	assertManifest(t, explicitManifest)
}

func assertManifest(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatalf("export wrote an empty manifest: %s", path)
	}
}

func TestUnlockDerivesInterruptedRun(t *testing.T) {
	covers(t, "RES-7")
	for _, removeLock := range []bool{false, true} {
		removeLock := removeLock
		t.Run(map[bool]string{false: "lock", true: "metadata"}[removeLock], func(t *testing.T) {
			e := support.NewEnv(t)
			e.StartRun("unlock", 1, false)
			support.WaitUntil(t, 15*time.Second, func() (bool, string) {
				return support.JobProcesses(t, e.Root, "") == 1, "the job did not start"
			})
			support.KillStrays(t, e.Root)
			support.WaitForInterrupted(t, e, "unlock")
			if removeLock {
				lockPath := filepath.Join(e.Base, "projects", "unlock", "running.lock")
				if err := os.Remove(lockPath); err != nil {
					t.Fatal(err)
				}
			}
			e.MustRotari("unlock", "unlock")
			if state := e.CheckState("unlock"); state != "ready" {
				t.Fatalf("after unlock: state %q", state)
			}
		})
	}
}

func TestShowBasedirsListsKnownStateDirectories(t *testing.T) {
	covers(t, "RES-8")
	e := support.NewEnv(t)
	e.FinishedJobRun("basedirs")
	out := e.MustRotari("show", "--basedirs").Stdout
	if !strings.Contains(out, "Known state directories: 1") {
		t.Fatalf("show --basedirs did not report one known directory: %s", out)
	}
	if !strings.Contains(out, e.Base) {
		t.Fatalf("show --basedirs omitted %q: %s", e.Base, out)
	}
}

func TestHistoryUsesLastRunThenNewestRun(t *testing.T) {
	covers(t, "RES-15")
	e := support.NewEnv(t)
	first, _ := e.FinishedJobRun("history")
	e.MustRotari("add", "-p", "history", "--", "sh", "-c", "exit 3")
	e.Rotari("run", "-p", "history", "--quiet")
	second := showRunID(t, e.MustRotari("show", "-p", "history", "--json").Stdout)
	if first == second {
		t.Fatalf("second run was not created: %q", second)
	}

	writeLastRunID(t, e, "history", first)
	if got := showRunID(t, e.MustRotari("show", "-p", "history", "--failed", "--json").Stdout); got != first {
		t.Fatalf("history selected %q, want last_run_id %q", got, first)
	}
	writeLastRunID(t, e, "history", "")
	if got := showRunID(t, e.MustRotari("show", "-p", "history", "--failed", "--json").Stdout); got != second {
		t.Fatalf("history selected %q, want newest run %q", got, second)
	}
}

func showRunID(t *testing.T, output string) string {
	t.Helper()
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(output), &shown); err != nil {
		t.Fatal(err)
	}
	return shown.RunID
}

func writeLastRunID(t *testing.T, e *support.Env, project, runID string) {
	t.Helper()
	path := filepath.Join(e.Base, "projects", project, "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if runID == "" {
		delete(meta, "last_run_id")
	} else {
		meta["last_run_id"] = runID
	}
	data, err = json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWaitResolvesActiveAndFinishedSelectors(t *testing.T) {
	covers(t, "RES-16")
	t.Run("one active project is selected", func(t *testing.T) {
		e := support.NewEnv(t)
		run := e.StartRun("single", 1, true)
		r := e.Rotari("wait", "--timeout", "50ms")
		if r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, run.RunID) {
			t.Fatalf("wait without selector did not select the only active run: %s", r)
		}
	})

	t.Run("multiple active projects require selection", func(t *testing.T) {
		e := support.NewEnv(t)
		first := e.StartRun("first", 1, true)
		second := e.StartRun("second", 1, true)
		r := e.Rotari("wait", "--timeout", "50ms")
		out := r.Stderr + r.Stdout
		if r.Code == 0 || !strings.Contains(out, first.Project) || !strings.Contains(out, second.Project) {
			t.Fatalf("wait did not list multiple active projects: %s", r)
		}
	})

	t.Run("finished project returns its latest run", func(t *testing.T) {
		e := support.NewEnv(t)
		runID, _ := e.FinishedJobRun("finished")
		r := e.MustRotari("wait", "finished")
		if !strings.Contains(r.Stdout, runID) {
			t.Fatalf("wait project selector omitted run %q: %s", runID, r)
		}
	})
}

// TestCommandsThatCreateAProjectRegisterItsBasedir creates a project in a
// basedir of its own with each command that can create one, and checks that
// `show --basedirs` lists the basedir after the write but not after its dry
// run, which writes nothing. (copy cannot create a project: it copies from
// one of the project's own runs.)
func TestCommandsThatCreateAProjectRegisterItsBasedir(t *testing.T) {
	covers(t, "RES-8")
	for _, test := range []struct {
		name string
		args func(e *support.Env, baseDir string) []string
	}{
		{"add", func(_ *support.Env, baseDir string) []string {
			return []string{"add", "-b", baseDir, "-p", "fresh", "--", "true"}
		}},
		{"import", func(e *support.Env, baseDir string) []string {
			manifest := filepath.Join(e.Root, "manifest.json")
			if err := os.WriteFile(manifest, []byte(`{"version":1,"jobs":[{"name":"imported","command":["true"]}]}`), 0o600); err != nil {
				t.Fatal(err)
			}
			return []string{"import", "-b", baseDir, manifest, "fresh"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			baseDir := filepath.Join(e.Root, "other-basedir")
			args := test.args(e, baseDir)
			e.MustRotari(append([]string{args[0], "--dry-run"}, args[1:]...)...)
			if out := e.MustRotari("show", "--basedirs").Stdout; strings.Contains(out, baseDir) {
				t.Fatalf("%s --dry-run registered %s:\n%s", test.name, baseDir, out)
			}
			e.MustRotari(args...)
			if out := e.MustRotari("show", "--basedirs").Stdout; !strings.Contains(out, baseDir) {
				t.Fatalf("%s did not register %s:\n%s", test.name, baseDir, out)
			}
		})
	}
}

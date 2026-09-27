package resolution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

package resolution

import (
	"os"
	"path/filepath"
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

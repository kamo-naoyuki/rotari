package coordination

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestRunFilesRemainAuthoritativeWithoutRegistryEntry(t *testing.T) {
	covers(t, "CORE-1")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("p")
	registryEntry := filepath.Join(e.Master, "runs", runID+".json")
	if err := os.Remove(registryEntry); err != nil {
		t.Fatal(err)
	}
	out := e.MustRotari("show", "--basedir", e.Base, "--project-name", "p", "--run-id", runID).Stdout
	if !strings.Contains(out, runID) {
		t.Fatalf("show omitted run %q after registry entry removal: %s", runID, out)
	}
}

func TestPersistedRunStateIsReadableAfterServerShutdown(t *testing.T) {
	covers(t, "CORE-2")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("p")
	if e.Rotari("server", "status").Code == 0 {
		e.MustRotari("server", "shutdown")
	}
	if r := e.Rotari("server", "status"); r.Code == 0 {
		t.Fatalf("server still running: %s", r)
	}
	out := e.MustRotari("show", "--basedir", e.Base, "--project-name", "p", "--run-id", runID).Stdout
	if !strings.Contains(out, runID) {
		t.Fatalf("show omitted persisted run %q after server shutdown: %s", runID, out)
	}
}

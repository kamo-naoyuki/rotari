package runregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestRegisterIsIdempotentAndRejectsConflict(t *testing.T) {
	registry := Open(t.TempDir())
	location := Location{BaseDir: t.TempDir(), ProjectName: "demo", RunID: "run-1"}

	if err := registry.Register(location); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(location); err != nil {
		t.Fatalf("idempotent registration failed: %v", err)
	}
	conflict := location
	conflict.ProjectName = "other"
	if err := registry.Register(conflict); err == nil || !strings.Contains(err.Error(), "another location") {
		t.Fatalf("conflicting registration error = %v", err)
	}

	resolved, found, err := registry.Lookup(location.RunID)
	if err != nil {
		t.Fatal(err)
	}
	wantBaseDir, err := filepath.Abs(location.BaseDir)
	if err != nil {
		t.Fatal(err)
	}
	if !found || resolved.BaseDir != wantBaseDir || resolved.ProjectName != location.ProjectName {
		t.Fatalf("resolved = %+v, found = %v", resolved, found)
	}
}

func TestLookupRejectsMalformedEntry(t *testing.T) {
	masterDir := t.TempDir()
	registry := Open(masterDir)
	if err := os.MkdirAll(filepath.Join(masterDir, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := registry.entryPath("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{invalid}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, found, err := registry.Lookup("run-1"); err == nil || found {
		t.Fatalf("found = %v, error = %v; want malformed-entry error", found, err)
	}
}

func TestEntryPathRejectsUnsafeRunIDs(t *testing.T) {
	registry := Open(t.TempDir())
	for _, runID := range []string{"", ".", "..", "nested/run-1", "../outside", "run\\1", "/tmp/outside"} {
		if _, err := registry.entryPath(runID); err == nil {
			t.Fatalf("entryPath accepted unsafe run ID %q", runID)
		}
	}
}

func TestValidRejectsTraversalInputs(t *testing.T) {
	for _, location := range []Location{
		{BaseDir: "/tmp/rotari", ProjectName: "../demo", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "nested/demo", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "demo", RunID: "../run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "demo", RunID: "nested/run-1"},
		{BaseDir: "relative/base", ProjectName: "demo", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: ".", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "demo", RunID: "."},
	} {
		if location.Valid() {
			t.Fatalf("Valid accepted unsafe location %#v", location)
		}
	}
}

func TestExistsRejectsUnsafeLocations(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !(Location{BaseDir: baseDir, ProjectName: "demo", RunID: "run-1"}).Exists() {
		t.Fatal("Exists rejected an existing run")
	}
	for _, location := range []Location{
		{BaseDir: "relative", ProjectName: "demo", RunID: "run-1"},
		{BaseDir: baseDir, ProjectName: "../outside", RunID: "run-1"},
		{BaseDir: baseDir, ProjectName: "demo", RunID: "../outside"},
	} {
		if location.Exists() {
			t.Fatalf("Exists accepted unsafe location %#v", location)
		}
	}
}

func TestOrphansSkipsBrokenEntries(t *testing.T) {
	masterDir := t.TempDir()
	runsDir := filepath.Join(masterDir, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "broken.json"), []byte("{broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, "invalid.json"), []byte(`{"base_dir":"relative","project_name":"demo","run_id":"run-1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, skipped, err := Open(masterDir).Orphans()
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped = %v, want both broken entries", skipped)
	}
}

func TestLookupRejectsIncompleteEntry(t *testing.T) {
	registry := Open(t.TempDir())
	if err := registry.Register(Location{BaseDir: t.TempDir(), ProjectName: "", RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := registry.Lookup("run-1"); err == nil || found {
		t.Fatalf("Lookup() err=%v found=%v, want invalid registry metadata rejection", err, found)
	}
}

func TestRemoveOrphanKeepsChangedAndReappearedEntries(t *testing.T) {
	registry := Open(t.TempDir())
	baseDir := t.TempDir()
	orphan := Location{BaseDir: baseDir, ProjectName: "demo", RunID: "orphan"}
	changed := Location{BaseDir: baseDir, ProjectName: "demo", RunID: "changed"}
	reappeared := Location{BaseDir: baseDir, ProjectName: "demo", RunID: "reappeared"}
	for _, location := range []Location{orphan, changed, reappeared} {
		if err := registry.Register(location); err != nil {
			t.Fatal(err)
		}
	}
	// After the scan, one entry is rewritten and one run directory returns.
	moved := changed
	moved.ProjectName = "other"
	path, err := registry.entryPath(changed.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", "reappeared"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		planned Location
		removed bool
	}{{orphan, true}, {changed, false}, {reappeared, false}} {
		removed, err := registry.RemoveOrphan(test.planned)
		if err != nil || removed != test.removed {
			t.Errorf("RemoveOrphan(%s) = %v, %v; want %v", test.planned.RunID, removed, err, test.removed)
		}
	}
}

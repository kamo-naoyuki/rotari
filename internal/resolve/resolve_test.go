package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type runLocation = runregistry.Location

func registerRunLocation(location runLocation) error {
	registry, err := runregistry.Default()
	if err != nil {
		return err
	}
	return registry.Register(location)
}

func TestJobSelectionPassesThroughPlainJobIDs(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()

	gotBaseDir, gotProject, gotJobIDs, err := JobSelection(baseDir, "demo", []string{"job-1", "job-2"})
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != baseDir || gotProject != "demo" || strings.Join(gotJobIDs, ",") != "job-1,job-2" {
		t.Fatalf("got = (%q, %q, %v)", gotBaseDir, gotProject, gotJobIDs)
	}
}

func TestJobSelectionResolvesBareRunIDAndStripsIt(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	runID := "20260922-000000-00000000"
	if err := registerRunLocation(runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", runID), 0o755); err != nil {
		t.Fatal(err)
	}

	gotBaseDir, gotProject, gotJobIDs, err := JobSelection("", "", []string{runID, "job-1"})
	if err != nil {
		t.Fatal(err)
	}
	wantBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != wantBaseDir || gotProject != "demo" || strings.Join(gotJobIDs, ",") != "job-1" {
		t.Fatalf("got = (%q, %q, %v)", gotBaseDir, gotProject, gotJobIDs)
	}
}

func TestJobSelectionResolvesAttemptID(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	runID := "20260922-000000-00000000"
	if err := registerRunLocation(runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", runID), 0o755); err != nil {
		t.Fatal(err)
	}
	attemptID := state.MakeAttemptID(runID, "job-1", 0)

	gotBaseDir, gotProject, gotJobIDs, err := JobSelection("", "", []string{attemptID})
	if err != nil {
		t.Fatal(err)
	}
	wantBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != wantBaseDir || gotProject != "demo" || strings.Join(gotJobIDs, ",") != attemptID {
		t.Fatalf("got = (%q, %q, %v)", gotBaseDir, gotProject, gotJobIDs)
	}
}

func TestJobSelectionRejectsMixedRuns(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	otherRunID := "20260922-000000-11111111"
	attemptID := state.MakeAttemptID("20260922-000000-00000000", "job-1", 0)

	if _, _, _, err := JobSelection("", "", []string{otherRunID, attemptID}); err == nil ||
		!strings.Contains(err.Error(), "belongs to run") {
		t.Fatalf("JobSelection() error = %v, want run mismatch error", err)
	}
	if _, _, _, err := JobSelection("", "", []string{otherRunID, "20260922-000000-00000000"}); err == nil ||
		!strings.Contains(err.Error(), "selection mixes run") {
		t.Fatalf("JobSelection() error = %v, want mixed-run error", err)
	}
}

func TestSplitProjectOrRun(t *testing.T) {
	runID := "20260925-120000-12345678"
	project, runIDs, err := SplitProjectOrRun([]string{"demo", runID, "demo"})
	if err != nil || project != "demo" || len(runIDs) != 1 || runIDs[0] != runID {
		t.Fatalf("split = %q, %q, %v", project, runIDs, err)
	}
	if _, _, err := SplitProjectOrRun([]string{"demo", "other"}); err == nil {
		t.Fatal("two different projects were accepted")
	}
}

func TestExistingRunUsesRegistryAndRejectsConflicts(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	location := runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: "run-1"}
	if err := registerRunLocation(location); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}

	gotBaseDir, gotProject, err := ExistingRun("", "", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != baseDir || gotProject != "demo" {
		t.Fatalf("target = %q, %q; want %q, demo", gotBaseDir, gotProject, baseDir)
	}
	if _, _, err := ExistingRun(t.TempDir(), "", "run-1"); err == nil {
		t.Fatal("conflicting basedir was accepted")
	}
	if _, _, err := ExistingRun("", "other", "run-1"); err == nil {
		t.Fatal("conflicting project was accepted")
	}
}

func TestExistingRunRejectsStaleRegistryEntry(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	location := runLocation{BaseDir: t.TempDir(), ProjectName: "demo", RunID: "missing-run"}
	if err := registerRunLocation(location); err != nil {
		t.Fatal(err)
	}

	_, _, err := ExistingRun("", "", location.RunID)
	if err == nil || !strings.Contains(err.Error(), "run \"missing-run\" is registered but its run directory is missing") {
		t.Fatalf("ExistingRun() error = %v, want stale registry error", err)
	}
}

func TestAttemptUsesRunRegistry(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	runID := "20260926-000000-0000abcd"
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := registerRunLocation(runLocation{BaseDir: paths.BaseDir, ProjectName: paths.ProjectName, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	attemptID := state.MakeAttemptID(runID, "job-1", 0)
	gotBaseDir, gotProject, gotRunID, gotJobID, err := Attempt(attemptID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != baseDir || gotProject != "demo" || gotRunID != runID || gotJobID != "job-1" {
		t.Fatalf("resolved attempt = %q, %q, %q, %q", gotBaseDir, gotProject, gotRunID, gotJobID)
	}
}

func TestRunIDUsesValidMetaLastRun(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-from-meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "newer-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := state.DefaultMeta()
	meta.LastRunID = "run-from-meta"
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := RunID(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run-from-meta" {
		t.Fatalf("run ID = %q, want run-from-meta", runID)
	}
}

func TestRunIDFallsBackToNewestRun(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	olderPath := filepath.Join(paths.RunsDir, "older-run")
	newerPath := filepath.Join(paths.RunsDir, "newer-run")
	if err := os.MkdirAll(olderPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newerPath, 0o755); err != nil {
		t.Fatal(err)
	}
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	if err := os.Chtimes(olderPath, older, older); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newerPath, newer, newer); err != nil {
		t.Fatal(err)
	}
	meta := state.DefaultMeta()
	meta.LastRunID = "missing-run"
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := RunID(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "newer-run" {
		t.Fatalf("run ID = %q, want newer-run", runID)
	}
}

func TestRunIDDoesNotFallbackForMissingRequestedRun(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "existing-run"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = RunID(paths, "missing-run")
	if err == nil || !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("error = %v, want exact missing-run error", err)
	}
}

func TestRunIDResolvesLatestAlias(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "latest-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := state.DefaultMeta()
	meta.LastRunID = "latest-run"
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := RunID(paths, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "latest-run" {
		t.Fatalf("run ID = %q, want latest-run", runID)
	}
}

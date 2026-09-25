package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterRunLocationIsIdempotentAndRejectsConflict(t *testing.T) {
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	location := runLocation{BaseDir: t.TempDir(), ProjectName: "demo", RunID: "run-1"}

	if err := registerRunLocation(location); err != nil {
		t.Fatal(err)
	}
	if err := registerRunLocation(location); err != nil {
		t.Fatalf("idempotent registration failed: %v", err)
	}
	conflict := location
	conflict.ProjectName = "other"
	if err := registerRunLocation(conflict); err == nil || !strings.Contains(err.Error(), "another location") {
		t.Fatalf("conflicting registration error = %v", err)
	}

	resolved, found, err := resolveRunLocation(location.RunID)
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

func TestResolveRunLocationRejectsMalformedEntry(t *testing.T) {
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	dir := filepath.Join(masterDir, "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := runLocationPath(dir, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{invalid}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, found, err := resolveRunLocation("run-1"); err == nil || found {
		t.Fatalf("found = %v, error = %v; want malformed-entry error", found, err)
	}
}

func TestRunLocationPathRejectsUnsafeRunIDs(t *testing.T) {
	for _, runID := range []string{"", "../run-1", "nested/run-1"} {
		if _, err := runLocationPath(t.TempDir(), runID); err == nil {
			t.Fatalf("run ID %q was accepted", runID)
		}
	}
}

func TestValidRunRegistryLocationRejectsTraversalInputs(t *testing.T) {
	for _, location := range []runLocation{
		{BaseDir: "/tmp/rotari", ProjectName: "../demo", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "nested/demo", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "demo", RunID: "../run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "demo", RunID: "nested/run-1"},
		{BaseDir: "relative/base", ProjectName: "demo", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: ".", RunID: "run-1"},
		{BaseDir: "/tmp/rotari", ProjectName: "demo", RunID: "."},
	} {
		if validRunRegistryLocation(location) {
			t.Fatalf("validRunRegistryLocation accepted unsafe location %#v", location)
		}
	}
}

func TestDeleteRunRejectsUnsafeRunIDs(t *testing.T) {
	paths := pathSet{RunsDir: t.TempDir()}
	for _, runID := range []string{"", "../run-1", "nested/run-1", "run/..", "run/."} {
		if err := deleteRun(paths, runID); err == nil {
			t.Fatalf("deleteRun accepted unsafe run ID %q", runID)
		}
	}
}

func TestResolveJobSelectionTargetPassesThroughPlainJobIDs(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()

	gotBaseDir, gotProject, gotJobIDs, err := resolveJobSelectionTarget(baseDir, "demo", []string{"job-1", "job-2"})
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != baseDir || gotProject != "demo" || strings.Join(gotJobIDs, ",") != "job-1,job-2" {
		t.Fatalf("got = (%q, %q, %v)", gotBaseDir, gotProject, gotJobIDs)
	}
}

func TestResolveJobSelectionTargetResolvesBareRunIDAndStripsIt(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	runID := "20260922-000000-00000000"
	if err := registerRunLocation(runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", runID), 0o755); err != nil {
		t.Fatal(err)
	}

	gotBaseDir, gotProject, gotJobIDs, err := resolveJobSelectionTarget("", "", []string{runID, "job-1"})
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

func TestResolveJobSelectionTargetResolvesAttemptID(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	runID := "20260922-000000-00000000"
	if err := registerRunLocation(runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", runID), 0o755); err != nil {
		t.Fatal(err)
	}
	attemptID := makeAttemptID(runID, "job-1", 0)

	gotBaseDir, gotProject, gotJobIDs, err := resolveJobSelectionTarget("", "", []string{attemptID})
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

func TestResolveJobSelectionTargetRejectsMixedRuns(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	otherRunID := "20260922-000000-11111111"
	attemptID := makeAttemptID("20260922-000000-00000000", "job-1", 0)

	if _, _, _, err := resolveJobSelectionTarget("", "", []string{otherRunID, attemptID}); err == nil ||
		!strings.Contains(err.Error(), "belongs to run") {
		t.Fatalf("resolveJobSelectionTarget() error = %v, want run mismatch error", err)
	}
	if _, _, _, err := resolveJobSelectionTarget("", "", []string{otherRunID, "20260922-000000-00000000"}); err == nil ||
		!strings.Contains(err.Error(), "selection mixes run") {
		t.Fatalf("resolveJobSelectionTarget() error = %v, want mixed-run error", err)
	}
}

func TestSplitProjectOrRunSelectors(t *testing.T) {
	runID := "20260925-120000-12345678"
	project, runIDs, err := splitProjectOrRunSelectors([]string{"demo", runID, "demo"})
	if err != nil || project != "demo" || len(runIDs) != 1 || runIDs[0] != runID {
		t.Fatalf("split = %q, %q, %v", project, runIDs, err)
	}
	if _, _, err := splitProjectOrRunSelectors([]string{"demo", "other"}); err == nil {
		t.Fatal("two different projects were accepted")
	}
}

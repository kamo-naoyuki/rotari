package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/runregistry"
)

func TestRunRegistryGCFindsAndAppliesOnlyOrphans(t *testing.T) {
	masterDir := t.TempDir()
	baseDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	locations := []runLocation{
		{BaseDir: baseDir, ProjectName: "demo", RunID: "missing"},
		{BaseDir: baseDir, ProjectName: "demo", RunID: "live"},
	}
	for _, location := range locations {
		if err := registerRunLocation(location); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", "live"), 0o755); err != nil {
		t.Fatal(err)
	}

	orphans, skipped, err := runregistry.Open(masterDir).Orphans()
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 || orphans[0].RunID != "missing" {
		t.Fatalf("orphans = %+v, want only missing run", orphans)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, want none", skipped)
	}
	cache := runRegistryGCCache{CreatedAt: nowRFC3339(), Entries: orphans}
	if err := writeJSON(filepath.Join(masterDir, "gc.json"), cache); err != nil {
		t.Fatal(err)
	}

	if code := applyRunRegistryGC(masterDir); code != 0 {
		t.Fatalf("applyRunRegistryGC() = %d, want 0", code)
	}
	if _, found, err := resolveRunLocation("missing"); err != nil || found {
		t.Fatalf("missing entry: found=%v, err=%v; want removed", found, err)
	}
	if _, found, err := resolveRunLocation("live"); err != nil || !found {
		t.Fatalf("live entry: found=%v, err=%v; want retained", found, err)
	}
}

func TestApplyRunRegistryGCRejectsExpiredPlan(t *testing.T) {
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	cache := runRegistryGCCache{CreatedAt: "2000-01-01T00:00:00Z"}
	if err := writeJSON(filepath.Join(masterDir, "gc.json"), cache); err != nil {
		t.Fatal(err)
	}
	if code := applyRunRegistryGC(masterDir); code == 0 {
		t.Fatal("applyRunRegistryGC() succeeded with an expired plan")
	}
}

func TestApplyRunRegistryGCSkipsChangedOrReappearedEntries(t *testing.T) {
	masterDir := t.TempDir()
	baseDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	changed := runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: "changed"}
	reappeared := runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: "reappeared"}
	for _, location := range []runLocation{changed, reappeared} {
		if err := registerRunLocation(location); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeJSON(filepath.Join(masterDir, "gc.json"), runRegistryGCCache{
		CreatedAt: nowRFC3339(), Entries: []runLocation{changed, reappeared},
	}); err != nil {
		t.Fatal(err)
	}
	changed.ProjectName = "other"
	if err := writeJSON(filepath.Join(masterDir, "runs", "changed.json"), changed); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", "reappeared"), 0o755); err != nil {
		t.Fatal(err)
	}

	if code := applyRunRegistryGC(masterDir); code != 0 {
		t.Fatalf("applyRunRegistryGC() = %d, want 0", code)
	}
	if _, found, err := resolveRunLocation("changed"); err != nil || !found {
		t.Fatalf("changed entry: found=%v, err=%v; want retained", found, err)
	}
	if _, found, err := resolveRunLocation("reappeared"); err != nil || !found {
		t.Fatalf("reappeared entry: found=%v, err=%v; want retained", found, err)
	}
}

func TestApplyRunRegistryGCRejectsFuturePlan(t *testing.T) {
	masterDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	if err := writeJSON(filepath.Join(masterDir, "gc.json"), runRegistryGCCache{
		CreatedAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	if applyRunRegistryGC(masterDir) == 0 {
		t.Fatal("applyRunRegistryGC() succeeded with a future-dated plan")
	}
}

func TestCmdGCUsesExplicitMasterDirectory(t *testing.T) {
	masterDir := t.TempDir()
	if code := cmdGC([]string{"--masterdir", masterDir}); code != 0 {
		t.Fatalf("cmdGC() = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(masterDir, "gc.json")); err != nil {
		t.Fatalf("GC plan was not written: %v", err)
	}
}

func TestCmdGCAcceptsPositionalMasterDirectory(t *testing.T) {
	masterDir := t.TempDir()
	t.Setenv(envMasterDir, t.TempDir())
	if code := cmdGC([]string{masterDir}); code != 0 {
		t.Fatalf("cmdGC positional master directory = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(masterDir, "gc.json")); err != nil {
		t.Fatalf("GC plan was not written to positional master directory: %v", err)
	}
	if code := cmdGC([]string{"--apply", masterDir}); code != 0 {
		t.Fatalf("cmdGC positional master directory apply = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(masterDir, "gc.json")); !os.IsNotExist(err) {
		t.Fatalf("GC plan was not removed after positional apply: %v", err)
	}
	if code := cmdGC([]string{"--masterdir", masterDir, t.TempDir()}); code != 1 {
		t.Fatalf("cmdGC accepted positional master directory with --masterdir: exit code = %d", code)
	}
}

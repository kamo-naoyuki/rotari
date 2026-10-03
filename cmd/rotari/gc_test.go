package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
)

// gcFixture registers an orphan run, a live run, and a basedir that is gone.
func gcFixture(t *testing.T) (masterDir, missingBaseDir string) {
	t.Helper()
	masterDir = t.TempDir()
	baseDir := t.TempDir()
	missingBaseDir = filepath.Join(t.TempDir(), "removed-basedir")
	t.Setenv("ROTARI_MASTERDIR", masterDir)
	if err := basedirregistry.Open(masterDir).Register(missingBaseDir); err != nil {
		t.Fatal(err)
	}
	for _, location := range []runLocation{
		{BaseDir: baseDir, ProjectName: "demo", RunID: "missing"},
		{BaseDir: baseDir, ProjectName: "demo", RunID: "live"},
	} {
		if err := registerRunLocation(location); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", "live"), 0o755); err != nil {
		t.Fatal(err)
	}
	return masterDir, missingBaseDir
}

func registered(t *testing.T, runID string) bool {
	t.Helper()
	_, found, err := resolveRunLocation(runID)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestCmdGCRemovesOnlyOrphansAndDryRunKeepsThem(t *testing.T) {
	masterDir, missingBaseDir := gcFixture(t)

	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int { return cmdGC([]string{"--dry-run", masterDir}) }); code != 0 {
		t.Fatalf("gc --dry-run = %d:\n%s", code, output.String())
	}
	for _, want := range []string{"dry run: found 1 orphan run registry entry", "missing ->", "dry run: found 1 missing basedir registry", missingBaseDir} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("gc --dry-run output does not contain %q:\n%s", want, output.String())
		}
	}
	if !registered(t, "missing") {
		t.Fatal("gc --dry-run removed the orphan entry")
	}

	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdGC([]string{"--masterdir", masterDir}) }); code != 0 {
		t.Fatalf("gc = %d:\n%s", code, output.String())
	}
	if !strings.Contains(output.String(), "removed 1 orphan run registry entry") || !strings.Contains(output.String(), "removed 1 missing basedir registry") {
		t.Errorf("gc output:\n%s", output.String())
	}
	if registered(t, "missing") || !registered(t, "live") {
		t.Fatal("gc did not remove only the orphan entry")
	}
	if baseDirs, err := basedirregistry.Open(masterDir).BaseDirs(); err != nil || len(baseDirs) != 0 {
		t.Fatalf("basedir registry after gc = %v, %v; want empty", baseDirs, err)
	}
}

func TestCmdGCRejectsTwoMasterDirectories(t *testing.T) {
	masterDir := t.TempDir()
	if code := cmdGC([]string{"--masterdir", masterDir, t.TempDir()}); code != 1 {
		t.Fatalf("cmdGC accepted positional master directory with --masterdir: exit code = %d", code)
	}
}

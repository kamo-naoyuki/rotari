package main

import (
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdWaitTimesOutForMalformedSummary(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-1"
	summaryPath := filepath.Join(paths.RunsDir, runID, "summary.json")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(summaryPath, []byte("{incomplete"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdWait([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", runID, "--timeout", "1ms"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "timed out waiting for run run-1") {
		t.Fatalf("cmdWait exit code = %d, stderr = %q", code, output)
	}
}

func TestCmdWaitRejectsMissingRegisteredRunDirectory(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, "missing-run"); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdWait([]string{"--run-id", "missing-run"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "run \"missing-run\" is registered but its run directory is missing") {
		t.Fatalf("cmdWait exit code = %d, stderr = %q", code, output)
	}
}

func TestResolveActiveRunTarget(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, defaultMeta()); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
		t.Fatal(err)
	}

	runID, err := resolveActiveRunTarget(baseDir, "demo")
	if err != nil {
		t.Fatalf("resolveActiveRunTarget returned error: %v", err)
	}
	if runID != "active-run" {
		t.Fatalf("run ID = %q, want active-run", runID)
	}
}

func TestResolveWaitTargetByProjectRunNameAndRunID(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "build")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-id", RunName: "nightly"}); err != nil {
		t.Fatal(err)
	}
	// A registered run has its directory, which a run ID selector requires
	// like --run-id does.
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-id"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, "run-id"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		selector string
		want     resolve.Run
	}{
		{selector: "build", want: resolve.Run{BaseDir: baseDir, ProjectName: "build", RunID: "run-id"}},
		{selector: "nightly", want: resolve.Run{BaseDir: baseDir, ProjectName: "build", RunID: "run-id"}},
		{selector: "run-id", want: resolve.Run{BaseDir: baseDir, ProjectName: "build", RunID: "run-id"}},
	}
	for _, test := range tests {
		got, err := resolveWaitTarget(baseDir, "", test.selector)
		if err != nil {
			t.Fatalf("resolveWaitTarget(%q): %v", test.selector, err)
		}
		if got != test.want {
			t.Fatalf("resolveWaitTarget(%q) = %#v, want %#v", test.selector, got, test.want)
		}
	}
}

func TestResolveWaitTargetRejectsAmbiguousRunName(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	for _, projectName := range []string{"alpha", "beta"} {
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: projectName + "-run", RunName: "nightly"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := resolveWaitTarget(baseDir, "", "nightly"); err == nil || !strings.Contains(err.Error(), "matches more than one target") {
		t.Fatalf("resolveWaitTarget() error = %v, want ambiguity error", err)
	}
}

func TestResolveActiveWaitTargetsFindsAllProjects(t *testing.T) {
	baseDir := t.TempDir()
	for _, projectName := range []string{"beta", "alpha"} {
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: projectName + "-run"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := resolveActiveWaitTargets(baseDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ProjectName != "alpha" || got[1].ProjectName != "beta" {
		t.Fatalf("active targets = %#v, want alpha then beta", got)
	}
}

func TestCmdWaitReportsInterruptedRunWithoutSummary(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths := writeInterruptedResetProject(t, baseDir)

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdWait([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", "run-1", "--timeout", "10s"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "run run-1 was interrupted before it wrote a summary") || strings.Contains(string(output), "timed out") {
		t.Fatalf("cmdWait exit code = %d, stderr = %q", code, output)
	}
	if !strings.Contains(string(output), "rotari unlock") {
		t.Fatalf("cmdWait stderr does not suggest unlock: %q", output)
	}
	if _, err := os.Stat(paths.MetaFile); err != nil {
		t.Fatalf("cmdWait must not change project state: %v", err)
	}
}

func TestCmdWaitKeepsWaitingForActiveRunWithoutSummary(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths := writeInterruptedResetProject(t, baseDir)
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdWait([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", "run-1", "--timeout", "1ms"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "timed out waiting for run run-1") {
		t.Fatalf("cmdWait exit code = %d, stderr = %q", code, output)
	}
}

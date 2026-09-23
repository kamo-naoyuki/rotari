package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdWaitReturnsCompletedRunExitCode(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-1"
	summary := RunSummary{
		RunID: runID, RunName: "nightly", Status: "failed", ExitCode: 2,
		Results: []JobResult{{ID: "job-1", ExitCode: 0}, {ID: "job-2", ExitCode: 2}},
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), summary); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdWait([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", runID})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("cmdWait exit code = %d, want 2", code)
	}
	for _, want := range []string{"=== Run failed ===", "nightly", "Success: 1", "Failed: 1"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("cmdWait output does not contain %q:\n%s", want, output)
		}
	}
}

func TestCmdWaitAcceptsMultipleRunIDs(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())

	baseDir := t.TempDir()
	pathsA, err := resolvePaths(baseDir, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	pathsB, err := resolvePaths(baseDir, "beta")
	if err != nil {
		t.Fatal(err)
	}
	runA := "run-a"
	runB := "run-b"
	if err := writeJSON(filepath.Join(pathsA.RunsDir, runA, "summary.json"), RunSummary{RunID: runA, Status: "success", Results: []JobResult{{ID: "job-a", ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(pathsB.RunsDir, runB, "summary.json"), RunSummary{RunID: runB, Status: "failed", ExitCode: 3, Results: []JobResult{{ID: "job-b", ExitCode: 3}}}); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(pathsA, runA); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(pathsB, runB); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdWait([]string{"--run-id", runA, runB})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Fatalf("cmdWait exit code = %d, want 3", code)
	}
	for _, want := range []string{"Project: alpha", "Project: beta", "Run: run-a", "Run: run-b"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("cmdWait output does not contain %q:\n%s", want, output)
		}
	}
}

func TestCmdWaitTimesOutForMalformedSummary(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
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
	paths, err := resolvePaths(baseDir, "demo")
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
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, defaultMeta()); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "active-run"}); err != nil {
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
	paths, err := resolvePaths(baseDir, "build")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "run-id", RunName: "nightly"}); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, "run-id"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		selector string
		want     waitTarget
	}{
		{selector: "build", want: waitTarget{baseDir: baseDir, projectName: "build", runID: "run-id"}},
		{selector: "nightly", want: waitTarget{baseDir: baseDir, projectName: "build", runID: "run-id"}},
		{selector: "run-id", want: waitTarget{baseDir: baseDir, projectName: "build", runID: "run-id"}},
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
		paths, err := resolvePaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: projectName + "-run", RunName: "nightly"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := resolveWaitTarget(baseDir, "", "nightly"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("resolveWaitTarget() error = %v, want ambiguity error", err)
	}
}

func TestResolveActiveWaitTargetsFindsAllProjects(t *testing.T) {
	baseDir := t.TempDir()
	for _, projectName := range []string{"beta", "alpha"} {
		paths, err := resolvePaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: projectName + "-run"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := resolveActiveWaitTargets(baseDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].projectName != "alpha" || got[1].projectName != "beta" {
		t.Fatalf("active targets = %#v, want alpha then beta", got)
	}
}

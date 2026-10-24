package main

import (
	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeleteAllRunsResetsMetadata(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"run-1", "run-2"} {
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := registerRun(paths, runID); err != nil {
			t.Fatal(err)
		}
	}
	meta := defaultMeta()
	meta.Phase = "finished"
	meta.LastRunID = "run-2"
	meta.LastRunExitCode = 7
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	if code := cmdDelete([]string{"--basedir", baseDir, "--project-name", "demo"}); code == 0 {
		t.Fatal("cmdDelete without a run or --all succeeded")
	}
	if _, err := os.Stat(paths.RunsDir); err != nil {
		t.Fatalf("cmdDelete without a run or --all removed history: %v", err)
	}
	if code := cmdDelete([]string{"--basedir", baseDir, "--project-name", "demo", "--all"}); code != 0 {
		t.Fatalf("cmdDelete exit code = %d, want 0", code)
	}
	if _, err := os.Stat(paths.RunsDir); !os.IsNotExist(err) {
		t.Fatalf("runs directory remains after deleting all history: %v", err)
	}
	updated, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Phase != "collecting" || updated.LastRunID != "" || updated.LastRunExitCode != 0 {
		t.Fatalf("metadata = %+v, want reset collecting state", updated)
	}
	for _, runID := range []string{"run-1", "run-2"} {
		if _, found, err := resolveRunLocation(runID); err != nil || found {
			t.Fatalf("registry entry for %q: found=%v, err=%v; want removed", runID, found, err)
		}
	}
}

func TestClearRunHistoryRemovesRegistryEntry(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-1"
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}

	if err := queueEditor().DeleteRun(baseDir, "demo", runID); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}
	if _, found, err := resolveRunLocation(runID); err != nil || found {
		t.Fatalf("registry entry: found=%v, err=%v; want removed", found, err)
	}
}

func TestCmdDeleteRejectsUnsafeProjectAndRunIDsAtCLI(t *testing.T) {
	baseDir := t.TempDir()
	for _, args := range [][]string{
		{"--basedir", baseDir, "--project-name", "../outside", "--all"},
		{"--basedir", baseDir, "--project-name", "demo", "--run-id", "../outside"},
		{"--basedir", baseDir, "--project-name", "demo", "--run-id", "nested/run-1"},
	} {
		if cmdDelete(args) == 0 {
			t.Fatalf("cmdDelete accepted unsafe CLI input: %v", args)
		}
	}

	if cmdAdd([]string{"--basedir", baseDir, "--project-name", "../outside", "echo", "ok"}) == 0 {
		t.Fatal("cmdAdd accepted unsafe project name through CLI")
	}
}

// writeDeleteProject writes an idle project with one registered run.
func writeDeleteProject(t *testing.T, baseDir, name, runID string) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, runID)
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestCmdDeleteProjectNeedsAnExplicitProject(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths := writeDeleteProject(t, baseDir, "demo", "run-1")
	t.Setenv(envProjectName, "demo")
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"project from the environment", []string{"--project"}, "--project needs the project named with --project-name/-p"},
		{"run ID", []string{"--project", "-p", "demo", "run-1"}, "cannot be combined with a run ID or --all"},
		{"run-id option", []string{"--project", "-p", "demo", "--run-id", "run-1"}, "cannot be combined with a run ID or --all"},
		{"all runs", []string{"--project", "-p", "demo", "--all"}, "cannot be combined with a run ID or --all"},
	} {
		t.Run(test.name, func(t *testing.T) {
			code, stderr := captureStderr(t, func() int { return cmdDelete(append([]string{"--basedir", baseDir}, test.args...)) })
			if code != 1 || !strings.Contains(stderr, test.want) {
				t.Fatalf("exit code = %d, stderr = %q; want %q", code, stderr, test.want)
			}
			if _, err := os.Stat(paths.MetaFile); err != nil {
				t.Fatalf("refused delete removed the project: %v", err)
			}
		})
	}
}

func TestCmdDeleteProjectRemovesProjectAndItsRuns(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths := writeDeleteProject(t, baseDir, "demo", "run-1")
	other := writeDeleteProject(t, baseDir, "other", "run-2")
	if code := cmdDelete([]string{"--basedir", baseDir, "--project", "-p", "demo"}); code != 0 {
		t.Fatalf("cmdDelete exit code = %d", code)
	}
	if _, err := os.Stat(paths.ProjectDir); !os.IsNotExist(err) {
		t.Fatalf("project directory still exists: %v", err)
	}
	if _, err := os.Stat(other.MetaFile); err != nil {
		t.Fatalf("other project was removed: %v", err)
	}
	if _, found, err := resolveRunLocation("run-1"); err != nil || found {
		t.Fatalf("deleted run registry entry: found=%v, err=%v", found, err)
	}
	if _, found, err := resolveRunLocation("run-2"); err != nil || !found {
		t.Fatalf("other run registry entry: found=%v, err=%v", found, err)
	}
}

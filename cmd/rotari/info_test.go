package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCmdInfoReportsContextAndActiveRun(t *testing.T) {
	baseDir := t.TempDir()
	masterDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.ProjectDir, "runs", "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.json"), []byte(`{"executor":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	projectConfig := filepath.Join(paths.ProjectDir, "config.json")
	if err := os.WriteFile(projectConfig, []byte(`{"executor":"local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	code := captureShowStdout(t, &output, func() int {
		return run([]string{"info", "--basedir", baseDir, "--project-name", "demo", "--masterdir", masterDir, "--json"})
	})
	if code != 0 {
		t.Fatalf("info exit code = %d, output = %s", code, output.String())
	}
	var report infoReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode info JSON: %v\n%s", err, output.String())
	}
	if report.MasterDir != masterDir || report.BaseDir != baseDir || report.Project != "demo" {
		t.Fatalf("resolved context = master %q, basedir %q, project %q", report.MasterDir, report.BaseDir, report.Project)
	}
	if !containsConfigSource(report.LoadedConfigs, filepath.Join(baseDir, "config.json")) || !containsConfigSource(report.LoadedConfigs, projectConfig) || len(report.VisibleConfigs.Common) == 0 || len(report.VisibleConfigs.Projects["demo"]) == 0 {
		t.Fatalf("config visibility = %#v, loaded = %#v", report.VisibleConfigs, report.LoadedConfigs)
	}
	if len(report.RunLocks) != 1 || report.RunLocks[0].State != state.LockActive || report.RunLocks[0].Coordinator == nil || !*report.RunLocks[0].Coordinator {
		t.Fatalf("run locks = %#v, want an active local coordinator", report.RunLocks)
	}
	if len(report.ActiveRuns) != 1 || report.ActiveRuns[0].RunID != "run-1" {
		t.Fatalf("active runs = %#v, want run-1", report.ActiveRuns)
	}
}

func containsConfigSource(sources []config.Source, path string) bool {
	for _, source := range sources {
		if source.Path == path {
			return true
		}
	}
	return false
}

func TestCmdInfoShowsAmbiguousProjectsAndDoesNotCleanStaleLock(t *testing.T) {
	baseDir := t.TempDir()
	masterDir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(baseDir, "projects", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := state.ResolveProjectPaths(baseDir, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: 999999999, RunID: "old-run", Host: host}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	code := captureShowStdout(t, &output, func() int {
		return run([]string{"info", "--basedir", baseDir, "--masterdir", masterDir, "--json"})
	})
	if code != 0 {
		t.Fatalf("info exit code = %d, output = %s", code, output.String())
	}
	var report infoReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode info JSON: %v\n%s", err, output.String())
	}
	if report.Project != "" || len(report.ProjectChoices) != 2 {
		t.Fatalf("project selection = %q, choices = %#v", report.Project, report.ProjectChoices)
	}
	if len(report.RunLocks) != 1 || report.RunLocks[0].State != state.LockStale {
		t.Fatalf("run locks = %#v, want stale lock", report.RunLocks)
	}
	if _, err := os.Stat(paths.LockFile); err != nil {
		t.Fatalf("info removed or changed stale lock: %v", err)
	}
}

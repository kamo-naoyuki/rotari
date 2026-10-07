package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/config"
)

func workspaceCWD(t *testing.T) string {
	t.Helper()
	oldConfig, oldCommand, oldPath := cliConfig, cliConfigCommand, cliConfigPath
	oldFiles, oldExplicit, oldDefaults := cliFileConfig, cliLocationExplicit, cliLocationDefaults
	t.Cleanup(func() {
		cliConfig, cliConfigCommand, cliConfigPath = oldConfig, oldCommand, oldPath
		cliFileConfig, cliLocationExplicit, cliLocationDefaults = oldFiles, oldExplicit, oldDefaults
	})
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Error(err)
		}
	})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(envBaseDir, "")
	t.Setenv(envProjectName, "")
	return dir
}

func TestWorkspaceInit(t *testing.T) {
	for _, args := range [][]string{nil, {"state"}, {"../state", "demo"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := workspaceCWD(t)
			if code := run(append([]string{"init"}, args...)); code != 0 {
				t.Fatalf("init = %d", code)
			}
			loaded, err := config.LoadScope("workspace", dir)
			if err != nil {
				t.Fatal(err)
			}
			base := ".rotari-state"
			if len(args) > 0 {
				base = args[0]
			}
			if loaded.Values["basedir"] != base {
				t.Fatalf("values = %#v", loaded.Values)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 || entries[0].Name() != config.WorkspaceFile {
				t.Fatalf("init side effects = %v", entries)
			}
			before, _ := os.ReadFile(config.WorkspaceFile)
			if code := run([]string{"init", "replacement"}); code == 0 {
				t.Fatal("overwrote workspace")
			}
			after, _ := os.ReadFile(config.WorkspaceFile)
			if string(before) != string(after) {
				t.Fatal("workspace changed")
			}
		})
	}
}

func TestWorkspaceInitRejectsInvalidArguments(t *testing.T) {
	workspaceCWD(t)
	for _, args := range [][]string{{"/absolute"}, {""}, {"state", "../bad"}, {"state", `bad\name`}, {"state", "latest"}, {"a", "b", "c"}} {
		if run(append([]string{"init"}, args...)) == 0 {
			t.Fatalf("accepted %v", args)
		}
	}
	if _, err := os.Stat(config.WorkspaceFile); !os.IsNotExist(err) {
		t.Fatalf("workspace created: %v", err)
	}
}

func TestWorkspaceLocationAndMergedFiles(t *testing.T) {
	dir := workspaceCWD(t)
	if err := os.WriteFile(config.WorkspaceFile, []byte("basedir = 'state'\nproject-name = 'workspace'\n[run]\nretry = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "state")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "config.toml"), []byte("project-name = 'base'\n[run]\nexecutor = 'local'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cliConfigCommand = "run"
	if err := loadCLIConfig(nil); err != nil {
		t.Fatal(err)
	}
	if configString("basedir", "") != base || configString("project-name", "") != "base" || configInt("retry", 0) != 2 {
		t.Fatalf("config = %#v", cliConfig)
	}
	if cliFileConfig.Values["basedir"] != "state" {
		t.Fatalf("file values changed: %#v", cliFileConfig.Values)
	}
	t.Setenv(envProjectName, "env")
	if err := loadCLIConfig([]string{"--project-name", "cli"}); err != nil {
		t.Fatal(err)
	}
	if configString("project-name", "") != "cli" {
		t.Fatalf("CLI location = %#v", cliConfig)
	}
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(child); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envProjectName, "")
	if err := loadCLIConfig(nil); err != nil {
		t.Fatal(err)
	}
	if configInt("retry", 0) != 0 || configString("basedir", "") == base {
		t.Fatalf("inherited parent = %#v", cliConfig)
	}
}

func TestWorkspacePositionalProjectLoadsSelectedLayer(t *testing.T) {
	dir := workspaceCWD(t)
	base := filepath.Join(dir, "state")
	if err := os.WriteFile(config.WorkspaceFile, []byte("basedir = 'state'\nproject-name = 'default-from-file'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(base, "projects", "selected")
	if err := os.MkdirAll(selected, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(selected, "config.toml"), []byte("quiet = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envProjectName, "environment")
	for _, command := range []string{"check", "reset", "unlock", "jobs", "show", "export", "wait"} {
		cliConfigCommand = command
		if err := loadCLIConfig([]string{"selected"}); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
		if !configBool("quiet", false) || cliConfigPath != filepath.Join(selected, "config.toml") {
			t.Fatalf("%s chose wrong layer: %#v", command, cliFileConfig)
		}
	}
}

func TestWorkspaceSelectorScanDoesNotReadJobArguments(t *testing.T) {
	oldCommand := cliConfigCommand
	t.Cleanup(func() { cliConfigCommand = oldCommand })
	cliConfigCommand = "add"
	if base, project := configLocationArgs([]string{"echo", "--basedir", "/job-value", "--project-name", "job"}); base != "" || project != "" {
		t.Fatalf("job arguments selected %s/%s", base, project)
	}
	cliConfigCommand = "run"
	id := "20261007-000000-abcdef01"
	if selected := configRunIDArg([]string{"--run-name", id}); selected != "" {
		t.Fatalf("run name became ID %q", selected)
	}
	if selected := configRunIDArg([]string{"-r", id}); selected != id {
		t.Fatalf("run selector = %q", selected)
	}
}

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// writeProjectRun writes project "exp" under baseDir with one saved run whose
// second job failed when failed is set, and registers the run.
func writeProjectRun(t *testing.T, baseDir, runID string, failed bool) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, "exp")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "a", Name: "a", Command: []string{"true"}},
		{ID: "b", Name: "b", Command: []string{"false"}},
	}}); err != nil {
		t.Fatal(err)
	}
	exitCode, status := 0, "finished"
	if failed {
		exitCode, status = 1, "failed"
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: status, ExitCode: exitCode, Results: []model.JobResult{
		{ID: "a", ExitCode: 0}, {ID: "b", ExitCode: exitCode},
	}}); err != nil {
		t.Fatal(err)
	}
	registry, err := runregistry.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(runregistry.Location{BaseDir: baseDir, ProjectName: "exp", RunID: runID}); err != nil {
		t.Fatal(err)
	}
}

// hintCommands returns the commands printed under heading, one per line.
func hintCommands(output, heading string) []string {
	_, after, found := strings.Cut(output, heading)
	if !found {
		return nil
	}
	var commands []string
	for _, line := range strings.Split(after, "\n")[1:] {
		if !strings.HasPrefix(line, "  rotari ") {
			break
		}
		commands = append(commands, strings.TrimPrefix(line, "  rotari "))
	}
	return commands
}

// TestShowProjectListHintsWorkForListedBaseDirs runs the commands the project
// list suggests, with its placeholders filled in, for a project outside the
// default state directory.
func TestShowProjectListHintsWorkForListedBaseDirs(t *testing.T) {
	// The default state directory comes from XDG_STATE_HOME, or from
	// ROTARI_BASEDIR, which also counts as an explicit --basedir.
	for _, variable := range []string{"XDG_STATE_HOME", "ROTARI_BASEDIR"} {
		t.Run(variable, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			t.Setenv("ROTARI_BASEDIR", "")
			t.Setenv(variable, t.TempDir())
			baseDir, _ := filepath.Abs(t.TempDir())
			testProjectListHints(t, baseDir, true)
		})
	}
	t.Run("default state directory", func(t *testing.T) {
		stateHome := t.TempDir()
		t.Setenv("XDG_STATE_HOME", stateHome)
		t.Setenv("ROTARI_BASEDIR", "")
		testProjectListHints(t, filepath.Join(stateHome, "rotari"), false)
	})
}

func TestBareShowIgnoresImplicitLocationDefaults(t *testing.T) {
	stateHome := t.TempDir()
	defaultBase := filepath.Join(stateHome, "rotari")
	workspaceBase := t.TempDir()
	envBase := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("ROTARI_BASEDIR", envBase)
	t.Setenv("ROTARI_PROJECT_NAME", "env-only")
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig, cliConfigCommand = map[string]any{"basedir": workspaceBase, "project-name": "workspace-only"}, "show"
	t.Cleanup(func() { cliConfig, cliConfigCommand = oldConfig, oldCommand })

	writeProjectRun(t, defaultBase, "20261007-110000-00000001", false)
	writeProjectRun(t, workspaceBase, "20261007-110001-00000002", false)
	writeProjectRun(t, envBase, "20261007-110002-00000003", false)

	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int { return cmdShow(nil) }); code != 0 {
		t.Fatalf("bare show exit = %d: %s", code, output.String())
	}
	for _, project := range []string{"exp"} {
		if !strings.Contains(output.String(), project) {
			t.Errorf("bare show omitted project %q: %s", project, output.String())
		}
	}
	for _, baseDir := range []string{defaultBase, workspaceBase, envBase} {
		if !strings.Contains(output.String(), baseDir) {
			t.Errorf("bare show omitted registered basedir %q: %s", baseDir, output.String())
		}
	}

	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdShow([]string{"--basedir", workspaceBase}) }); code != 0 {
		t.Fatalf("show --basedir exit = %d: %s", code, output.String())
	}
	if strings.Contains(output.String(), defaultBase) || strings.Contains(output.String(), envBase) {
		t.Fatalf("explicit basedir did not scope show: %s", output.String())
	}
}

// testProjectListHints lists baseDir's projects and runs the suggested
// commands; wantBaseDir says whether they must name the basedir.
func testProjectListHints(t *testing.T, baseDir string, wantBaseDir bool) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	failedRun := "20260101-000000-aaaaaaaa"
	writeProjectRun(t, baseDir, failedRun, true)

	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int { return cmdShow([]string{"--basedir", baseDir}) }); code != 0 {
		t.Fatalf("show --basedir exit = %d:\n%s", code, output.String())
	}
	text := output.String()
	fill := strings.NewReplacer("BASEDIR", baseDir, "PROJECT", "exp", "RUN_ID", failedRun)
	commands := hintCommands(text, "To show runs in a project:")
	commands = append(commands, hintCommands(text, "To summarize a run's failures by cause:")...)
	if len(commands) != 2 {
		t.Fatalf("hints = %q, want a show and a lineage command:\n%s", commands, text)
	}
	for _, command := range commands {
		if strings.Contains(command, "-b BASEDIR") != wantBaseDir {
			t.Errorf("hint %q names the basedir: %v, want %v", command, !wantBaseDir, wantBaseDir)
		}
	}
	for _, command := range commands {
		args := strings.Fields(fill.Replace(command))
		var hinted bytes.Buffer
		code := captureShowStdout(t, &hinted, func() int { return run(args) })
		if code != 0 {
			t.Errorf("hinted command %q exit = %d:\n%s", "rotari "+strings.Join(args, " "), code, hinted.String())
		}
	}
	if !strings.Contains(text, "LAST RESULT") || !strings.Contains(text, "failed 1/2") {
		t.Fatalf("project list does not show the last run's result:\n%s", text)
	}
}

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func captureResetStderr(t *testing.T, args []string) (int, string) {
	t.Helper()
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdReset(args)
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(output)
}

// useNonTerminalStdin replaces stdin with a pipe, whose read end is never a
// terminal, so isTerminal(os.Stdin) reports false.
func useNonTerminalStdin(t *testing.T) {
	t.Helper()
	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = reader.Close()
	})
}

func TestCmdResetNonInteractiveRejectionWarnsWhenJobsMayStillRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"retained"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: -1, RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	// A job directory without a status file reports a job that has not finished.
	jobDir := filepath.Join(paths.RunsDir, "run-1", "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "command.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	useNonTerminalStdin(t)

	code, output := captureResetStderr(t, []string{"--basedir", baseDir, "--project-name", "demo"})
	if code == 0 {
		t.Fatal("cmdReset accepted an interrupted run without --recover")
	}
	for _, want := range []string{
		"1 of 1 job(s) appear to still be running",
		"Do not recover until you have independently confirmed those jobs have actually stopped.",
		"rotari reset --basedir",
		"--recover",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr = %q, want it to contain %q", output, want)
		}
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("queue commands = %#v, want unchanged", queue.Commands)
	}
}

func TestCmdResetFinishesCompletedCancellationBeforeReset(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"queued"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "cancelling", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1", ExitCode: 130, FinishedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(paths.LockFile) })

	if code := cmdReset([]string{"--basedir", baseDir, "--project-name", "demo", "--quiet"}); code != 0 {
		t.Fatalf("cmdReset exit code = %d, want 0", code)
	}
	if _, err := os.Stat(paths.LockFile); !os.IsNotExist(err) {
		t.Fatalf("run lock remains after finished cancellation: %v", err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands = %#v, want empty", queue.Commands)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("metadata phase = %q, want collecting", meta.Phase)
	}
}

func TestCmdResetRejectsProjectNameWithPathSeparator(t *testing.T) {
	for _, projectName := range []string{"bad/name", `bad\name`} {
		t.Run(projectName, func(t *testing.T) {
			baseDir := t.TempDir()
			if code := cmdReset([]string{"--basedir", baseDir, "--project-name", projectName}); code != 1 {
				t.Fatalf("cmdReset exit code = %d, want 1", code)
			}
			if code := cmdReset([]string{"--basedir", baseDir, projectName}); code != 1 {
				t.Fatalf("cmdReset positional project exit code = %d, want 1", code)
			}
			entries, err := os.ReadDir(baseDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("rejected reset created state: %v", entries)
			}
		})
	}
}

func writeInterruptedResetProject(t *testing.T, baseDir string) state.ProjectPaths {
	t.Helper()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"retained"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	return paths
}

func TestCmdResetInteractiveDeclineKeepsInterruptedRun(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeInterruptedResetProject(t, baseDir)
	usePromptStdin(t, "n\n")

	code, output := captureResetStderr(t, []string{"--basedir", baseDir, "--project-name", "demo"})
	if code != 1 || !strings.Contains(output, "reset cancelled") {
		t.Fatalf("cmdReset exit code = %d, stderr = %q, want reset cancelled", code, output)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("queue commands = %#v, want unchanged", queue.Commands)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "running" {
		t.Fatalf("metadata phase = %q, want running", meta.Phase)
	}
}

func TestCmdResetInteractiveConfirmationRecoversInterruptedRun(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeInterruptedResetProject(t, baseDir)
	usePromptStdin(t, "yes\n")

	if code := cmdReset([]string{"--basedir", baseDir, "--project-name", "demo", "--quiet"}); code != 0 {
		t.Fatalf("cmdReset exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands = %#v, want empty", queue.Commands)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("metadata phase = %q, want collecting", meta.Phase)
	}
}

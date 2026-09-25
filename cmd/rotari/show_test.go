package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestFormatDisplayTimestampUsesJST(t *testing.T) {
	t.Setenv("TZ", "Asia/Tokyo")
	if got := model.FormatDisplayTimestamp("2026-09-16T00:00:01Z"); got != "2026-09-16 09:00:01 JST" {
		t.Fatalf("formatDisplayTimestamp() = %q, want JST display", got)
	}
	if got := model.FormatDisplayTimestamp("-"); got != "-" {
		t.Fatalf("formatDisplayTimestamp(-) = %q, want unchanged marker", got)
	}
}

func TestCmdShowDisplaysFinishedArrayTaskFromStatusJSON(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "array-run"
	task := 1
	queue := Queue{Commands: []QueuedCommand{{ID: "array", Command: []string{"true"}, Executor: "slurm", Array: &ArraySpec{First: 1, Last: 1}}}}
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(paths.RunsDir, runID, "array-1")
	if err := writeJSON(filepath.Join(jobDir, "command.json"), JobSpec{ID: "array-1", Command: []string{"true"}, Executor: "slurm", ArrayTaskID: &task, ArrayFirst: 1, ArrayLast: 1}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "status.json"), slurmStatus{Phase: "running", ExitCode: 0, FinishedAt: "2026-09-18T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.MetaFile), Meta{Phase: "running", LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{RunID: runID, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(paths.LockFile) })

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", runID, "--no-pager"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "array-1") || !strings.Contains(string(output), " 0 ") || !strings.Contains(string(output), "Job status: success: 1, failed: 0, blocked: 0, running: 0, pending: 0") {
		t.Fatalf("code=%d output=%q", code, output)
	}
}

func TestCmdShowResolvesProjectFromRunID(t *testing.T) {
	masterDir := t.TempDir()
	baseDir := t.TempDir()
	t.Setenv(envMasterDir, masterDir)
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "job-1", Name: "analysis", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{RunID: runID, Status: "finished", Results: []JobResult{{ID: "job-1", ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "job-1", "command.json"), JobSpec{ID: "job-1", Name: "analysis", Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unregisterRun(runID) })

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--run-id", runID, "--no-pager"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Run: "+runID) {
		t.Fatalf("cmdShow code=%d output=%q", code, output)
	}

	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code = cmdShow([]string{"--run-id", runID, "--job-id", "job-1", "--no-pager"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err = io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Job: job-1") {
		t.Fatalf("cmdShow job code=%d output=%q", code, output)
	}

	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code = cmdShow([]string{"--no-pager", runID})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err = io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Run: "+runID) {
		t.Fatalf("cmdShow positional run code=%d output=%q", code, output)
	}

	attemptID := makeAttemptID(runID, "job-1", 0)
	attemptDir, err := specificAttemptJobDir(runDir, "job-1", attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(attemptDir, "command.json"), JobSpec{ID: "job-1", Name: "analysis", Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code = cmdShow([]string{"--no-pager", attemptID})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err = io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Job: job-1") || !strings.Contains(string(output), "Attempt ID: "+attemptID) {
		t.Fatalf("cmdShow positional attempt code=%d output=%q", code, output)
	}

	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code = cmdShow([]string{"--run-id", runID, "--job-name", "analysis", "--no-pager"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err = io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Job: job-1") {
		t.Fatalf("cmdShow --run-id --job-name code=%d output=%q", code, output)
	}

	for _, selector := range []string{"job-1", "analysis"} {
		reader, writer, err = os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = writer
		code = cmdShow([]string{"--basedir", baseDir, "--no-pager", selector})
		_ = writer.Close()
		os.Stdout = oldStdout
		output, err = io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Job: job-1") {
			t.Fatalf("cmdShow selector=%q code=%d output=%q", selector, code, output)
		}
	}
	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code = cmdShow([]string{"--basedir", baseDir, "--job-name", "analysis", "--no-pager"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err = io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Job: job-1") {
		t.Fatalf("cmdShow --job-name code=%d output=%q", code, output)
	}
}

func TestShowRunDisplaysAcceptedStatus(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "accepted-run"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{RunID: runID, Status: "finished", Results: []JobResult{{ID: "job-1", ExitCode: 0, Accepted: true}}}); err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	code := showRun(paths, runID, false)
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "success (accepted)") {
		t.Fatalf("showRun code=%d output=%q", code, output)
	}
}

func TestCmdShowResolvesRunNameAcrossProjects(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "named-run"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{RunID: runID, RunName: "nightly", Status: "finished"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{}); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--no-pager", "nightly"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Project: demo") || !strings.Contains(string(output), "Run: nightly (named-run)") {
		t.Fatalf("cmdShow code=%d output=%q", code, output)
	}
}

func TestCmdShowJobSelectorsPreferCurrentQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "queued-job", Name: "queued", Command: []string{"echo", "queued"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "latest-run", "commands.json"), Queue{Commands: []QueuedCommand{{ID: "queued-job", Name: "queued", Command: []string{"echo", "latest"}}}}); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"--basedir", baseDir, "--job-id", "queued-job", "--no-pager"},
		{"--basedir", baseDir, "--job-name", "queued", "--no-pager"},
	} {
		oldStdout := os.Stdout
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = writer
		code := cmdShow(args)
		_ = writer.Close()
		os.Stdout = oldStdout
		output, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if code != 0 || !strings.Contains(string(output), "SHOW MODE: PROJECT / QUEUE / JOB") || !strings.Contains(string(output), "queued-job") || strings.Contains(string(output), "latest") {
			t.Fatalf("cmdShow args=%#v code=%d output=%q", args, code, output)
		}
	}
}

func TestSelectRunIDUsesValidMetaLastRun(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-from-meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "newer-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "run-from-meta"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := selectRunID(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run-from-meta" {
		t.Fatalf("run ID = %q, want run-from-meta", runID)
	}
}

func TestSelectRunIDFallsBackToNewestRun(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	olderPath := filepath.Join(paths.RunsDir, "older-run")
	newerPath := filepath.Join(paths.RunsDir, "newer-run")
	if err := os.MkdirAll(olderPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newerPath, 0o755); err != nil {
		t.Fatal(err)
	}
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	if err := os.Chtimes(olderPath, older, older); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newerPath, newer, newer); err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "missing-run"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := selectRunID(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "newer-run" {
		t.Fatalf("run ID = %q, want newer-run", runID)
	}
}

func TestSelectRunIDDoesNotFallbackForMissingRequestedRun(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "existing-run"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = selectRunID(paths, "missing-run")
	if err == nil || !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("error = %v, want exact missing-run error", err)
	}
}

func TestSelectRunIDResolvesLatestAlias(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "latest-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "latest-run"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := selectRunID(paths, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "latest-run" {
		t.Fatalf("run ID = %q, want latest-run", runID)
	}
}

func TestCmdShowDisplaysCurrentQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Name: "greeting", Command: []string{"printf", "hello"}, Origin: &JobOrigin{RunID: "run-1", JobID: "job-old", Status: "success"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--no-pager"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0", code)
	}
	for _, want := range []string{"Base directory: " + baseDir, "Project: demo", "Queue: " + paths.QueueFile + " (1 jobs)", "Project state: idle", "Runner server: stopped", "Runs: 0", "Queue:\nJOB ID", "job-1", "greeting", "run-1/job-old", "success", "printf hello", "To execute these jobs:", "rotari run -b '" + baseDir + "' -p 'demo'"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("cmdShow output does not contain %q:\n%s", want, output)
		}
	}
	if strings.Count(string(output), "=== SHOW MODE:") != 1 {
		t.Fatalf("project overview emitted duplicate headers:\n%s", output)
	}
}

func TestCmdShowWarnsAndSucceedsWhenProjectHasNoRunsOrQueue(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := resolvePaths(baseDir, "demo"); err != nil {
		t.Fatal(err)
	}

	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo"})
	os.Stdout, os.Stderr = oldStdout, oldStderr
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("cmdShow emitted unexpected stderr: %q", stderr)
	}
	if !strings.Contains(string(stdout), "No runs found.") {
		t.Fatalf("cmdShow did not show empty run list: %q", stdout)
	}
}

func TestCmdShowDisplaysResolvedConfigPaths(t *testing.T) {
	baseDir := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	projectDir := filepath.Join(baseDir, "projects", "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	globalConfig := filepath.Join(configHome, "rotari", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(globalConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalConfig, []byte("executor: slurm\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseConfig := filepath.Join(baseDir, "config.yaml")
	if err := os.WriteFile(baseConfig, []byte("retry: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectConfig := filepath.Join(projectDir, "config.yaml")
	if err := os.WriteFile(projectConfig, []byte("local-concurrency: 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Name: "greeting", Command: []string{"printf", "hello"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--no-pager"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0", code)
	}
	want := "Config: " + projectConfig
	if !strings.Contains(string(output), want) {
		t.Fatalf("cmdShow output does not contain highest-priority config path %q:\n%s", want, output)
	}
	if strings.Contains(string(output), globalConfig) || strings.Contains(string(output), baseConfig) {
		t.Fatalf("cmdShow output contains lower-priority config paths:\n%s", output)
	}
}

func TestCmdShowDisplaysActiveRunBeforeQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "queued-job", Command: []string{"echo", "queued"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	runID := "active-run"
	runQueue := Queue{Commands: []QueuedCommand{{ID: "active-job", Command: []string{"echo", "active"}}}}
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), runQueue); err != nil {
		t.Fatal(err)
	}
	if err := state.AcquireRunLock(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: runID}); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(paths.LockFile)

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--no-pager"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0", code)
	}
	text := string(output)
	for _, want := range []string{"Runs: 1", runID, "Queue:\nJOB ID", "queued-job", "echo queued"} {
		if !strings.Contains(text, want) {
			t.Fatalf("cmdShow output does not contain %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"active-job", "echo active"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("cmdShow output unexpectedly contains %q:\n%s", unwanted, text)
		}
	}

	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code = cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--queue", "--no-pager"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err = io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Queue:\nJOB ID") || !strings.Contains(string(output), "queued-job") || strings.Contains(string(output), "active-job") {
		t.Fatalf("--queue did not force current queue display, code=%d output=%q", code, output)
	}
}

func TestCmdShowDisplaysInterruptedRunBeforeQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "queued-job", Command: []string{"echo", "queued"}}}}); err != nil {
		t.Fatal(err)
	}
	runID := "interrupted-run"
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), Queue{Commands: []QueuedCommand{{ID: "run-job", Command: []string{"echo", "from-run"}}}}); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: -1, RunID: runID, Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: runID}); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--no-pager"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0", code)
	}
	text := string(output)
	for _, want := range []string{"Runs: 1", runID, "Queue:\nJOB ID", "queued-job", "echo queued"} {
		if !strings.Contains(text, want) {
			t.Fatalf("cmdShow output does not contain %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"run-job", "echo from-run"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("cmdShow output unexpectedly contains %q:\n%s", unwanted, text)
		}
	}
}

func TestCmdShowRejectsLogsForCurrentQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--logs"})
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), "logs and failed filters require --run-id") {
		t.Fatalf("cmdShow exit code = %d, stderr = %q", code, output)
	}
}

func TestPagerWriterFallsBackWhenPagerCannotStart(t *testing.T) {
	t.Setenv("PAGER", filepath.Join(t.TempDir(), "missing-pager"))
	content := strings.Repeat("line\n", pagerLineLimit) + "partial"
	var output bytes.Buffer
	writer := &pagerWriter{output: &output}

	written, err := writer.Write([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if written != len(content) {
		t.Fatalf("written = %d, want %d", written, len(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if output.String() != content {
		t.Fatalf("fallback output = %q, want %q", output.String(), content)
	}
}

func TestCmdShowFailedLogsFiltersSuccessfulJobs(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-1"
	for _, job := range []struct {
		id, name, status, output string
		command                  []string
	}{
		{id: "ok-job", name: "success", status: "0\n", output: "successful output\n", command: []string{"echo", "ok"}},
		{id: "bad-job", name: "failure", status: "3\n", output: "failed output\n", command: []string{"false"}},
	} {
		jobDir := filepath.Join(paths.RunsDir, runID, job.id)
		if err := writeJSON(filepath.Join(jobDir, "command.json"), JobSpec{ID: job.id, Name: job.name, Command: job.command}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(jobDir, "status"), []byte(job.status), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(jobDir, "output"), []byte(job.output), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{
		"--basedir", baseDir, "--project-name", "demo", "--run-id", runID, "--failed-logs", "--no-pager",
	})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0", code)
	}
	text := string(output)
	for _, want := range []string{"Base directory: " + baseDir, "Project: demo", "Run: " + runID, "bad-job", "failure", "Status: 3 (failed)", "Command: false", "failed output"} {
		if !strings.Contains(text, want) {
			t.Fatalf("failed logs do not contain %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"ok-job", "successful output"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("failed logs unexpectedly contain %q:\n%s", unwanted, text)
		}
	}
}

func TestShowJobDisplaysPersistedDetails(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID, jobID := "run-1", "job-1"
	runDir := filepath.Join(paths.RunsDir, runID)
	jobDir := filepath.Join(runDir, jobID)
	job := JobSpec{
		ID: jobID, Name: "analysis", Command: []string{"python", "work.py"},
		Executor: "slurm", ExecutorOptions: []string{"--partition", "gpu"}, DependsOn: []string{"setup"},
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{
		ID: job.ID, Name: job.Name, Command: job.Command, Executor: job.Executor,
		ExecutorOptions: job.ExecutorOptions, DependsOn: job.DependsOn,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{{ID: jobID, Hosts: []string{"node-a"}}}}); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"status": "0\n", "submitted_at": "2026-01-01T00:00:00Z\n",
		"finished_at": "2026-01-01T00:01:00Z\n", "output": "completed\n",
	} {
		if err := os.WriteFile(filepath.Join(jobDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var output bytes.Buffer
	if code := showJob(&output, paths, runID, jobID); code != 0 {
		t.Fatalf("showJob exit code = %d, want 0", code)
	}
	for _, want := range []string{
		"analysis", "slurm", "--partition gpu", "setup", "node-a", "Status: 0", "python work.py", "completed",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("showJob output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestShowJobSurfacesAccountingUnavailableFromSummary(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID, jobID := "run-1", "job-1"
	runDir := filepath.Join(paths.RunsDir, runID)
	jobDir := filepath.Join(runDir, jobID)
	job := JobSpec{ID: jobID, Command: []string{"python", "work.py"}, Executor: "slurm"}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{
		ID: job.ID, Command: job.Command, Executor: job.Executor,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		t.Fatal(err)
	}
	// No "status" file and no "status.json" -- as when squeue and sacct were
	// both unreachable -- leaves summary.json as the only source of the result.
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{
		{ID: jobID, ExitCode: 1, Error: "Slurm accounting result and wrapper status are unavailable"},
	}}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := showJob(&output, paths, runID, jobID); code != 0 {
		t.Fatalf("showJob exit code = %d, want 0", code)
	}
	if want := "Status: 1 (Slurm accounting result and wrapper status are unavailable)"; !strings.Contains(output.String(), want) {
		t.Fatalf("showJob output does not contain %q:\n%s", want, output.String())
	}
}

func TestShowJobDisplaysDiagnoses(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID, jobID := "run-1", "job-1"
	runDir := filepath.Join(paths.RunsDir, runID)
	jobDir := filepath.Join(runDir, jobID)
	job := JobSpec{ID: jobID, Command: []string{"false"}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{{ID: jobID, Command: job.Command}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "command.json"), job); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{Results: []JobResult{{ID: jobID, ExitCode: 1, Diagnoses: []ruleDiagnosis{{Name: "CUDA/GPU memory exhausted", Evidence: "CUDA out of memory", Suggestion: "Reduce batch size"}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "status"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := showJob(&output, paths, runID, jobID); code != 0 {
		t.Fatalf("showJob exit code = %d, want 0", code)
	}
	for _, want := range []string{"Diagnosis:", "CUDA/GPU memory exhausted", "Evidence: CUDA out of memory", "Next: Reduce batch size"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("showJob output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestShowJobRejectsTraversalInRunAndJobIDs(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	escapedDir := filepath.Join(filepath.Dir(paths.RunsDir), "outside", "job-1")
	if err := os.MkdirAll(escapedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(escapedDir, "output"), []byte("escaped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		runID string
		jobID string
	}{
		{runID: "../outside", jobID: "job-1"},
		{runID: "run-1", jobID: "../outside"},
	} {
		if code := showJob(&bytes.Buffer{}, paths, tc.runID, tc.jobID); code == 0 {
			t.Fatalf("showJob accepted unsafe values runID=%q jobID=%q", tc.runID, tc.jobID)
		}
	}
}

func TestShowJobFollowsCarriedForwardOrigin(t *testing.T) {
	paths, err := resolvePaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	jobID := "job-1"
	currentRunDir := filepath.Join(paths.RunsDir, "run-2")
	origin := &JobOrigin{RunID: "run-1", JobID: jobID, Status: "success"}
	if err := writeJSON(filepath.Join(currentRunDir, "commands.json"), Queue{Commands: []QueuedCommand{{
		ID: jobID, Name: "carried", Command: []string{"echo", "original"}, Origin: origin,
	}}}); err != nil {
		t.Fatal(err)
	}
	originJobDir := filepath.Join(paths.RunsDir, origin.RunID, origin.JobID)
	if err := writeJSON(filepath.Join(originJobDir, "command.json"), JobSpec{ID: jobID, Name: "carried", Command: []string{"echo", "original"}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, origin.RunID, "commands.json"), Queue{Commands: []QueuedCommand{{
		ID: jobID, Name: "carried", Command: []string{"echo", "original"},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(originJobDir, "status"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(originJobDir, "output"), []byte("original output\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := showJob(&output, paths, "run-2", jobID); code != 0 {
		t.Fatalf("showJob exit code = %d, want 0", code)
	}
	for _, want := range []string{"carried forward from run run-1", "Run: run-1", "original output"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("showJob output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestShowQueueJobPrintsMatchingJob(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{
		{ID: "job-1", Name: "build", Stage: "compile", Command: []string{"echo", "build"}, DependsOn: []string{"prepare"}},
	}}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := showQueueJob(paths, queue, "job-1")
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("showQueueJob exit code = %d, want 0", code)
	}
	for _, want := range []string{"Job: job-1", "Name: build", "Stage: compile", "Depends on: prepare", "Command: echo build"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("showQueueJob output does not contain %q:\n%s", want, output)
		}
	}
}

func TestShowQueueJobColorsLabelsInTTYMode(t *testing.T) {
	oldCheck := terminalCheck
	terminalCheck = func(*os.File) bool { return true }
	defer func() { terminalCheck = oldCheck }()

	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Name: "build", Command: []string{"echo", "build"}, DependsOn: []string{"prepare"}}}}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := showQueueJob(paths, queue, "job-1")
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("showQueueJob exit code = %d, want 0", code)
	}
	for _, want := range []string{cyan("Name:"), cyan("Executor:"), cyan("Command:")} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("showQueueJob output does not contain colored label %q:\n%s", want, output)
		}
	}
}

func TestShowQueueDisplaysArrayTaskColumn(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{
		{ID: "train", Name: "train", Stage: "training", Command: []string{"echo", "train"}, Array: &ArraySpec{First: 1, Last: 2}},
	}}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := showQueue(paths, queue)
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("showQueue exit code = %d, want 0", code)
	}
	for _, want := range []string{"TASK", "STAGE", "training", "train-1", "train-2"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("showQueue output does not contain %q:\n%s", want, output)
		}
	}

	reader, writer, err = os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code = showQueueJob(paths, queue, "train-1")
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err = io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "Array task: 1 (range 1-2)") {
		t.Fatalf("showQueueJob code=%d output does not contain array task info:\n%s", code, output)
	}
}

func TestShowQueueJobReportsMissingJob(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"echo", "build"}}}}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := showQueueJob(paths, queue, "missing")
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 || !strings.Contains(string(output), `job "missing" not found in current queue`) {
		t.Fatalf("showQueueJob exit code = %d, stderr = %q", code, output)
	}
}

func TestShowRunsListsRunsSortedByRecency(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	olderRun := filepath.Join(paths.RunsDir, "run-old")
	newerRun := filepath.Join(paths.RunsDir, "run-new")
	if err := writeJSON(filepath.Join(olderRun, "summary.json"), RunSummary{
		RunID: "run-old", Status: "finished", ExitCode: 0, StartedAt: "2026-09-16T00:00:00Z", FinishedAt: "2026-09-16T00:00:01Z",
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(newerRun, "summary.json"), RunSummary{
		RunID: "run-new", Status: "failed", ExitCode: 1, StartedAt: "2026-09-17T00:00:00Z", FinishedAt: "2026-09-17T00:00:01Z",
	}); err != nil {
		t.Fatal(err)
	}
	newerTime := time.Now()
	olderTime := newerTime.Add(-time.Hour)
	if err := os.Chtimes(olderRun, olderTime, olderTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newerRun, newerTime, newerTime); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := showRuns(paths)
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("showRuns exit code = %d, want 0", code)
	}
	text := string(output)
	if !strings.Contains(text, "Runs: 2") {
		t.Fatalf("showRuns did not display the run count:\n%s", text)
	}
	for _, want := range []string{"To show jobs in a run:", "rotari show -p PROJECT -r RUN_ID"} {
		if !strings.Contains(text, want) {
			t.Fatalf("showRuns did not display %q:\n%s", want, text)
		}
	}
	newIndex := strings.Index(text, "run-new")
	oldIndex := strings.Index(text, "run-old")
	if newIndex == -1 || oldIndex == -1 || newIndex > oldIndex {
		t.Fatalf("showRuns did not list run-new before run-old:\n%s", text)
	}
}

func TestShowRunsReportsNoRunsWhenDirectoryMissing(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := showRuns(paths)
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(output), "No runs found.") {
		t.Fatalf("showRuns exit code = %d, stdout = %q", code, output)
	}
}

func TestCmdShowProjectsListsProjectSummaries(t *testing.T) {
	baseDir := t.TempDir()
	demo, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	example, err := resolvePaths(baseDir, "example")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(demo.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(demo.MetaFile, Meta{Phase: "finished", LastRunID: "demo-run"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(demo.RunsDir, "demo-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(example.LockFile, LockInfo{PID: os.Getpid(), RunID: "example-run"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(example.LockFile) })

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0", code)
	}
	for _, want := range []string{baseDir, "Projects: 2", "PROJECT", "QUEUED", "RUNS", "demo", "demo-run", "example", "running", "To show runs in a project:", "rotari show -p PROJECT"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("cmdShow project listing output does not contain %q:\n%s", want, output)
		}
	}
	for _, unwanted := range []string{"To show jobs in a run:", "rotari show -p PROJECT -r latest"} {
		if strings.Contains(string(output), unwanted) {
			t.Fatalf("cmdShow project listing output unexpectedly contains %q:\n%s", unwanted, output)
		}
	}
}

func TestCmdShowFallsBackToProjectsWithWarningWhenProjectIsAmbiguous(t *testing.T) {
	baseDir := t.TempDir()
	for _, projectName := range []string{"demo", "example"} {
		paths, err := resolvePaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.QueueFile, Queue{}); err != nil {
			t.Fatal(err)
		}
	}

	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	code := cmdShow([]string{"--basedir", baseDir})
	os.Stdout, os.Stderr = oldStdout, oldStderr
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0; stderr=%q", code, stderr)
	}
	if len(stderr) != 0 {
		t.Fatalf("cmdShow emitted unexpected warning: %q", stderr)
	}
	for _, want := range []string{"Projects: 2", "demo", "example"} {
		if !strings.Contains(string(stdout), want) {
			t.Fatalf("cmdShow fallback output does not contain %q:\n%s", want, stdout)
		}
	}
}

func TestCmdShowListsProjectsAcrossKnownBaseDirs(t *testing.T) {
	masterDir := t.TempDir()
	t.Setenv(envMasterDir, masterDir)
	baseDirs := []string{t.TempDir(), t.TempDir()}
	for index, baseDir := range baseDirs {
		paths, err := resolvePaths(baseDir, fmt.Sprintf("project-%d", index))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.QueueFile, Queue{}); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, fmt.Sprintf("run-%d", index)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := registerRun(paths, fmt.Sprintf("run-%d", index)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = unregisterRun(fmt.Sprintf("run-%d", index)) })
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow(nil)
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, output=%q", code, output)
	}
	text := string(output)
	for index, baseDir := range baseDirs {
		for _, want := range []string{baseDir, fmt.Sprintf("project-%d", index)} {
			if !strings.Contains(text, want) {
				t.Fatalf("cross-basedir project listing does not contain %q:\n%s", want, text)
			}
		}
	}
}

func TestCmdShowBaseDirsListsMasterRegistryEntries(t *testing.T) {
	masterDir := t.TempDir()
	knownBaseDir := t.TempDir()
	t.Setenv(envMasterDir, masterDir)
	if err := registerRunLocation(runLocation{BaseDir: knownBaseDir, ProjectName: "demo", RunID: "saved-run"}); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedirs"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdShow exit code = %d, want 0", code)
	}
	for _, want := range []string{"Master directory: " + masterDir, "Known state directories: 1", "run registry", knownBaseDir} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("cmdShow --basedirs output does not contain %q:\n%s", want, output)
		}
	}
}

func TestShowAndWebShareStatusFallbackChain(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := Queue{Commands: []QueuedCommand{
		{ID: "job-a", Command: []string{"true"}},
		{ID: "job-b", Command: []string{"true"}, Executor: "slurm"},
		{ID: "job-c", Command: []string{"true"}, DependsOn: []string{"job-a"}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	// job-a: the attempt's status file wins over a disagreeing summary result.
	if err := os.MkdirAll(filepath.Join(runDir, "job-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "job-a", "status"), []byte("3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// job-b: a still-running wrapper falls back to the terminal scheduler state.
	if err := writeJSON(filepath.Join(runDir, "job-b", "status.json"), slurmStatus{Phase: "running"}); err != nil {
		t.Fatal(err)
	}
	writeSchedulerStatus(filepath.Join(runDir, "job-b"), "FAILED")
	// job-c: no attempt directory, so the summary result decides.
	summary := RunSummary{RunID: runID, Status: "failed", ExitCode: 1, Results: []JobResult{
		{ID: "job-a", ExitCode: 0},
		{ID: "job-c", ExitCode: 1, Error: "blocked by failed dependency"},
	}}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		t.Fatal(err)
	}

	var runOutput bytes.Buffer
	code := captureShowStdout(t, &runOutput, func() int { return showRun(paths, runID, false) })
	if code != 0 || !strings.Contains(runOutput.String(), "Job status: success: 0, failed: 2, blocked: 1, running: 0, pending: 0") {
		t.Fatalf("showRun code=%d output=%q", code, runOutput.String())
	}
	var jobOutput bytes.Buffer
	if code := showJob(&jobOutput, paths, runID, "job-b"); code != 0 || !strings.Contains(jobOutput.String(), "failed (exit code 1)") {
		t.Fatalf("showJob code=%d output=%q", code, jobOutput.String())
	}

	jobs, err := loadWebJobs(runDir, summary)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"job-a": 3, "job-b": 1, "job-c": 1}
	for _, job := range jobs {
		if job.Result == nil || job.Result.ExitCode != want[job.ID] {
			t.Fatalf("web job %s result = %#v, want exit code %d", job.ID, job.Result, want[job.ID])
		}
		if job.ID == "job-c" && !strings.HasPrefix(job.Result.Error, "blocked") {
			t.Fatalf("web job-c result = %#v, want blocked summary result", job.Result)
		}
	}
	if len(jobs) != len(want) {
		t.Fatalf("web jobs = %#v, want %d jobs", jobs, len(want))
	}
}

func captureShowStdout(t *testing.T, output *bytes.Buffer, show func() int) int {
	t.Helper()
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := show()
	_ = writer.Close()
	os.Stdout = oldStdout
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	output.Write(data)
	return code
}

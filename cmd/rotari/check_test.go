package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestCheckProjectReadyAndEmpty(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}

	result, err := checkProject(paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "empty" || result.Runnable || result.Queued != 0 || result.Lock != "none" {
		t.Fatalf("empty check = %#v", result)
	}

	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	result, err = checkProject(paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "ready" || !result.Runnable || result.Queued != 1 || result.Lock != "none" {
		t.Fatalf("ready check = %#v", result)
	}
}

func TestCheckProjectValidatesQueueLikeRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Name: "train", DependsOn: []string{"prepare"}, Command: []string{"true"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}

	_, err = checkProject(paths)
	if err == nil || !strings.Contains(err.Error(), `job "train" depends on unknown job "prepare"`) {
		t.Fatalf("checkProject error = %v", err)
	}
}

func TestCheckProjectValidatesExecutorOptionsLikeRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Executor: "ssh", Command: []string{"true"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}

	_, err = checkProject(paths)
	if err == nil || !strings.Contains(err.Error(), "SSH executor requires its first executor option to be the target host") {
		t.Fatalf("checkProject error = %v", err)
	}
}

func TestCheckProjectDeepValidatesLocalEnvironment(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"./missing-command"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if _, err := checkProject(paths); err != nil {
		t.Fatalf("normal check unexpectedly inspected local command: %v", err)
	}
	if _, err := checkProjectWithOptions(paths, true); err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("deep check error = %v", err)
	}
}

func TestCheckProjectReportsRunningAndRemoteLocks(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		lock      LockInfo
		wantState string
		wantLock  string
	}{
		{name: "local runner", lock: LockInfo{PID: os.Getpid(), RunID: "run-local", Host: host}, wantState: "running", wantLock: "active"},
		{name: "remote lock", lock: LockInfo{PID: 1, RunID: "run-remote", Host: host + "-remote"}, wantState: "locked", wantLock: "remote"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, err := resolvePaths(baseDir, "demo")
			if err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(paths.LockFile, test.lock); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: test.lock.RunID}); err != nil {
				t.Fatal(err)
			}
			writeTestRunStateFiles(t, paths, test.lock.RunID)
			if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
				t.Fatal(err)
			}
			result, err := checkProject(paths)
			if err != nil {
				t.Fatal(err)
			}
			if result.State != test.wantState || result.Runnable || result.Lock != test.wantLock || result.RunID != test.lock.RunID || !result.QueuedKnown || result.Queued != 1 {
				t.Fatalf("check = %#v", result)
			}
		})
	}
}

func TestCheckProjectDoesNotRemoveStaleLock(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: -1, RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")

	result, err := checkProject(paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "interrupted" || result.Runnable || result.Lock != "stale" || result.RunID != "run-1" || !result.QueuedKnown || result.Queued != 1 {
		t.Fatalf("check = %#v", result)
	}
	if _, err := os.Stat(paths.LockFile); err != nil {
		t.Fatalf("check removed stale lock: %v", err)
	}
}

func TestCheckProjectKeepsRunningStateWhenQueueIsUnreadable(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: os.Getpid(), RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	if err := os.WriteFile(paths.QueueFile, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := checkProject(paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "running" || result.Runnable || result.QueuedKnown {
		t.Fatalf("check = %#v", result)
	}
}

func TestCheckProjectReportsReadyWithStaleLockOnIdleProject(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, LockInfo{PID: -1, RunID: "old-run", Host: host}); err != nil {
		t.Fatal(err)
	}

	result, err := checkProject(paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "ready" || !result.Runnable || result.Lock != "stale" {
		t.Fatalf("check = %#v", result)
	}
	if _, err := os.Stat(paths.LockFile); err != nil {
		t.Fatalf("check removed stale lock: %v", err)
	}
}

func TestCmdCheckExitCode(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--basedir", baseDir, "--project-name", "demo"}
	if code := cmdCheck(args); code != 1 {
		t.Fatalf("empty cmdCheck exit code = %d, want 1", code)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if code := cmdCheck(args); code != 0 {
		t.Fatalf("ready cmdCheck exit code = %d, want 0", code)
	}
}

func TestCmdCheckAcceptsPositionalProjectName(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}

	if code := cmdCheck([]string{"--basedir", baseDir, "demo"}); code != 0 {
		t.Fatalf("cmdCheck positional project exit code = %d, want 0", code)
	}
	if code := cmdCheck([]string{"--basedir", baseDir, "--project-name", "demo", "other"}); code != 1 {
		t.Fatalf("cmdCheck accepted positional project with --project-name: exit code = %d", code)
	}
}

func TestCmdCheckPositionalProjectOverridesEnvironmentDefault(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envProjectName, "other")

	if code := cmdCheck([]string{"--basedir", baseDir, "demo"}); code != 0 {
		t.Fatalf("cmdCheck positional project exit code = %d, want 0", code)
	}
}

func TestCmdCheckJSON(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}

	output, code := captureCheckStdout(t, []string{"--basedir", baseDir, "--project-name", "demo", "--json"})
	if code != 0 {
		t.Fatalf("cmdCheck exit code = %d, want 0", code)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["project"] != "demo" || decoded["state"] != "ready" || decoded["runnable"] != true || decoded["queued"] != float64(1) {
		t.Fatalf("JSON output = %#v", decoded)
	}
}

func TestCmdCheckDeepFlag(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, Queue{Commands: []QueuedCommand{{ID: "job-1", Command: []string{"./missing-command"}}}}); err != nil {
		t.Fatal(err)
	}
	if code := cmdCheck([]string{"--basedir", baseDir, "--project-name", "demo", "--deep"}); code != 1 {
		t.Fatalf("cmdCheck --deep exit code = %d, want 1", code)
	}
}

func captureCheckStdout(t *testing.T, args []string) ([]byte, int) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = oldStdout }()
	code := cmdCheck(args)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return output, code
}

func TestWriteProjectCheckJSON(t *testing.T) {
	tests := []struct {
		name       string
		result     projectCheck
		wantQueued *int
		wantRunID  string
	}{
		{name: "ready", result: projectCheck{State: "ready", Runnable: true, Queued: 2, QueuedKnown: true, Lock: "none"}, wantQueued: intPointer(2)},
		{name: "unknown queue", result: projectCheck{State: "running", Lock: "active", RunID: "run-1"}, wantRunID: "run-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := writeProjectCheck(&output, "demo", test.result, true); err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Project  string `json:"project"`
				State    string `json:"state"`
				Runnable bool   `json:"runnable"`
				Queued   *int   `json:"queued"`
				Lock     string `json:"lock"`
				RunID    string `json:"run_id"`
			}
			if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Project != "demo" || decoded.State != test.result.State || decoded.Runnable != test.result.Runnable || decoded.Lock != test.result.Lock || decoded.RunID != test.wantRunID {
				t.Fatalf("JSON output = %#v", decoded)
			}
			if (decoded.Queued == nil) != (test.wantQueued == nil) || decoded.Queued != nil && *decoded.Queued != *test.wantQueued {
				t.Fatalf("queued = %#v, want %#v", decoded.Queued, test.wantQueued)
			}
			if test.wantRunID == "" && strings.Contains(output.String(), `"run_id"`) {
				t.Fatalf("empty run_id was not omitted: %s", output.String())
			}
		})
	}
}

func intPointer(value int) *int {
	return &value
}

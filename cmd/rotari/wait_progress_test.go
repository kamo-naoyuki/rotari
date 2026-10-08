package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestWaitProgressModes(t *testing.T) {
	for _, mode := range []string{"text", "quiet", "json", "quiet-json", "finished", "absent"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ROTARI_MASTERDIR", t.TempDir())
			paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			runDir := filepath.Join(paths.RunsDir, "run-1")
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if mode != "absent" {
				journal, err := state.NewProgressJournal(runDir)
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range []model.ProgressEvent{
					{Progress: true, Message: "=== Run started ===\n  Project: demo"},
					{Progress: true, Message: "Job running: job-1\n  Command: true"},
					{Progress: true, Completed: 1, Total: 2, Succeeded: 1},
					{Progress: true, Message: "Retrying job job-2"},
					{Progress: true, Message: "Job failed: job-2\n  Exit code: 7"},
				} {
					if err := journal.Append(event); err != nil {
						t.Fatal(err)
					}
				}
			}
			summary := model.RunSummary{RunID: "run-1", Status: "failed", ExitCode: 1}
			if err := writeJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
				t.Fatal(err)
			}
			args := []string{"--basedir", paths.BaseDir, "--project-name", "demo", "--run-id", "run-1"}
			if mode != "finished" {
				// A summary may exist before finalization. The active lock must
				// keep wait attached and its timeout must not cancel the run.
				if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1"}); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--timeout", "1ns")
			}
			if strings.Contains(mode, "quiet") {
				args = append(args, "--quiet")
			}
			if strings.Contains(mode, "json") {
				args = append(args, "--json")
			}
			var stdout []byte
			code, stderr := captureStderr(t, func() int {
				result, output := captureWorkflowStdout(t, func() int { return cmdWait(args) })
				stdout = output
				return result
			})
			if code != 1 {
				t.Fatalf("exit = %d, stdout = %s, stderr = %s", code, stdout, stderr)
			}
			if mode == "finished" {
				if string(stdout) != formatRunCompletion(paths, "run-1", summary) || stderr != "" {
					t.Fatalf("finished run replayed progress: %s; %s", stdout, stderr)
				}
				return
			}
			if !strings.Contains(stderr, "timed out waiting") {
				t.Fatalf("missing timeout: %s", stderr)
			}
			if _, err := os.Stat(paths.LockFile); err != nil {
				t.Fatalf("wait changed run lock: %v", err)
			}
			switch mode {
			case "text":
				output := string(stdout)
				if !strings.Contains(output, "=== Run attached ===") || !strings.Contains(output, "progress: 1/2 succeeded=1 failed=0") {
					t.Fatalf("wait did not show its current snapshot: %s", output)
				}
				for _, old := range []string{"=== Run started ===", "Job running:", "Retrying", "Job failed:"} {
					if strings.Contains(output, old) {
						t.Fatalf("wait replayed prior event %q: %s", old, output)
					}
				}
			case "quiet":
				if len(stdout) != 0 {
					t.Fatalf("quiet wait printed attachment progress: %s", stdout)
				}
			case "absent":
				if string(stdout) != "=== Run attached ===\nPress Ctrl-D to stop waiting; Ctrl-C to cancel the run.\n" {
					t.Fatalf("wait without a journal should only mark the attachment: %s", stdout)
				}
			default:
				if len(stdout) != 0 {
					t.Fatalf("unexpected progress: %s", stdout)
				}
			}
		})
	}
}

func TestWaitQuietCompletedResult(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1", Status: "finished"}); err != nil {
		t.Fatal(err)
	}
	for _, machine := range []bool{false, true} {
		args := []string{"--basedir", paths.BaseDir, "--project-name", "demo", "--run-id", "run-1", "--quiet"}
		if machine {
			args = append(args, "--json")
		}
		code, output := captureWorkflowStdout(t, func() int { return cmdWait(args) })
		if code != 0 {
			t.Fatalf("wait exit = %d", code)
		}
		if machine {
			var summary model.RunSummary
			if err := json.Unmarshal(output, &summary); err != nil || summary.RunID != "run-1" {
				t.Fatalf("quiet JSON = %s, %v", output, err)
			}
		} else if len(output) != 0 {
			t.Fatalf("quiet completion = %s", output)
		}
	}
}

func TestWaitDetachStopsWaitingButKeepsRunActive(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	useInProcessSupervisor(t)
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--", "sh", "-c", "sleep 30"}); code != 0 {
		t.Fatalf("cmdAdd exit = %d", code)
	}
	if code := cmdRun([]string{"--basedir", baseDir, "--project-name", "demo", "--async", "--quiet"}); code != 0 {
		t.Fatalf("cmdRun exit = %d", code)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		t.Fatal(err)
	}
	target := resolve.Run{BaseDir: baseDir, ProjectName: "demo", RunID: lock.RunID}
	defer cancelWaitTarget(target, &waitOutput{})
	for _, quiet := range []bool{false, true} {
		detach := make(chan struct{}, 1)
		detach <- struct{}{}
		code, output := captureWorkflowStdout(t, func() int {
			return waitTargetsWithControl([]resolve.Run{target}, time.Time{}, false, false, quiet, nil, detach)
		})
		if code != 0 || !quiet && !strings.Contains(string(output), "Stopped waiting; runs continue in the background.") || quiet && len(output) != 0 {
			t.Fatalf("detach quiet=%t result = %d, %s", quiet, code, output)
		}
	}
	phase, err := project.RunPhaseOf(paths, lock.RunID)
	if err != nil || phase != project.RunPhaseRunning {
		t.Fatalf("Ctrl-D stopped run: phase=%s, err=%v", phase, err)
	}
}

func TestWaitProgressCursorRecovery(t *testing.T) {
	runDir := t.TempDir()
	path := filepath.Join(runDir, state.ProgressFileName)
	first := "{\"progress\":true,\"completed\":1,\"total\":2,\"succeeded\":1}\n"
	last := "{\"progress\":true,\"completed\":2,\"total\":2,\"succeeded\":2}"
	if err := os.WriteFile(path, []byte(first+"malformed\n"+last), 0o600); err != nil {
		t.Fatal(err)
	}
	var cursor state.ProgressCursor
	printer := runProgressPrinter{lastCompleted: -1, lastSucceeded: -1, lastFailed: -1}
	var stdout []byte
	code, stderr := captureStderr(t, func() int {
		result, output := captureWorkflowStdout(t, func() int {
			if err := readWaitProgress(&cursor, runDir, &printer, &waitOutput{}); err != nil {
				t.Error(err)
				return 1
			}
			return 0
		})
		stdout = output
		return result
	})
	if code != 0 || !strings.Contains(stderr, "skipping malformed run progress") || !strings.Contains(string(stdout), "progress: 1/2") || strings.Contains(string(stdout), "progress: 2/2") {
		t.Fatalf("partial/malformed read: %d; %s; %s", code, stdout, stderr)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString("\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("append newline: %v; %v", writeErr, closeErr)
	}
	_, output := captureWorkflowStdout(t, func() int {
		for range 2 {
			if err := readWaitProgress(&cursor, runDir, &printer, &waitOutput{}); err != nil {
				t.Error(err)
			}
		}
		return 0
	})
	if strings.Count(string(output), "progress: 2/2") != 1 || strings.Contains(string(output), "progress: 1/2") {
		t.Fatalf("cursor replayed or lost events: %s", output)
	}
}

func TestRunProgressPrinterControlHints(t *testing.T) {
	for _, hint := range []string{"", "Press Ctrl-D to detach; Ctrl-C to cancel."} {
		printer := runProgressPrinter{controlHint: hint}
		_, output := captureWorkflowStdout(t, func() int {
			printer.print(serverinternal.Response{Progress: true, Message: "=== Run started ==="})
			return 0
		})
		if strings.Contains(string(output), "Ctrl-C to cancel") != (hint != "") {
			t.Fatalf("control hint %q: %s", hint, output)
		}
	}
}

func TestWaitProgressReadFailureFallback(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "timeout"
		if completed {
			name = "completed-transition"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("ROTARI_MASTERDIR", t.TempDir())
			paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			runDir := filepath.Join(paths.RunsDir, "run-1")
			if err := os.MkdirAll(filepath.Join(runDir, state.ProgressFileName), 0o755); err != nil {
				t.Fatal(err)
			}
			summary := model.RunSummary{RunID: "run-1", Status: "failed", ExitCode: 7}
			if err := writeJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1"}); err != nil {
				t.Fatal(err)
			}
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			oldStderr := os.Stderr
			os.Stderr = writer
			defer func() { os.Stderr = oldStderr; _ = writer.Close() }()
			var stderr strings.Builder
			done := make(chan error, 1)
			go func() {
				scanner := bufio.NewScanner(reader)
				var transitionErr error
				transitioned := false
				for scanner.Scan() {
					line := scanner.Text()
					stderr.WriteString(line + "\n")
					// Finalize only after wait actually tried the optional journal.
					if completed && !transitioned && strings.Contains(line, "failed to read progress") {
						transitionErr = os.Remove(paths.LockFile)
						transitioned = true
					}
				}
				if transitionErr == nil {
					transitionErr = scanner.Err()
				}
				done <- transitionErr
			}()
			var result waitResult
			_, stdout := captureWorkflowStdout(t, func() int {
				result = waitForRun(paths.BaseDir, "demo", "run-1", time.Now().Add(2*time.Second), false, false, false)
				return result.exitCode
			})
			os.Stderr = oldStderr
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if strings.Count(stderr.String(), "failed to read progress") != 1 {
				t.Fatalf("expected one journal warning: %s", stderr.String())
			}
			if completed {
				if result.exitCode != 7 || result.timedOut || string(stdout) != "=== Run attached ===\nPress Ctrl-D to stop waiting; Ctrl-C to cancel the run.\n"+formatRunCompletion(paths, "run-1", summary) {
					t.Fatalf("summary fallback = %+v; %s; %s", result, stdout, stderr.String())
				}
			} else {
				if result.exitCode != 1 || !result.timedOut || !strings.Contains(stderr.String(), "timed out waiting") || string(stdout) != "=== Run attached ===\nPress Ctrl-D to stop waiting; Ctrl-C to cancel the run.\n" {
					t.Fatalf("timeout fallback = %+v; %s; %s", result, stdout, stderr.String())
				}
				if _, err := os.Stat(paths.LockFile); err != nil {
					t.Fatalf("wait changed active lock: %v", err)
				}
			}
		})
	}
}

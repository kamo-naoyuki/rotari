package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCmdShowNoLogs(t *testing.T) {
	t.Setenv(envMasterDir, t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "plain", Name: "analysis", Command: []string{"true"}},
		{ID: "array", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{RunID: runID, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unregisterRun(runID) })
	attempts := map[string]string{}
	for _, job := range []string{"plain", "array-1"} {
		for index := range 2 {
			attempt := makeAttemptID(runID, job, index)
			dir, err := specificAttemptJobDir(runDir, job, attempt)
			if err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(filepath.Join(dir, "command.json"), model.JobSpec{ID: job, Command: []string{"true", strconv.Itoa(index)}}); err != nil {
				t.Fatal(err)
			}
			for _, stream := range []string{"output", "stdout", "stderr"} {
				if err := os.WriteFile(filepath.Join(dir, stream), []byte("hidden-"+stream+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if index == 0 {
				attempts[job] = attempt
			}
		}
	}

	for _, tty := range []bool{false, true} {
		for _, target := range []struct {
			name string
			args []string
			job  string
		}{
			{"job", []string{"--job-id", "plain"}, "plain"},
			{"name", []string{"--job-name", "analysis"}, "plain"},
			{"positional", []string{"plain"}, "plain"},
			{"older-attempt", []string{attempts["plain"]}, "plain"},
			{"task", []string{"--job-id", "array-1"}, "array-1"},
			{"task-attempt", []string{attempts["array-1"]}, "array-1"},
		} {
			t.Run(target.name+map[bool]string{false: "/pipe", true: "/tty"}[tty], func(t *testing.T) {
				oldCheck := terminalCheck
				terminalCheck = func(*os.File) bool { return tty }
				t.Cleanup(func() { terminalCheck = oldCheck })
				args := []string{"--basedir", baseDir, "-p", "demo", "--no-logs", "--no-pager"}
				args = append(args, target.args...)
				var output bytes.Buffer
				code := captureShowStdout(t, &output, func() int { return cmdShow(args) })
				text := output.String()
				if code != 0 {
					t.Fatalf("show exit=%d output=%s", code, text)
				}
				for _, want := range []string{"Run:", runID, "Job:", target.job, "Command:", "true", "Execution state:", "Attempt ID:"} {
					if !strings.Contains(text, want) {
						t.Errorf("missing %q in %s", want, text)
					}
				}
				for _, unwanted := range []string{"hidden-", "OUTPUT:", "STDOUT:", "STDERR:"} {
					if strings.Contains(text, unwanted) {
						t.Errorf("unexpected %q in %s", unwanted, text)
					}
				}
				if strings.Contains(target.name, "attempt") {
					if !strings.Contains(text, attempts[target.job]+" (selected)") || !strings.Contains(text, "true 0") {
						t.Errorf("older attempt not selected: %s", text)
					}
				} else if !strings.Contains(text, "true 1") {
					t.Errorf("latest attempt not selected: %s", text)
				}
			})
		}
	}

	t.Run("environment", func(t *testing.T) {
		t.Setenv("ROTARI_SHOW_NO_LOGS", "true")
		var output bytes.Buffer
		code := captureShowStdout(t, &output, func() int {
			return cmdShow([]string{"--basedir", baseDir, "-p", "demo", "-j", "plain", "--no-pager"})
		})
		if code != 0 || !strings.Contains(output.String(), "Command:") || strings.Contains(output.String(), "hidden-") {
			t.Fatalf("environment default: exit=%d output=%s", code, output.String())
		}
	})

	for _, args := range [][]string{
		{}, {"--run-id", runID}, {"--job-id", "array"},
		{"--job-id", "plain", "--logs"}, {"--job-id", "plain", "--failed-logs"},
		{"--job-id", "plain", "--follow"}, {"--job-id", "plain", "--stream", "stdout"},
		{"--job-id", "plain", "--tail", "1"}, {"--job-id", "plain", "--tail", "0"},
		{"--job-id", "plain", "--json"}, {"--job-id", "plain", "--report"},
		{"--job-id", "plain", "--artifacts"}, {"--job-id", "plain", "--wide"},
	} {
		t.Run("reject/"+strings.Join(args, "_"), func(t *testing.T) {
			options := append([]string{"--basedir", baseDir, "-p", "demo", "--no-logs"}, args...)
			code, stderr := captureStderr(t, func() int { return cmdShow(options) })
			if code != 1 || !strings.Contains(stderr, "--no-logs") {
				t.Fatalf("exit=%d stderr=%q", code, stderr)
			}
		})
	}

	t.Run("queued", func(t *testing.T) {
		if err := writeJSON(paths.QueueFile, queue); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(paths.MetaFile, model.Meta{Phase: "collecting", LastRunID: runID}); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(paths.LockFile); err != nil {
			t.Fatal(err)
		}
		for _, view := range [][]string{nil, {"--queue"}} {
			args := append([]string{"--basedir", baseDir, "-p", "demo", "--no-logs"}, view...)
			var output bytes.Buffer
			code := captureShowStdout(t, &output, func() int { return cmdShow(append(args, "-j", "plain")) })
			if code != 0 || !strings.Contains(output.String(), "Command: true") || !strings.Contains(output.String(), "Job: plain") {
				t.Fatalf("queue details: exit=%d output=%s", code, output.String())
			}
			code, stderr := captureStderr(t, func() int { return cmdShow(append(args, "-j", "array")) })
			if code != 1 || !strings.Contains(stderr, noLogsJobError) {
				t.Fatalf("whole queued array accepted: exit=%d stderr=%s", code, stderr)
			}
		}
	})
}

func TestShowNoLogsPreservesCarriedDetails(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	job := model.QueuedCommand{ID: "job-1", Command: []string{"true"}}
	originDir := filepath.Join(paths.RunsDir, "source", job.ID)
	if err := writeJSON(filepath.Join(paths.RunsDir, "source", "commands.json"), model.Queue{Commands: []model.QueuedCommand{job}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(originDir, "command.json"), model.JobSpec{ID: job.ID, Command: job.Command}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(originDir, "output"), []byte("hidden-carried-log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	job.Origin = &model.JobOrigin{RunID: "source", JobID: job.ID, Status: "success"}
	if err := writeJSON(filepath.Join(paths.RunsDir, "destination", "commands.json"), model.Queue{Commands: []model.QueuedCommand{job}}); err != nil {
		t.Fatal(err)
	}
	for _, accepted := range []bool{false, true} {
		if err := writeJSON(filepath.Join(paths.RunsDir, "destination", "summary.json"), model.RunSummary{
			RunID: "destination", Status: "finished", Results: []model.JobResult{{ID: job.ID, Accepted: accepted}},
		}); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if code := showJobAttempt(&output, paths, "destination", job.ID, "", 0, true); code != 0 {
			t.Fatalf("exit=%d output=%s", code, output.String())
		}
		note := "carried forward from run source"
		if accepted {
			note = "success (accepted)"
		}
		for _, want := range []string{note, "Run: source", "Command: true"} {
			if !strings.Contains(output.String(), want) {
				t.Errorf("missing %q in %s", want, output.String())
			}
		}
		if strings.Contains(output.String(), "hidden-carried-log") || strings.Contains(output.String(), "OUTPUT:") {
			t.Fatalf("carried log not suppressed: %s", output.String())
		}
	}
}

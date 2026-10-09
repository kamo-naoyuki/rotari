package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCmdInfoReportsContextAndActiveRun(t *testing.T) {
	baseDir, masterDir := setupInfoActiveRun(t)
	report := runInfoJSON(t, "--basedir", baseDir, "--project-name", "demo", "--masterdir", masterDir)
	if report.MasterDir != masterDir || report.BaseDir != baseDir || report.Project != "demo" {
		t.Fatalf("resolved context = master %q, basedir %q, project %q", report.MasterDir, report.BaseDir, report.Project)
	}
	if len(report.VisibleConfigs.Common) == 0 || len(report.VisibleConfigs.Projects["demo"]) == 0 {
		t.Fatalf("config visibility = %#v", report.VisibleConfigs)
	}
	if len(report.RunLocks) != 1 || report.RunLocks[0].State != state.LockActive || report.RunLocks[0].Coordinator == nil || !*report.RunLocks[0].Coordinator {
		t.Fatalf("run locks = %#v, want an active local coordinator", report.RunLocks)
	}
	if len(report.ActiveRuns) != 1 || report.ActiveRuns[0].RunID != "run-1" {
		t.Fatalf("active runs = %#v, want run-1", report.ActiveRuns)
	}
	if got := report.ActiveRuns[0].Jobs; got == nil || got.Finished != 0 || got.Alive != 1 || got.Gone != 0 || got.Unknown != 0 {
		t.Fatalf("job liveness = %#v, want one live local process group", got)
	}
}

func setupInfoActiveRun(t *testing.T) (baseDir, masterDir string) {
	t.Helper()
	baseDir, masterDir = t.TempDir(), t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	jobDir := filepath.Join(runDir, "job-a")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeInfoFile(t, filepath.Join(baseDir, "config.json"), `{"executor":"local"}`)
	projectConfig := filepath.Join(paths.ProjectDir, "config.json")
	writeInfoFile(t, projectConfig, `{"executor":"local"}`)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(state.ContextPath(runDir), model.RunContext{Hostname: host}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), model.JobSpec{ID: "job-a", Executor: "local"}); err != nil {
		t.Fatal(err)
	}
	process := exec.Command("sleep", "30")
	process.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-process.Process.Pid, syscall.SIGKILL)
		_ = process.Wait()
	})
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), []byte(fmt.Sprintf("%d\n", process.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	return baseDir, masterDir
}

func writeInfoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runInfoJSON(t *testing.T, args ...string) infoReport {
	t.Helper()
	var output bytes.Buffer
	code := captureShowStdout(t, &output, func() int { return run(append([]string{"info"}, append(args, "--json")...)) })
	if code != 0 {
		t.Fatalf("info exit code = %d, output = %s", code, output.String())
	}
	var report infoReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode info JSON: %v\n%s", err, output.String())
	}
	return report
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

func TestCmdInfoMarksUncreatedProjectWithoutMoreLines(t *testing.T) {
	baseDir := t.TempDir()
	masterDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var text bytes.Buffer
	code := captureShowStdout(t, &text, func() int {
		return run([]string{"info", "--basedir", baseDir, "--project-name", "default", "--masterdir", masterDir})
	})
	if code != 0 {
		t.Fatalf("info exit code = %d, output = %s", code, text.String())
	}
	if !strings.Contains(text.String(), "Project:   default (not created)") {
		t.Fatalf("missing uncreated-project marker: %s", text.String())
	}
	if lines := strings.Count(text.String(), "\n"); lines > 20 {
		t.Fatalf("empty info output grew beyond the 20-line glance budget: %d lines\n%s", lines, text.String())
	}

	var jsonOutput bytes.Buffer
	code = captureShowStdout(t, &jsonOutput, func() int {
		return run([]string{"info", "--basedir", baseDir, "--project-name", "default", "--masterdir", masterDir, "--json"})
	})
	if code != 0 {
		t.Fatalf("info --json exit code = %d, output = %s", code, jsonOutput.String())
	}
	var report infoReport
	if err := json.Unmarshal(jsonOutput.Bytes(), &report); err != nil {
		t.Fatalf("decode info JSON: %v\n%s", err, jsonOutput.String())
	}
	if report.ProjectExists {
		t.Fatalf("project_exists = true for absent project %q", report.Project)
	}
}

func TestInfoJobLivenessUnknownAndTerminalAttempts(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, hostname, executor, pid, status string
		finalResult                           bool
		want                                  infoJobLiveness
	}{
		{"gone", host, "local", "999999999", "", false, infoJobLiveness{Gone: 1}},
		{"missing-pid", host, "local", "", "", false, infoJobLiveness{Unknown: 1}},
		{"invalid-pid", host, "local", "-1", "", false, infoJobLiveness{Unknown: 1}},
		{"remote", host + "-remote", "local", "999999999", "", false, infoJobLiveness{Unknown: 1}},
		{"missing-host", "", "local", "999999999", "", false, infoJobLiveness{Unknown: 1}},
		{"scheduler", host, "slurm", "999999999", "", false, infoJobLiveness{Unknown: 1}},
		{"finished", host, "local", "999999999", "0", true, infoJobLiveness{Finished: 1}},
		{"retry-pending", host, "local", "999999999", "1", false, infoJobLiveness{Pending: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runDir := writeInfoLivenessFixture(t, test.hostname, test.executor, test.pid, test.status, test.finalResult)
			got, err := infoRunJobLiveness(runDir)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				got = &infoJobLiveness{}
			}
			if *got != test.want {
				t.Fatalf("liveness = %+v, want %+v", *got, test.want)
			}
		})
	}
}

func TestInfoJobLivenessCountsCarriedResultsAsFinished(t *testing.T) {
	runDir := t.TempDir()
	if err := state.WriteJSON(filepath.Join(runDir, state.CarriedResultsFileName), model.RunSummary{
		Results: []model.JobResult{{ID: "kept-1"}, {ID: "kept-2"}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := infoRunJobLiveness(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Finished != 2 {
		t.Fatalf("job liveness = %#v, want two finished carried jobs", got)
	}
}

func TestInfoJobLivenessCountsJobsThatHaveNotStartedAsPending(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	runDir := writeInfoLivenessFixture(t, host, "local", "999999999", "", false)
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "job-a", Command: []string{"sleep", "60"}},
		{ID: "waiting-1", Command: []string{"true"}, DependsOn: []string{"job-a"}},
		{ID: "waiting-2", Command: []string{"true"}, DependsOn: []string{"job-a"}},
		{ID: "kept", Command: []string{"true"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, state.CarriedResultsFileName), model.RunSummary{
		Results: []model.JobResult{{ID: "kept"}},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := infoRunJobLiveness(runDir)
	if err != nil {
		t.Fatal(err)
	}
	want := infoJobLiveness{Finished: 1, Pending: 2, Gone: 1}
	if got == nil || *got != want {
		t.Fatalf("job liveness = %+v, want %+v", got, want)
	}
}

func writeInfoLivenessFixture(t *testing.T, hostname, executorName, pid, status string, finalResult bool) string {
	t.Helper()
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-a")
	if err := state.WriteJSON(state.ContextPath(runDir), model.RunContext{Hostname: hostname}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), model.JobSpec{ID: "job-a", Executor: executorName}); err != nil {
		t.Fatal(err)
	}
	if pid != "" {
		writeInfoFile(t, filepath.Join(jobDir, "pid"), pid)
	}
	if status != "" {
		writeInfoFile(t, filepath.Join(jobDir, "status"), status)
	}
	if finalResult {
		if err := state.WriteJSON(filepath.Join(jobDir, state.FinalResultFileName), model.JobResult{ID: "job-a"}); err != nil {
			t.Fatal(err)
		}
	}
	return runDir
}

func TestInfoJobCountsStayOnRunLine(t *testing.T) {
	var output bytes.Buffer
	captureShowStdout(t, &output, func() int {
		printInfoRuns([]infoRun{{Project: "demo", RunID: "run-1", Jobs: &infoJobLiveness{Finished: 4, Pending: 5, Alive: 2, Gone: 1, Unknown: 3}}})
		return 0
	})
	if strings.Count(output.String(), "\n") != 2 || !strings.Contains(output.String(), "jobs=finished:4 pending:5 alive:2 gone:1 unknown:3") {
		t.Fatalf("job counts must stay on the existing run line:\n%s", output.String())
	}
}

func TestInfoColorsTTYButNotPipesOrJSON(t *testing.T) {
	baseDir, masterDir := t.TempDir(), t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	oldTerminalCheck := terminalCheck
	defer func() { terminalCheck = oldTerminalCheck }()

	var colored bytes.Buffer
	terminalCheck = func(*os.File) bool { return true }
	code := captureShowStdout(t, &colored, func() int {
		return run([]string{"info", "--basedir", baseDir, "--project-name", "demo", "--masterdir", masterDir})
	})
	if code != 0 {
		t.Fatalf("colored info exit code = %d, output = %s", code, colored.String())
	}
	for _, want := range []string{"\033[36mMasterdir:\033[0m", "\033[36mConfig files visible:\033[0m", "\033[33m(not created)\033[0m"} {
		if !strings.Contains(colored.String(), want) {
			t.Fatalf("TTY output missing color %q:\n%s", want, colored.String())
		}
	}
	if strings.Contains(colored.String(), "Loaded config sources:") {
		t.Fatalf("text output retained loaded-source section:\n%s", colored.String())
	}

	var plain bytes.Buffer
	terminalCheck = func(*os.File) bool { return false }
	code = captureShowStdout(t, &plain, func() int {
		return run([]string{"info", "--basedir", baseDir, "--project-name", "demo", "--masterdir", masterDir})
	})
	if code != 0 || strings.Contains(plain.String(), "\033[") || strings.Contains(plain.String(), "Loaded config sources:") {
		t.Fatalf("pipe output code=%d contains ANSI or loaded-source section:\n%s", code, plain.String())
	}

	jsonReport := runInfoJSON(t, "--basedir", baseDir, "--project-name", "demo", "--masterdir", masterDir)
	encoded, err := json.Marshal(jsonReport)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "loaded_config_sources") || strings.Contains(string(encoded), "\033[") {
		t.Fatalf("JSON contains removed config sources or ANSI: %s", encoded)
	}
}

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/report"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/webui"
)

func createAIReportFixture(t *testing.T) (string, state.ProjectPaths, string, string) {
	t.Helper()
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID, jobID := "run-1", "job-1"
	runDir := filepath.Join(paths.RunsDir, runID)
	jobDir := filepath.Join(runDir, jobID)
	command := model.QueuedCommand{ID: jobID, Name: "train", Command: []string{"python", "train.py"}, Executor: "slurm", DependsOn: []string{"prepare"}}
	if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{command}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "failed", ExitCode: 1, Results: []model.JobResult{{ID: jobID, ExitCode: 1, Error: "exit status 1", Diagnoses: []model.RuleDiagnosis{{Name: "Python exception", Evidence: "ValueError: bad value", Suggestion: "Inspect the traceback"}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "context.json"), model.RunContext{CWD: "/work/demo", Hostname: "worker-1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 120)
	for index := range lines {
		lines[index] = "log-line-" + fmt.Sprintf("%03d", index+1)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "output"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return baseDir, paths, runID, jobID
}

func TestCmdShowReportRejectsCombinedFlags(t *testing.T) {
	baseDir := t.TempDir()
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := cmdShow([]string{"--basedir", baseDir, "--report", "--json"})
	_ = writer.Close()
	os.Stderr = oldStderr
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("cmdShow(... --report --json) = %d, want 1", code)
	}
	if !strings.Contains(string(output), "--report cannot be combined with") {
		t.Fatalf("stderr = %q, want report-combination error", output)
	}
}

func TestShowReportAndWebAPIUseCommonReport(t *testing.T) {
	baseDir, paths, runID, jobID := createAIReportFixture(t)
	want, err := report.Build(jsonStore(), paths, runID, jobID, false, "")
	if err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", runID, "--job-id", jobID, "--report"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || string(output) != want {
		t.Fatalf("show --report code=%d output=%q, want %q", code, output, want)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/report?project_name=demo&run_id="+runID+"&job_id="+jobID, nil)
	recorder := httptest.NewRecorder()
	webui.Handler(webOptions(baseDir, "", false, true)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != want {
		t.Fatalf("report API status=%d body=%q, want %q", recorder.Code, recorder.Body.String(), want)
	}
}

func TestCmdShowReportSelectsAttemptID(t *testing.T) {
	baseDir, paths, fixtureRunID, jobID := createAIReportFixture(t)
	runID := "20260922-010000-00000000"
	if err := os.Rename(filepath.Join(paths.RunsDir, fixtureRunID), filepath.Join(paths.RunsDir, runID)); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	oldAttemptID := makeAttemptID(runID, jobID, 0)
	latestAttemptID := makeAttemptID(runID, jobID, 1)
	oldAttemptDir, err := specificAttemptJobDir(runDir, jobID, oldAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	latestAttemptDir, err := specificAttemptJobDir(runDir, jobID, latestAttemptID)
	if err != nil {
		t.Fatal(err)
	}
	for _, attemptDir := range []string{oldAttemptDir, latestAttemptDir} {
		if err := os.MkdirAll(attemptDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(oldAttemptDir, stateFileStatus), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldAttemptDir, stateFileFinishedAt), []byte("2026-09-22T01:00:00Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldAttemptDir, stateFileOutput), []byte("old-attempt-log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "finished", Results: []model.JobResult{{ID: jobID, AttemptID: latestAttemptID, ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--job-id", oldAttemptID, "--report"})
	_ = writer.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- Attempt ID: `" + oldAttemptID + "`", "- Status: failed", "old-attempt-log"} {
		if code != 0 || !strings.Contains(string(output), want) {
			t.Fatalf("show --report attempt code=%d output does not contain %q:\n%s", code, want, output)
		}
	}
	if strings.Contains(string(output), latestAttemptID) {
		t.Fatalf("show --report attempt contains latest attempt ID %q:\n%s", latestAttemptID, output)
	}
}

func TestWebAPISelectedJobsUsesRunReport(t *testing.T) {
	baseDir, _, runID, jobID := createAIReportFixture(t)
	request := httptest.NewRequest(http.MethodGet, "/api/report?project_name=demo&run_id="+runID+"&job_ids="+jobID, nil)
	recorder := httptest.NewRecorder()
	webui.Handler(webOptions(baseDir, "", false, true)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("selected report API status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "# rotari run report") {
		t.Fatalf("selected report is not a run report:\n%s", recorder.Body.String())
	}
}

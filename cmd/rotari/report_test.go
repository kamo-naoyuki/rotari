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
)

func createAIReportFixture(t *testing.T) (string, pathSet, string, string) {
	t.Helper()
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID, jobID := "run-1", "job-1"
	runDir := filepath.Join(paths.runsDir, runID)
	jobDir := filepath.Join(runDir, jobID)
	command := QueuedCommand{ID: jobID, Name: "train", Command: []string{"python", "train.py"}, Executor: "slurm", DependsOn: []string{"prepare"}}
	if err := writeJSON(paths.queueFile, Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), Queue{Commands: []QueuedCommand{command}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{RunID: runID, Status: "failed", ExitCode: 1, Results: []JobResult{{ID: jobID, ExitCode: 1, Error: "exit status 1", Diagnoses: []ruleDiagnosis{{Name: "Python exception", Evidence: "ValueError: bad value", Suggestion: "Inspect the traceback"}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "context.json"), RunContext{CWD: "/work/demo", Hostname: "worker-1"}); err != nil {
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

func TestBuildAIReportIncludesDiagnosisAndBoundedLog(t *testing.T) {
	_, paths, runID, jobID := createAIReportFixture(t)
	report, err := buildAIReport(paths, runID, jobID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# rotari job report", "Python exception", "Evidence: ValueError: bad value", "Next: Inspect the traceback", "log-line-021", "log-line-120"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report does not contain %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "log-line-020") {
		t.Fatalf("report contains log content before the final %d lines", reportLogLines)
	}
}

func TestBuildAIReportRedactsKnownAndTypicalSensitiveValues(t *testing.T) {
	_, paths, runID, jobID := createAIReportFixture(t)
	runDir := filepath.Join(paths.runsDir, runID)
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte(`{"cwd":"/work/demo","hostname":"worker-1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.runsDir, runID, jobID, "output"), []byte("failed at /home/alice/private.txt on node-1.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := buildAIReport(paths, runID, jobID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"/work/demo", "worker-1", "/home/alice/private.txt", "node-1.example.com"} {
		if strings.Contains(report, unwanted) {
			t.Fatalf("report contains unredacted value %q:\n%s", unwanted, report)
		}
	}
	for _, want := range []string{"[REDACTED_PATH]", "[REDACTED_HOST]", "complete redaction is not guaranteed"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report does not contain %q:\n%s", want, report)
		}
	}
}

func TestBuildAIReportRedactsMultipleSecretsAndKeepsTheFirstVisibleMarker(t *testing.T) {
	_, paths, runID, jobID := createAIReportFixture(t)
	runDir := filepath.Join(paths.runsDir, runID)
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte(`{"cwd":"/tmp/build-logs/run-42","hostname":"cluster-gpu-01.example.com"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, jobID, "output"), []byte("error: /home/alice/project/data/checkpoint.bin on cluster-gpu-01.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := buildAIReport(paths, runID, jobID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"/tmp/build-logs/run-42", "cluster-gpu-01.example.com", "/home/alice/project/data/checkpoint.bin"} {
		if strings.Contains(report, unwanted) {
			t.Fatalf("report contains unredacted value %q:\n%s", unwanted, report)
		}
	}
	if !strings.Contains(report, "[REDACTED_PATH]") || !strings.Contains(report, "[REDACTED_HOST]") {
		t.Fatalf("report should redact both paths and hostnames:\n%s", report)
	}
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

func TestBuildJobAIReportIncludesSuccessfulJobLog(t *testing.T) {
	_, paths, runID, jobID := createAIReportFixture(t)
	if err := writeJSON(filepath.Join(paths.runsDir, runID, "summary.json"), RunSummary{RunID: runID, Status: "finished", Results: []JobResult{{ID: jobID, ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	report, err := buildAIReport(paths, runID, jobID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "- Status: success") || !strings.Contains(report, "log-line-120") {
		t.Fatalf("successful job report does not contain its status and log:\n%s", report)
	}
}

func TestShowReportAndWebAPIUseCommonReport(t *testing.T) {
	baseDir, paths, runID, jobID := createAIReportFixture(t)
	want, err := buildAIReport(paths, runID, jobID, false)
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
	newWebHandler(baseDir, "", false).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != want {
		t.Fatalf("report API status=%d body=%q, want %q", recorder.Code, recorder.Body.String(), want)
	}
}

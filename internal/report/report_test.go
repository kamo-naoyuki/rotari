package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/web"
)

func testStore() state.Store {
	return state.NewStore(0o755, 0o644)
}

func createFixture(t *testing.T) (string, state.ProjectPaths, string, string) {
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
	if err := state.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{command}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "failed", ExitCode: 1, Results: []model.JobResult{{ID: jobID, ExitCode: 1, Error: "exit status 1", Diagnoses: []model.RuleDiagnosis{{Name: "Python exception", Evidence: "ValueError: bad value", Suggestion: "Inspect the traceback"}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{CWD: "/work/demo", Hostname: "worker-1"}); err != nil {
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

func TestBuildIncludesDiagnosisAndBoundedLog(t *testing.T) {
	_, paths, runID, jobID := createFixture(t)
	report, err := Build(testStore(), paths, runID, jobID, false, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# rotari job report", "Python exception", "Evidence: ValueError: bad value", "Next: Inspect the traceback", diagnose.OutdatedNote, "log-line-021", "log-line-120"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report does not contain %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "log-line-020") {
		t.Fatalf("report contains log content before the final %d lines", reportLogLines)
	}
}

func TestBuildRedactsKnownAndTypicalSensitiveValues(t *testing.T) {
	_, paths, runID, jobID := createFixture(t)
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte(`{"cwd":"/work/demo","hostname":"worker-1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.RunsDir, runID, jobID, "output"), []byte("failed at /home/alice/private.txt on node-1.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Build(testStore(), paths, runID, jobID, false, "")
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

func TestBuildRedactsMultipleSecretsAndKeepsTheFirstVisibleMarker(t *testing.T) {
	_, paths, runID, jobID := createFixture(t)
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte(`{"cwd":"/tmp/build-logs/run-42","hostname":"cluster-gpu-01.example.com"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, jobID, "output"), []byte("error: /home/alice/project/data/checkpoint.bin on cluster-gpu-01.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Build(testStore(), paths, runID, jobID, false, "")
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

func TestBuildJobReportIncludesSuccessfulJobLog(t *testing.T) {
	_, paths, runID, jobID := createFixture(t)
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{RunID: runID, Status: "finished", Results: []model.JobResult{{ID: jobID, ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	report, err := Build(testStore(), paths, runID, jobID, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "- Status: success") || !strings.Contains(report, "log-line-120") {
		t.Fatalf("successful job report does not contain its status and log:\n%s", report)
	}
}

func TestFormatRunReportIncludesFailedJobsOnly(t *testing.T) {
	run := web.Run{
		RunSummary: model.RunSummary{RunID: "run-1", Status: "failed", ExitCode: 1, StartedAt: "start", FinishedAt: "finish"},
		CWD:        "/work/project",
		Jobs: []web.Job{
			{ID: "failed", Name: "failed-job", Result: &model.JobResult{ID: "failed", ExitCode: 1, Error: "boom"}},
			{ID: "success", Name: "success-job", Result: &model.JobResult{ID: "success", ExitCode: 0}},
		},
	}
	paths := state.ProjectPaths{ProjectName: "demo"}
	report := formatRunAIReport(paths, run, true)
	if !strings.Contains(report, "failed-job") || strings.Contains(report, "success-job") {
		t.Fatalf("failed-only report = %s", report)
	}
}

func TestReportStatusAndValueHelpers(t *testing.T) {
	if got := reportJobStatus(web.Job{SchedulerState: "pending"}, false); got != "pending" {
		t.Fatalf("scheduler status = %q", got)
	}
	if got := reportJobStatus(web.Job{}, true); got != "running" {
		t.Fatalf("running status = %q", got)
	}
	if got := reportJobStatus(web.Job{}, false); got != "pending" {
		t.Fatalf("pending status = %q", got)
	}
	if got := reportJobStatus(web.Job{Result: &model.JobResult{Error: "blocked by dependency"}}, false); got != "blocked" {
		t.Fatalf("blocked status = %q", got)
	}
	if got := reportJobStatus(web.Job{Result: &model.JobResult{ExitCode: 0}}, false); got != "success" {
		t.Fatalf("success status = %q", got)
	}
	if got := reportJobStatus(web.Job{Result: &model.JobResult{ExitCode: 1}}, false); got != "failed" {
		t.Fatalf("failed status = %q", got)
	}
	if reportValue("") != "-" || reportValue("value") != "value" {
		t.Fatal("report value helpers returned unexpected results")
	}
}

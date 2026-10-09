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
	if err := os.WriteFile(filepath.Join(jobDir, state.StdoutFileName), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return baseDir, paths, runID, jobID
}

func TestBuildIncludesDiagnosisAndBoundedLog(t *testing.T) {
	_, paths, runID, jobID := createFixture(t)
	report, err := Build(testStore(), paths, runID, jobID, false, "", true)
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
	if err := os.WriteFile(filepath.Join(paths.RunsDir, runID, jobID, state.StderrFileName), []byte("failed at /home/alice/private.txt on node-1.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Build(testStore(), paths, runID, jobID, false, "", true)
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
	if err := os.WriteFile(filepath.Join(runDir, jobID, state.StderrFileName), []byte("error: /home/alice/project/data/checkpoint.bin on cluster-gpu-01.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Build(testStore(), paths, runID, jobID, false, "", true)
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
	report, err := Build(testStore(), paths, runID, jobID, false, "", true)
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
			{ID: "failed", Name: "failed-job", ExecutionStatus: "failed", Result: &model.JobResult{ID: "failed", ExitCode: 1, Error: "boom"}},
			{ID: "cancelled", Name: "cancelled-job", ExecutionStatus: "cancelled", Result: &model.JobResult{ID: "cancelled", ExitCode: 130, Error: model.MarkedCancelledError}},
			{ID: "kept", Name: "kept-job", ExecutionStatus: "failed (carried)", Carried: true, Result: &model.JobResult{ID: "kept", ExitCode: 2}},
			{ID: "success", Name: "success-job", ExecutionStatus: "success", Result: &model.JobResult{ID: "success", ExitCode: 0}},
			{ID: "waiting", Name: "waiting-job", ExecutionStatus: "not started"},
		},
	}
	paths := state.ProjectPaths{ProjectName: "demo"}
	report := formatRunAIReport(paths, run, true)
	for _, name := range []string{"failed-job", "cancelled-job", "kept-job"} {
		if !strings.Contains(report, name) {
			t.Errorf("failed-only report omits %s:\n%s", name, report)
		}
	}
	if strings.Contains(report, "success-job") || strings.Contains(report, "waiting-job") {
		t.Fatalf("failed-only report = %s", report)
	}
	if !strings.Contains(report, "- Status: cancelled\n") {
		t.Fatalf("cancelled job is not labeled as show labels it:\n%s", report)
	}
}

// Reports label jobs as show does: the shared execution status, accepted
// results as success (accepted), and carried results marked as carried.
func TestReportJobStatusMatchesShowLabels(t *testing.T) {
	for _, test := range []struct {
		job  web.Job
		want string
	}{
		{web.Job{ExecutionStatus: "waiting (recorded)", SchedulerState: "pending"}, "waiting (recorded)"},
		{web.Job{ExecutionStatus: "running (recorded)"}, "running (recorded)"},
		{web.Job{ExecutionStatus: "not started"}, "not started"},
		{web.Job{ExecutionStatus: "cancelled", Result: &model.JobResult{ExitCode: 130}}, "cancelled"},
		{web.Job{ExecutionStatus: "blocked", Result: &model.JobResult{ExitCode: 1, Error: "blocked by dependency"}}, "blocked"},
		{web.Job{ExecutionStatus: "failed (carried)", Carried: true, Result: &model.JobResult{ExitCode: 1}}, "failed (carried)"},
		{web.Job{ExecutionStatus: "success", Result: &model.JobResult{ExitCode: 0, Accepted: true}}, "success (accepted)"},
		{web.Job{ExecutionStatus: "success (carried)", Carried: true, Result: &model.JobResult{ExitCode: 0, Accepted: true}}, "success (accepted) (carried)"},
		{web.Job{}, "unknown"},
	} {
		if got := reportJobStatus(test.job); got != test.want {
			t.Errorf("reportJobStatus(%+v) = %q, want %q", test.job, got, test.want)
		}
	}
}

func TestReportValueHelpers(t *testing.T) {
	if reportValue("") != "-" || reportValue("value") != "value" {
		t.Fatal("report value helpers returned unexpected results")
	}
}

func TestReportLogExcerptKeepsEvidenceFarFromTheEnd(t *testing.T) {
	lines := make([]string, 300)
	for index := range lines {
		lines[index] = fmt.Sprintf("line-%03d", index+1)
	}
	lines[49] = "ValueError: bad value"
	lines[199] = "WARNING: retrying shard"
	output, description := reportLogExcerpt(strings.Join(lines, "\n"), []string{"ValueError: bad value", "retrying shard", ""})
	want := []string{
		"line-030", "ValueError: bad value", "line-055", // around the first evidence
		"[... 124 lines omitted ...]",
		"line-180", "WARNING: retrying shard", "line-205", // around the second evidence
		"[... 75 lines omitted ...]",
		"line-281", "line-300", // the final lines
	}
	for _, text := range want {
		if !strings.Contains(output, text) {
			t.Errorf("excerpt does not contain %q:\n%s", text, output)
		}
	}
	for _, text := range []string{"line-029", "line-056", "line-179", "line-206", "line-280"} {
		if strings.Contains(output, text+"\n") {
			t.Errorf("excerpt contains omitted %q:\n%s", text, output)
		}
	}
	if !strings.Contains(description, "around the diagnosis evidence") {
		t.Errorf("description = %q", description)
	}
}

func TestReportLogExcerptFallsBackToTheTail(t *testing.T) {
	lines := make([]string, 150)
	for index := range lines {
		lines[index] = fmt.Sprintf("line-%03d", index+1)
	}
	output, description := reportLogExcerpt(strings.Join(lines, "\n"), []string{"not in the log"})
	if strings.Contains(output, "line-050") || !strings.Contains(output, "line-051") || !strings.Contains(output, "line-150") || strings.Contains(output, "omitted") {
		t.Errorf("excerpt is not the last %d lines:\n%s", reportLogLines, output)
	}
	if description != fmt.Sprintf("last %d lines, at most %d characters", reportLogLines, reportLogChars) {
		t.Errorf("description = %q", description)
	}
	if output, _ := reportLogExcerpt("", []string{"anything"}); output != "" {
		t.Errorf("empty log excerpt = %q", output)
	}
}

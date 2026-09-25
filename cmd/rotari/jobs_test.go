package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestPrintJobsTableAlignsMultipleRows(t *testing.T) {
	rows := []jobsRow{
		{state: "success", project: "demo", attemptID: "att_20260922-103800-afc43f9f-6aa4af5d9-1-0", startedAt: time.Date(2026, 9, 22, 10, 38, 0, 0, time.UTC), elapsed: 2 * time.Second},
		{state: "failed", project: "demo", attemptID: "att_20260922-103755-cf6e9512-6aa4af5d9-2-0", startedAt: time.Date(2026, 9, 22, 10, 37, 55, 0, time.UTC), elapsed: 2 * time.Second},
	}
	var output bytes.Buffer
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	printJobsTable(rows, false)
	_ = writer.Close()
	os.Stdout = oldStdout
	if _, err := output.ReadFrom(reader); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("printJobsTable() wrote %d lines, want 3: %q", len(lines), output.String())
	}
	if !strings.HasPrefix(lines[0], "STATE") || !strings.Contains(lines[1], rows[0].attemptID) || !strings.Contains(lines[2], rows[1].attemptID) {
		t.Fatalf("unexpected jobs table output: %q", output.String())
	}
}

func TestCollectJobsAcrossProjectsIncludesRecentFinishedJobs(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	longRunningStarted := now.Add(-48 * time.Hour)
	recentFinished := now.Add(-2 * time.Hour)
	oldFinished := now.Add(-48 * time.Hour)

	writeTestJobsRun(t, baseDir, "train", "20260922-090000-00000001", "train-job", longRunningStarted, recentFinished, 1)
	writeTestJobsRun(t, baseDir, "report", "20260922-080000-00000002", "report-job", longRunningStarted.Add(time.Hour), recentFinished.Add(time.Hour), 0)
	writeTestJobsRun(t, baseDir, "train", "20260920-090000-00000003", "old-job", oldFinished.Add(-time.Minute), oldFinished, 1)

	rows, err := collectJobs(baseDir, []string{"report", "train"}, now, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("collectJobs() returned %d rows, want 2: %#v", len(rows), rows)
	}
	if rows[0].project != "report" || rows[0].state != "success" {
		t.Fatalf("first row = %#v, want recent report success", rows[0])
	}
	if rows[0].elapsed != 46*time.Hour {
		t.Fatalf("success elapsed = %s, want 46h", rows[0].elapsed)
	}
	if rows[1].project != "train" || rows[1].state != "failed" {
		t.Fatalf("second row = %#v, want recent train failure", rows[1])
	}
	if rows[1].elapsed != 46*time.Hour {
		t.Fatalf("failure elapsed = %s, want 46h", rows[1].elapsed)
	}
	for _, row := range rows {
		if strings.Contains(row.attemptID, "old-job") {
			t.Fatalf("old job was included: %#v", row)
		}
	}
}

func TestCmdJobsAcceptsPositionalProjectName(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Now()
	writeTestJobsRun(t, baseDir, "train", "20260922-090000-00000001", "train-job", now.Add(-2*time.Minute), now.Add(-time.Minute), 0)
	writeTestJobsRun(t, baseDir, "report", "20260922-080000-00000002", "report-job", now.Add(-2*time.Minute), now.Add(-time.Minute), 0)
	t.Setenv(envProjectName, "report")

	output, code := captureJobsStdout(t, []string{"--basedir", baseDir, "--since", "24h", "train"})
	if code != 0 {
		t.Fatalf("cmdJobs positional project exit code = %d, want 0", code)
	}
	if !strings.Contains(output, "train-job") || strings.Contains(output, "report-job") {
		t.Fatalf("cmdJobs positional project output = %q", output)
	}
	if code := cmdJobs([]string{"--basedir", baseDir, "--project-name", "train", "report"}); code != 1 {
		t.Fatalf("cmdJobs accepted positional project with --project-name: exit code = %d", code)
	}
}

func captureJobsStdout(t *testing.T, args []string) (string, int) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	code := cmdJobs(args)
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output), code
}

func TestCollectRunJobsStopsAtOldCompletedRun(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	writeTestJobsRun(t, baseDir, "demo", "20260920-090000-00000001", "old-job", now.Add(-48*time.Hour), now.Add(-25*time.Hour), 0)

	rows, include, stop, err := collectRunJobs(paths, "20260920-090000-00000001", now, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if include || len(rows) != 0 || !stop {
		t.Fatalf("collectRunJobs() = rows %d, include %v, stop %v; want no rows, no include, and stop", len(rows), include, stop)
	}
}

func TestCollectRunJobsIncludesTerminalJobUsingStartedAtWhenFinishedAtMissing(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	started := now.Add(-2 * time.Hour)
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260922-090000-00000001"
	jobID := "failed-job"
	attemptID := makeAttemptID(runID, jobID, 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: jobID, Command: []string{"false"}}}}
	if err := writeJSON(filepath.Join(runDir, stateFileCommandsJSON), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, jobID, "attempts", attemptID)
	if err := writeJSON(filepath.Join(jobDir, commandJSONName), model.JobSpec{ID: jobID, AttemptID: attemptID, Command: []string{"false"}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, stateFileSummaryJSON), model.RunSummary{RunID: runID, Status: "failed", StartedAt: started.Format(time.RFC3339), Results: []model.JobResult{{ID: jobID, AttemptID: attemptID, ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, stateFileSubmittedAt), started); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(jobDir, stateFileStatus), []byte("1\n")); err != nil {
		t.Fatal(err)
	}

	rows, include, stop, err := collectRunJobs(paths, runID, now, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !include || stop || len(rows) != 1 {
		t.Fatalf("collectRunJobs() = rows %d, include %v, stop %v; want one row, include, and no stop", len(rows), include, stop)
	}
	if rows[0].state != "failed" || !rows[0].finishedAt.IsZero() || rows[0].elapsed >= 0 {
		t.Fatalf("row = %#v, want failed with unknown finish and elapsed", rows[0])
	}
}

func TestFormatJobElapsed(t *testing.T) {
	cases := map[time.Duration]string{
		0:                             "0s",
		59 * time.Second:              "59s",
		3*time.Minute + 2*time.Second: "3m 02s",
		2*time.Hour + 4*time.Minute:   "2h 04m",
	}
	for input, want := range cases {
		if got := formatJobElapsed(input); got != want {
			t.Errorf("formatJobElapsed(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestParseJobsSince(t *testing.T) {
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{value: "", want: defaultJobsSince},
		{value: "30m", want: 30 * time.Minute},
		{value: "0s", want: 0},
	} {
		got, err := parseJobsSince(test.value)
		if err != nil || got != test.want {
			t.Errorf("parseJobsSince(%q) = (%s, %v), want (%s, nil)", test.value, got, err, test.want)
		}
	}

	for _, value := range []string{"invalid", "-1m", "-1s"} {
		if _, err := parseJobsSince(value); err == nil {
			t.Errorf("parseJobsSince(%q) succeeded, want error", value)
		}
	}
}

func TestParseJobsFormat(t *testing.T) {
	columns, err := parseJobsFormat("%s %.12b %p %.24a %n %.20c %t %f %e")
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 9 || columns[1].code != 'b' || columns[1].width != 12 || columns[5].code != 'c' || columns[5].width != 20 || columns[7].code != 'f' {
		t.Fatalf("parsed columns = %#v", columns)
	}
	for _, format := range []string{"", "state", "%x", "%.0p", "%2p"} {
		if _, err := parseJobsFormat(format); err == nil {
			t.Fatalf("parseJobsFormat(%q) succeeded, want error", format)
		}
	}
}

func writeTestJobsRun(t *testing.T, baseDir, project, runID, jobID string, started, finished time.Time, exitCode int) {
	t.Helper()
	paths, err := resolvePaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	attemptID := makeAttemptID(runID, jobID, 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: jobID, Command: []string{"true"}}}}
	if err := writeJSON(filepath.Join(runDir, stateFileCommandsJSON), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, jobID, "attempts", attemptID)
	if err := writeJSON(filepath.Join(jobDir, commandJSONName), model.JobSpec{ID: jobID, AttemptID: attemptID, Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, stateFileSummaryJSON), model.RunSummary{RunID: runID, Status: model.RunStatus(exitCode), StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), Results: []model.JobResult{{ID: jobID, AttemptID: attemptID, ExitCode: exitCode}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, stateFileSubmittedAt), started); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, stateFileFinishedAt), finished); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(jobDir, stateFileStatus), []byte(formatInt(exitCode)+"\n")); err != nil {
		t.Fatal(err)
	}
}

func writeTestTimestamp(path string, value time.Time) error {
	return writeTestFile(path, []byte(value.Format(time.RFC3339)+"\n"))
}

func writeTestFile(path string, data []byte) error {
	if err := ensureTestDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ensureTestDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func formatInt(value int) string {
	if value == 0 {
		return "0"
	}
	return "1"
}

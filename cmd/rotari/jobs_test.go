package main

import (
	"bytes"
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
	recentStarted := now.Add(-2 * time.Hour)
	recentFinished := recentStarted.Add(3 * time.Minute)
	oldFinished := now.Add(-48 * time.Hour)

	writeTestJobsRun(t, baseDir, "train", "20260922-090000-00000001", "train-job", recentStarted, recentFinished, 1)
	writeTestJobsRun(t, baseDir, "report", "20260922-080000-00000002", "report-job", recentStarted.Add(time.Hour), recentFinished.Add(time.Hour), 0)
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
	if rows[0].elapsed != 3*time.Minute {
		t.Fatalf("success elapsed = %s, want 3m", rows[0].elapsed)
	}
	if rows[1].project != "train" || rows[1].state != "failed" {
		t.Fatalf("second row = %#v, want recent train failure", rows[1])
	}
	if rows[1].elapsed != 3*time.Minute {
		t.Fatalf("failure elapsed = %s, want 3m", rows[1].elapsed)
	}
	for _, row := range rows {
		if strings.Contains(row.attemptID, "old-job") {
			t.Fatalf("old job was included: %#v", row)
		}
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

func TestParseJobsFormat(t *testing.T) {
	columns, err := parseJobsFormat("%s %.12b %p %.24a %n %.20c %t %e")
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 8 || columns[1].code != 'b' || columns[1].width != 12 || columns[5].code != 'c' || columns[5].width != 20 {
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
	queue := Queue{Commands: []QueuedCommand{{ID: jobID, Command: []string{"true"}}}}
	if err := writeJSON(filepath.Join(runDir, stateFileCommandsJSON), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, jobID, "attempts", attemptID)
	if err := writeJSON(filepath.Join(jobDir, commandJSONName), JobSpec{ID: jobID, AttemptID: attemptID, Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, stateFileSummaryJSON), RunSummary{RunID: runID, Status: model.RunStatus(exitCode), StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), Results: []JobResult{{ID: jobID, AttemptID: attemptID, ExitCode: exitCode}}}); err != nil {
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

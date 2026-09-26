package joblist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func testStore() state.Store {
	return state.NewStore(0o755, 0o644)
}

func TestCollectAcrossProjectsIncludesRecentFinishedJobs(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	longRunningStarted := now.Add(-48 * time.Hour)
	recentFinished := now.Add(-2 * time.Hour)
	oldFinished := now.Add(-48 * time.Hour)

	writeTestJobsRun(t, baseDir, "train", "20260922-090000-00000001", "train-job", longRunningStarted, recentFinished, 1)
	writeTestJobsRun(t, baseDir, "report", "20260922-080000-00000002", "report-job", longRunningStarted.Add(time.Hour), recentFinished.Add(time.Hour), 0)
	writeTestJobsRun(t, baseDir, "train", "20260920-090000-00000003", "old-job", oldFinished.Add(-time.Minute), oldFinished, 1)

	rows, err := Collect(testStore(), baseDir, []string{"report", "train"}, now, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("Collect() returned %d rows, want 2: %#v", len(rows), rows)
	}
	if rows[0].Project != "report" || rows[0].State != "success" {
		t.Fatalf("first row = %#v, want recent report success", rows[0])
	}
	if rows[0].Elapsed != 46*time.Hour {
		t.Fatalf("success elapsed = %s, want 46h", rows[0].Elapsed)
	}
	if rows[1].Project != "train" || rows[1].State != "failed" {
		t.Fatalf("second row = %#v, want recent train failure", rows[1])
	}
	if rows[1].Elapsed != 46*time.Hour {
		t.Fatalf("failure elapsed = %s, want 46h", rows[1].Elapsed)
	}
	for _, row := range rows {
		if strings.Contains(row.AttemptID, "old-job") {
			t.Fatalf("old job was included: %#v", row)
		}
	}
}

func TestCollectRunStopsAtOldCompletedRun(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	writeTestJobsRun(t, baseDir, "demo", "20260920-090000-00000001", "old-job", now.Add(-48*time.Hour), now.Add(-25*time.Hour), 0)

	rows, include, stop, err := collectRun(testStore(), paths, "20260920-090000-00000001", now, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if include || len(rows) != 0 || !stop {
		t.Fatalf("collectRun() = rows %d, include %v, stop %v; want no rows, no include, and stop", len(rows), include, stop)
	}
}

func TestCollectRunIncludesTerminalJobUsingStartedAtWhenFinishedAtMissing(t *testing.T) {
	baseDir := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	started := now.Add(-2 * time.Hour)
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260922-090000-00000001"
	jobID := "failed-job"
	attemptID := state.MakeAttemptID(runID, jobID, 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: jobID, Command: []string{"false"}}}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, jobID, "attempts", attemptID)
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), model.JobSpec{ID: jobID, AttemptID: attemptID, Command: []string{"false"}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "failed", StartedAt: started.Format(time.RFC3339), Results: []model.JobResult{{ID: jobID, AttemptID: attemptID, ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, "submitted_at"), started); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(jobDir, "status"), []byte("1\n")); err != nil {
		t.Fatal(err)
	}

	rows, include, stop, err := collectRun(testStore(), paths, runID, now, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !include || stop || len(rows) != 1 {
		t.Fatalf("collectRun() = rows %d, include %v, stop %v; want one row, include, and no stop", len(rows), include, stop)
	}
	if rows[0].State != "failed" || !rows[0].FinishedAt.IsZero() || rows[0].Elapsed >= 0 {
		t.Fatalf("row = %#v, want failed with unknown finish and elapsed", rows[0])
	}
}

func TestFormatElapsed(t *testing.T) {
	cases := map[time.Duration]string{
		0:                             "0s",
		59 * time.Second:              "59s",
		3*time.Minute + 2*time.Second: "3m 02s",
		2*time.Hour + 4*time.Minute:   "2h 04m",
	}
	for input, want := range cases {
		if got := FormatElapsed(input); got != want {
			t.Errorf("FormatElapsed(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestParseSince(t *testing.T) {
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{value: "", want: DefaultSince},
		{value: "30m", want: 30 * time.Minute},
		{value: "0s", want: 0},
	} {
		got, err := ParseSince(test.value)
		if err != nil || got != test.want {
			t.Errorf("ParseSince(%q) = (%s, %v), want (%s, nil)", test.value, got, err, test.want)
		}
	}

	for _, value := range []string{"invalid", "-1m", "-1s"} {
		if _, err := ParseSince(value); err == nil {
			t.Errorf("ParseSince(%q) succeeded, want error", value)
		}
	}
}

func writeTestJobsRun(t *testing.T, baseDir, project, runID, jobID string, started, finished time.Time, exitCode int) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	attemptID := state.MakeAttemptID(runID, jobID, 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: jobID, Command: []string{"true"}}}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, jobID, "attempts", attemptID)
	if err := state.WriteJSON(filepath.Join(jobDir, "command.json"), model.JobSpec{ID: jobID, AttemptID: attemptID, Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: model.RunStatus(exitCode), StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), Results: []model.JobResult{{ID: jobID, AttemptID: attemptID, ExitCode: exitCode}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, "submitted_at"), started); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, "finished_at"), finished); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(jobDir, "status"), []byte(formatInt(exitCode)+"\n")); err != nil {
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

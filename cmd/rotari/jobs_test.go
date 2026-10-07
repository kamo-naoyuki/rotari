package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestPrintJobsTableAlignsMultipleRows(t *testing.T) {
	rows := []joblist.Row{
		{State: "success", Project: "demo", AttemptID: "att_20260922-103800-afc43f9f-6aa4af5d9-1-0", StartedAt: time.Date(2026, 9, 22, 10, 38, 0, 0, time.UTC), Elapsed: 2 * time.Second},
		{State: "failed", Project: "demo", AttemptID: "att_20260922-103755-cf6e9512-6aa4af5d9-2-0", StartedAt: time.Date(2026, 9, 22, 10, 37, 55, 0, time.UTC), Elapsed: 2 * time.Second},
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
	if !strings.HasPrefix(lines[0], "STATE") || !strings.Contains(lines[1], rows[0].AttemptID) || !strings.Contains(lines[2], rows[1].AttemptID) {
		t.Fatalf("unexpected jobs table output: %q", output.String())
	}
}

func TestFormatJobsRowColorsStateWithoutChangingColumnWidth(t *testing.T) {
	oldCheck := terminalCheck
	terminalCheck = func(*os.File) bool { return true }
	defer func() { terminalCheck = oldCheck }()

	values := []string{"success", "demo"}
	colored := formatJobsRow(values, []int{7, 4}, []byte{'s', 'p'})
	if colored != ansiGreen+"success"+ansiReset+"  demo" {
		t.Fatalf("colored row = %q, want visible width-preserving row", colored)
	}

	values[0] = "failed"
	colored = formatJobsRow(values, []int{7, 4}, []byte{'s', 'p'})
	if colored != ansiRed+"failed"+ansiReset+"   demo" {
		t.Fatalf("failed row = %q, want visible width-preserving row", colored)
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

func TestCmdJobsListsAcrossBasedirsByDefault(t *testing.T) {
	stateHome := t.TempDir()
	defaultBase := filepath.Join(stateHome, "rotari")
	envBase := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv(envBaseDir, envBase)
	t.Setenv(envProjectName, "env-only")
	masterDir := t.TempDir()
	t.Setenv(envMasterDir, masterDir)
	writeTestJobsRun(t, defaultBase, "alpha", "20261007-100000-00000001", "alpha-job", time.Now().Add(-time.Minute), time.Now(), 0)
	writeTestJobsRun(t, defaultBase, "beta", "20261007-100001-00000002", "beta-job", time.Now().Add(-time.Minute), time.Now(), 0)
	writeTestJobsRun(t, defaultBase, "old", "20261006-100001-00000004", "old-job", time.Now().Add(-49*time.Hour), time.Now().Add(-48*time.Hour), 0)
	writeTestJobsRun(t, envBase, "env-only", "20261007-100002-00000003", "env-job", time.Now().Add(-time.Minute), time.Now(), 0)
	registry := basedirregistry.Open(masterDir)
	if err := registry.Register(envBase); err != nil {
		t.Fatal(err)
	}

	output, code := captureJobsStdout(t, nil)
	if code != 0 {
		t.Fatalf("cmdJobs exit = %d, output = %q", code, output)
	}
	for _, want := range []string{"alpha-job", "beta-job", "env-job", "BASEDIR"} {
		if !strings.Contains(output, want) {
			t.Errorf("jobs output missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "old-job") {
		t.Fatalf("default jobs listing included work older than 1d: %s", output)
	}
	output, code = captureJobsStdout(t, []string{"--basedir", envBase, "--project-name", "env-only"})
	if code != 0 || !strings.Contains(output, "env-job") || strings.Contains(output, "alpha-job") {
		t.Fatalf("explicit jobs scope = %d, %q", code, output)
	}
}

func TestCmdJobsRejectsRemovedAllBasedirsOption(t *testing.T) {
	if code := cmdJobs([]string{"--all-basedirs"}); code != 1 {
		t.Fatalf("cmdJobs accepted removed --all-basedirs option: exit code = %d", code)
	}
}

func TestCrossBasedirJobsPreservesConfiguredFormat(t *testing.T) {
	master := t.TempDir()
	t.Setenv(envMasterDir, master)
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig, cliConfigCommand = map[string]any{"format": "%a"}, "jobs"
	t.Cleanup(func() { cliConfig, cliConfigCommand = oldConfig, oldCommand })
	for _, name := range []string{"left", "right"} {
		base := filepath.Join(t.TempDir(), name)
		writeTestJobsRun(t, base, name, "20261008-100000-00000001", name+"-job", time.Now().Add(-time.Minute), time.Now(), 0)
		if err := basedirregistry.Open(master).Register(base); err != nil {
			t.Fatal(err)
		}
	}
	output, code := captureJobsStdout(t, nil)
	if code != 0 || !strings.HasPrefix(output, "ATTEMPT_ID\n") || strings.Contains(output, "BASEDIR") || !strings.Contains(output, "left-job") || !strings.Contains(output, "right-job") {
		t.Fatalf("configured cross-basedir format was lost: code=%d output=%q", code, output)
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
	paths, err := state.ResolveProjectPaths(baseDir, project)
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

func TestCmdJobsEmptyResultNamesItsScope(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir, _ := filepath.Abs(t.TempDir())
	tests := []struct {
		name string
		args []string
		want []string
		not  []string
	}{
		{"one basedir", []string{"--basedir", baseDir}, []string{"in state directory " + baseDir + " (finished within 1d)"}, []string{"--all-basedirs"}},
		{"all default directories without activity", nil, []string{"No running or recently finished jobs found"}, []string{"--all-basedirs"}},
		{"specified window", []string{"--since", "7d"}, []string{"finished within 7d"}, []string{"--all-basedirs"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if code := captureShowStdout(t, &output, func() int { return cmdJobs(test.args) }); code != 0 {
				t.Fatalf("jobs exit = %d:\n%s", code, output.String())
			}
			for _, want := range test.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("jobs output does not contain %q:\n%s", want, output.String())
				}
			}
			for _, unwanted := range test.not {
				if strings.Contains(output.String(), unwanted) {
					t.Errorf("jobs output contains %q:\n%s", unwanted, output.String())
				}
			}
		})
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	rotarimcp "github.com/kamo-naoyuki/rotari/internal/mcp"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestTerminalCommandUsesSharedJobInfo(t *testing.T) {
	baseDir := t.TempDir()
	project, runID, jobID := "demo", "run-1", "job-1"
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{
		ID: jobID, Name: "train", Command: []string{"python", "train.py"}, WorkingDirectory: "/work/private",
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{
		RunID: runID, Status: "failed", ExitCode: 1,
		Results: []model.JobResult{{ID: jobID, ExitCode: 1, Error: "exit status 1"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{CWD: "/work/private"}); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, jobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, state.StdoutFileName), []byte("terminal-shared-log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	want, err := rotarimcp.GetJobInfo(rotarimcp.GetJobInfoInput{
		BaseDir: baseDir, Project: project, RunID: runID, JobID: jobID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--basedir", baseDir,
		"--project", project,
		"--run-id", runID,
		"--job-id", jobID,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != want.Report {
		t.Fatalf("terminal report differs from shared job info\n got: %q\nwant: %q", got, want.Report)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestShowRunJobJSONResolvesResultsAndArrays(t *testing.T) {
	t.Setenv(envMasterDir, t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := makeRunID()
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "job-1", Name: "train", Command: []string{"true"}},
		{ID: "array", Name: "batch", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2}},
	}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: []model.JobResult{{ID: "job-1", ExitCode: 0}, {ID: "array-1", ExitCode: 3}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runDir, "job-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "job-1", "status"), []byte("7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unregisterRun(runID) })

	for _, test := range []struct {
		selector string
		ids      []string
		finished []bool
	}{
		{"job-1", []string{"job-1"}, []bool{true}},
		{"array", []string{"array-1", "array-2"}, []bool{true, false}},
		{"array-1", []string{"array-1"}, []bool{true}},
	} {
		t.Run(test.selector, func(t *testing.T) {
			assertRunJobJSON(t, baseDir, runID, test.selector, test.ids, test.finished)
		})
	}
	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int {
		return cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", runID, "--job-id", "missing", "--json"})
	}); code == 0 {
		t.Fatal("unknown job was accepted")
	}
}

func assertRunJobJSON(t *testing.T, baseDir, runID, selector string, ids []string, finished []bool) {
	t.Helper()
	var output bytes.Buffer
	code := captureShowStdout(t, &output, func() int {
		return cmdShow([]string{"--basedir", baseDir, "--project-name", "demo", "--run-id", runID, "--job-id", selector, "--json"})
	})
	if code != 0 {
		t.Fatalf("show exit = %d, output = %s", code, output.String())
	}
	var result struct {
		RunID string `json:"run_id"`
		JobID string `json:"job_id"`
		Jobs  []struct {
			Job      model.JobSpec    `json:"job"`
			Finished bool             `json:"finished"`
			Result   *model.JobResult `json:"result"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.RunID != runID || result.JobID != selector || len(result.Jobs) != len(ids) {
		t.Fatalf("unexpected job view: %+v", result)
	}
	for index, job := range result.Jobs {
		if job.Job.ID != ids[index] || job.Finished != finished[index] || (job.Result != nil) != finished[index] {
			t.Errorf("job %d = %+v", index, job)
		}
		if selector == "job-1" && job.Result != nil && job.Result.ExitCode != 7 {
			t.Errorf("status file did not override summary: %+v", job.Result)
		}
	}
}

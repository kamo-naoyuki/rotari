package lifecycle

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestWorkflowExportUsesListedRunOrderForTimestampTies(t *testing.T) {
	covers(t, "RUN-11")
	e := support.NewEnv(t)
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "snapshots", "--job-name", "job", "--", "echo", "first"))
	e.MustRotari("run", "-p", "snapshots", "--quiet")
	firstRun := readSummary(t, e, "snapshots")
	e.MustRotari("copy", "-p", "snapshots", "--run-id", firstRun.RunID, "--overwrite", "--quiet")
	e.MustRotari("change", "-p", "snapshots", "--job-id", jobID, "--quiet", "--", "echo", "second")
	e.MustRotari("run", "-p", "snapshots", "--quiet")
	secondRun := readSummary(t, e, "snapshots")

	const finishedAt = "2026-10-04T11:38:27Z"
	for _, run := range []conformanceSummary{firstRun, secondRun} {
		_, _ = summaryResult(t, run, jobID)
		attemptDirs, err := filepath.Glob(filepath.Join(e.Base, "projects", "snapshots", "runs", run.RunID, jobID, "attempts", "*"))
		if err != nil || len(attemptDirs) != 1 {
			t.Fatalf("attempt dirs for run %s = %v, %v; want one", run.RunID, attemptDirs, err)
		}
		if err := os.WriteFile(filepath.Join(attemptDirs[0], "finished_at"), []byte(finishedAt+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	listed := []conformanceSummary{firstRun, secondRun}
	commands := map[string][]string{
		firstRun.RunID:  {"echo", "first"},
		secondRun.RunID: {"echo", "second"},
	}
	if listed[0].RunID < listed[1].RunID {
		listed[0], listed[1] = listed[1], listed[0]
	}

	manifestPath := filepath.Join(e.Root, "merged.json")
	e.MustRotari("export", "--basedir", e.Base, "--project-name", "snapshots", "--run-id", listed[0].RunID, "--run-id", listed[1].RunID, "--format", "json", "--output", manifestPath)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Jobs []struct {
			Command   []string `json:"command"`
			AttemptID string   `json:"attempt_id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	wantAttempt, _ := summaryResult(t, listed[1], jobID)
	if len(manifest.Jobs) != 1 || manifest.Jobs[0].AttemptID != wantAttempt || len(manifest.Jobs[0].Command) != 2 || manifest.Jobs[0].Command[1] != commands[listed[1].RunID][1] {
		t.Fatalf("exported jobs = %#v, want snapshot from last listed run %s", manifest.Jobs, listed[1].RunID)
	}
}

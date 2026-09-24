package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestAttemptIDRoundTripAndDirectories(t *testing.T) {
	runID := "20260924-120000-abcdef01"
	attemptID := MakeAttemptID(runID, "job-1", 2)
	payload, err := DecodeAttemptID(attemptID)
	if err != nil || payload.RunID != runID || payload.JobID != "job-1" || payload.Number != 2 {
		t.Fatalf("DecodeAttemptID() = %#v, %v", payload, err)
	}
	runDir := filepath.Join(t.TempDir(), runID)
	job := model.JobSpec{ID: "job-1", AttemptID: attemptID}
	want := filepath.Join(runDir, "job-1", "attempts", attemptID)
	if got, err := AttemptJobDir(runDir, job); err != nil || got != want {
		t.Fatalf("AttemptJobDir() = %q, %v; want %q", got, err, want)
	}
	if _, err := SpecificAttemptJobDir(runDir, "job-1", "../bad"); err == nil {
		t.Fatal("SpecificAttemptJobDir() accepted unsafe attempt ID")
	}
}

func TestLatestAttemptIDSelectsHighestValidAttempt(t *testing.T) {
	runID := "20260924-120000-abcdef01"
	runDir := filepath.Join(t.TempDir(), runID)
	attemptsDir := filepath.Join(runDir, "job-1", "attempts")
	if err := os.MkdirAll(attemptsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{MakeAttemptID(runID, "job-1", 1), MakeAttemptID(runID, "job-1", 3), "att-other-job-9", "invalid"} {
		if err := os.Mkdir(filepath.Join(attemptsDir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LatestAttemptID(runDir, "job-1")
	if err != nil || got != MakeAttemptID(runID, "job-1", 3) {
		t.Fatalf("LatestAttemptID() = %q, %v", got, err)
	}
}

func TestContextAndFinalizeRun(t *testing.T) {
	store := NewStore(0o700, 0o600)
	runDir := filepath.Join(t.TempDir(), "run-1")
	input := model.RunContext{CWD: "/work", Hostname: "host-1"}
	if err := SaveContext(store, runDir, input); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadContext(store, runDir)
	if err != nil || loaded.CWD != input.CWD || loaded.Hostname != input.Hostname {
		t.Fatalf("LoadContext() = %#v, %v", loaded, err)
	}
	queue, meta, err := FinalizeRun(model.Queue{Commands: []model.QueuedCommand{{ID: "job"}}}, model.Meta{}, "run-1", 3, time.Date(2026, 9, 24, 1, 2, 3, 0, time.FixedZone("JST", 9*60*60)))
	if err != nil || len(queue.Commands) != 0 || meta.Phase != "finished" || meta.LastRunID != "run-1" || meta.LastRunExitCode != 3 || meta.UpdatedAt != "2026-09-23T16:02:03Z" {
		t.Fatalf("FinalizeRun() = %#v, %#v, %v", queue, meta, err)
	}
	if _, _, err := FinalizeRun(model.Queue{}, model.Meta{}, "", 0, time.Time{}); err == nil {
		t.Fatal("FinalizeRun() accepted empty run ID")
	}
}

func TestResolveProjectNamePrecedence(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv(projectNameEnv, "from-env")
	if got, err := ResolveProjectName(baseDir, "from-cli"); err != nil || got != "from-cli" {
		t.Fatalf("CLI project name = %q, %v", got, err)
	}
	if got, err := ResolveProjectName(baseDir, ""); err != nil || got != "from-env" {
		t.Fatalf("environment project name = %q, %v", got, err)
	}
	t.Setenv(projectNameEnv, "")
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "only"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveProjectName(baseDir, ""); err != nil || got != "only" {
		t.Fatalf("single project name = %q, %v", got, err)
	}
}

func TestResolveProjectPathsUsesProjectLayout(t *testing.T) {
	paths, err := ResolveProjectPaths("/state", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if paths.ProjectDir != "/state/projects/demo" || paths.QueueFile != "/state/projects/demo/queue.json" || paths.RunsDir != "/state/projects/demo/runs" {
		t.Fatalf("paths = %#v", paths)
	}
	if _, err := ResolveProjectPaths("/state", "../demo"); err == nil {
		t.Fatal("ResolveProjectPaths() accepted unsafe project name")
	}
}

package main

import (
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func defaultMeta() model.Meta {
	return model.Meta(state.DefaultMeta())
}

func loadMeta(path string) (model.Meta, error) {
	meta, err := state.LoadMeta(path)
	return model.Meta(meta), err
}

func makeAttemptID(runID, jobID string, number int) string {
	return state.MakeAttemptID(runID, jobID, number)
}

func loadRunSummary(path string) (model.RunSummary, error) {
	summary, err := state.LoadRunSummary(path)
	return model.RunSummary(summary), err
}

func loadQueue(path string) (model.Queue, error) {
	queue, err := state.LoadQueue(path)
	return model.Queue(queue), err
}

func writeJSON(path string, value any) error {
	return state.WriteJSON(path, value)
}

func latestAttemptJobDir(runDir, jobID string) (string, error) {
	return state.LatestAttemptJobDir(runDir, jobID)
}

func specificAttemptJobDir(runDir, jobID, attemptID string) (string, error) {
	return state.SpecificAttemptJobDir(runDir, jobID, attemptID)
}

func decodeAttemptID(attemptID string) (state.AttemptIDPayload, error) {
	return state.DecodeAttemptID(attemptID)
}

func latestAttemptID(runDir, jobID string) (string, error) {
	return state.LatestAttemptID(runDir, jobID)
}

func validatedStateFile(basePath, fileName string) (string, error) {
	return state.ValidatedStateFile(basePath, fileName)
}

func validatedJobDir(runDir, jobID string) (string, error) {
	return state.SafeJoin(runDir, jobID)
}

func validatedRunDir(paths state.ProjectPaths, runID string) (string, error) {
	return state.SafeJoin(paths.RunsDir, runID)
}

func writeTestRunStateFiles(t *testing.T, paths state.ProjectPaths, runID string) {
	t.Helper()
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "context.json"), model.RunContext{}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{}); err != nil {
		t.Fatal(err)
	}
}

func TestStateLoadMetaDefaultsToCollectingAndTimestamp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")

	meta, err := state.LoadMeta(path)
	if err != nil {
		t.Fatalf("state.LoadMeta returned error for missing file: %v", err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("state.LoadMeta phase = %q, want %q", meta.Phase, "collecting")
	}
	if meta.UpdatedAt == "" {
		t.Fatal("state.LoadMeta set an empty UpdatedAt for a missing meta file")
	}
}

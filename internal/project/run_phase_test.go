package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// TestRunPhaseOf gives each run of one project the phase that a waiting
// reader must see: the run holding the lock is running even before it has
// written anything, and an inactive run is finished only with a valid
// summary.
func TestRunPhaseOf(t *testing.T) {
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		pid   int
		runID string
		want  RunPhase
	}{
		{"locked run before its snapshot", os.Getpid(), "current", RunPhaseRunning},
		{"older run with a summary", os.Getpid(), "settled", RunPhaseFinished},
		{"older run with an invalid summary", os.Getpid(), "broken", RunPhaseEnded},
		{"older run without a summary", os.Getpid(), "abandoned", RunPhaseEnded},
		{"current run of a dead supervisor", -1, "current", RunPhaseInterrupted},
		{"older run beside an interrupted one", -1, "settled", RunPhaseFinished},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			for path, value := range map[string]any{
				paths.LockFile: model.LockInfo{PID: test.pid, RunID: "current", Host: host},
				paths.MetaFile: model.Meta{Phase: "running", LastRunID: "current"},
				filepath.Join(paths.RunsDir, "settled", "summary.json"): model.RunSummary{RunID: "settled", Status: "failed", ExitCode: 1},
			} {
				if err := state.WriteJSON(path, value); err != nil {
					t.Fatal(err)
				}
			}
			for _, dir := range []string{"current", "broken", "abandoned"} {
				if err := os.MkdirAll(filepath.Join(paths.RunsDir, dir), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(paths.RunsDir, "broken", "summary.json"), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := RunPhaseOf(paths, test.runID); err != nil || got != test.want {
				t.Fatalf("RunPhaseOf(%s) = %q, %v; want %q", test.runID, got, err, test.want)
			}
			if _, err := os.Stat(paths.LockFile); err != nil {
				t.Fatalf("RunPhaseOf removed the lock: %v", err)
			}
		})
	}
}

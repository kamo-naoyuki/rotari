package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeOverviewProject(t *testing.T, baseDir, name string, queue model.Queue, meta model.Meta, runIDs ...string) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}
	for _, runID := range runIDs {
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestOverviewsDescribesEachProject(t *testing.T) {
	baseDir := t.TempDir()
	if overviews, err := Overviews(baseDir, false); err != nil || overviews != nil {
		t.Fatalf("Overviews without projects = %v, %v", overviews, err)
	}
	alpha := writeOverviewProject(t, baseDir, "alpha",
		model.Queue{Commands: []model.QueuedCommand{{ID: "a", Command: []string{"true"}}, {ID: "b", Command: []string{"true"}}}},
		model.Meta{Phase: "finished", LastRunID: "run-2"}, "run-1", "run-2")
	if err := state.WriteJSON(filepath.Join(alpha.RunsDir, "run-2", "summary.json"), model.RunSummary{
		RunID: "run-2", Status: "failed",
		Results: []model.JobResult{{ID: "a", ExitCode: 0}, {ID: "b", ExitCode: 2}, {ID: "c", ExitCode: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alpha.RunsDir, "stray-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	writeOverviewProject(t, baseDir, "beta", model.Queue{}, model.Meta{Phase: "running", LastRunID: "run-x"}, "run-x")
	writeOverviewProject(t, baseDir, "gamma", model.Queue{}, model.Meta{Phase: "collecting"})
	if err := os.WriteFile(filepath.Join(baseDir, "projects", "not-a-project"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	overviews, err := Overviews(baseDir, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []Overview{
		{BaseDir: baseDir, Name: "alpha", Queued: 2, Runs: 2, State: Idle, LastRun: LastRun{ID: "run-2", Status: "failed", Jobs: 3, Failed: 2}},
		{BaseDir: baseDir, Name: "beta", Runs: 1, State: Interrupted, LastRun: LastRun{ID: "run-x"}},
		{BaseDir: baseDir, Name: "gamma", State: Idle},
	}
	if !reflect.DeepEqual(overviews, want) {
		t.Fatalf("Overviews =\n%#v\nwant\n%#v", overviews, want)
	}
}

func TestOverviewsReportsUnreadableProject(t *testing.T) {
	baseDir := t.TempDir()
	writeOverviewProject(t, baseDir, "broken", model.Queue{}, model.Meta{Phase: "exploded"})
	if _, err := Overviews(baseDir, false); err == nil || !strings.Contains(err.Error(), `failed to check project "broken" state`) {
		t.Fatalf("Overviews error = %v", err)
	}

	baseDir = t.TempDir()
	paths := writeOverviewProject(t, baseDir, "badqueue", model.Queue{}, model.Meta{Phase: "collecting"})
	if err := os.WriteFile(paths.QueueFile, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Overviews(baseDir, false); err == nil || !strings.Contains(err.Error(), `failed to load queue for project "badqueue"`) {
		t.Fatalf("Overviews error = %v", err)
	}
}

func TestCountRunsWithoutRunsDirectory(t *testing.T) {
	if count := CountRuns(state.ProjectPaths{RunsDir: filepath.Join(t.TempDir(), "missing")}); count != 0 {
		t.Fatalf("CountRuns = %d", count)
	}
}

func TestLastRunIgnoresUnsafeRunID(t *testing.T) {
	paths := state.ProjectPaths{RunsDir: t.TempDir()}
	if last := lastRun(paths, "../outside"); last != (LastRun{ID: "../outside"}) {
		t.Fatalf("lastRun = %#v", last)
	}
}

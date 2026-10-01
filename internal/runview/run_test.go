package runview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestLoadRunResolvesSummaryStatuses(t *testing.T) {
	paths := state.ProjectPaths{RunsDir: t.TempDir()}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o700, 0o600)
	if err := store.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "failed", Name: "failed", Command: []string{"false"}},
		{ID: "success", Name: "success", Command: []string{"true"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{
		RunID:   "run-1",
		RunName: "experiment",
		Results: []model.JobResult{
			{ID: "failed", ExitCode: 1},
			{ID: "success", ExitCode: 0},
		},
	}); err != nil {
		t.Fatal(err)
	}

	run, err := LoadRun(paths, "run-1", store)
	if err != nil {
		t.Fatal(err)
	}
	if run.Name != "experiment" || len(run.Jobs) != 2 {
		t.Fatalf("run = %+v", run)
	}
	if run.Jobs[0].Status != runlineage.StatusFailed || run.Jobs[1].Status != runlineage.StatusSuccess {
		t.Fatalf("job statuses = %+v", run.Jobs)
	}
}

func TestLoadRunDoesNotCarryAnewlyBlockedJob(t *testing.T) {
	paths := state.ProjectPaths{RunsDir: t.TempDir()}
	runDir := filepath.Join(paths.RunsDir, "run-2")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o700, 0o600)
	origin := &model.JobOrigin{RunID: "run-1", JobID: "job-1", Status: "success"}
	if err := store.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "job-1", Name: "job", Command: []string{"false"}, Origin: origin},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{
		{ID: "job-1", ExitCode: 1, Error: "blocked by failed dependency"},
	}}); err != nil {
		t.Fatal(err)
	}

	run, err := LoadRun(paths, "run-2", store)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Jobs) != 1 || run.Jobs[0].Status != runlineage.StatusBlocked || run.Jobs[0].Carried {
		t.Fatalf("job = %+v, want blocked and not carried", run.Jobs)
	}
}

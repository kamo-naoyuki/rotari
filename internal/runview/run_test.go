package runview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/rundiff"
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
	if run.Jobs[0].Status != rundiff.StatusFailed || run.Jobs[1].Status != rundiff.StatusSuccess {
		t.Fatalf("job statuses = %+v", run.Jobs)
	}
}

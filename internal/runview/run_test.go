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

func TestLoadRunMarksFinalResultsOfAnActiveRun(t *testing.T) {
	paths := state.ProjectPaths{RunsDir: t.TempDir()}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	store := state.NewStore(0o700, 0o600)
	if err := store.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "final", Name: "final", Command: []string{"false"}},
		{ID: "retrying", Name: "retrying", Command: []string{"false"}},
		{ID: "running", Name: "running", Command: []string{"sleep", "9"}},
	}}); err != nil {
		t.Fatal(err)
	}
	// No summary yet: two jobs have failed, and only "final" has no retry left.
	for _, jobID := range []string{"final", "retrying"} {
		if err := os.MkdirAll(filepath.Join(runDir, jobID), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, jobID, "status"), []byte("3\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oom := []model.RuleDiagnosis{{Name: "CUDA/GPU memory exhausted", Evidence: "CUDA out of memory"}}
	if err := store.WriteJSON(filepath.Join(runDir, "final", state.FinalResultFileName), model.JobResult{ID: "final", ExitCode: 3, DiagnosisStatus: model.DiagnosisMatched, Diagnoses: oom}); err != nil {
		t.Fatal(err)
	}

	run, err := LoadRun(paths, "run-1", store)
	if err != nil {
		t.Fatal(err)
	}
	jobs := map[string]runlineage.Job{}
	for _, job := range run.Jobs {
		jobs[job.Spec.ID] = job
	}
	if job := jobs["final"]; !job.Final || job.Status != runlineage.StatusFailed || job.Result.ExitCode != 3 || len(job.Result.Diagnoses) != 1 {
		t.Errorf("final job = %+v, want a final failure with its diagnosis", job)
	}
	if job := jobs["retrying"]; job.Final || job.Status != runlineage.StatusFailed {
		t.Errorf("retrying job = %+v, want a failure that is not final", job)
	}
	if job := jobs["running"]; job.Final || job.Status != runlineage.StatusUnfinished {
		t.Errorf("running job = %+v, want unfinished and not final", job)
	}
}

func TestPreviousRunFollowsStartOrder(t *testing.T) {
	paths := state.ProjectPaths{ProjectName: "demo", RunsDir: t.TempDir()}
	// Both runs start in the same second and the later one's ID sorts first,
	// so only the load samples order them.
	first, second := "20260101-000000-bbbbbbbb", "20260101-000000-aaaaaaaa"
	for runID, at := range map[string]string{first: "2026-01-01T00:00:00.1Z", second: "2026-01-01T00:00:00.9Z"} {
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(paths.RunsDir, runID, state.LoadSamplesFileName), []byte(`{"at":"`+at+`","one":1,"five":1,"fifteen":1}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := PreviousRun(paths, second); err != nil || got != first {
		t.Fatalf("PreviousRun(second) = %q, %v; want %q", got, err, first)
	}
	if _, err := PreviousRun(paths, first); err == nil {
		t.Fatal("PreviousRun(first) found an earlier run")
	}
}

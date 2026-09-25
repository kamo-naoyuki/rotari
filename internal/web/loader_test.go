package web

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestLoadQueueStateBuildsRunsFromCallbacks(t *testing.T) {
	state, err := LoadQueueState(QueueLoader{
		ProjectName: "demo",
		Queue: func() (model.Queue, error) {
			return model.Queue{Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"echo"}}}}, nil
		},
		Lock: func() (model.LockInfo, error) {
			return model.LockInfo{RunID: "run-2", PID: 42, StartedAt: "lock-start"}, nil
		},
		Runs: func() ([]string, error) { return []string{"run-1", "run-2"}, nil },
		Summary: func(runID string) (model.RunSummary, error) {
			if runID == "run-1" {
				return model.RunSummary{RunID: runID, StartedAt: "start-1", FinishedAt: "finish-1", ExitCode: 0}, nil
			}
			return model.RunSummary{}, assertNotFound{}
		},
		Jobs: func(runID string, summary model.RunSummary) ([]Job, error) {
			return []Job{{ID: runID + "-job", Result: &model.JobResult{ID: runID + "-job", ExitCode: 0}}}, nil
		},
		Context: func(string) (model.RunContext, error) { return model.RunContext{CWD: "/work"}, nil },
		Samples: func(string) []model.LoadSample { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.QueueName != "demo" || state.RunningRunID != "run-2" || state.RunnerPID != 42 {
		t.Fatalf("state = %#v", state)
	}
	if len(state.Runs) != 2 || state.Runs[0].RunID != "run-2" || state.Runs[0].Status != "running" || !state.Runs[0].Running {
		t.Fatalf("runs = %#v", state.Runs)
	}
	if state.Runs[1].RunID != "run-1" || len(state.Runs[1].Jobs) != 1 {
		t.Fatalf("sorted runs = %#v", state.Runs)
	}
}

func TestLoadJobsProjectsSummaryAndOrigin(t *testing.T) {
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run-1")
	writeTestFile(t, filepath.Join(runsDir, "run-0", "job-1", "submitted_at"), "submitted")
	writeTestFile(t, filepath.Join(runsDir, "run-0", "job-1", "finished_at"), "finished")
	origin := &model.JobOrigin{RunID: "run-0", JobID: "job-1", Status: "success"}
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Name: "demo", Stage: "build", Command: []string{"echo", "ok"}, Origin: origin}}},
		model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" || jobs[0].Result == nil || jobs[0].Result.ExitCode != 0 {
		t.Fatalf("jobs = %#v, want one finished job", jobs)
	}
	if jobs[0].Stage != "build" || jobs[0].Origin != origin || jobs[0].SubmittedAt != "submitted" || jobs[0].FinishedAt != "finished" {
		t.Fatalf("job projection = %#v, want origin and timestamps", jobs[0])
	}
}

func TestLoadJobsPrefersAttemptStatusOverSummary(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "20260925-000000-00000000")
	writeTestFile(t, filepath.Join(runDir, "job-1", "status"), "3")
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"false"}}}},
		model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 0, Hosts: []string{"node1"}}}},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Result == nil || jobs[0].Result.ExitCode != 3 || len(jobs[0].Result.Hosts) != 1 {
		t.Fatalf("jobs = %#v, want attempt exit code with summary metadata", jobs)
	}
}

func TestLoadJobsSelectedAttemptUsesItsOwnOutcome(t *testing.T) {
	runID := "20260925-000000-00000000"
	runDir := filepath.Join(t.TempDir(), runID)
	first := state.MakeAttemptID(runID, "job-1", 1)
	second := state.MakeAttemptID(runID, "job-1", 2)
	writeTestFile(t, filepath.Join(runDir, "job-1", "attempts", first, "status.json"), `{"phase":"finished","exit_code":2,"finished_at":"first-finish"}`)
	writeTestFile(t, filepath.Join(runDir, "job-1", "attempts", second, "status"), "0")
	jobs, err := LoadJobs(
		state.NewStore(0o700, 0o600),
		runDir,
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}},
		model.RunSummary{Results: []model.JobResult{{ID: "job-1", AttemptID: second, ExitCode: 0}}},
		first,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].AttemptID != first || jobs[0].Result == nil || jobs[0].Result.ExitCode != 2 || jobs[0].Result.AttemptID != first {
		t.Fatalf("jobs = %#v, want selected attempt outcome", jobs)
	}
	if jobs[0].FinishedAt != "first-finish" {
		t.Fatalf("finished at = %q, want selected attempt wrapper time", jobs[0].FinishedAt)
	}
	if len(jobs[0].Attempts) != 2 || jobs[0].Attempts[0].ID != second || jobs[0].Attempts[1].Result == nil || jobs[0].Attempts[1].Result.ExitCode != 2 {
		t.Fatalf("attempts = %#v, want newest first with results", jobs[0].Attempts)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadQueueStateFallsBackToRunningSummaryWhenMissing(t *testing.T) {
	state, err := LoadQueueState(QueueLoader{
		ProjectName: "demo",
		Queue:       func() (model.Queue, error) { return model.Queue{}, nil },
		Lock:        func() (model.LockInfo, error) { return model.LockInfo{RunID: "run-1", StartedAt: "started"}, nil },
		Runs:        func() ([]string, error) { return []string{"run-1"}, nil },
		Summary:     func(string) (model.RunSummary, error) { return model.RunSummary{}, assertNotFound{} },
		Jobs: func(runID string, summary model.RunSummary) ([]Job, error) {
			if summary.Status != "running" || summary.RunID != runID || summary.StartedAt != "started" {
				t.Fatalf("fallback summary = %#v", summary)
			}
			return nil, nil
		},
		Context: func(string) (model.RunContext, error) { return model.RunContext{}, nil },
		Samples: func(string) []model.LoadSample { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Runs) != 1 || state.Runs[0].Status != "running" || !state.Runs[0].Running {
		t.Fatalf("state = %#v, want one running fallback run", state)
	}
}

type assertNotFound struct{}

func (assertNotFound) Error() string { return "not found" }

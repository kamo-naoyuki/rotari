package web

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
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
	origin := &model.JobOrigin{RunID: "run-0", JobID: "job-1", Status: "success"}
	jobs, err := LoadJobs(
		model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Name: "demo", Command: []string{"echo", "ok"}, Origin: origin}}},
		model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}},
		nil,
		JobLoader{
			Origins:             map[string]*model.JobOrigin{"job-1": origin},
			LatestAttemptDir:    func(string) (string, error) { return "/run/job-1", nil },
			SpecificAttemptDir:  func(string, string) (string, error) { return "", nil },
			ListAttemptIDs:      func(string) []string { return nil },
			ReadTimestamp:       func(string, string) string { return "" },
			ReadJobTimestamp:    func(string, string) string { return "" },
			LoadSchedulerState:  func(string) string { return "running" },
			LoadSchedulerResult: func(string, model.JobSpec) (model.JobResult, bool) { return model.JobResult{}, false },
			SchedulerFinishedAt: func(string) string { return "" },
			LoadLocalResult:     func(string, model.JobSpec) (model.JobResult, bool) { return model.JobResult{}, false },
			LoadTerminalState:   func(string) (int, bool) { return 0, false },
			ResolveTimestamps:   func(string, *model.JobOrigin) (string, string) { return "submitted", "finished" },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" || jobs[0].Result == nil || jobs[0].Result.ExitCode != 0 {
		t.Fatalf("jobs = %#v, want one finished job", jobs)
	}
	if jobs[0].Origin != origin || jobs[0].SubmittedAt != "submitted" || jobs[0].FinishedAt != "finished" {
		t.Fatalf("job projection = %#v, want origin and timestamps", jobs[0])
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

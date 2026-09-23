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

type assertNotFound struct{}

func (assertNotFound) Error() string { return "not found" }

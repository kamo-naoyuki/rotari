package run

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestPlanSelectionCarriesFinishedResults(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "ok", Command: []string{"true"}},
		{ID: "failed", Command: []string{"false"}},
	}}
	plan, err := PlanSelection(queue, "failed", nil, true, Reference{
		RunID: "run-1",
		Results: map[string]model.JobResult{
			"ok":     {ID: "ok", ExitCode: 0, AttemptID: "att-ok"},
			"failed": {ID: "failed", ExitCode: 1, AttemptID: "att-failed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["failed"] || plan.Execute["ok"] {
		t.Fatalf("execute = %#v, want only failed", plan.Execute)
	}
	if got := plan.CarriedResults["ok"].AttemptID; got != "att-ok" {
		t.Fatalf("carried attempt = %q, want att-ok", got)
	}
	if got := plan.CarriedOrigins["ok"].RunID; got != "run-1" {
		t.Fatalf("carried origin run = %q, want run-1", got)
	}
}

func TestPlanSelectionSelectsArrayTasksIndividually(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"run"}, Array: &model.ArraySpec{First: 1, Last: 2},
	}}}
	plan, err := PlanSelection(queue, "failed", nil, true, Reference{RunID: "run-1", Results: map[string]model.JobResult{
		"array-1": {ID: "array-1", ExitCode: 0},
		"array-2": {ID: "array-2", ExitCode: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || !plan.Execute["array-2"] {
		t.Fatalf("execute = %#v, want only array-2", plan.Execute)
	}
	if plan.CarriedResults["array-1"].ID != "array-1" {
		t.Fatalf("carried array result = %#v", plan.CarriedResults["array-1"])
	}
}

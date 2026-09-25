package run

import (
	"errors"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type fakeOriginResults struct {
	lastRunID string
	runs      map[string]map[string]model.JobResult
	attempts  map[string]model.JobResult
	loads     int
}

func (source *fakeOriginResults) LastRunID() (string, error) {
	if source.lastRunID == "" {
		return "", errors.New("no previous run")
	}
	return source.lastRunID, nil
}

func (source *fakeOriginResults) RunResults(runID string) (map[string]model.JobResult, error) {
	source.loads++
	results, ok := source.runs[runID]
	if !ok {
		return nil, errors.New("missing summary")
	}
	return results, nil
}

func (source *fakeOriginResults) AttemptResult(origin model.JobOrigin) (model.JobResult, bool, error) {
	result, ok := source.attempts[origin.AttemptID]
	return result, ok, nil
}

func (source *fakeOriginResults) Origin(runID, jobID string, result model.JobResult) *model.JobOrigin {
	return &model.JobOrigin{RunID: runID, JobID: jobID, AttemptID: result.AttemptID, Status: "from-" + runID}
}

func TestPlanRerunWithoutSelectionExecutesEveryCommand(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "a"}, {ID: "b", Array: &model.ArraySpec{First: 1, Last: 2}}}}
	plan, err := PlanRerun(queue, "", nil, "", true, &fakeOriginResults{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 2 || !plan.Execute["a"] || !plan.Execute["b"] || plan.CarriedResults != nil {
		t.Fatalf("plan = %#v, want every command", plan)
	}
}

func TestPlanRerunCarriesFinishedResultsFromReferenceRun(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "ok", Command: []string{"true"}},
		{ID: "failed", Command: []string{"false"}},
		{ID: "pending", Command: []string{"true"}},
	}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {
		"ok":     {ID: "ok", ExitCode: 0, AttemptID: "att-ok"},
		"failed": {ID: "failed", ExitCode: 1, AttemptID: "att-failed"},
	}}}
	plan, err := PlanRerun(queue, "failed", nil, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["failed"] || plan.Execute["ok"] || plan.Execute["pending"] {
		t.Fatalf("execute = %#v, want only failed", plan.Execute)
	}
	if got := plan.CarriedResults["ok"].AttemptID; got != "att-ok" {
		t.Fatalf("carried attempt = %q, want att-ok", got)
	}
	if origin := plan.CarriedOrigins["ok"]; origin == nil || origin.RunID != "run-1" || origin.Status != "from-run-1" {
		t.Fatalf("carried origin = %#v, want fallback origin from run-1", origin)
	}
	if _, carried := plan.CarriedResults["pending"]; carried {
		t.Fatal("unfinished job was carried")
	}
	if source.loads != 1 {
		t.Fatalf("run summary loads = %d, want one cached load", source.loads)
	}
}

func TestPlanRerunFallsBackToLastRun(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "done"}}}
	source := &fakeOriginResults{lastRunID: "latest", runs: map[string]map[string]model.JobResult{"latest": {"done": {ID: "done", ExitCode: 0}}}}
	plan, err := PlanRerun(queue, "failed", nil, "", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CarriedOrigins["done"].RunID != "latest" {
		t.Fatalf("origin = %#v, want latest run", plan.CarriedOrigins["done"])
	}
	if _, err := PlanRerun(queue, "failed", nil, "", true, &fakeOriginResults{}); err == nil || err.Error() != "no previous run" {
		t.Fatalf("error = %v, want LastRunID error", err)
	}
}

func TestPlanRerunUsesOriginAndAttempt(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "copied", Origin: &model.JobOrigin{RunID: "source", JobID: "orig"}},
		{ID: "pinned", Origin: &model.JobOrigin{RunID: "source", JobID: "other", AttemptID: "att-old"}},
	}}
	source := &fakeOriginResults{
		runs:     map[string]map[string]model.JobResult{"source": {"orig": {ID: "orig", ExitCode: 0}, "other": {ID: "other", ExitCode: 0, AttemptID: "att-new"}}},
		attempts: map[string]model.JobResult{"att-old": {ID: "other", ExitCode: 1, AttemptID: "att-old"}},
	}
	plan, err := PlanRerun(queue, "failed", nil, "", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["copied"] || !plan.Execute["pinned"] {
		t.Fatalf("execute = %#v, want the pinned failed attempt", plan.Execute)
	}
	if result := plan.CarriedResults["copied"]; result.ID != "copied" {
		t.Fatalf("carried result = %#v, want destination ID", result)
	}
	if _, ok := plan.CarriedOrigins["copied"]; ok {
		t.Fatal("a command with its own origin got a fallback origin")
	}
}

func TestPlanRerunSelectsArrayTasksIndividually(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Command: []string{"run"}, Array: &model.ArraySpec{First: 1, Last: 2},
	}}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {
		"array-1": {ID: "array-1", ExitCode: 0},
		"array-2": {ID: "array-2", ExitCode: 1},
	}}}
	plan, err := PlanRerun(queue, "failed", nil, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || !plan.Execute["array-2"] {
		t.Fatalf("execute = %#v, want only array-2", plan.Execute)
	}
	if plan.CarriedResults["array-1"].ID != "array-1" || plan.CarriedOrigins["array-1"] == nil {
		t.Fatalf("carried = %#v / %#v", plan.CarriedResults, plan.CarriedOrigins)
	}

	plan, err = PlanRerun(queue, "failed", nil, "run-1", false, source)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["array"] || plan.Execute["array-2"] {
		t.Fatalf("whole-array execute = %#v, want the array command", plan.Execute)
	}
}

func TestPlanRerunReportsUnknownJobIDs(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "known"}}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {}}}
	_, err := PlanRerun(queue, "job-id", []string{"missing", "also-missing"}, "run-1", false, source)
	if err == nil || err.Error() != "job IDs not found in queue: also-missing, missing" {
		t.Fatalf("PlanRerun() error = %v", err)
	}
}

func TestPlanRerunImportedWorkflowDispositions(t *testing.T) {
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{
		{ID: "reuse", Name: "reuse", Command: []string{"true"}, Origin: &model.JobOrigin{RunID: "source", JobID: "reuse"}},
		{ID: "accept", Name: "accept", Command: []string{"true"}, Accepted: true, Origin: &model.JobOrigin{RunID: "source", JobID: "accept"}},
		{ID: "forced", Name: "forced", Command: []string{"true"}, Force: true, Origin: &model.JobOrigin{RunID: "source", JobID: "forced"}},
		{ID: "downstream", Name: "downstream", Command: []string{"true"}, DependsOn: []string{"forced"}, Origin: &model.JobOrigin{RunID: "source", JobID: "downstream"}},
		{ID: "fresh", Name: "fresh", Command: []string{"true"}},
	}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"source": {
		"reuse":      {ID: "reuse", ExitCode: 0},
		"accept":     {ID: "accept", ExitCode: 1, Error: "boom"},
		"forced":     {ID: "forced", ExitCode: 0},
		"downstream": {ID: "downstream", ExitCode: 0},
	}}}
	plan, err := PlanRerun(queue, "", nil, "", true, source)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"reuse": false, "accept": false, "forced": true, "downstream": true, "fresh": true} {
		if plan.Execute[id] != want {
			t.Fatalf("execute[%s] = %v, want %v (plan %#v)", id, plan.Execute[id], want, plan.Execute)
		}
	}
	if accepted := plan.CarriedResults["accept"]; !accepted.Accepted || accepted.ExitCode != 0 || accepted.Error != "" {
		t.Fatalf("accepted result = %#v", accepted)
	}
	if _, carried := plan.CarriedResults["downstream"]; carried {
		t.Fatal("downstream job kept its carried result")
	}
}

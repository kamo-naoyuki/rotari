package run

import (
	"errors"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type fakeOriginResults struct {
	runs     map[string]map[string]model.JobResult
	attempts map[string]model.JobResult
	loads    int
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
	plan, err := PlanRerun(queue, "", nil, model.CommandSelector{}, jobfilter.Filter{}, "", true, &fakeOriginResults{})
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
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
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

func TestPlanRerunFallsBackToReferenceRun(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "done"}}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"latest": {"done": {ID: "done", ExitCode: 0}}}}
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "latest", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CarriedOrigins["done"].RunID != "latest" {
		t.Fatalf("origin = %#v, want latest run", plan.CarriedOrigins["done"])
	}
	// Without a reference run, a job without an origin has nothing to fall
	// back to; planning must not look up the last run, which may already be
	// the run being planned.
	if _, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "", true, source); !errors.Is(err, ErrNoReferenceRun) {
		t.Fatalf("error = %v, want ErrNoReferenceRun", err)
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
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "", true, source)
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
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || !plan.Execute["array-2"] {
		t.Fatalf("execute = %#v, want only array-2", plan.Execute)
	}
	if plan.CarriedResults["array-1"].ID != "array-1" || plan.CarriedOrigins["array-1"] == nil {
		t.Fatalf("carried = %#v / %#v", plan.CarriedResults, plan.CarriedOrigins)
	}

	plan, err = PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", false, source)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["array"] || plan.Execute["array-2"] {
		t.Fatalf("whole-array execute = %#v, want the array command", plan.Execute)
	}
}

func TestPlanRerunAppliesResultFiltersToArrayTasks(t *testing.T) {
	queue := model.Queue{
		Commands: []model.QueuedCommand{
			{ID: "array", Command: []string{"run"}, Array: &model.ArraySpec{First: 1, Last: 2}},
			{ID: "single", Command: []string{"run"}},
		},
	}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {
		"array-1": {ID: "array-1", ExitCode: 1},
		"array-2": {ID: "array-2", ExitCode: 3},
		"single":  {ID: "single", ExitCode: 1},
	}}}
	filter := jobfilter.Filter{ExitCodes: []int{3}}
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, filter, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array-1"] || !plan.Execute["array-2"] || plan.Execute["single"] {
		t.Fatalf("execute = %#v, want only array-2", plan.Execute)
	}
}

func TestPlanRerunReportsUnknownJobIDs(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "known"}}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {}}}
	_, err := PlanRerun(queue, "job-id", []string{"missing", "also-missing"}, model.CommandSelector{}, jobfilter.Filter{}, "run-1", false, source)
	if err == nil || err.Error() != "job IDs not found in queue: also-missing, missing" {
		t.Fatalf("PlanRerun() error = %v", err)
	}
}

// TestPlanRerunMarkedStatuses checks that a marked status replaces the
// recorded result's for both selection and carry-forward, and that an
// imported-looking queue is planned like any other.
func TestPlanRerunMarkedStatuses(t *testing.T) {
	origin := func(id string) *model.JobOrigin { return &model.JobOrigin{RunID: "source", JobID: id} }
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "reuse", Name: "reuse", Command: []string{"true"}, Origin: origin("reuse")},
		{ID: "accept", Name: "accept", Command: []string{"true"}, MarkedStatus: model.StatusSuccess, Origin: origin("accept")},
		{ID: "dropped", Name: "dropped", Command: []string{"true"}, MarkedStatus: model.StatusUnfinished, Origin: origin("dropped")},
		{ID: "redo", Name: "redo", Command: []string{"true"}, MarkedStatus: model.StatusFailed, Origin: origin("redo")},
		{ID: "halted", Name: "halted", Command: []string{"true"}, MarkedStatus: model.StatusCancelled, Origin: origin("halted")},
		{ID: "downstream", Name: "downstream", Command: []string{"true"}, DependsOn: []string{"dropped"}, Origin: origin("downstream")},
		{ID: "fresh", Name: "fresh", Command: []string{"true"}},
	}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{
		"source": {
			"reuse":      {ID: "reuse", ExitCode: 0},
			"accept":     {ID: "accept", ExitCode: 1, Error: "boom", DiagnosisStatus: model.DiagnosisNoMatch},
			"dropped":    {ID: "dropped", ExitCode: 0},
			"redo":       {ID: "redo", ExitCode: 0},
			"halted":     {ID: "halted", ExitCode: 0},
			"downstream": {ID: "downstream", ExitCode: 0},
		},
		"run-1": {},
	}}
	plan, err := PlanRerun(queue, "failed,unfinished", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"reuse": false, "accept": false, "dropped": true, "redo": true, "halted": true, "downstream": true, "fresh": true}
	for id, execute := range want {
		if plan.Execute[id] != execute {
			t.Fatalf("execute[%s] = %v, want %v (plan %#v)", id, plan.Execute[id], execute, plan.Execute)
		}
	}
	if accepted := plan.CarriedResults["accept"]; !accepted.Accepted || accepted.ExitCode != 0 || accepted.Error != "" || accepted.DiagnosisStatus != "" {
		t.Fatalf("accepted result = %#v", accepted)
	}

	plan, err = PlanRerun(queue, "unfinished", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if redo := plan.CarriedResults["redo"]; redo.ExitCode == 0 || redo.Error != model.MarkedFailedError {
		t.Fatalf("result marked failed = %#v", redo)
	}
	if halted := plan.CarriedResults["halted"]; model.ResultStatus(halted, true) != model.StatusCancelled {
		t.Fatalf("result marked cancelled = %#v", halted)
	}

	plan, err = PlanRerun(queue, "", nil, model.CommandSelector{}, jobfilter.Filter{}, "", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != len(queue.Commands) {
		t.Fatalf("execute = %#v, want every job without a selection", plan.Execute)
	}
}

func TestPlanRerunRejectsMarkWithoutResult(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "new", Command: []string{"true"}, MarkedStatus: model.StatusSuccess}}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {}}}
	if _, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source); err == nil {
		t.Fatal("PlanRerun accepted a job marked success without a result")
	}
}

func TestPlanRerunExecutesDependentsOfExecutingJobs(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "sweep", Name: "sweep", Command: []string{"false"}},
		{ID: "collect", Name: "collect", Command: []string{"true"}, DependsOnFinished: []string{"sweep"}},
		{ID: "report", Name: "report", Command: []string{"true"}, DependsOn: []string{"collect"}},
		{ID: "strict", Name: "strict", Command: []string{"true"}, DependsOn: []string{"other"}},
		{ID: "other", Name: "other", Command: []string{"true"}},
	}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {
		"sweep":   {ID: "sweep", ExitCode: 1},
		"collect": {ID: "collect", ExitCode: 0},
		"report":  {ID: "report", ExitCode: 0},
		"strict":  {ID: "strict", ExitCode: 0},
		"other":   {ID: "other", ExitCode: 0},
	}}}
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"sweep", "collect", "report"} {
		if !plan.Execute[id] {
			t.Fatalf("execute = %#v, want %s to execute", plan.Execute, id)
		}
		if _, carried := plan.CarriedResults[id]; carried && id != "sweep" {
			t.Fatalf("%s is both executed and carried: %#v", id, plan.CarriedResults)
		}
	}
	if plan.Execute["strict"] || plan.Execute["other"] {
		t.Fatalf("execute = %#v, want jobs outside the executing chain carried", plan.Execute)
	}
}

func TestPlanRerunScopeNarrowsSelection(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prep", Command: []string{"prep"}, Stage: "setup"},
		{ID: "train", Command: []string{"train"}, Stage: "train"},
		{ID: "sweep", Command: []string{"sweep"}, Stage: "setup", Array: &model.ArraySpec{First: 1, Last: 2}},
	}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {
		"prep":    {ID: "prep", ExitCode: 1},
		"train":   {ID: "train", ExitCode: 1},
		"sweep-1": {ID: "sweep-1", ExitCode: 0},
		"sweep-2": {ID: "sweep-2", ExitCode: 1},
	}}}
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{Stage: "train"}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 1 || !plan.Execute["train"] {
		t.Fatalf("execute = %#v, want only the failed job in stage train", plan.Execute)
	}
	for _, id := range []string{"prep", "sweep-1", "sweep-2"} {
		if _, carried := plan.CarriedResults[id]; !carried {
			t.Fatalf("carried = %#v, want %s carried", plan.CarriedResults, id)
		}
	}

	plan, err = PlanRerun(queue, "failed", nil, model.CommandSelector{Stage: "setup"}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 2 || !plan.Execute["prep"] || !plan.Execute["sweep-2"] {
		t.Fatalf("execute = %#v, want prep and the failed array task", plan.Execute)
	}

	if _, err := PlanRerun(queue, "failed", nil, model.CommandSelector{Stage: "missing"}, jobfilter.Filter{}, "run-1", true, source); err == nil {
		t.Fatal("PlanRerun accepted a stage without jobs")
	}

	plan, err = PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{NotStages: []string{"setup"}}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 1 || !plan.Execute["train"] {
		t.Fatalf("execute = %#v, want only the failed job outside stage setup", plan.Execute)
	}
	if _, carried := plan.CarriedResults["prep"]; !carried {
		t.Fatalf("carried = %#v, want the excluded failed job carried", plan.CarriedResults)
	}

	plan, err = PlanRerun(queue, "failed", nil, model.CommandSelector{Stage: "setup"}, jobfilter.Filter{NotStages: []string{"setup"}}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 0 {
		t.Fatalf("execute = %#v, want nothing when the filter excludes the scope", plan.Execute)
	}
}

func TestPlanRerunRejectsJobIDsWithResultSelection(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "ok", Command: []string{"true"}}}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {"ok": {ID: "ok"}}}}
	if _, err := PlanRerun(queue, "failed", []string{"ok"}, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source); err == nil ||
		err.Error() != `job IDs cannot be combined with result selection "failed"` {
		t.Fatalf("error = %v, want job IDs rejected with a result selection", err)
	}
	if _, err := PlanRerun(queue, "job-id", []string{"ok"}, model.CommandSelector{}, jobfilter.Filter{NotStages: []string{"setup"}}, "run-1", true, source); err == nil ||
		err.Error() != "job IDs cannot be combined with a job filter" {
		t.Fatalf("error = %v, want job IDs rejected with a job filter", err)
	}
}

// TestPlanRerunUnfinishedMarkDropsResult checks that a job or task marked
// unfinished has no result: it is neither matched by --failed nor carried,
// and --unfinished selects it.
func TestPlanRerunUnfinishedMarkDropsResult(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "edited", Command: []string{"true"}, MarkedStatus: model.StatusUnfinished},
		{ID: "bad", Command: []string{"false"}},
		{ID: "sweep", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2}, TaskMarkedStatus: map[string]string{"sweep-2": model.StatusUnfinished}},
	}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {
		"edited": {ID: "edited"}, "bad": {ID: "bad", ExitCode: 1}, "sweep-1": {ID: "sweep-1"}, "sweep-2": {ID: "sweep-2"},
	}}}
	plan, err := PlanRerun(queue, "failed", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 1 || !plan.Execute["bad"] {
		t.Fatalf("execute = %#v, want only the failed job", plan.Execute)
	}
	for _, id := range []string{"edited", "sweep-2"} {
		if _, carried := plan.CarriedResults[id]; carried {
			t.Fatalf("carried = %#v, want %s's result dropped", plan.CarriedResults, id)
		}
	}
	if _, carried := plan.CarriedResults["sweep-1"]; !carried {
		t.Fatalf("carried = %#v, want the unmarked task carried", plan.CarriedResults)
	}
	plan, err = PlanRerun(queue, "unfinished", nil, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 2 || !plan.Execute["edited"] || !plan.Execute["sweep-2"] {
		t.Fatalf("execute = %#v, want the marked job and task", plan.Execute)
	}
}

func TestPlanRerunRequestedArrayTaskExecutesAlone(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "eval", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 3}},
	}}
	source := &fakeOriginResults{runs: map[string]map[string]model.JobResult{"run-1": {
		"eval-1": {ID: "eval-1"}, "eval-2": {ID: "eval-2", ExitCode: 1}, "eval-3": {ID: "eval-3"},
	}}}
	plan, err := PlanRerun(queue, "job-id", []string{"eval-3"}, model.CommandSelector{}, jobfilter.Filter{}, "run-1", true, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Execute) != 1 || !plan.Execute["eval-3"] {
		t.Fatalf("execute = %#v, want only task 3", plan.Execute)
	}
	if _, carried := plan.CarriedResults["eval-2"]; !carried {
		t.Fatalf("carried = %#v, want the other tasks carried", plan.CarriedResults)
	}
}

package projectrun

import (
	"errors"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
)

func plannedRun(source string, execute []string, carry []string) PlannedRun {
	planned := PlannedRun{SourceRunID: source, Plan: run.Plan{Execute: map[string]bool{}, CarriedResults: map[string]model.JobResult{}}}
	for _, id := range execute {
		planned.Plan.Execute[id] = true
		planned.Queue.Commands = append(planned.Queue.Commands, model.QueuedCommand{ID: id, Name: "job-" + id, Command: []string{"true"}})
	}
	for _, id := range carry {
		planned.Plan.Execute[id] = false
		planned.Plan.CarriedResults[id] = model.JobResult{ID: id}
		planned.Queue.Commands = append(planned.Queue.Commands, model.QueuedCommand{ID: id, Command: []string{"true"}})
	}
	return planned
}

// TestPlanRevisionTellsPlansApart gives plans that differ in one fact each;
// every one gets its own revision, and the same plan always the same.
func TestPlanRevisionTellsPlansApart(t *testing.T) {
	base := plannedRun("r1", []string{"b", "a"}, []string{"c"})
	revision := PlanRevision("abc", base)
	if !strings.HasPrefix(revision, "abc.") || PlanRevision("abc", plannedRun("r1", []string{"a", "b"}, []string{"c"})) != revision {
		t.Fatalf("PlanRevision = %q, want the project revision and an order-independent plan hash", revision)
	}
	for name, other := range map[string]string{
		"project":  PlanRevision("abd", base),
		"source":   PlanRevision("abc", plannedRun("r2", []string{"a", "b"}, []string{"c"})),
		"executed": PlanRevision("abc", plannedRun("r1", []string{"a"}, []string{"b", "c"})),
		"carried":  PlanRevision("abc", plannedRun("r1", []string{"a", "b"}, nil)),
	} {
		if other == revision {
			t.Errorf("a plan with another %s has the same revision %q", name, revision)
		}
	}
}

func TestCheckRunPlanRevision(t *testing.T) {
	previewed := plannedRun("r1", []string{"a"}, []string{"b", "c"})
	started := plannedRun("r1", []string{"a", "b"}, []string{"c"})
	revision := PlanRevision("abc", previewed)
	if err := CheckRunPlanRevision("", "abc", started); err != nil {
		t.Errorf("no revision: %v", err)
	}
	if err := CheckRunPlanRevision("abc", "abc", started); err != nil {
		t.Errorf("a project revision, as check reports, refused the plan: %v", err)
	}
	if err := CheckRunPlanRevision(revision, "abc", previewed); err != nil {
		t.Errorf("the previewed plan was refused: %v", err)
	}
	err := CheckRunPlanRevision(revision, "abc", started)
	if !errors.Is(err, ErrPlanChanged) || !strings.Contains(err.Error(), "execute 2 job(s) (a job-a, b job-b)") {
		t.Errorf("another plan: error = %v, want ErrPlanChanged naming the jobs", err)
	}
	if got := projectPart(revision); got != "abc" {
		t.Errorf("projectPart(%q) = %q", revision, got)
	}
}

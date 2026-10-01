package jobfilter

import (
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestFilterExcludesNegatedStagesAndMatrices(t *testing.T) {
	filter := Filter{NotStages: []string{"prepare", "report"}, NotMatrices: []string{"sweep"}}
	for _, test := range []struct {
		name    string
		command model.QueuedCommand
		want    bool
	}{
		{name: "excluded stage", command: model.QueuedCommand{Stage: "prepare"}, want: false},
		{name: "second excluded stage", command: model.QueuedCommand{Stage: "report"}, want: false},
		{name: "other stage", command: model.QueuedCommand{Stage: "train"}, want: true},
		{name: "no stage", command: model.QueuedCommand{}, want: true},
		{name: "excluded matrix", command: model.QueuedCommand{Matrix: &model.MatrixSpec{BaseName: "sweep"}}, want: false},
		{name: "other matrix", command: model.QueuedCommand{Matrix: &model.MatrixSpec{BaseName: "grid"}}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := filter.MatchesCommand(test.command); got != test.want {
				t.Fatalf("MatchesCommand(%+v) = %t, want %t", test.command, got, test.want)
			}
		})
	}
}

func TestFilterMatchesCommandPattern(t *testing.T) {
	filter := Filter{Command: `python .*\.py`}
	if !filter.MatchesCommand(model.QueuedCommand{Command: []string{"python", "train.py"}}) {
		t.Fatal("filter command pattern did not match python job")
	}
	if filter.MatchesCommand(model.QueuedCommand{Command: []string{"bash", "train.sh"}}) {
		t.Fatal("filter command pattern matched a different command")
	}
}

func TestFilterMatchesExitCodesAndFailureKinds(t *testing.T) {
	filter := Filter{ExitCodes: []int{124}, FailureKinds: []string{"timeout"}}
	if !filter.matchesResult(model.JobResult{ExitCode: 124, Error: "timed out after 1s"}, true) {
		t.Fatal("exit code filter did not match a selected result")
	}
	if filter.matchesResult(model.JobResult{ExitCode: 3, Error: "timed out after 1s"}, true) {
		t.Fatal("exit code filter matched an excluded result")
	}
	if !filter.matchesResult(model.JobResult{ExitCode: 124, Error: "timed out after 1s"}, true) {
		t.Fatal("failure kind filter did not match a timeout result")
	}
	if filter.matchesResult(model.JobResult{ExitCode: 0}, true) {
		t.Fatal("failure kind filter matched a success result")
	}
	if filter.matchesResult(model.JobResult{ExitCode: 1}, false) {
		t.Fatal("unfinished result matched a result filter")
	}
}

func TestSelectsCombinesSelectionAndEveryJobCondition(t *testing.T) {
	failed := Job{ID: "a", Result: model.JobResult{ExitCode: 3}, Finished: true, Attributes: Attributes{Hosts: []string{"worker-01"}}}
	matchDiagnosis := func([]string) bool { return true }
	for _, test := range []struct {
		name      string
		filter    Filter
		selection string
		job       Job
		want      bool
	}{
		{name: "no conditions", selection: "", job: Job{}, want: true},
		{name: "all", selection: "all", job: Job{}, want: true},
		{name: "selection matches", selection: "failed", job: failed, want: true},
		{name: "selection excludes", selection: "success", job: failed, want: false},
		{name: "exit code with selection", filter: Filter{ExitCodes: []int{1}}, selection: "failed", job: failed, want: false},
		{name: "exit code without selection", filter: Filter{ExitCodes: []int{3}}, job: failed, want: true},
		{name: "unfinished never matches exit code", filter: Filter{ExitCodes: []int{0}}, job: Job{}, want: false},
		{name: "definition", filter: Filter{Changed: true, ChangedIDs: map[string]bool{"b": true}}, job: failed, want: false},
		{name: "host", filter: Filter{Hosts: []string{"other"}}, selection: "failed", job: failed, want: false},
		{name: "diagnosis without provider", filter: Filter{Diagnoses: []string{"oom"}}, job: failed, want: false},
		{name: "diagnosis", filter: Filter{Diagnoses: []string{"oom"}}, job: Job{ID: "a", Result: failed.Result, Finished: true, Diagnosis: matchDiagnosis}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.filter.Selects(test.selection, test.job); got != test.want {
				t.Fatalf("Selects(%q, %+v) = %t, want %t", test.selection, test.job, got, test.want)
			}
		})
	}
}

func TestSelectsArrayUsesAggregateSelectionAndAnyTaskCondition(t *testing.T) {
	tasks := []Job{
		{ID: "a-1", Result: model.JobResult{ExitCode: 1}, Finished: true},
		{ID: "a-2", Result: model.JobResult{ExitCode: 2}, Finished: true},
		{ID: "a-3", Finished: true},
	}
	if !(Filter{ExitCodes: []int{2}}).SelectsArray("failed", tasks) {
		t.Fatal("exit code of a later failed task did not select the array")
	}
	if (Filter{ExitCodes: []int{4}}).SelectsArray("", tasks) {
		t.Fatal("exit code no task has selected the array")
	}
	if (Filter{}).SelectsArray("success", tasks) {
		t.Fatal("an array with failed tasks matched success")
	}
	unfinished := append(tasks[:2:2], Job{ID: "a-3"})
	if (Filter{}).SelectsArray("failed", unfinished) || !(Filter{}).SelectsArray("unfinished", unfinished) {
		t.Fatal("an array with an unfinished task is not unfinished as a whole")
	}
}

func TestHasRunConditions(t *testing.T) {
	if (Filter{NotStages: []string{"a"}, Command: "x", Changed: true}).HasRunConditions() {
		t.Fatal("definition conditions reported as run conditions")
	}
	for _, filter := range []Filter{{ExitCodes: []int{1}}, {FailureKinds: []string{"oom"}}, {Diagnoses: []string{"x"}}, {Hosts: []string{"h"}}, {LongerThan: time.Second}} {
		if !filter.HasRunConditions() {
			t.Fatalf("%+v has no run conditions", filter)
		}
	}
}

func TestEmptyFilterMatchesEveryCommand(t *testing.T) {
	filter := Filter{}
	if !filter.Empty() {
		t.Fatal("zero Filter is not empty")
	}
	if !filter.MatchesCommand(model.QueuedCommand{Stage: "prepare"}) {
		t.Fatal("empty filter excluded a command")
	}
	if (Filter{NotStages: []string{"prepare"}}).Empty() {
		t.Fatal("filter with a negated stage is empty")
	}
}

func TestFilterMatchesExecutionAttributes(t *testing.T) {
	started := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Hour)
	filter := Filter{
		Hosts:          []string{"worker-*"},
		StartedAfter:   timePtr(started),
		FinishedBefore: timePtr(finished.Add(time.Minute)),
		LongerThan:     2 * time.Hour,
		ShorterThan:    3 * time.Hour,
	}
	attributes := Attributes{Hosts: []string{"worker-01"}, StartedAt: started, FinishedAt: finished, Now: finished}
	if !filter.MatchesAttributes(attributes) {
		t.Fatal("execution attribute filter did not match")
	}
	if filter.MatchesAttributes(Attributes{Hosts: []string{"other"}, StartedAt: started, FinishedAt: finished, Now: finished}) {
		t.Fatal("host filter matched an unrelated host")
	}
	if filter.MatchesAttributes(Attributes{Hosts: []string{"worker-01"}, StartedAt: started, Now: finished}) {
		t.Fatal("missing finished time matched a finished-time filter")
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}

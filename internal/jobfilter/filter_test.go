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
	if !filter.MatchesResult(model.JobResult{ExitCode: 124, Error: "timed out after 1s"}, true) {
		t.Fatal("exit code filter did not match a selected result")
	}
	if filter.MatchesResult(model.JobResult{ExitCode: 3, Error: "timed out after 1s"}, true) {
		t.Fatal("exit code filter matched an excluded result")
	}
	if !filter.MatchesResult(model.JobResult{ExitCode: 124, Error: "timed out after 1s"}, true) {
		t.Fatal("failure kind filter did not match a timeout result")
	}
	if filter.MatchesResult(model.JobResult{ExitCode: 0}, true) {
		t.Fatal("failure kind filter matched a success result")
	}
	if filter.MatchesResult(model.JobResult{ExitCode: 1}, false) {
		t.Fatal("unfinished result matched a result filter")
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

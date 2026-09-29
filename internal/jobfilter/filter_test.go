package jobfilter

import (
	"testing"

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

package projectrun

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestOmissionReportUsesResultsAndExpandedOrigins(t *testing.T) {
	runner, paths := testRunner(t)
	source := model.Queue{Commands: []model.QueuedCommand{
		{ID: "failed", Name: "failed", Command: []string{"false"}},
		{ID: "dependent", Command: []string{"true"}, DependsOnFinished: []string{"failed"}},
		{ID: "array", Command: []string{"task"}, Array: &model.ArraySpec{First: 1, Last: 3}},
	}}
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "source", "commands.json"), source); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "source", "summary.json"), model.RunSummary{Results: []model.JobResult{
		{ID: "failed", ExitCode: 2}, {ID: "dependent", ExitCode: 0},
		{ID: "array-1", ExitCode: 0}, {ID: "array-2", ExitCode: 3},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		commands []model.QueuedCommand
		want     []string
	}{
		{name: "success dependent is not a failure", want: []string{"array-2", "array-3", "failed"}},
		{name: "command array origin", commands: []model.QueuedCommand{
			{ID: "copied", Command: []string{"task"}, Array: &model.ArraySpec{First: 1, Last: 3}, Origin: &model.JobOrigin{RunID: "source", JobID: "array"}},
		}, want: []string{"failed"}},
		{name: "task origins", commands: []model.QueuedCommand{
			{ID: "copied", Command: []string{"task"}, Array: &model.ArraySpec{First: 1, Last: 3}, TaskOrigins: map[string]*model.JobOrigin{
				"copied-2": {RunID: "source", JobID: "array-2"},
				"copied-3": {RunID: "other", JobID: "array-3"},
			}},
		}, want: []string{"array-3", "failed"}},
		{name: "same id with another origin", commands: []model.QueuedCommand{
			{ID: "failed", Command: []string{"false"}, Origin: &model.JobOrigin{RunID: "other", JobID: "failed"}},
		}, want: []string{"array-2", "array-3", "failed"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			omitted, err := runner.OmittedFailedOrUnfinished(paths, model.Queue{Commands: test.commands}, "source")
			if err != nil || !reflect.DeepEqual(omitted, test.want) {
				t.Fatalf("omitted = %v, %v; want %v", omitted, err, test.want)
			}
		})
	}
}

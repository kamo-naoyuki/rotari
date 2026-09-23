package main

import (
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestResultSelectionMatches(t *testing.T) {
	tests := []struct {
		name      string
		selection string
		finished  bool
		exitCode  int
		want      bool
	}{
		{name: "failed result", selection: "failed", finished: true, exitCode: 1, want: true},
		{name: "successful result is not failed", selection: "failed", finished: true, exitCode: 0, want: false},
		{name: "unfinished result", selection: "unfinished", finished: false, exitCode: 0, want: true},
		{name: "unfinished failed result is not success", selection: "success", finished: false, exitCode: 1, want: false},
		{name: "combined filters", selection: "failed,unfinished", finished: false, exitCode: 0, want: true},
		{name: "success result", selection: "success", finished: true, exitCode: 0, want: true},
		{name: "failed result is not success", selection: "success", finished: true, exitCode: 1, want: false},
		{name: "combined success and failed", selection: "success,failed", finished: true, exitCode: 0, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := model.ResultSelectionMatches(test.selection, test.finished, test.exitCode); got != test.want {
				t.Fatalf("resultSelectionMatches(%q, %t, %d) = %t, want %t", test.selection, test.finished, test.exitCode, got, test.want)
			}
		})
	}
}

func TestFilteredRunMatchesCopySelection(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "source-run"
	queue := Queue{Commands: []QueuedCommand{
		{ID: "success", Name: "success", Command: []string{"success"}},
		{ID: "failed", Name: "failed", Command: []string{"failed"}},
		{ID: "unfinished", Name: "unfinished", Command: []string{"unfinished"}},
	}}
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{RunID: runID, Results: []JobResult{
		{ID: "success", ExitCode: 0}, {ID: "failed", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		selection string
		want      map[string]bool
	}{
		{selection: "failed", want: map[string]bool{"failed": true}},
		{selection: "unfinished", want: map[string]bool{"unfinished": true}},
		{selection: "success", want: map[string]bool{"success": true}},
		{selection: "failed,unfinished", want: map[string]bool{"failed": true, "unfinished": true}},
		{selection: "success,failed", want: map[string]bool{"success": true, "failed": true}},
	}
	for _, test := range tests {
		t.Run(test.selection, func(t *testing.T) {
			if err := writeJSON(paths.QueueFile, Queue{}); err != nil {
				t.Fatal(err)
			}
			if _, err := copyRunToQueue(baseDir, "default", runID, test.selection, nil, false); err != nil {
				t.Fatal(err)
			}
			copied, err := loadQueue(paths.QueueFile)
			if err != nil {
				t.Fatal(err)
			}
			copiedIDs := make(map[string]bool, len(copied.Commands))
			for _, command := range copied.Commands {
				copiedIDs[command.ID] = true
			}
			plan, err := planRerunSelection(paths, queue, test.selection, nil, runID, true)
			if err != nil {
				t.Fatal(err)
			}
			if len(copiedIDs) != len(test.want) || len(plan.Execute) != len(test.want) {
				t.Fatalf("selection=%q copied=%#v execute=%#v want=%#v", test.selection, copiedIDs, plan.Execute, test.want)
			}
			for jobID := range test.want {
				if !copiedIDs[jobID] || !plan.Execute[jobID] {
					t.Fatalf("selection=%q copied=%#v execute=%#v want job %q", test.selection, copiedIDs, plan.Execute, jobID)
				}
			}
		})
	}
}

func TestAggregatedJobResultForArray(t *testing.T) {
	array := &ArraySpec{First: 1, Last: 3}

	tests := []struct {
		name     string
		results  map[string]JobResult
		wantDone bool
		wantExit int
	}{
		{
			name: "unfinished until every task has a result",
			results: map[string]JobResult{
				"job-1": {ID: "job-1", ExitCode: 0},
				"job-2": {ID: "job-2", ExitCode: 0},
			},
			wantDone: false,
		},
		{
			name: "failed when any task fails",
			results: map[string]JobResult{
				"job-1": {ID: "job-1", ExitCode: 0},
				"job-2": {ID: "job-2", ExitCode: 7},
				"job-3": {ID: "job-3", ExitCode: 0},
			},
			wantDone: true,
			wantExit: 7,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, done := aggregatedJobResult("job", array, test.results)
			if done != test.wantDone || result.ExitCode != test.wantExit {
				t.Fatalf("aggregatedJobResult() = (exit=%d, done=%t), want (exit=%d, done=%t)", result.ExitCode, done, test.wantExit, test.wantDone)
			}
		})
	}
}

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

func TestFilteredRunUsesCopiedJobOrigins(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "source-run"
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "success", Name: "success", Command: []string{"success"}},
		{ID: "failed", Name: "failed", Command: []string{"failed"}},
		{ID: "unfinished", Name: "unfinished", Command: []string{"unfinished"}},
	}}
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Results: []model.JobResult{
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
			if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
				t.Fatal(err)
			}
			if _, err := copyRunToQueue(baseDir, "default", runID, "all", nil, false); err != nil {
				t.Fatal(err)
			}
			copied, err := loadQueue(paths.QueueFile)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := planRerunSelection(paths, copied, test.selection, nil, "", true)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Execute) != len(test.want) {
				t.Fatalf("selection=%q execute=%#v want=%#v", test.selection, plan.Execute, test.want)
			}
			for jobID := range test.want {
				if !plan.Execute[jobID] {
					t.Fatalf("selection=%q execute=%#v want job %q", test.selection, plan.Execute, jobID)
				}
			}
		})
	}
}

func TestFilteredRunUsesEachCopiedOrigin(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	for runID, result := range map[string]model.JobResult{
		"old-run":    {ID: "old-job", ExitCode: 1},
		"latest-run": {ID: "latest-job", ExitCode: 0},
	} {
		if err := writeJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{RunID: runID, Results: []model.JobResult{result}}); err != nil {
			t.Fatal(err)
		}
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "copied-old", Command: []string{"old"}, Origin: &model.JobOrigin{RunID: "old-run", JobID: "old-job"}},
		{ID: "copied-latest", Command: []string{"latest"}, Origin: &model.JobOrigin{RunID: "latest-run", JobID: "latest-job"}},
	}}
	plan, err := planRerunSelection(paths, queue, "failed", nil, "latest-run", true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["copied-old"] || plan.Execute["copied-latest"] {
		t.Fatalf("execute = %#v, want only the job copied from old-run", plan.Execute)
	}
}

func TestImportedWorkflowExecutesFailedJobAndDownstream(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "source-run"
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{RunID: runID, Results: []model.JobResult{
		{ID: "source-failed", ExitCode: 1}, {ID: "source-downstream", ExitCode: 0}, {ID: "source-independent", ExitCode: 0},
	}}); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{
		{ID: "failed", Name: "failed", Command: []string{"false"}, Origin: &model.JobOrigin{RunID: runID, JobID: "source-failed"}},
		{ID: "downstream", Name: "downstream", Command: []string{"true"}, DependsOn: []string{"failed"}, Origin: &model.JobOrigin{RunID: runID, JobID: "source-downstream"}},
		{ID: "independent", Name: "independent", Command: []string{"true"}, Origin: &model.JobOrigin{RunID: runID, JobID: "source-independent"}},
	}}
	plan, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["failed"] || !plan.Execute["downstream"] || plan.Execute["independent"] {
		t.Fatalf("execute = %#v", plan.Execute)
	}
}

func TestImportedWorkflowAcceptsFailedSourceResult(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "source-run"
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{RunID: runID, Results: []model.JobResult{{ID: "source", AttemptID: "attempt", ExitCode: 7, Error: "source failed"}}}); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{WorkflowImport: true, Commands: []model.QueuedCommand{{
		ID: "destination", Command: []string{"false"}, Accepted: true,
		Origin: &model.JobOrigin{RunID: runID, JobID: "source", AttemptID: "attempt", Status: "failed"},
	}}}
	plan, err := planRerunSelection(paths, queue, "", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	result := plan.CarriedResults["destination"]
	if plan.Execute["destination"] || result.ExitCode != 0 || !result.Accepted || result.Error != "" || plan.CarriedOrigins["destination"].Status != "failed" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestAggregatedJobResultForArray(t *testing.T) {
	array := &model.ArraySpec{First: 1, Last: 3}

	tests := []struct {
		name     string
		results  map[string]model.JobResult
		wantDone bool
		wantExit int
	}{
		{
			name: "unfinished until every task has a result",
			results: map[string]model.JobResult{
				"job-1": {ID: "job-1", ExitCode: 0},
				"job-2": {ID: "job-2", ExitCode: 0},
			},
			wantDone: false,
		},
		{
			name: "failed when any task fails",
			results: map[string]model.JobResult{
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
			result, done := model.AggregatedJobResult("job", array, test.results)
			if done != test.wantDone || result.ExitCode != test.wantExit {
				t.Fatalf("AggregatedJobResult() = (exit=%d, done=%t), want (exit=%d, done=%t)", result.ExitCode, done, test.wantExit, test.wantDone)
			}
		})
	}
}

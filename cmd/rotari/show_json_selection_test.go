package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// TestShowJSONAppliesFailedSelection gives every JSON run view the same run
// and expects --failed to keep the same jobs the table keeps: failed and
// blocked jobs with any non-zero exit code, but no successful ones.
func TestShowJSONAppliesFailedSelection(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260101-000000-aaaaaaaa"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "tr", Name: "train", Command: []string{"./train.sh"}, Array: &model.ArraySpec{First: 1, Last: 4}},
		{ID: "ok", Name: "ok", Command: []string{"true"}},
		{ID: "ev", Name: "eval", Command: []string{"./eval.sh"}, DependsOn: []string{"ok"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "failed", ExitCode: 1, Results: []model.JobResult{
		{ID: "tr-1", ExitCode: 1}, {ID: "tr-2", ExitCode: 0}, {ID: "tr-3", ExitCode: 124, Error: "timed out after 5s"}, {ID: "tr-4", ExitCode: 0},
		{ID: "ok", ExitCode: 0},
		{ID: "ev", ExitCode: 1, Error: "blocked by failed dependency"},
	}}); err != nil {
		t.Fatal(err)
	}
	location := []string{"--basedir", baseDir, "--project-name", "demo", "--run-id", runID}
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"run", nil, []string{"ev", "tr-1", "tr-3"}},
		{"array job", []string{"--job-id", "tr"}, []string{"tr-1", "tr-3"}},
		{"failed job", []string{"--job-id", "ev"}, []string{"ev"}},
		{"successful job", []string{"--job-id", "ok"}, []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append(append(append([]string{}, location...), test.args...), "--json", "--failed")
			var output bytes.Buffer
			if code := captureShowStdout(t, &output, func() int { return cmdShow(args) }); code != 0 {
				t.Fatalf("show %q exit = %d:\n%s", args, code, output.String())
			}
			var shown struct {
				Summary *struct {
					Results []model.JobResult `json:"results"`
				} `json:"summary"`
				Jobs []struct {
					Job model.JobSpec `json:"job"`
				} `json:"jobs"`
			}
			if err := json.Unmarshal(output.Bytes(), &shown); err != nil {
				t.Fatalf("show %q JSON: %v\n%s", args, err, output.String())
			}
			got := []string{}
			if len(shown.Jobs) > 0 {
				for _, job := range shown.Jobs {
					got = append(got, job.Job.ID)
				}
			} else if shown.Summary != nil {
				for _, result := range shown.Summary.Results {
					got = append(got, result.ID)
				}
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("show %q selects %q, want %q", args, got, test.want)
			}
		})
	}
}

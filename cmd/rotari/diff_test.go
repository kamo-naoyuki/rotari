package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/rundiff"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCmdLineageListsSummarizesAndComparesRuns(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	// Both runs start in the same second, and the later one's ID sorts first,
	// so only the load samples order them.
	first, second := "20260101-000000-bbbbbbbb", "20260101-000000-aaaaaaaa"
	writeRun := func(runID, sampleAt string, commands []model.QueuedCommand, results []model.JobResult) {
		t.Helper()
		runDir := filepath.Join(paths.RunsDir, runID)
		if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: commands}); err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "finished", Results: results}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "load_samples.jsonl"), []byte(`{"at":"`+sampleAt+`","one":1,"five":1,"fifteen":1}`+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeRun(first, "2026-01-01T00:00:00.100000000Z", []model.QueuedCommand{
		{ID: "train", Name: "train", Command: []string{"false"}},
		{ID: "eval", Name: "eval", Command: []string{"true"}},
	}, []model.JobResult{{ID: "train", ExitCode: 1}, {ID: "eval", ExitCode: 0}})
	writeRun(second, "2026-01-01T00:00:00.900000000Z", []model.QueuedCommand{
		{ID: "train", Name: "train", Command: []string{"true"}},
		{ID: "eval", Name: "eval", Command: []string{"true"}},
	}, []model.JobResult{{ID: "train", ExitCode: 0}, {ID: "eval", ExitCode: 0}})
	args := []string{"--basedir", baseDir, "--project-name", "demo"}

	var output bytes.Buffer
	code := captureShowStdout(t, &output, func() int { return cmdLineage(append(args, second)) })
	text := output.String()
	if code != 0 {
		t.Fatalf("lineage summary exit = %d, output:\n%s", code, text)
	}
	for _, want := range []string{"Run: " + second, "Summary: jobs 2, succeeded 2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("lineage output does not contain %q:\n%s", want, text)
		}
	}

	output.Reset()
	code = captureShowStdout(t, &output, func() int { return cmdLineage(append(args, first, second)) })
	text = output.String()
	if code != 0 {
		t.Fatalf("lineage comparison exit = %d, output:\n%s", code, text)
	}
	for _, want := range []string{first + " -> " + second, "fixed 1", "train", "command: false -> true", "1 unchanged job(s) hidden"} {
		if !strings.Contains(text, want) {
			t.Fatalf("lineage comparison does not contain %q:\n%s", want, text)
		}
	}

	output.Reset()
	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdLineage(args) }); code != 0 {
		t.Fatalf("lineage exit = %d", code)
	}
	lineage := output.String()
	if firstAt, secondAt := strings.Index(lineage, first), strings.Index(lineage, second); firstAt < 0 || secondAt < firstAt {
		t.Fatalf("lineage does not list runs oldest first:\n%s", lineage)
	}
	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdLineage(append(args, "--json")) }); code != 0 {
		t.Fatalf("lineage --json exit = %d", code)
	}
	var entries []rundiff.LineageEntry
	if err := json.Unmarshal(output.Bytes(), &entries); err != nil {
		t.Fatalf("lineage JSON: %v\n%s", err, output.String())
	}
	if len(entries) != 2 || entries[0].Changes != nil || entries[0].Counts.Failed != 1 ||
		entries[1].Counts.Succeeded != 2 || entries[1].Changes == nil || entries[1].Changes.Fixed != 1 {
		t.Fatalf("lineage entries = %+v", entries)
	}

	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdLineage(append(args, "--json", second)) }); code != 0 {
		t.Fatalf("lineage RUN exit = %d", code)
	}
	var single rundiff.RunSummary
	if err := json.Unmarshal(output.Bytes(), &single); err != nil {
		t.Fatalf("single run JSON: %v\n%s", err, output.String())
	}
	if single.Run.ID != second || single.Counts.Succeeded != 2 {
		t.Fatalf("single run summary = %+v", single)
	}

	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdLineage(append(args, "--json", first, second)) }); code != 0 {
		t.Fatalf("lineage RUN_A RUN_B exit = %d", code)
	}
	var unified rundiff.Result
	if err := json.Unmarshal(output.Bytes(), &unified); err != nil {
		t.Fatalf("unified comparison JSON: %v\n%s", err, output.String())
	}
	if unified.From.ID != first || unified.To.ID != second || unified.Summary.Fixed != 1 {
		t.Fatalf("unified comparison = %+v", unified)
	}

}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "value") != "value" || firstNonEmpty("", "") != "" {
		t.Fatal("firstNonEmpty returned unexpected results")
	}
}

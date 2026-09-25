package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/rundiff"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestCmdDiffComparesRunWithItsPredecessor(t *testing.T) {
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
	code := captureShowStdout(t, &output, func() int { return cmdDiff(append(args, second)) })
	text := output.String()
	if code != 0 {
		t.Fatalf("cmdDiff exit = %d, output:\n%s", code, text)
	}
	for _, want := range []string{first + " -> " + second, "fixed 1", "train", "command: false -> true", "1 unchanged job(s) hidden"} {
		if !strings.Contains(text, want) {
			t.Fatalf("diff output does not contain %q:\n%s", want, text)
		}
	}

	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdDiff(append(args, "--json", first, second)) }); code != 0 {
		t.Fatalf("cmdDiff --json exit = %d", code)
	}
	var result rundiff.Result
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("diff JSON: %v\n%s", err, output.String())
	}
	if result.From.ID != first || result.To.ID != second || result.Summary.Fixed != 1 || len(result.Jobs) != 2 {
		t.Fatalf("diff JSON = %+v", result)
	}

	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdShow(append(args, "--lineage")) }); code != 0 {
		t.Fatalf("show --lineage exit = %d", code)
	}
	lineage := output.String()
	if firstAt, secondAt := strings.Index(lineage, first), strings.Index(lineage, second); firstAt < 0 || secondAt < firstAt {
		t.Fatalf("lineage does not list runs oldest first:\n%s", lineage)
	}
	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdShow(append(args, "--lineage", "--json")) }); code != 0 {
		t.Fatalf("show --lineage --json exit = %d", code)
	}
	var entries []rundiff.LineageEntry
	if err := json.Unmarshal(output.Bytes(), &entries); err != nil {
		t.Fatalf("lineage JSON: %v\n%s", err, output.String())
	}
	if len(entries) != 2 || entries[0].Changes != nil || entries[0].Counts.Failed != 1 ||
		entries[1].Counts.Succeeded != 2 || entries[1].Changes == nil || entries[1].Changes.Fixed != 1 {
		t.Fatalf("lineage entries = %+v", entries)
	}

	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code = cmdDiff(append(args, first))
	os.Stderr = oldStderr
	_ = writer.Close()
	stderr, _ := io.ReadAll(reader)
	if code != 1 || !strings.Contains(string(stderr), "no earlier run") {
		t.Fatalf("diff of the first run exit = %d, stderr = %q", code, stderr)
	}
}

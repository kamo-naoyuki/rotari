package main

import (
	"bytes"
	"flag"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/jobfilter"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func parseJobFilterOptions(t *testing.T, command string, args ...string) (*jobFilterOptions, error) {
	t.Helper()
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	options := cliJobFilterOptions(fs, true)
	return options, cliParse(fs, args)
}

func TestFilterResultCombinesWithShortForms(t *testing.T) {
	options, err := parseJobFilterOptions(t, "run", "--failed", "--filter-result", "unfinished")
	if err != nil {
		t.Fatal(err)
	}
	if got := options.resultSelection(); got != "failed,unfinished" {
		t.Fatalf("resultSelection() = %q, want failed,unfinished", got)
	}
	if _, err := parseJobFilterOptions(t, "run", "--filter-result", "broken"); err == nil || !strings.Contains(err.Error(), `invalid choice "broken"`) {
		t.Fatalf("--filter-result broken error = %v", err)
	}
}

func TestFilterStageIsTheLongFormOfStage(t *testing.T) {
	options, err := parseJobFilterOptions(t, "copy", "--filter-stage", "train", "--filter-not-matrix", "sweep", "--filter-not-matrix", "grid")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := options.scope()
	if err != nil || scope.Stage != "train" {
		t.Fatalf("scope() = %+v, %v, want stage train", scope, err)
	}
	if got := options.filter(); !reflect.DeepEqual(got, jobfilter.Filter{NotMatrices: []string{"sweep", "grid"}}) {
		t.Fatalf("filter() = %+v", got)
	}

	options, err = parseJobFilterOptions(t, "copy", "--stage", "train", "--filter-stage", "train")
	if err != nil {
		t.Fatal(err)
	}
	if scope, err := options.scope(); err != nil || scope.Stage != "train" {
		t.Fatalf("scope() with the same stage twice = %+v, %v", scope, err)
	}
	options, err = parseJobFilterOptions(t, "copy", "--stage", "train", "--filter-stage", "eval")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := options.scope(); err == nil || err.Error() != `--stage "train" and --filter-stage "eval" differ` {
		t.Fatalf("scope() with two stages error = %v", err)
	}
}

func TestFilterCommandIsParsedAndMatchesTheJobCommand(t *testing.T) {
	options, err := parseJobFilterOptions(t, "change", "--filter-command", `python .*\.py`)
	if err != nil {
		t.Fatal(err)
	}
	if got := options.filter(); got.Command != `python .*\.py` {
		t.Fatalf("filter() = %+v, want command regex %q", got, `python .*\.py`)
	}
	if !options.filter().MatchesCommand(model.QueuedCommand{Command: []string{"python", "train.py"}}) {
		t.Fatal("filter().MatchesCommand did not match the job command")
	}
	if options.filter().MatchesCommand(model.QueuedCommand{Command: []string{"bash", "train.sh"}}) {
		t.Fatal("filter().MatchesCommand matched the wrong job command")
	}
}

func TestFilterExitCodeAndFailureKindAreParsed(t *testing.T) {
	options, err := parseJobFilterOptions(t, "show", "--filter-exit-code", "1", "--filter-exit-code", "2", "--filter-failure-kind", "timeout", "--filter-failure-kind", "oom")
	if err != nil {
		t.Fatal(err)
	}
	if got := options.filter(); !reflect.DeepEqual(got.ExitCodes, []int{1, 2}) || !reflect.DeepEqual(got.FailureKinds, []string{"timeout", "oom"}) {
		t.Fatalf("filter() = %+v, want exit codes [1 2] and failure kinds [timeout oom]", got)
	}
	if _, err := parseJobFilterOptions(t, "show", "--filter-failure-kind", "bogus"); err == nil {
		t.Fatal("--filter-failure-kind bogus accepted an invalid kind")
	}
}

func TestExecutionFiltersAreParsed(t *testing.T) {
	options, err := parseJobFilterOptions(t, "show",
		"--filter-host", "worker-*",
		"--filter-started-after", "2026-09-30T10:00:00Z",
		"--filter-finished-before", "2026-09-30T12:00:00Z",
		"--filter-longer-than", "2h",
		"--filter-shorter-than", "3h")
	if err != nil {
		t.Fatal(err)
	}
	filter := options.filter()
	if !reflect.DeepEqual(filter.Hosts, []string{"worker-*"}) || filter.StartedAfter == nil || filter.FinishedBefore == nil || filter.LongerThan != 2*time.Hour || filter.ShorterThan != 3*time.Hour {
		t.Fatalf("filter() = %+v, want execution filters", filter)
	}
	if _, err := parseJobFilterOptions(t, "show", "--filter-started-after", "not-a-time"); err == nil {
		t.Fatal("invalid started time was accepted")
	}
	if _, err := parseJobFilterOptions(t, "show", "--filter-longer-than", "0s"); err == nil {
		t.Fatal("zero duration was accepted")
	}
}

func TestFilterDiagnosisIsParsedAndValidated(t *testing.T) {
	options, err := parseJobFilterOptions(t, "show", "--filter-diagnosis", "cuda-gpu-memory-exhausted", "--filter-diagnosis", "Python IMPORT")
	if err != nil {
		t.Fatal(err)
	}
	if got := options.filter().Diagnoses; !reflect.DeepEqual(got, []string{"cuda-gpu-memory-exhausted", "Python IMPORT"}) {
		t.Fatalf("diagnoses = %#v", got)
	}
	if _, err := parseJobFilterOptions(t, "show", "--filter-diagnosis", "unknown-rule"); err == nil {
		t.Fatal("unknown diagnosis was accepted")
	}
}

func TestHelpListsFilterOptionsUnderTheirOwnHeading(t *testing.T) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	var output strings.Builder
	fs.SetOutput(&output)
	cliString(fs, "basedir", "")
	cliJobFilterOptions(fs, true)
	if err := cliParse(fs, []string{"--help"}); err != flag.ErrHelp {
		t.Fatalf("cliParse(--help) error = %v", err)
	}
	help := output.String()
	general, filters, found := strings.Cut(help, "\nFilters:\n")
	if !found {
		t.Fatalf("help has no Filters heading:\n%s", help)
	}
	if strings.Contains(general, "-filter-") || !strings.Contains(general, "-basedir") || !strings.Contains(general, "-stage") {
		t.Fatalf("general options = %q, want every option except --filter-*", general)
	}
	for _, name := range []string{"-filter-result", "-filter-stage", "-filter-command", "-filter-not-stage", "-filter-matrix", "-filter-not-matrix"} {
		if !strings.Contains(filters, name) {
			t.Fatalf("filters = %q, want %s", filters, name)
		}
	}
}

func TestCmdShowFilterNotStageExcludesRunJobs(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "prep-job", Stage: "prep", Command: []string{"false"}},
		{ID: "train-bad", Stage: "train", Command: []string{"false"}},
		{ID: "loose-bad", Command: []string{"false"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: "run-1", Status: "finished", Results: []model.JobResult{
		{ID: "prep-job", ExitCode: 1}, {ID: "train-bad", ExitCode: 1}, {ID: "loose-bad", ExitCode: 1},
	}}); err != nil {
		t.Fatal(err)
	}
	args := []string{"--basedir", baseDir, "--project-name", "demo", "--run-id", "run-1"}

	var output bytes.Buffer
	code := captureShowStdout(t, &output, func() int { return cmdShow(append(args, "--filter-not-stage", "prep", "--failed")) })
	text := output.String()
	if code != 0 || !strings.Contains(text, "train-bad") || !strings.Contains(text, "loose-bad") || strings.Contains(text, "prep-job") {
		t.Fatalf("cmdShow --filter-not-stage code=%d output:\n%s", code, text)
	}
	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdShow(append(args, "--filter-not-stage", "prep", "--json")) }); code != 1 {
		t.Fatalf("cmdShow --filter-not-stage --json exit code = %d, want 1", code)
	}
}

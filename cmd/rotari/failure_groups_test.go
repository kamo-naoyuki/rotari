package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestFailureMemberLabels(t *testing.T) {
	task := func(id, name string, number int) runlineage.FailureMember {
		return runlineage.FailureMember{ID: id, Name: name, ArrayTaskID: &number}
	}
	tests := []struct {
		name    string
		members []runlineage.FailureMember
		limit   int
		want    string
	}{
		{"named array", []runlineage.FailureMember{task("a-3", "train[3]", 3), task("a-7", "train[7]", 7)}, 10, "train[3,7]"},
		{"unnamed array", []runlineage.FailureMember{task("abc-2", "", 2), task("abc-5", "", 5)}, 10, "abc[2,5]"},
		{"plain jobs and arrays", []runlineage.FailureMember{
			task("a-1", "train[1]", 1), {ID: "e1", Name: "eval-splittest"}, task("a-2", "train[2]", 2), {ID: "x9"},
		}, 10, "train[1] eval-splittest train[2] x9"},
		{"two arrays", []runlineage.FailureMember{task("a-1", "train[1]", 1), task("b-1", "sweep[1]", 1)}, 10, "train[1] sweep[1]"},
		{"limit", []runlineage.FailureMember{task("a-1", "train[1]", 1), task("a-2", "train[2]", 2), task("a-3", "train[3]", 3), {ID: "e1", Name: "eval"}}, 2, "train[1,2] +2 more"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := failureMemberLabels(test.members, test.limit); got != test.want {
				t.Fatalf("failureMemberLabels() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestShowAndLineageAgreeOnFailureGroups checks that show and lineage group
// one run's failures the same way, in text and JSON, and that show groups
// only the jobs its table lists.
func TestShowAndLineageAgreeOnFailureGroups(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260101-000000-aaaaaaaa"
	runDir := filepath.Join(paths.RunsDir, runID)
	oom := []model.RuleDiagnosis{{Name: "CUDA/GPU memory exhausted", Evidence: "CUDA out of memory", Suggestion: "Reduce batch size."}}
	valueError := []model.RuleDiagnosis{{Name: "Python type or value error", Evidence: "ValueError: shard 2", Suggestion: "Fix the value."}}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{
		{ID: "tr", Name: "train", Command: []string{"./train.sh"}, Array: &model.ArraySpec{First: 1, Last: 4}},
		{ID: "ev", Name: "eval", Command: []string{"./eval.sh"}, DependsOn: []string{"train"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "failed", ExitCode: 1, Results: []model.JobResult{
		{ID: "tr-1", AttemptID: "att-tr-1", ExitCode: 1, DiagnosisStatus: model.DiagnosisMatched, Diagnoses: oom},
		{ID: "tr-2", AttemptID: "att-tr-2", ExitCode: 2, DiagnosisStatus: model.DiagnosisMatched, Diagnoses: valueError},
		{ID: "tr-3", AttemptID: "att-tr-3", ExitCode: 3, DiagnosisStatus: model.DiagnosisMatched, Diagnoses: oom},
		{ID: "tr-4", AttemptID: "att-tr-4", ExitCode: 0},
		{ID: "ev", ExitCode: 1, Error: "blocked by failed dependency"},
	}}); err != nil {
		t.Fatal(err)
	}
	args := []string{"--basedir", baseDir, "--project-name", "demo"}
	run := func(command func([]string) int, extra ...string) string {
		t.Helper()
		var output bytes.Buffer
		if code := captureShowStdout(t, &output, func() int { return command(append(append([]string{}, args...), extra...)) }); code != 0 {
			t.Fatalf("exit = %d, output:\n%s", code, output.String())
		}
		return output.String()
	}

	var shown showJSON
	if err := json.Unmarshal([]byte(run(cmdShow, "--run-id", runID, "--json")), &shown); err != nil {
		t.Fatal(err)
	}
	var summary runlineage.RunSummary
	if err := json.Unmarshal([]byte(run(cmdLineage, "--json", runID)), &summary); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shown.Failures, summary.Failures) {
		t.Fatalf("show --json failures = %+v\nlineage --json failures = %+v", shown.Failures, summary.Failures)
	}
	var causes []string
	for _, group := range summary.Failures {
		causes = append(causes, group.Cause)
	}
	if want := []string{"CUDA/GPU memory exhausted", "Python type or value error", model.FailureKindBlocked}; !reflect.DeepEqual(causes, want) {
		t.Fatalf("causes = %q, want %q", causes, want)
	}
	if got := summary.Failures[0].ExitCodes; !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("OOM exit codes = %v, want [1 3]", got)
	}

	for name, text := range map[string]string{
		"show":    run(cmdShow, "--run-id", runID),
		"lineage": run(cmdLineage, runID),
	} {
		for _, want := range []string{
			"Failures by cause:",
			"2 CUDA/GPU memory exhausted (exit 1,3): train[1,3]",
			"e.g. CUDA out of memory",
			"show: rotari show -j att-tr-1",
			"fix: Reduce batch size.",
			"1 Python type or value error (exit 2): train[2]",
			"1 blocked (exit 1): eval",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s output does not contain %q:\n%s", name, want, text)
			}
		}
	}

	// show points to the compact summary before its job table, and the
	// command works as printed.
	text := run(cmdShow, "--run-id", runID)
	summaryAt, tableAt := strings.Index(text, "Failure summary: rotari lineage "), strings.Index(text, "Jobs:")
	if summaryAt < 0 || tableAt < 0 || summaryAt > tableAt {
		t.Fatalf("show does not point to the failure summary before the job table:\n%s", text)
	}
	line, _, _ := strings.Cut(text[summaryAt:], "\n")
	hinted := strings.Fields(strings.TrimPrefix(line, "Failure summary: rotari lineage "))
	for index, field := range hinted {
		hinted[index] = strings.Trim(field, "'") // the test's values need no shell quoting
	}
	var lineageOutput bytes.Buffer
	if code := captureShowStdout(t, &lineageOutput, func() int { return cmdLineage(hinted) }); code != 0 || !strings.Contains(lineageOutput.String(), "Failures by cause:") {
		t.Fatalf("hinted lineage %q exit = %d, does not summarize failures:\n%s", hinted, code, lineageOutput.String())
	}

	if text := run(cmdShow, "--run-id", runID, "--success"); strings.Contains(text, "Failures by cause:") {
		t.Fatalf("show --success groups jobs its table does not list:\n%s", text)
	}
}

func TestDiffCause(t *testing.T) {
	tests := []struct {
		from, to, want string
	}{
		{"", "", "-"},
		{"oom", "oom", "oom"},
		{"oom", "timeout", "oom -> timeout"},
		{"oom", "", "oom -> -"},
		{"", "timeout", "- -> timeout"},
	}
	for _, test := range tests {
		if got := diffCause(runlineage.JobDiff{FromCause: test.from, ToCause: test.to}); got != test.want {
			t.Errorf("diffCause(%q, %q) = %q, want %q", test.from, test.to, got, test.want)
		}
	}
}

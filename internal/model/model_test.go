package model

import (
	"strings"
	"testing"
	"time"
)

func TestParseAndValidateArrayRange(t *testing.T) {
	array, err := ParseArrayRange("1-3,7")
	if err != nil || array.First != 1 || array.Last != 7 || len(array.Tasks) != 4 {
		t.Fatalf("ParseArrayRange() = %#v, %v", array, err)
	}
	for _, value := range []string{"", "1-", "3-1", "1,1", "1-2-3"} {
		if _, err := ParseArrayRange(value); err == nil {
			t.Errorf("ParseArrayRange(%q) accepted invalid input", value)
		}
	}
	for name, array := range map[string]ArraySpec{
		"negative":     {First: -1, Last: 2},
		"reversed":     {First: 2, Last: 1},
		"wrong bounds": {First: 1, Last: 3, Tasks: []int{2, 3}},
		"unsorted":     {First: 1, Last: 3, Tasks: []int{1, 3, 2}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateArraySpec(&array); err == nil {
				t.Fatalf("ValidateArraySpec(%#v) accepted invalid input", array)
			}
		})
	}
}

func TestValidateEnvironmentAndNames(t *testing.T) {
	if err := ValidateEnvironment([]string{"PATH=/bin", "EMPTY=", "NAME=value=with=equals"}); err != nil {
		t.Fatal(err)
	}
	for _, environment := range [][]string{{"=value"}, {"1NAME=value"}, {"NAME"}, {"NAME=value\x00"}} {
		if err := ValidateEnvironment(environment); err == nil {
			t.Errorf("ValidateEnvironment(%q) accepted invalid input", environment)
		}
	}
	for name, want := range map[string]bool{"PATH": true, "_private2": true, "A_B": true, "": false, "1PATH": false, "PATH-NAME": false, "PATH.NAME": false} {
		if got := ValidEnvironmentName(name); got != want {
			t.Errorf("ValidEnvironmentName(%q) = %t, want %t", name, got, want)
		}
	}
}

func TestParseAndExpandMatrix(t *testing.T) {
	first, err := ParseMatrixDimension("python=3.10,3.11")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseMatrixDimension("cuda=cpu,cuda")
	if err != nil {
		t.Fatal(err)
	}
	combinations := ExpandMatrix([]MatrixDimension{first, second})
	if len(combinations) != 4 || combinations[0][0].Value != "3.10" || combinations[0][1].Value != "cpu" || combinations[3][0].Value != "3.11" || combinations[3][1].Value != "cuda" {
		t.Fatalf("ExpandMatrix = %#v, want four ordered combinations", combinations)
	}
}

func TestParseMatrixDimensionRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "=value", "bad-key=value", "key=", "key=a,,b", "key=a,a"} {
		if _, err := ParseMatrixDimension(value); err == nil {
			t.Errorf("ParseMatrixDimension(%q) returned nil error", value)
		}
	}
}

func TestClearIncompleteMatrixGroups(t *testing.T) {
	dimensions := []MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	commands := []QueuedCommand{
		{ID: "one", Matrix: &MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []MatrixValue{{Name: "SEED", Value: "1"}}}},
		{ID: "other", Command: []string{"true"}},
	}
	ClearIncompleteMatrixGroups(commands)
	if commands[0].Matrix != nil {
		t.Fatalf("incomplete matrix provenance was not cleared: %#v", commands[0].Matrix)
	}
}

func TestValidateMatrixGroupsRejectsInconsistentExpandedJob(t *testing.T) {
	dimensions := []MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	commands := []QueuedCommand{
		{ID: "one", Name: "train-SEED1", Command: []string{"train"}, Environment: []string{"SEED=1"}, Matrix: &MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []MatrixValue{{Name: "SEED", Value: "1"}}, BaseName: "train"}},
		{ID: "two", Name: "train-SEED2", Command: []string{"different"}, Environment: []string{"SEED=2"}, Matrix: &MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []MatrixValue{{Name: "SEED", Value: "2"}}, BaseName: "train"}},
	}
	if err := ValidateMatrixGroups(commands); err == nil {
		t.Fatal("ValidateMatrixGroups accepted inconsistent commands")
	}
}

func TestDependenciesReady(t *testing.T) {
	job := JobSpec{ID: "train", DependsOn: []string{"prepare"}}
	jobs := map[string]JobSpec{"prepare": {ID: "prepare"}}
	if ready, blockedBy := DependenciesReady(job, map[string]JobResult{}, jobs); ready || blockedBy != "" {
		t.Fatalf("unfinished dependency = %t, %q", ready, blockedBy)
	}
	if ready, blockedBy := DependenciesReady(job, map[string]JobResult{"prepare": {ID: "prepare", ExitCode: 1}}, jobs); ready || blockedBy != "prepare" {
		t.Fatalf("failed dependency = %t, %q", ready, blockedBy)
	}
	if ready, blockedBy := DependenciesReady(job, map[string]JobResult{"prepare": {ID: "prepare", ExitCode: 0}}, jobs); !ready || blockedBy != "" {
		t.Fatalf("successful dependency = %t, %q", ready, blockedBy)
	}
}

func TestResultSelectionAndAggregation(t *testing.T) {
	if got := ResultSelection(true, true, false); got != "failed,unfinished" {
		t.Fatalf("ResultSelection() = %q", got)
	}
	if !ResultSelectionMatches("failed,success", true, 1) || !ResultSelectionMatches("unfinished", false, 0) || ResultSelectionMatches("success", true, 1) {
		t.Fatal("ResultSelectionMatches() returned an unexpected result")
	}
	array := &ArraySpec{First: 1, Last: 2}
	result, finished := AggregatedJobResult("array", array, map[string]JobResult{
		"array-1": {ID: "array-1", ExitCode: 0}, "array-2": {ID: "array-2", ExitCode: 9, Error: "failed"},
	})
	if !finished || result.ID != "array" || result.ExitCode != 9 || result.Error != "failed" {
		t.Fatalf("AggregatedJobResult() = %#v, %t", result, finished)
	}
}

func TestFormatDisplayTimestamp(t *testing.T) {
	if got := FormatDisplayTimestamp("2026-09-24T00:00:00Z"); got != "2026-09-24 09:00:00 JST" {
		t.Fatalf("FormatDisplayTimestamp() = %q", got)
	}
	for _, value := range []string{"", "-", "not-a-timestamp"} {
		if got := FormatDisplayTimestamp(value); got != value {
			t.Errorf("FormatDisplayTimestamp(%q) = %q", value, got)
		}
	}
}

func TestFinishedDependenciesExpandAndValidate(t *testing.T) {
	commands := []QueuedCommand{
		{ID: "a", Name: "a", Stage: "sweep", Command: []string{"true"}},
		{ID: "b", Name: "b", Stage: "sweep", Command: []string{"true"}},
		{ID: "collect", Name: "collect", Command: []string{"true"}, DependsOnFinished: []string{"sweep"}},
	}
	if err := ValidateQueueDependencies(commands); err != nil {
		t.Fatalf("valid finished dependency rejected: %v", err)
	}
	jobs := QueueToJobs(commands)
	if got := jobs[2].DependsOnFinished; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("expanded finished dependencies = %v, want stage members", got)
	}
	if got := FormatDependencies([]string{"x"}, []string{"y"}, ","); got != "x,finished:y" {
		t.Fatalf("FormatDependencies = %q", got)
	}

	unknown := append([]QueuedCommand(nil), commands...)
	unknown[2].DependsOnFinished = []string{"missing"}
	if err := ValidateQueueDependencies(unknown); err == nil || !strings.Contains(err.Error(), "unknown job") {
		t.Fatalf("unknown finished dependency error = %v", err)
	}
	cycle := []QueuedCommand{
		{ID: "x", Name: "x", Command: []string{"true"}, DependsOn: []string{"y"}},
		{ID: "y", Name: "y", Command: []string{"true"}, DependsOnFinished: []string{"x"}},
	}
	if err := ValidateQueueDependencies(cycle); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("mixed-kind cycle error = %v", err)
	}
	both := []QueuedCommand{
		{ID: "x", Name: "x", Command: []string{"true"}},
		{ID: "y", Name: "y", Command: []string{"true"}, DependsOn: []string{"x"}, DependsOnFinished: []string{"x"}},
	}
	if err := ValidateQueueDependencies(both); err == nil || !strings.Contains(err.Error(), "both depends_on and depends_on_finished") {
		t.Fatalf("duplicate dependency kind error = %v", err)
	}
}

func TestClearMatrixGroupRewritesFinishedDependencies(t *testing.T) {
	commands := []QueuedCommand{
		{ID: "m1", Name: "train-1", Command: []string{"true"}, Matrix: &MatrixSpec{GroupID: "g", BaseName: "train"}},
		{ID: "m2", Name: "train-2", Command: []string{"true"}, Matrix: &MatrixSpec{GroupID: "g", BaseName: "train"}},
		{ID: "collect", Name: "collect", Command: []string{"true"}, DependsOnFinished: []string{"train"}},
	}
	ClearMatrixGroup(commands, "g")
	if got := commands[2].DependsOnFinished; len(got) != 2 || got[0] != "train-1" || got[1] != "train-2" {
		t.Fatalf("rewritten finished dependencies = %v, want matrix members", got)
	}
}

func TestRetryDelayForAppliesBackoffAndCap(t *testing.T) {
	job := JobSpec{RetryDelay: "10s", RetryBackoff: 3, RetryMaxDelay: "1m"}
	want := []time.Duration{10 * time.Second, 30 * time.Second, time.Minute, time.Minute}
	for index, expected := range want {
		if got := job.RetryDelayFor(index + 1); got != expected {
			t.Fatalf("RetryDelayFor(%d) = %s, want %s", index+1, got, expected)
		}
	}
	if got := (JobSpec{}).RetryDelayFor(3); got != 0 {
		t.Fatalf("RetryDelayFor without a delay = %s, want immediate", got)
	}
	if got := (JobSpec{RetryDelay: "5s"}).RetryDelayFor(4); got != 5*time.Second {
		t.Fatalf("RetryDelayFor without backoff = %s, want a constant delay", got)
	}
	three := 3
	if got := FormatRetryPolicy(JobSpec{Retry: &three, RetryDelay: "10s", RetryBackoff: 2, RetryMaxDelay: "1m"}); got != "3 (delay 10s, backoff x2, max 1m)" {
		t.Fatalf("FormatRetryPolicy = %q", got)
	}
	for _, invalid := range [][]any{{"soon", 0.0, ""}, {"", 0.5, ""}, {"", 0.0, "-1s"}} {
		if err := ValidateRetryBackoff(invalid[0].(string), invalid[1].(float64), invalid[2].(string)); err == nil {
			t.Fatalf("ValidateRetryBackoff(%v) accepted an invalid value", invalid)
		}
	}
}

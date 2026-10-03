package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// TestRunDiffGroupsArrayTasksThatReadTheSame compares an array whose tasks
// share a definition change but differ in result, with a carried task
// between failing ones, and a job outside the array.
func TestRunDiffGroupsArrayTasksThatReadTheSame(t *testing.T) {
	timeout := []runlineage.Change{{Field: "timeout", From: "5s", To: "60s"}}
	task := func(name, from, to, transition, cause string, carried bool) runlineage.JobDiff {
		return runlineage.JobDiff{Name: name, FromStatus: from, ToStatus: to, Transition: transition, FromCause: cause, ToCause: cause, Carried: carried, Changes: timeout}
	}
	result := runlineage.Result{Jobs: []runlineage.JobDiff{
		task("train[1]", "success", "success", runlineage.TransitionUnchanged, "", true),
		task("train[2]", "failed", "failed", runlineage.TransitionStillFailing, "OOM", false),
		task("train[3]", "success", "success", runlineage.TransitionUnchanged, "", true),
		task("train[4]", "failed", "failed", runlineage.TransitionStillFailing, "OOM", false),
		task("train[5]", "failed", "success", runlineage.TransitionFixed, "", false),
		{Name: "eval", FromStatus: "failed", ToStatus: "failed", Transition: runlineage.TransitionStillFailing, FromCause: "KeyError", ToCause: "KeyError"},
	}}
	var output bytes.Buffer
	writeRunDiff(&output, state.ProjectPaths{ProjectName: "exp"}, result, false)
	text := output.String()
	for _, want := range []string{
		"\ntrain[1,3]  ", "unchanged (carried)",
		"\ntrain[2,4]  ", "\ntrain[5]  ", "\neval  ",
		"Definition changes:\n  train[1,2,3,4,5]\n    timeout: 5s -> 60s\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("comparison lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "timeout (carried)") || strings.Count(text, "\ntrain[") != 3 {
		t.Errorf("comparison does not group the tasks or still marks carried in CHANGES:\n%s", text)
	}
}

// TestRunDiffNamesTheJobsItHides hides unchanged jobs, one rerun with the
// same result and carried array tasks, and checks that the hidden line names
// them, marking the carried ones.
func TestRunDiffNamesTheJobsItHides(t *testing.T) {
	unchanged := func(name string, carried bool) runlineage.JobDiff {
		return runlineage.JobDiff{Name: name, FromStatus: "success", ToStatus: "success", Transition: runlineage.TransitionUnchanged, Carried: carried}
	}
	result := runlineage.Result{Jobs: []runlineage.JobDiff{
		unchanged("prep[1]", true), unchanged("prep[2]", true),
		{Name: "train", FromStatus: "failed", ToStatus: "success", Transition: runlineage.TransitionFixed},
		unchanged("eval-splitval", false),
	}}
	var output bytes.Buffer
	writeRunDiff(&output, state.ProjectPaths{ProjectName: "exp"}, result, false)
	if want := "3 unchanged job(s) hidden: prep[1,2] (carried), eval-splitval\n"; !strings.Contains(output.String(), want) {
		t.Fatalf("comparison does not name the hidden jobs, want %q:\n%s", want, output.String())
	}
}

package rundiff

import (
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func job(name, status string, command ...string) Job {
	return Job{Spec: model.JobSpec{ID: name + "-id", Name: name, Command: command}, Status: status}
}

func TestCompareClassifiesTransitionsAndChanges(t *testing.T) {
	from := Run{ID: "run-1", StartedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:02:00Z", Jobs: []Job{
		job("fixed", StatusFailed, "false"),
		job("still", StatusFailed, "false"),
		job("newly", StatusSuccess, "true"),
		job("same", StatusSuccess, "true"),
		job("gone", StatusSuccess, "true"),
		job("blocked", StatusBlocked, "true"),
	}}
	fromEnv := job("env", StatusSuccess, "true")
	fromEnv.Spec.Environment = []string{"LR=0.1", "KEEP=1"}
	from.Jobs = append(from.Jobs, fromEnv)

	toEnv := job("env", StatusSuccess, "true")
	toEnv.Spec.Environment = []string{"KEEP=1", "LR=0.01"}
	carried := job("same", StatusSuccess, "true")
	carried.Carried = true
	to := Run{ID: "run-2", Name: "retry", Jobs: []Job{
		job("fixed", StatusSuccess, "sh", "-c", "exit 0"),
		job("still", StatusFailed, "false"),
		job("newly", StatusFailed, "true"),
		carried,
		job("added", StatusSuccess, "true"),
		job("blocked", StatusSuccess, "true"),
		toEnv,
	}}
	result := Compare(from, to)

	transitions := map[string]string{}
	for _, diff := range result.Jobs {
		transitions[diff.Name] = diff.Transition
	}
	want := map[string]string{
		"fixed": TransitionFixed, "still": TransitionStillFailing, "newly": TransitionNewlyFailing,
		"same": TransitionUnchanged, "added": TransitionAdded, "gone": TransitionRemoved,
		"blocked": TransitionFixed, "env": TransitionUnchanged,
	}
	if !reflect.DeepEqual(transitions, want) {
		t.Fatalf("transitions = %v, want %v", transitions, want)
	}
	wantSummary := Summary{Fixed: 2, StillFailing: 1, NewlyFailing: 1, Added: 1, Removed: 1, Changed: 2, Carried: 1}
	if result.Summary != wantSummary {
		t.Fatalf("summary = %+v, want %+v", result.Summary, wantSummary)
	}
	if result.From.Elapsed != "2m0s" || result.To.Elapsed != "" || result.To.Name != "retry" {
		t.Fatalf("run info = %+v -> %+v", result.From, result.To)
	}
	if last := result.Jobs[len(result.Jobs)-1]; last.Name != "gone" {
		t.Fatalf("removed jobs should follow the newer run's jobs, got %q last", last.Name)
	}
	for _, diff := range result.Jobs {
		switch diff.Name {
		case "fixed":
			if len(diff.Changes) != 1 || !reflect.DeepEqual(diff.Changes[0], Change{Field: "command", From: "false", To: "sh -c 'exit 0'"}) {
				t.Fatalf("fixed changes = %+v", diff.Changes)
			}
		case "env":
			if len(diff.Changes) != 1 || !reflect.DeepEqual(diff.Changes[0].Added, []string{"LR=0.01"}) || !reflect.DeepEqual(diff.Changes[0].Removed, []string{"LR=0.1"}) {
				t.Fatalf("env changes = %+v, want LR swapped and order ignored", diff.Changes)
			}
		}
	}
}

func TestCompareMatchesUnnamedJobsByID(t *testing.T) {
	from := Run{Jobs: []Job{{Spec: model.JobSpec{ID: "abc", Command: []string{"true"}}, Status: StatusFailed}}}
	to := Run{Jobs: []Job{{Spec: model.JobSpec{ID: "abc", Command: []string{"true"}}, Status: StatusSuccess}}}
	result := Compare(from, to)
	if len(result.Jobs) != 1 || result.Jobs[0].Name != "abc" || result.Jobs[0].Transition != TransitionFixed {
		t.Fatalf("jobs = %+v, want one fixed job matched by ID", result.Jobs)
	}
}

func TestComparePrefersOriginAndDoesNotGuessAcrossRuns(t *testing.T) {
	from := Run{ID: "run-1", Jobs: []Job{
		job("same", StatusFailed, "old"),
		job("other", StatusFailed, "false"),
	}}
	matched := job("renamed", StatusSuccess, "new")
	matched.Origin = &model.JobOrigin{RunID: "run-1", JobID: "same-id"}
	foreign := job("other", StatusSuccess, "new")
	foreign.Origin = &model.JobOrigin{RunID: "run-0", JobID: "other-id"}
	result := Compare(from, Run{ID: "run-2", Jobs: []Job{matched, foreign}})
	if result.Summary.Fixed != 1 || result.Summary.Added != 1 || result.Summary.Removed != 1 {
		t.Fatalf("summary = %+v, jobs = %+v", result.Summary, result.Jobs)
	}
	if result.Jobs[0].FromID != "same-id" || result.Jobs[0].Transition != TransitionFixed {
		t.Fatalf("origin match = %+v", result.Jobs[0])
	}
	if result.Jobs[1].Transition != TransitionAdded || result.Jobs[1].Name != "other" {
		t.Fatalf("foreign origin match = %+v", result.Jobs[1])
	}
	if result.Jobs[2].Transition != TransitionRemoved || result.Jobs[2].Name != "other" {
		t.Fatalf("removed old job = %+v", result.Jobs[2])
	}
}

func TestSummarizeCountsResolvedStatuses(t *testing.T) {
	run := Run{Jobs: []Job{
		job("success", StatusSuccess, "true"),
		job("failed", StatusFailed, "false"),
		job("blocked", StatusBlocked, "true"),
		job("unfinished", StatusUnfinished, "true"),
	}}
	if got, want := Summarize(run), (Counts{Jobs: 4, Succeeded: 1, Failed: 1, Blocked: 1, Unfinished: 1}); got != want {
		t.Fatalf("summary = %+v, want %+v", got, want)
	}
}

func TestSummarizeDiagnosesGroupsFailureReasons(t *testing.T) {
	matched := job("matched", StatusFailed, "false")
	matched.Diagnoses = []string{"OOM", "GPU"}
	noMatch := job("no-match", StatusFailed, "false")
	noMatch.DiagnosisStatus = model.DiagnosisNoMatch
	success := job("success", StatusSuccess, "true")
	success.Diagnoses = []string{"ignored"}
	run := Run{Jobs: []Job{matched, noMatch, success}}
	got := SummarizeDiagnoses(run)
	want := []DiagnosisCount{{Name: "GPU", Count: 1}, {Name: "OOM", Count: 1}, {Name: model.DiagnosisNoMatch, Count: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnoses = %+v, want %+v", got, want)
	}
}

func TestLineageCountsRunsAndChangesFromPrevious(t *testing.T) {
	first := Run{ID: "run-1", Jobs: []Job{job("a", StatusFailed, "false"), job("b", StatusBlocked, "true")}}
	second := Run{ID: "run-2", Jobs: []Job{job("a", StatusSuccess, "true"), job("b", StatusSuccess, "true"), job("c", StatusUnfinished, "true")}}
	entries := Lineage([]Run{first, second})
	if len(entries) != 2 || entries[0].Changes != nil {
		t.Fatalf("entries = %+v, want no changes for the first run", entries)
	}
	if entries[0].Counts != (Counts{Jobs: 2, Failed: 1, Blocked: 1}) {
		t.Fatalf("first counts = %+v", entries[0].Counts)
	}
	if entries[1].Counts != (Counts{Jobs: 3, Succeeded: 2, Unfinished: 1}) {
		t.Fatalf("second counts = %+v", entries[1].Counts)
	}
	if changes := entries[1].Changes; changes == nil || changes.Fixed != 2 || changes.Added != 1 || changes.Changed != 1 {
		t.Fatalf("second changes = %+v", changes)
	}
}

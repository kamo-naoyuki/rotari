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

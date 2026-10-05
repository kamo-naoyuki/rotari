package jobstatus

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestResolveJobPrefersAttemptOverSummary(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status"), "3\n")
	summary := model.JobResult{ID: "job-1", AttemptID: "att-1", ExitCode: 0, Hosts: []string{"summary-host"}}
	job := ReadJob(testStore(), jobDir, summary, true)
	if job.Source != SourceStatus || job.ExitCode != 3 || job.Blocked() {
		t.Fatalf("ReadJob() = %#v, want status file exit code", job)
	}
	result, ok := job.Result(model.JobSpec{ID: "job-1"})
	if !ok || result.ExitCode != 3 || result.AttemptID != "att-1" || !reflect.DeepEqual(result.Hosts, []string{"summary-host"}) {
		t.Fatalf("Result() = %#v, %v, want summary metadata with attempt exit code", result, ok)
	}
}

func TestResolveJobFallsBackToSummary(t *testing.T) {
	jobDir := filepath.Join(t.TempDir(), "missing")
	tests := []struct {
		name         string
		summary      model.JobResult
		wantBlocked  bool
		wantAccepted bool
	}{
		{name: "failure", summary: model.JobResult{ID: "job-1", ExitCode: 1, Error: "submit failed"}},
		{name: "blocked", summary: model.JobResult{ID: "job-1", ExitCode: 1, Error: "blocked by failed dependency"}, wantBlocked: true},
		{name: "accepted", summary: model.JobResult{ID: "job-1", ExitCode: 0, Accepted: true}, wantAccepted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := ReadJob(testStore(), jobDir, test.summary, true)
			if job.Source != SourceSummary || job.ExitCode != test.summary.ExitCode {
				t.Fatalf("ReadJob() = %#v, want summary fallback", job)
			}
			if job.Blocked() != test.wantBlocked || job.Accepted() != test.wantAccepted {
				t.Fatalf("Blocked() = %v, Accepted() = %v", job.Blocked(), job.Accepted())
			}
			if result, ok := job.Result(model.JobSpec{ID: "job-1"}); !ok || !reflect.DeepEqual(result, test.summary) {
				t.Fatalf("Result() = %#v, %v, want summary result", result, ok)
			}
		})
	}
}

func TestResolveJobBlockedOnlyFromSummary(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status"), "1\n")
	job := ReadJob(testStore(), jobDir, model.JobResult{ID: "job-1", ExitCode: 1, Error: "blocked by failed dependency"}, true)
	if job.Blocked() {
		t.Fatal("a job with its own status was reported as blocked")
	}
}

func TestResolveJobWithoutOutcome(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status.json"), `{"phase":"running","hosts":["node1"]}`)
	job := ReadJob(testStore(), jobDir, model.JobResult{}, false)
	if job.Finished() {
		t.Fatalf("ReadJob() = %#v, want unfinished job", job)
	}
	if _, ok := job.Result(model.JobSpec{ID: "job-1"}); ok {
		t.Fatal("unfinished job returned a result")
	}
	if !reflect.DeepEqual(job.Hosts(), []string{"node1"}) {
		t.Fatalf("Hosts() = %#v, want wrapper hosts", job.Hosts())
	}
}

func TestManualRetryPendingHidesSupersededTerminalResult(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status"), "9\n")
	writeFile(t, filepath.Join(jobDir, state.ManualRetryPendingFileName), `{"request_id":"req-1"}`)
	job := ReadJob(testStore(), jobDir, model.JobResult{ID: "job-1", ExitCode: 9}, true)
	if job.Finished() {
		t.Fatalf("ReadJob with pending manual retry = %#v; old result must be superseded", job)
	}
	if _, ok := job.Result(model.JobSpec{ID: "job-1"}); ok {
		t.Fatal("pending manual retry exposed the old terminal result")
	}
}

func TestResolveAttemptIgnoresSummaryForOlderAttempt(t *testing.T) {
	jobDir := t.TempDir()
	writeFile(t, filepath.Join(jobDir, "status.json"), `{"phase":"running","hosts":["old-host"]}`)
	summary := model.JobResult{ID: "job-1", AttemptID: "att-2", ExitCode: 0, Hosts: []string{"summary-host"}}
	attempt := ReadAttempt(testStore(), jobDir)

	older := ResolveAttempt(attempt, false, summary, true)
	if older.Finished() || older.HasSummary {
		t.Fatalf("ResolveAttempt(older) = %#v, want unfinished attempt without summary", older)
	}
	if _, ok := older.Result(model.JobSpec{ID: "job-1"}); ok {
		t.Fatal("older unfinished attempt returned the summary result")
	}

	latest := ResolveAttempt(attempt, true, summary, true)
	if latest.Source != SourceSummary || !latest.HasSummary {
		t.Fatalf("ResolveAttempt(latest) = %#v, want summary fallback", latest)
	}
}

// TestRecordedResultsPrefersTheSummaryToCarriedResults reads a run's
// recorded results before and after its summary exists, and for a run with
// neither file.
func TestRecordedResultsPrefersTheSummaryToCarriedResults(t *testing.T) {
	runDir := t.TempDir()
	if got := RecordedResults(runDir, nil); len(got) != 0 {
		t.Fatalf("without files: %v", got)
	}
	carried := model.RunSummary{RunID: "run-2", Results: []model.JobResult{{ID: "kept", ExitCode: 0, AttemptID: "att-1"}}}
	if err := state.WriteJSON(filepath.Join(runDir, state.CarriedResultsFileName), carried); err != nil {
		t.Fatal(err)
	}
	if got := RecordedResults(runDir, nil); len(got) != 1 || got["kept"].AttemptID != "att-1" {
		t.Fatalf("before the summary: %v", got)
	}
	summary := model.RunSummary{Results: []model.JobResult{{ID: "kept", ExitCode: 0, AttemptID: "att-1"}, {ID: "ran", ExitCode: 3}}}
	if got := RecordedResults(runDir, &summary); len(got) != 2 || got["ran"].ExitCode != 3 {
		t.Fatalf("with the summary: %v", got)
	}
}

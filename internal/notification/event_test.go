package notification

import (
	"reflect"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestNewBatchFiltersAndLimitsJobs(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	events := []Event{
		NewJobEvent("demo", "run-1", "nightly", model.JobSpec{ID: "ok"}, model.JobResult{ID: "ok"}, now),
		NewJobEvent("demo", "run-1", "nightly", model.JobSpec{ID: "bad-1"}, model.JobResult{ID: "bad-1", ExitCode: 1}, now),
		NewJobEvent("demo", "run-1", "nightly", model.JobSpec{ID: "bad-2"}, model.JobResult{ID: "bad-2", ExitCode: 1}, now),
		NewRunEvent("demo", model.RunSummary{RunID: "run-1", RunName: "nightly", Status: "failed"}, now),
	}
	settings := Defaults().Webhook.ChannelSettings
	settings.MaxJobs = 1
	batch := NewBatch(events, settings, now)
	if len(batch.Events) != 2 || batch.Events[0].Job.ID != "bad-1" || batch.Events[1].Kind != RunFinished {
		t.Fatalf("events = %#v", batch.Events)
	}
	if batch.OmittedJobs != 1 {
		t.Fatalf("omitted jobs = %d", batch.OmittedJobs)
	}
}

func TestNewPayloadSelectsAvailableFields(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	event := NewJobEvent("demo", "run-1", "nightly",
		model.JobSpec{ID: "job-1", Name: "train", Command: []string{"false"}},
		model.JobResult{ID: "job-1", ExitCode: 1, DiagnosisStatus: model.DiagnosisMatched, Diagnoses: []model.RuleDiagnosis{{Name: "Exit failure", Suggestion: "inspect logs"}}}, now)
	event.SetTimestamps("2026-01-02T03:04:00Z", "2026-01-02T03:04:05Z")
	event.SetDiagnosisOutdated(true)
	batch := NewBatch([]Event{event}, Defaults().Webhook.ChannelSettings, now)
	payload := NewPayload(batch, []string{"project", "job_name", "exit_code", "duration", "diagnosis_name", "diagnosis_suggestion", "diagnosis_outdated", "run_name"})
	if len(payload.Events) != 1 {
		t.Fatalf("events = %#v", payload.Events)
	}
	want := []Field{
		{Name: "project", Value: "demo"}, {Name: "job_name", Value: "train"},
		{Name: "exit_code", Value: 1}, {Name: "duration", Value: "5s"},
		{Name: "diagnosis_name", Value: "Exit failure"}, {Name: "diagnosis_suggestion", Value: "inspect logs"},
		{Name: "diagnosis_outdated", Value: true},
		{Name: "run_name", Value: "nightly"},
	}
	if got := payload.Events[0].Fields; !reflect.DeepEqual(got, want) {
		t.Fatalf("fields = %#v, want %#v", got, want)
	}
}

func TestNewPayloadAddsRunCounts(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	settings := Defaults().Webhook.ChannelSettings
	settings.JobSuccess = true
	batch := NewBatch([]Event{
		NewJobEvent("demo", "run-1", "", model.JobSpec{ID: "ok"}, model.JobResult{ID: "ok"}, now),
		NewJobEvent("demo", "run-1", "", model.JobSpec{ID: "bad"}, model.JobResult{ID: "bad", ExitCode: 2}, now),
		NewRunEvent("demo", model.RunSummary{RunID: "run-1", Status: "failed", ExitCode: 1, Results: []model.JobResult{{ID: "ok"}, {ID: "bad", ExitCode: 2}}}, now),
	}, settings, now)
	payload := NewPayload(batch, []string{"run_status", "exit_code", "success_count", "failure_count", "total_count"})
	got := payload.Events[2].Fields
	want := []Field{{Name: "run_status", Value: "failed"}, {Name: "exit_code", Value: 1}, {Name: "success_count", Value: 1}, {Name: "failure_count", Value: 1}, {Name: "total_count", Value: 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("run fields = %#v, want %#v", got, want)
	}
}

// Run events are chosen by the run's outcome: a run that finished with exit
// code 0 is a success whatever its recorded status text; any other run,
// including a cancelled one, is a failure.
func TestRunEventsAreChosenByOutcome(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	successOnly := ChannelSettings{RunSuccess: true}
	failureOnly := ChannelSettings{RunFailure: true}
	for _, test := range []struct {
		summary model.RunSummary
		success bool
	}{
		{model.RunSummary{RunID: "ok", Status: "finished", ExitCode: 0}, true},
		{model.RunSummary{RunID: "bad", Status: "failed", ExitCode: 1}, false},
		{model.RunSummary{RunID: "stopped", Status: "cancelled", ExitCode: 1}, false},
	} {
		event := NewRunEvent("demo", test.summary, now)
		if successOnly.Includes(event) != test.success || failureOnly.Includes(event) == test.success {
			t.Errorf("run %s (status %q): success channel %t, failure channel %t; want success=%t",
				test.summary.RunID, test.summary.Status, successOnly.Includes(event), failureOnly.Includes(event), test.success)
		}
	}
}

package notification

import (
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

package web

import (
	"testing"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
)

// A job's and each attempt's elapsed time is measured from the raw times
// before they are formatted for display: to the finish when a result exists,
// to now while running, and left empty when an end is unknown.
func TestFormatQueueDisplayTimesSetsElapsed(t *testing.T) {
	submitted := time.Now().Add(-90 * time.Second).UTC()
	state := &QueueState{Runs: []Run{{Jobs: []Job{{
		SubmittedAt: submitted.Format(time.RFC3339Nano),
		FinishedAt:  submitted.Add(3*time.Minute + 5*time.Second).Format(time.RFC3339Nano),
		Result:      &model.JobResult{},
		Attempts: []Attempt{
			{SubmittedAt: submitted.Format(time.RFC3339Nano)},
			{SubmittedAt: submitted.Format(time.RFC3339Nano), Result: &model.JobResult{}},
		},
	}}}}}
	FormatQueueDisplayTimes(state)
	job := state.Runs[0].Jobs[0]
	if job.Elapsed != "3m 05s" {
		t.Errorf("finished job elapsed = %q, want 3m 05s", job.Elapsed)
	}
	if got := job.Attempts[0].Elapsed; got != "1m 30s" && got != "1m 31s" {
		t.Errorf("running attempt elapsed = %q, want about 1m 30s", got)
	}
	if got := job.Attempts[1].Elapsed; got != "" {
		t.Errorf("finished attempt without a finish time elapsed = %q, want empty", got)
	}
}

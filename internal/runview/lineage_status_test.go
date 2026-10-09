package runview

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
)

func TestLineageStatusClassifiesResolvedJobs(t *testing.T) {
	for _, test := range []struct {
		name   string
		job    jobstatus.Job
		status string
	}{
		{"unfinished", jobstatus.ResolveJob(jobstatus.Attempt{}, model.JobResult{}, false), runlineage.StatusUnfinished},
		{"success", jobstatus.ResolveJob(jobstatus.Attempt{}, model.JobResult{ExitCode: 0}, true), runlineage.StatusSuccess},
		{"accepted", jobstatus.ResolveJob(jobstatus.Attempt{}, model.JobResult{ExitCode: 0, Accepted: true}, true), runlineage.StatusSuccess},
		{"failed", jobstatus.ResolveJob(jobstatus.Attempt{}, model.JobResult{ExitCode: 3}, true), runlineage.StatusFailed},
		{"cancelled counts as failed", jobstatus.ResolveJob(jobstatus.Attempt{}, model.JobResult{ExitCode: 130, Error: model.MarkedCancelledError}, true), runlineage.StatusFailed},
		{"blocked", jobstatus.ResolveJob(jobstatus.Attempt{}, model.JobResult{ExitCode: 1, Error: "blocked by failed dependency"}, true), runlineage.StatusBlocked},
		{"attempt failure", jobstatus.ResolveJob(jobstatus.Attempt{ExitCode: 2, Source: jobstatus.SourceStatus}, model.JobResult{}, false), runlineage.StatusFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := LineageStatus(test.job); got != test.status {
				t.Fatalf("LineageStatus() = %q, want %q", got, test.status)
			}
		})
	}
}

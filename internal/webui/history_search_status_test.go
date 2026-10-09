package webui

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

// History search matches the job label the run view shows, so an accepted
// carried result keeps both markers.
func TestHistorySearchJobStatusUsesTheJobLabel(t *testing.T) {
	job := webprojection.Job{ID: "job", ExecutionStatus: "success (accepted) (carried)", Carried: true, Result: &model.JobResult{ID: "job", Accepted: true}}
	if got := historySearchJobRecord(webprojection.HistorySearchRecord{}, job).JobStatus; got != "success (accepted) (carried)" {
		t.Fatalf("history search job status = %q", got)
	}
}

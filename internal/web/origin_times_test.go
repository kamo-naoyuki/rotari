package web

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestResolveOriginTimestampsPrefersLocalAndOrigin(t *testing.T) {
	origin := &model.JobOrigin{RunID: "run-1", JobID: "job-1", SubmittedAt: "origin-start", FinishedAt: "origin-finish"}
	gotSubmitted, gotFinished := ResolveOriginTimestamps("local-start", "", origin, func(string, string) (string, string) {
		return "source-start", "source-finish"
	})
	if gotSubmitted != "local-start" || gotFinished != "origin-finish" {
		t.Fatalf("timestamps = %q, %q", gotSubmitted, gotFinished)
	}
}

func TestResolveOriginTimestampsUsesSourceFallback(t *testing.T) {
	origin := &model.JobOrigin{RunID: "run-1", JobID: "job-1"}
	gotSubmitted, gotFinished := ResolveOriginTimestamps("", "", origin, func(runID, jobID string) (string, string) {
		if runID != "run-1" || jobID != "job-1" {
			t.Fatalf("source args = %q, %q", runID, jobID)
		}
		return "source-start", "source-finish"
	})
	if gotSubmitted != "source-start" || gotFinished != "source-finish" {
		t.Fatalf("timestamps = %q, %q", gotSubmitted, gotFinished)
	}
}

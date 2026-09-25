package jobstatus

import (
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestResolveOriginTimestampsPrefersLocalAndOrigin(t *testing.T) {
	origin := &model.JobOrigin{RunID: "run-1", JobID: "job-1", SubmittedAt: "origin-start", FinishedAt: "origin-finish"}
	gotSubmitted, gotFinished := resolveOriginTimestamps("local-start", "", origin, func(string, string) (string, string) {
		return "source-start", "source-finish"
	})
	if gotSubmitted != "local-start" || gotFinished != "origin-finish" {
		t.Fatalf("timestamps = %q, %q", gotSubmitted, gotFinished)
	}
}

func TestResolveOriginTimestampsUsesSourceFallback(t *testing.T) {
	origin := &model.JobOrigin{RunID: "run-1", JobID: "job-1"}
	gotSubmitted, gotFinished := resolveOriginTimestamps("", "", origin, func(runID, jobID string) (string, string) {
		if runID != "run-1" || jobID != "job-1" {
			t.Fatalf("source args = %q, %q", runID, jobID)
		}
		return "source-start", "source-finish"
	})
	if gotSubmitted != "source-start" || gotFinished != "source-finish" {
		t.Fatalf("timestamps = %q, %q", gotSubmitted, gotFinished)
	}
}

func TestTimestampsReadsSourceRun(t *testing.T) {
	runsDir := t.TempDir()
	writeFile(t, filepath.Join(runsDir, "run-1", "job-0", "submitted_at"), "source-start\n")
	writeFile(t, filepath.Join(runsDir, "run-1", "job-0", "finished_at"), "source-finish\n")
	writeFile(t, filepath.Join(runsDir, "run-2", "job-1", "submitted_at"), "local-start\n")
	submittedAt, finishedAt := Timestamps(filepath.Join(runsDir, "run-2"), "job-1", &model.JobOrigin{RunID: "run-1", JobID: "job-0"})
	if submittedAt != "local-start" || finishedAt != "source-finish" {
		t.Fatalf("Timestamps() = %q, %q, want local submit and source finish", submittedAt, finishedAt)
	}
}

func TestTimestampsRejectsUnsafeOriginRunID(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run-2")
	for _, runID := range []string{"../outside", `..\outside`, "a/b", `a\b`} {
		if submittedAt, finishedAt := Timestamps(runDir, "job-1", &model.JobOrigin{RunID: runID, JobID: "job-1"}); submittedAt != "" || finishedAt != "" {
			t.Fatalf("unsafe origin %q timestamps = %q, %q, want empty", runID, submittedAt, finishedAt)
		}
	}
}

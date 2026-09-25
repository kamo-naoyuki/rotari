package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestReadJobTimestampUsesLatestAttempt(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "20260923-000000-00000000")
	jobDir := filepath.Join(runDir, "job-1", "attempts", "att_20260923-000000-00000000-job-1-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "submitted_at"), []byte("2026-09-23T00:00:00Z\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := ReadJobTimestamp(runDir, "job-1", "submitted_at"); got != "2026-09-23T00:00:00Z" {
		t.Fatalf("ReadJobTimestamp() = %q, want latest attempt timestamp", got)
	}
	if got := ReadJobTimestamp(runDir, "../outside", "submitted_at"); got != "" {
		t.Fatalf("ReadJobTimestamp() accepted unsafe job ID: %q", got)
	}
	if got := ReadJobTimestamp(runDir, "job-1", "../submitted_at"); got != "" {
		t.Fatalf("ReadJobTimestamp() accepted unsafe state name: %q", got)
	}
}

func TestReadJobTimestampFallsBackToMetadataAndSchedulerStatus(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "20260923-000000-00000000")
	jobDir := filepath.Join(runDir, "job-1", "attempts", "att_20260923-000000-00000000-job-1-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "job.json"), []byte(`{"submitted_at":"2026-09-23T00:00:01Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "status.json"), []byte(`{"phase":"finished","exit_code":0,"finished_at":"2026-09-23T00:00:02Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := ReadJobTimestamp(runDir, "job-1", "submitted_at"); got != "2026-09-23T00:00:01Z" {
		t.Fatalf("ReadJobTimestamp(submitted_at) = %q, want metadata fallback", got)
	}
	if got := ReadJobTimestamp(runDir, "job-1", "finished_at"); got != "2026-09-23T00:00:02Z" {
		t.Fatalf("ReadJobTimestamp(finished_at) = %q, want scheduler status fallback", got)
	}
}

func TestLoadLocalJobResult(t *testing.T) {
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "finished_at"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "status"), []byte("7\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, ok := LoadLocalJobResult(jobDir, model.JobSpec{ID: "job-1", Command: []string{"echo", "failed"}})
	if !ok || result.ID != "job-1" || result.ExitCode != 7 {
		t.Fatalf("LoadLocalJobResult() = %#v, %v", result, ok)
	}
}

func TestListAttemptIDsSortsByAttemptNumber(t *testing.T) {
	runDir := t.TempDir()
	jobID := "job-1"
	attemptIDs := []string{
		MakeAttemptID("20260922-000000-00000000", jobID, 2),
		MakeAttemptID("20260922-000000-00000000", jobID, 0),
		MakeAttemptID("20260922-000000-00000000", jobID, 1),
	}
	for _, attemptID := range attemptIDs {
		if err := os.MkdirAll(filepath.Join(runDir, jobID, "attempts", attemptID), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	got := ListAttemptIDs(runDir, jobID)
	want := []string{attemptIDs[1], attemptIDs[2], attemptIDs[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("attempt IDs = %#v, want %#v", got, want)
	}
	if got := ListAttemptIDs(runDir, "../outside"); got != nil {
		t.Fatalf("ListAttemptIDs() accepted unsafe job ID: %#v", got)
	}
}

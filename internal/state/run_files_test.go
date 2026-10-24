package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/internal/model"
)

// LoadRunSummaryHead reads a summary's run facts without its per-job
// results, and rejects a summary from a newer rotari as LoadRunSummary does.
func TestLoadRunSummaryHead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	summary := model.RunSummary{RunID: "run-1", RunName: "sweep", Status: "failed", StartedAt: "a", FinishedAt: "b", ExitCode: 3,
		Results: []model.JobResult{{ID: "job-1", ExitCode: 3}}}
	if err := WriteJSON(path, summary); err != nil {
		t.Fatal(err)
	}
	head, err := LoadRunSummaryHead(path)
	if err != nil {
		t.Fatal(err)
	}
	if head.RunID != "run-1" || head.RunName != "sweep" || head.Status != "failed" || head.StartedAt != "a" || head.FinishedAt != "b" || head.ExitCode != 3 || head.Results != nil {
		t.Fatalf("head = %+v", head)
	}
	if err := os.WriteFile(path, []byte(`{"state_version":999999,"run_id":"run-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRunSummaryHead(path); err == nil || !strings.Contains(err.Error(), "upgrade") {
		t.Fatalf("newer summary error = %v, want one asking to upgrade", err)
	}
}

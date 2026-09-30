package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestWebhookNotificationManagerBatchesJobsUntilRunFinishes(t *testing.T) {
	paths := testWebhookPaths(t)
	if err := os.WriteFile(filepath.Join(paths.BaseDir, notification.FileName), []byte(`[webhook]
url = "https://example.invalid/hook"
job_success = true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runID := "run-batch"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: "failed", ExitCode: 1}); err != nil {
		t.Fatal(err)
	}

	manager := newWebhookNotificationManager()
	var sent []notification.Batch
	manager.send = func(_ notification.WebhookSettings, batch notification.Batch) {
		sent = append(sent, batch)
	}
	manager.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	manager.JobFinished(paths, runID, "", model.JobSpec{ID: "ok"}, model.JobResult{ID: "ok"})
	manager.JobFinished(paths, runID, "", model.JobSpec{ID: "bad"}, model.JobResult{ID: "bad", ExitCode: 1})
	manager.RunFinished(paths, runID, 1)

	if len(sent) != 1 || len(sent[0].Events) != 3 {
		t.Fatalf("sent batches = %#v", sent)
	}
}

func TestWebhookNotificationManagerAppliesJobSuccessSetting(t *testing.T) {
	paths := testWebhookPaths(t)
	if err := os.WriteFile(filepath.Join(paths.BaseDir, notification.FileName), []byte(`[webhook]
url = "https://example.invalid/hook"
job_success = false
`), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := newWebhookNotificationManager()
	var sent []notification.Batch
	manager.send = func(_ notification.WebhookSettings, batch notification.Batch) { sent = append(sent, batch) }
	manager.JobFinished(paths, "run-filter", "", model.JobSpec{ID: "ok"}, model.JobResult{ID: "ok"})

	manager.mu.Lock()
	for key := range manager.pending {
		manager.mu.Unlock()
		manager.flush(key)
		manager.mu.Lock()
	}
	manager.mu.Unlock()
	if len(sent) != 0 {
		t.Fatalf("sent batches = %#v", sent)
	}
}

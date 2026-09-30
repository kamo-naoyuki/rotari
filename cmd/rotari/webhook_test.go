package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSlackWebhookEncoder(t *testing.T) {
	encoded, err := (slackWebhookEncoder{}).Encode(runWebhookPayload{
		Project: "demo", Run: "nightly", Status: "failed", Success: 2, Failed: 1,
		FailedJobs: []string{"train"}, ShowCommand: "rotari show --failed-logs",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := encoded.(slackWebhookPayload)
	if !ok || payload.Text != "rotari demo: failed (1 failed)" || len(payload.Blocks) != 1 || payload.Blocks[0].Text.Type != "mrkdwn" {
		t.Fatalf("slack payload = %#v", encoded)
	}
	if !strings.Contains(payload.Blocks[0].Text.Text, "failed jobs: `train`") {
		t.Fatalf("slack payload text = %q", payload.Blocks[0].Text.Text)
	}
}

func TestTeamsWebhookEncoder(t *testing.T) {
	encoded, err := (teamsWebhookEncoder{}).Encode(runWebhookPayload{
		Project: "demo", Run: "nightly", Status: "failed", Success: 2, Failed: 1,
		FailedJobs: []string{"train"}, ShowCommand: "rotari show --failed-logs",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := encoded.(teamsWebhookPayload)
	if !ok || payload.Type != "MessageCard" || payload.Context != "http://schema.org/extensions" || payload.ThemeColor != "C62828" || len(payload.Sections) != 1 {
		t.Fatalf("Teams payload = %#v", encoded)
	}
	if payload.Sections[0].ActivityText != "Run completed with failed jobs." || len(payload.Sections[0].Facts) != 6 {
		t.Fatalf("Teams section = %#v", payload.Sections[0])
	}
}

func TestDiscordWebhookEncoder(t *testing.T) {
	encoded, err := (discordWebhookEncoder{}).Encode(runWebhookPayload{
		Project: "demo", Run: "nightly", Status: "failed", Success: 2, Failed: 1,
		FailedJobs: []string{"train"}, ShowCommand: "rotari show --failed-logs",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := encoded.(discordWebhookPayload)
	if !ok || payload.Content != "rotari demo: failed" || len(payload.Embeds) != 1 || payload.Embeds[0].Color != 0xc62828 {
		t.Fatalf("Discord payload = %#v", encoded)
	}
	if len(payload.Embeds[0].Fields) != 6 || payload.Embeds[0].Fields[4].Value != "train" || !strings.Contains(payload.Embeds[0].Fields[5].Value, "rotari show --failed-logs") {
		t.Fatalf("Discord fields = %#v", payload.Embeds[0].Fields)
	}
}

// testWebhookBatch builds a batch with one success, one failure, and the run's
// own completion.
func testWebhookBatch(t *testing.T, status string) notification.Batch {
	t.Helper()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	settings := notification.Defaults().Webhook.ChannelSettings
	settings.JobSuccess = true
	return notification.NewBatch([]notification.Event{
		notification.NewJobEvent("demo", "run-1", "nightly", model.JobSpec{ID: "ok"}, model.JobResult{ID: "ok"}, now),
		notification.NewJobEvent("demo", "run-1", "nightly", model.JobSpec{ID: "bad", Name: "train"}, model.JobResult{ID: "bad", ExitCode: 1}, now),
		notification.NewRunEvent("demo", model.RunSummary{RunID: "run-1", RunName: "nightly", Status: status}, now),
	}, settings, now)
}

func TestBatchWebhookPayloadSummarizesJobsAndRunStatus(t *testing.T) {
	payload := batchWebhookPayload(testWebhookBatch(t, "failed"))
	if payload.Event != string(notification.RunFinished) || payload.Status != "failed" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Success != 1 || payload.Failed != 1 || len(payload.FailedJobs) != 1 || payload.FailedJobs[0] != "train (bad)" {
		t.Fatalf("payload counts = %#v", payload)
	}
}

func TestSendWebhookBatchPostsEncodedPayload(t *testing.T) {
	var received slackWebhookPayload
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode Slack webhook: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	settings := notification.Defaults().Webhook
	settings.URL = server.URL
	settings.Format = "slack"
	sendWebhookBatch(settings, testWebhookBatch(t, "failed"))

	if requests != 1 || received.Text != "rotari demo: failed (1 failed)" {
		t.Fatalf("requests = %d, payload = %#v", requests, received)
	}
}

func TestSendWebhookBatchRejectsUnsupportedFormat(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	settings := notification.Defaults().Webhook
	settings.URL = server.URL
	settings.Format = "unknown"
	sendWebhookBatch(settings, testWebhookBatch(t, "success"))

	if requests != 0 {
		t.Fatalf("unsupported format requests = %d, want 0", requests)
	}
}

func testWebhookPaths(t *testing.T) state.ProjectPaths {
	t.Helper()
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

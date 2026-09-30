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
	input := notification.Payload{Events: []notification.PayloadEvent{{Event: notification.JobFinished, Fields: []notification.Field{
		{Name: "project", Value: "demo"}, {Name: "job_name", Value: "train"}, {Name: "job_status", Value: "failed"}, {Name: "diagnosis_suggestion", Value: "inspect logs"},
	}}}}
	encoded, err := (slackWebhookEncoder{}).Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := encoded.(slackWebhookPayload)
	if !ok || payload.Text != "rotari job finished: failed" || len(payload.Blocks) != 1 || payload.Blocks[0].Text.Type != "mrkdwn" {
		t.Fatalf("slack payload = %#v", encoded)
	}
	if !strings.Contains(payload.Blocks[0].Text.Text, "*diagnosis suggestion:* inspect logs") {
		t.Fatalf("slack payload text = %q", payload.Blocks[0].Text.Text)
	}
}

func TestTeamsWebhookEncoder(t *testing.T) {
	input := notification.Payload{Events: []notification.PayloadEvent{{Event: notification.RunFinished, Fields: []notification.Field{
		{Name: "project", Value: "demo"}, {Name: "run_status", Value: "failed"}, {Name: "failure_count", Value: 1},
	}}}}
	encoded, err := (teamsWebhookEncoder{}).Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := encoded.(teamsWebhookPayload)
	if !ok || payload.Type != "MessageCard" || payload.Context != "http://schema.org/extensions" || payload.ThemeColor != "C62828" || len(payload.Sections) != 1 {
		t.Fatalf("Teams payload = %#v", encoded)
	}
	if payload.Sections[0].ActivityTitle != "rotari run finished: failed" || len(payload.Sections[0].Facts) != 3 {
		t.Fatalf("Teams section = %#v", payload.Sections[0])
	}
}

func TestDiscordWebhookEncoder(t *testing.T) {
	input := notification.Payload{Events: []notification.PayloadEvent{{Event: notification.RunFinished, Fields: []notification.Field{
		{Name: "project", Value: "demo"}, {Name: "run_status", Value: "failed"}, {Name: "failure_count", Value: 1},
	}}}}
	encoded, err := (discordWebhookEncoder{}).Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := encoded.(discordWebhookPayload)
	if !ok || payload.Content != "rotari run finished: failed" || len(payload.Embeds) != 1 || payload.Embeds[0].Color != 0xc62828 {
		t.Fatalf("Discord payload = %#v", encoded)
	}
	if len(payload.Embeds[0].Fields) != 3 || payload.Embeds[0].Fields[2].Value != "1" {
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
		notification.NewJobEvent("demo", "run-1", "nightly", model.JobSpec{ID: "bad", Name: "train"}, model.JobResult{ID: "bad", ExitCode: 1, Diagnoses: []model.RuleDiagnosis{{Name: "First", Suggestion: "check logs"}, {Name: "Second", Suggestion: "check quota"}}}, now),
		notification.NewRunEvent("demo", model.RunSummary{RunID: "run-1", RunName: "nightly", Status: status, ExitCode: 1, Results: []model.JobResult{{ID: "ok"}, {ID: "bad", ExitCode: 1}}}, now),
	}, settings, now)
}

func TestWebhookPayloadHonorsSelectedFields(t *testing.T) {
	payload := notification.NewPayload(testWebhookBatch(t, "failed"), []string{"job_name", "job_status", "run_status", "failure_count"})
	if len(payload.Events) != 3 {
		t.Fatalf("payload = %#v", payload)
	}
	jobFields := payload.Events[1].Fields
	if len(jobFields) != 2 || jobFields[0].Name != "job_name" || jobFields[0].Value != "train" || jobFields[1].Value != "failed" {
		t.Fatalf("job fields = %#v", jobFields)
	}
	runFields := payload.Events[2].Fields
	if len(runFields) != 2 || runFields[0].Name != "run_status" || runFields[1].Name != "failure_count" {
		t.Fatalf("run fields = %#v", runFields)
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

	if requests != 1 || received.Text != "rotari run finished: failed" || len(received.Blocks) != 3 {
		t.Fatalf("requests = %d, payload = %#v", requests, received)
	}
}

func TestSendWebhookBatchHonorsFieldsForGenericJSON(t *testing.T) {
	var received notification.Payload
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode generic webhook: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	settings := notification.Defaults().Webhook
	settings.URL = server.URL
	settings.Fields = []string{"job_name", "diagnosis_name", "diagnosis_suggestion", "run_status", "failure_count"}
	sendWebhookBatch(settings, testWebhookBatch(t, "failed"))

	if len(received.Events) != 3 || len(received.Events[1].Fields) != 5 || len(received.Events[2].Fields) != 2 {
		t.Fatalf("generic payload = %#v", received)
	}
	if received.Events[1].Fields[1].Value != "First" || received.Events[1].Fields[2].Value != "Second" {
		t.Fatalf("diagnosis fields = %#v", received.Events[1].Fields)
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

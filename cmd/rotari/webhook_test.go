package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotifyRunWebhookSendsSummaryAndMarksRun(t *testing.T) {
	var received runWebhookPayload
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode webhook: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv(envWebhookURL, server.URL)
	t.Setenv(envWebhookOn, "failure")
	t.Setenv(envWebhookFormat, "")

	paths := testWebhookPaths(t)
	runID := "run-1"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{
		RunID: runID, RunName: "nightly", Status: "failed", ExitCode: 1,
		Results: []JobResult{{ID: "ok", ExitCode: 0}, {ID: "bad", ExitCode: 1, Error: "command exited with status 2"}},
	}); err != nil {
		t.Fatal(err)
	}

	notifyRunWebhook(paths, runID, 1)
	notifyRunWebhook(paths, runID, 1)
	if requests != 1 {
		t.Fatalf("webhook requests = %d, want 1", requests)
	}
	if received.Event != "run.finished" || received.Project != "demo" || received.Run != "nightly (run-1)" || received.Success != 1 || received.Failed != 1 || len(received.FailedJobs) != 1 || received.FailedJobs[0] != "bad" || received.ShowCommand != "rotari show --run-id 'run-1' --failed-logs --no-pager" {
		t.Fatalf("webhook payload = %#v", received)
	}
	if _, err := os.Stat(filepath.Join(runDir, "webhook.sent")); err != nil {
		t.Fatalf("webhook marker: %v", err)
	}
}

func TestWebhookShouldSend(t *testing.T) {
	tests := []struct {
		exitCode int
		setting  string
		want     bool
	}{
		{0, "", true}, {1, "always", true}, {0, "success", true}, {1, "success", false}, {1, "failure", true}, {0, "failure", false},
	}
	for _, test := range tests {
		if got := webhookShouldSend(test.exitCode, test.setting); got != test.want {
			t.Errorf("webhookShouldSend(%d, %q) = %v, want %v", test.exitCode, test.setting, got, test.want)
		}
	}
}

func TestWebhookSettingsUseProjectConfigAndEnvironmentOverrides(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "projects", "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte("webhook:\n  url: https://base.example/hook\n  on: success\n  format: slack\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), []byte("webhook:\n  url: https://project.example/hook\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envWebhookURL, "")
	t.Setenv(envWebhookOn, "")
	config := webhookSettings(paths)
	if config.URL != "https://project.example/hook" || config.On != "success" || config.Format != "slack" {
		t.Fatalf("config webhook settings = %#v", config)
	}
	t.Setenv(envWebhookURL, "https://env.example/hook")
	t.Setenv(envWebhookOn, "failure")
	t.Setenv(envWebhookFormat, "slack")
	config = webhookSettings(paths)
	if config.URL != "https://env.example/hook" || config.On != "failure" || config.Format != "slack" {
		t.Fatalf("environment webhook settings = %#v", config)
	}
}

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

func TestNotifyRunWebhookSendsSlackPayload(t *testing.T) {
	var received slackWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode Slack webhook: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv(envWebhookURL, server.URL)
	t.Setenv(envWebhookFormat, "slack")

	paths := testWebhookPaths(t)
	runID := "run-slack"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{
		RunID: runID, Status: "success", ExitCode: 0,
		Results: []JobResult{{ID: "build", ExitCode: 0}},
	}); err != nil {
		t.Fatal(err)
	}

	notifyRunWebhook(paths, runID, 0)
	if received.Text != "rotari demo: success (0 failed)" || len(received.Blocks) != 1 {
		t.Fatalf("Slack webhook payload = %#v", received)
	}
}

func TestNotifyRunWebhookSendsSlackFailureDetails(t *testing.T) {
	var received slackWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode Slack webhook: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv(envWebhookURL, server.URL)
	t.Setenv(envWebhookFormat, "slack")

	paths := testWebhookPaths(t)
	runID := "run-slack-failed"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{
		RunID: runID, Status: "failed", ExitCode: 1,
		Results: []JobResult{{ID: "train", ExitCode: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	notifyRunWebhook(paths, runID, 1)
	if received.Text != "rotari demo: failed (1 failed)" || len(received.Blocks) != 1 {
		t.Fatalf("Slack webhook payload = %#v", received)
	}
	blockText := received.Blocks[0].Text.Text
	if !strings.Contains(blockText, "failed jobs: `train`") || !strings.Contains(blockText, "show: `rotari show --run-id 'run-slack-failed' --failed-logs --no-pager`") {
		t.Fatalf("Slack failure details = %q", blockText)
	}
}

func TestNotifyRunWebhookSendsTeamsPayload(t *testing.T) {
	var received teamsWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode Teams webhook: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv(envWebhookURL, server.URL)
	t.Setenv(envWebhookFormat, "teams")

	paths := testWebhookPaths(t)
	runID := "run-teams"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{
		RunID: runID, Status: "success", ExitCode: 0,
		Results: []JobResult{{ID: "build", ExitCode: 0}},
	}); err != nil {
		t.Fatal(err)
	}

	notifyRunWebhook(paths, runID, 0)
	if received.Type != "MessageCard" || received.Summary != "rotari demo: success" || len(received.Sections) != 1 {
		t.Fatalf("Teams webhook payload = %#v", received)
	}
}

func TestNotifyRunWebhookSendsDiscordPayload(t *testing.T) {
	var received discordWebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode Discord webhook: %v", err)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv(envWebhookURL, server.URL)
	t.Setenv(envWebhookFormat, "discord")

	paths := testWebhookPaths(t)
	runID := "run-discord"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{
		RunID: runID, Status: "success", ExitCode: 0,
		Results: []JobResult{{ID: "build", ExitCode: 0}},
	}); err != nil {
		t.Fatal(err)
	}

	notifyRunWebhook(paths, runID, 0)
	if received.Content != "rotari demo: success" || len(received.Embeds) != 1 || received.Embeds[0].Color != 0x2e7d32 {
		t.Fatalf("Discord webhook payload = %#v", received)
	}
}

func TestNotifyRunWebhookRejectsUnsupportedFormat(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	t.Setenv(envWebhookURL, server.URL)
	t.Setenv(envWebhookFormat, "unknown")

	paths := testWebhookPaths(t)
	runID := "run-unknown-format"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), RunSummary{RunID: runID, Status: "success"}); err != nil {
		t.Fatal(err)
	}

	notifyRunWebhook(paths, runID, 0)
	if requests != 0 {
		t.Fatalf("unsupported format requests = %d, want 0", requests)
	}
	if _, err := os.Stat(filepath.Join(runDir, "webhook.sent")); !os.IsNotExist(err) {
		t.Fatalf("unsupported format marker error = %v", err)
	}
}

func testWebhookPaths(t *testing.T) pathSet {
	t.Helper()
	baseDir := t.TempDir()
	paths, err := resolvePaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

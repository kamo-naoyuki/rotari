package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type runWebhookPayload struct {
	Event       string   `json:"event"`
	Project     string   `json:"project"`
	Run         string   `json:"run"`
	Status      string   `json:"status"`
	ExitCode    int      `json:"exit_code"`
	Success     int      `json:"success"`
	Failed      int      `json:"failed"`
	FailedJobs  []string `json:"failed_jobs,omitempty"`
	ShowCommand string   `json:"show_command,omitempty"`
}

type webhookConfig struct {
	URL    string
	On     string
	Format string
}

type webhookEncoder interface {
	Encode(runWebhookPayload) (any, error)
}

type genericWebhookEncoder struct{}

func (genericWebhookEncoder) Encode(payload runWebhookPayload) (any, error) {
	return payload, nil
}

type slackWebhookEncoder struct{}

type slackWebhookPayload struct {
	Text   string       `json:"text"`
	Blocks []slackBlock `json:"blocks,omitempty"`
}

type slackBlock struct {
	Type string    `json:"type"`
	Text slackText `json:"text,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type teamsWebhookEncoder struct{}

type teamsWebhookPayload struct {
	Type       string         `json:"@type"`
	Context    string         `json:"@context"`
	Summary    string         `json:"summary"`
	ThemeColor string         `json:"themeColor"`
	Sections   []teamsSection `json:"sections"`
}

type teamsSection struct {
	ActivityTitle string      `json:"activityTitle"`
	ActivityText  string      `json:"activityText"`
	Facts         []teamsFact `json:"facts"`
}

type teamsFact struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type discordWebhookEncoder struct{}

type discordWebhookPayload struct {
	Content string         `json:"content"`
	Embeds  []discordEmbed `json:"embeds,omitempty"`
}

type discordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []discordField `json:"fields"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

func (slackWebhookEncoder) Encode(payload runWebhookPayload) (any, error) {
	text := fmt.Sprintf("rotari %s: %s (%d failed)", payload.Project, payload.Status, payload.Failed)
	blockText := fmt.Sprintf("*rotari run %s*\nproject: `%s`\nrun: `%s`\nsuccess: %d, failed: %d", payload.Status, payload.Project, payload.Run, payload.Success, payload.Failed)
	if len(payload.FailedJobs) > 0 {
		blockText += fmt.Sprintf("\nfailed jobs: `%s`", strings.Join(payload.FailedJobs, "`, `"))
	}
	if payload.ShowCommand != "" {
		blockText += fmt.Sprintf("\nshow: `%s`", payload.ShowCommand)
	}
	return slackWebhookPayload{
		Text: text,
		Blocks: []slackBlock{{
			Type: "section",
			Text: slackText{Type: "mrkdwn", Text: blockText},
		}},
	}, nil
}

func (teamsWebhookEncoder) Encode(payload runWebhookPayload) (any, error) {
	activityText := "Run completed successfully."
	themeColor := "2E7D32"
	if payload.Failed > 0 {
		activityText = "Run completed with failed jobs."
		themeColor = "C62828"
	}
	facts := []teamsFact{
		{Name: "Project", Value: payload.Project},
		{Name: "Run", Value: payload.Run},
		{Name: "Success", Value: fmt.Sprintf("%d", payload.Success)},
		{Name: "Failed", Value: fmt.Sprintf("%d", payload.Failed)},
	}
	if len(payload.FailedJobs) > 0 {
		facts = append(facts, teamsFact{Name: "Failed jobs", Value: strings.Join(payload.FailedJobs, ", ")})
	}
	if payload.ShowCommand != "" {
		facts = append(facts, teamsFact{Name: "Show command", Value: payload.ShowCommand})
	}
	return teamsWebhookPayload{
		Type: "MessageCard", Context: "http://schema.org/extensions",
		Summary: fmt.Sprintf("rotari %s: %s", payload.Project, payload.Status), ThemeColor: themeColor,
		Sections: []teamsSection{{
			ActivityTitle: fmt.Sprintf("rotari run %s", payload.Status),
			ActivityText:  activityText,
			Facts:         facts,
		}},
	}, nil
}

func (discordWebhookEncoder) Encode(payload runWebhookPayload) (any, error) {
	color := 0x2e7d32
	if payload.Failed > 0 {
		color = 0xc62828
	}
	fields := []discordField{
		{Name: "Project", Value: payload.Project, Inline: true},
		{Name: "Run", Value: payload.Run, Inline: true},
		{Name: "Success", Value: fmt.Sprintf("%d", payload.Success), Inline: true},
		{Name: "Failed", Value: fmt.Sprintf("%d", payload.Failed), Inline: true},
	}
	if len(payload.FailedJobs) > 0 {
		fields = append(fields, discordField{Name: "Failed jobs", Value: strings.Join(payload.FailedJobs, ", ")})
	}
	if payload.ShowCommand != "" {
		fields = append(fields, discordField{Name: "Show command", Value: fmt.Sprintf("`%s`", payload.ShowCommand)})
	}
	return discordWebhookPayload{
		Content: fmt.Sprintf("rotari %s: %s", payload.Project, payload.Status),
		Embeds: []discordEmbed{{
			Title:       fmt.Sprintf("rotari run %s", payload.Status),
			Description: "Run completed.", Color: color, Fields: fields,
		}},
	}, nil
}

var webhookEncoders = map[string]webhookEncoder{
	"json":    genericWebhookEncoder{},
	"discord": discordWebhookEncoder{},
	"slack":   slackWebhookEncoder{},
	"teams":   teamsWebhookEncoder{},
}

func notifyRunWebhook(paths pathSet, runID string, exitCode int) {
	config := webhookSettings(paths)
	if config.URL == "" || !webhookShouldSend(exitCode, config.On) {
		return
	}
	parsed, err := url.Parse(config.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		printErrorf("WARNING: invalid %s URL", envWebhookURL)
		return
	}
	runDir, err := validatedRunDir(paths, runID)
	if err != nil {
		printErrorf("WARNING: cannot prepare webhook notification: %v", err)
		return
	}
	marker := filepath.Join(runDir, "webhook.sent")
	if _, err := os.Stat(marker); err == nil {
		return
	} else if !os.IsNotExist(err) {
		printErrorf("WARNING: cannot inspect webhook notification marker: %v", err)
		return
	}
	summary, err := loadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil {
		printErrorf("WARNING: cannot load run summary for webhook: %v", err)
		return
	}
	payload := makeRunWebhookPayload(paths.queueName, runID, summary)
	encoder, ok := webhookEncoders[normalizeWebhookFormat(config.Format)]
	if !ok {
		printErrorf("WARNING: unsupported webhook format %q", config.Format)
		return
	}
	encodedPayload, err := encoder.Encode(payload)
	if err != nil {
		printErrorf("WARNING: cannot prepare webhook notification: %v", err)
		return
	}
	data, err := json.Marshal(encodedPayload)
	if err != nil {
		printErrorf("WARNING: cannot encode webhook notification: %v", err)
		return
	}
	request, err := http.NewRequest(http.MethodPost, config.URL, bytes.NewReader(data))
	if err != nil {
		printErrorf("WARNING: cannot create webhook notification: %v", err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		printErrorf("WARNING: webhook notification failed: %v", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		printErrorf("WARNING: webhook notification returned HTTP %d", response.StatusCode)
		return
	}
	if err := os.WriteFile(marker, []byte(nowRFC3339()+"\n"), stateFileMode()); err != nil {
		printErrorf("WARNING: cannot record webhook notification: %v", err)
	}
}

func webhookSettings(paths pathSet) webhookConfig {
	settings := map[string]any{}
	for _, configPath := range configPathsForRun(paths.baseDir, paths.queueName) {
		config, err := loadConfigFile(filepath.Dir(configPath))
		if err != nil {
			continue
		}
		mergeConfig(settings, config)
	}
	webhook, _ := settings["webhook"].(map[string]any)
	config := webhookConfig{Format: "json"}
	config.URL, _ = webhook["url"].(string)
	config.On, _ = webhook["on"].(string)
	config.Format, _ = webhook["format"].(string)
	if value := strings.TrimSpace(os.Getenv(envWebhookURL)); value != "" {
		config.URL = value
	}
	if value := strings.TrimSpace(os.Getenv(envWebhookOn)); value != "" {
		config.On = value
	}
	if value := strings.TrimSpace(os.Getenv(envWebhookFormat)); value != "" {
		config.Format = value
	}
	config.URL = strings.TrimSpace(config.URL)
	config.On = strings.TrimSpace(config.On)
	config.Format = normalizeWebhookFormat(config.Format)
	return config
}

func normalizeWebhookFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "json"
	}
	return format
}

func webhookShouldSend(exitCode int, setting string) bool {
	setting = strings.TrimSpace(strings.ToLower(setting))
	if setting == "" || setting == "always" || setting == "all" {
		return true
	}
	want := "success"
	if exitCode != 0 {
		want = "failure"
	}
	for _, value := range strings.Split(setting, ",") {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func makeRunWebhookPayload(project, runID string, summary RunSummary) runWebhookPayload {
	payload := runWebhookPayload{
		Event: "run.finished", Project: project, Run: formatRunLabel(runID, summary.RunName),
		Status: summary.Status, ExitCode: summary.ExitCode,
	}
	for _, result := range summary.Results {
		if result.ExitCode == 0 {
			payload.Success++
		} else {
			payload.Failed++
			payload.FailedJobs = append(payload.FailedJobs, result.ID)
		}
	}
	if payload.Failed > 0 {
		payload.ShowCommand = fmt.Sprintf("rotari show --run-id %s --failed-logs --no-pager", shellQuote(runID))
	}
	return payload
}

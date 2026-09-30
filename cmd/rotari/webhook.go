package main

import (
	"fmt"
	"strings"
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

func normalizeWebhookFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "json"
	}
	return format
}

package main

import (
	"fmt"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/notification"
)

type webhookEncoder interface {
	Encode(notification.Payload) (any, error)
}

type genericWebhookEncoder struct{}

func (genericWebhookEncoder) Encode(payload notification.Payload) (any, error) {
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

func (slackWebhookEncoder) Encode(payload notification.Payload) (any, error) {
	encoded := slackWebhookPayload{Text: payloadSummary(payload)}
	for _, event := range payload.Events {
		encoded.Blocks = append(encoded.Blocks, slackBlock{Type: "section", Text: slackText{Type: "mrkdwn", Text: markdownEvent(event)}})
	}
	if payload.OmittedJobs > 0 {
		encoded.Blocks = append(encoded.Blocks, slackBlock{Type: "section", Text: slackText{Type: "mrkdwn", Text: fmt.Sprintf("*omitted jobs:* %d", payload.OmittedJobs)}})
	}
	return encoded, nil
}

func (teamsWebhookEncoder) Encode(payload notification.Payload) (any, error) {
	encoded := teamsWebhookPayload{
		Type: "MessageCard", Context: "http://schema.org/extensions",
		Summary: payloadSummary(payload), ThemeColor: payloadColor(payload, "2E7D32", "C62828"),
	}
	for _, event := range payload.Events {
		facts := make([]teamsFact, 0, len(event.Fields))
		for _, field := range event.Fields {
			facts = append(facts, teamsFact{Name: fieldLabel(field.Name), Value: notification.FormatValue(field.Value)})
		}
		encoded.Sections = append(encoded.Sections, teamsSection{ActivityTitle: eventTitle(event), Facts: facts})
	}
	if payload.OmittedJobs > 0 {
		encoded.Sections = append(encoded.Sections, teamsSection{ActivityTitle: "rotari notification", Facts: []teamsFact{{Name: "Omitted jobs", Value: fmt.Sprint(payload.OmittedJobs)}}})
	}
	return encoded, nil
}

func (discordWebhookEncoder) Encode(payload notification.Payload) (any, error) {
	encoded := discordWebhookPayload{Content: payloadSummary(payload)}
	color := payloadColor(payload, 0x2e7d32, 0xc62828)
	for _, event := range payload.Events {
		fields := make([]discordField, 0, len(event.Fields))
		for _, field := range event.Fields {
			fields = append(fields, discordField{Name: fieldLabel(field.Name), Value: notification.FormatValue(field.Value), Inline: true})
		}
		encoded.Embeds = append(encoded.Embeds, discordEmbed{Title: eventTitle(event), Color: color, Fields: fields})
	}
	if payload.OmittedJobs > 0 {
		encoded.Embeds = append(encoded.Embeds, discordEmbed{Title: "rotari notification", Color: color, Fields: []discordField{{Name: "Omitted jobs", Value: fmt.Sprint(payload.OmittedJobs)}}})
	}
	return encoded, nil
}

func payloadSummary(payload notification.Payload) string {
	if len(payload.Events) == 0 {
		return "rotari notification"
	}
	return eventTitle(payload.Events[len(payload.Events)-1])
}

func eventTitle(event notification.PayloadEvent) string {
	title := "rotari " + strings.ReplaceAll(string(event.Event), ".", " ")
	statusName := "job_status"
	if event.Event == notification.RunFinished {
		statusName = "run_status"
	}
	if status, ok := fieldValue(event.Fields, statusName); ok {
		title += ": " + notification.FormatValue(status)
	}
	return title
}

func markdownEvent(event notification.PayloadEvent) string {
	lines := []string{"*" + eventTitle(event) + "*"}
	for _, field := range event.Fields {
		lines = append(lines, fmt.Sprintf("*%s:* %s", fieldLabel(field.Name), notification.FormatValue(field.Value)))
	}
	return strings.Join(lines, "\n")
}

func fieldLabel(name string) string {
	return strings.ReplaceAll(name, "_", " ")
}

func fieldValue(fields []notification.Field, name string) (any, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return nil, false
}

func payloadColor[T ~string | ~int](payload notification.Payload, success, failure T) T {
	for _, event := range payload.Events {
		for _, name := range []string{"run_status", "job_status"} {
			if status, ok := fieldValue(event.Fields, name); ok && notification.FormatValue(status) != "success" {
				return failure
			}
		}
	}
	return success
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

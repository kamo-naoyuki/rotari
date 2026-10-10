package webui

import (
	"encoding/json"
	"html"
	"strings"
)

// statusTones maps the base word of a displayed status to its tone, which
// names the status colour and icon (.status-pill.tone-* in
// web_sidebar_styles.css, colours in web_tokens.css). It is the Web UI's only
// status-to-colour rule: the jobs page is rendered with it in Go, and the page
// script receives it as statusTones and applies the same normalisation in
// statusTone (web_app_core.js); TestStatusToneMatchesInGoAndJS keeps the two
// in step.
var statusTones = map[string]string{
	"success":     "ok",
	"succeeded":   "ok",
	"finished":    "ok",
	"failed":      "bad",
	"unreadable":  "bad",
	"running":     "run",
	"in progress": "run",
	"blocked":     "warn",
	"interrupted": "warn",
	"pending":     "off",
	"unfinished":  "off",
	"waiting":     "off",
	"suspended":   "off",
	"incomplete":  "off",
	"cancelled":   "off",
	"unknown":     "off",
}

// statusBase reduces a displayed status such as "success (carried)",
// "running (recorded)", or "running ..." to the word statusTones is keyed by.
func statusBase(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	status = strings.TrimSpace(strings.TrimSuffix(status, "..."))
	if open := strings.Index(status, " ("); open >= 0 && strings.HasSuffix(status, ")") {
		status = status[:open]
	}
	return status
}

func statusTone(status string) string {
	if tone, ok := statusTones[statusBase(status)]; ok {
		return tone
	}
	return "off"
}

// statusPillClass is the class list of a status pill. A carried result keeps
// its tone and is drawn with a dashed outline.
func statusPillClass(status string) string {
	class := "status-pill tone-" + statusTone(status)
	if strings.Contains(strings.ToLower(status), "(carried)") {
		class += " carried"
	}
	return class
}

func statusPillHTML(status string) string {
	return `<span class="` + statusPillClass(status) + `">` + html.EscapeString(status) + `</span>`
}

func statusTonesJSON() string {
	data, _ := json.Marshal(statusTones)
	return string(data)
}

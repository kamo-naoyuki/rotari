package webui

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/url"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
)

//go:embed assets/web_template.html
var webTemplateHTML string

//go:embed assets/web_app_core.js
var webAppCoreJS string

//go:embed assets/web_app_actions.js
var webAppActionsJS string

//go:embed assets/web_app_logs.js
var webAppLogsJS string

//go:embed assets/web_app_tables.js
var webAppTablesJS string

//go:embed assets/web_app_charts.js
var webAppChartsJS string

//go:embed assets/web_app_matrix.js
var webAppMatrixJS string

//go:embed assets/web_app_notifications.js
var webAppNotificationsJS string

//go:embed assets/web_app_bootstrap.js
var webAppBootstrapJS string

//go:embed assets/web_static_bootstrap.js
var webStaticBootstrapJS string

//go:embed assets/cli_docs_template.html
var cliDocsTemplateHTML string

//go:embed assets/environment_template.html
var environmentTemplateHTML string

//go:embed assets/jobs_template.html
var jobsTemplateHTML string

//go:embed assets/web_info_styles.css
var webInfoStylesCSS string

//go:embed assets/web_styles.css
var webStylesCSS string

func composeWebHTML(executors []string, notifications bool, bootstrap string) string {
	executorJSON, _ := json.Marshal(executors)
	webAppJS := strings.Join([]string{webAppCoreJS, webAppActionsJS, webAppLogsJS, webAppTablesJS, webAppChartsJS, webAppMatrixJS, webAppNotificationsJS, webAppBootstrapJS}, "\n")
	template := strings.Replace(webTemplateHTML, "__ROTARI_WEB_APP__", webAppJS, 1)
	template = strings.Replace(template, "__ROTARI_EXECUTORS__", string(executorJSON), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_ICON__", faviconDataURL(webFaviconDarkSVG), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_DEFAULT__", strconv.FormatBool(notifications), 1)
	template = strings.Replace(template, "__ROTARI_STATIC_BOOTSTRAP__", bootstrap, 1)
	return template
}

func composeStaticBootstrap(state, logs, reports, configTargets, configs string) string {
	bootstrap := webStaticBootstrapJS
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_STATE_DATA__", state, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_LOGS_DATA__", logs, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_REPORTS_DATA__", reports, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_CONFIG_TARGETS_DATA__", configTargets, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_CONFIGS_DATA__", configs, 1)
	return bootstrap
}

func composeInfoHTML(template, homePath, content string) string {
	template = strings.Replace(template, "__ROTARI_FAVICON_LINKS__", faviconLinks(), 1)
	template = strings.Replace(template, "__ROTARI_INFO_STYLES__", webInfoStylesCSS, 1)
	template = strings.Replace(template, "__ROTARI_BRAND_ICON__", brandIcon(), 1)
	template = strings.Replace(template, "__ROTARI_HOME_PATH__", html.EscapeString(homePath), 1)
	template = strings.Replace(template, "__ROTARI_CONTENT__", content, 1)
	return template
}

func jobsHTML(homePath string, rows []joblist.Row, since string, canFilter bool) string {
	var builder strings.Builder
	if canFilter {
		builder.WriteString(`<form class="jobs-filter" method="get"><label for="jobs-since">Since</label><input id="jobs-since" name="since" value="`)
		builder.WriteString(html.EscapeString(since))
		builder.WriteString(`" placeholder="24h" inputmode="text"><button type="submit">Apply</button></form>`)
	}
	if len(rows) == 0 {
		builder.WriteString(`<p class="meta">No running or recently finished jobs found.</p>`)
		return composeInfoHTML(jobsTemplateHTML, homePath, builder.String())
	}
	builder.WriteString(`<section><table><thead><tr><th>State</th><th>Project</th><th>Job</th><th>Command</th><th>Attempt</th><th>Started</th><th>Finished</th><th>Elapsed</th></tr></thead><tbody>`)
	for _, row := range rows {
		builder.WriteString(`<tr><td class="jobs-state jobs-state-`)
		builder.WriteString(jobsStateClass(row.State))
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(row.State))
		builder.WriteString(`</td><td><a href="`)
		builder.WriteString(html.EscapeString(homePath))
		builder.WriteString(`project/`)
		builder.WriteString(url.PathEscape(row.Project))
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(row.Project))
		builder.WriteString(`</a>`)
		builder.WriteString(`</td><td><a href="`)
		builder.WriteString(html.EscapeString(homePath))
		builder.WriteString(`project/`)
		builder.WriteString(url.PathEscape(row.Project))
		builder.WriteString(`/run/`)
		builder.WriteString(url.PathEscape(row.RunID))
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(row.JobName))
		builder.WriteString(`</a></td><td><code>`)
		builder.WriteString(html.EscapeString(row.Command))
		builder.WriteString(`</code>`)
		writeJobsCopyButton(&builder, row.FullCommand, "command")
		builder.WriteString(`</td><td><code>`)
		builder.WriteString(html.EscapeString(row.AttemptID))
		builder.WriteString(`</code>`)
		writeJobsCopyButton(&builder, row.AttemptID, "attempt ID")
		builder.WriteString(`</td><td>`)
		builder.WriteString(html.EscapeString(joblist.FormatTimestamp(row.StartedAt)))
		builder.WriteString(`</td><td>`)
		if row.State == "running" {
			builder.WriteString(`-`)
		} else {
			builder.WriteString(html.EscapeString(joblist.FormatTimestamp(row.FinishedAt)))
		}
		builder.WriteString(`</td><td>`)
		builder.WriteString(html.EscapeString(joblist.FormatElapsed(row.Elapsed)))
		builder.WriteString(`</td></tr>`)
	}
	builder.WriteString(`</tbody></table></section>`)
	return composeInfoHTML(jobsTemplateHTML, homePath, builder.String())
}

func jobsStateClass(state string) string {
	switch state {
	case "success", "failed", "running":
		return state
	default:
		return "unknown"
	}
}

func writeJobsCopyButton(builder *strings.Builder, value, label string) {
	builder.WriteString(`<button class="jobs-copy" type="button" title="Copy `)
	builder.WriteString(html.EscapeString(label))
	builder.WriteString(`" aria-label="Copy `)
	builder.WriteString(html.EscapeString(label))
	builder.WriteString(`" data-copy-value="`)
	builder.WriteString(html.EscapeString(value))
	builder.WriteString(`" onclick="copyJobsValue(this)"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="9" width="11" height="11" rx="1"></rect><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg></button>`)
}

//go:embed assets/favicon-dark.svg
var webFaviconDarkSVG []byte

//go:embed assets/favicon-light.svg
var webFaviconLightSVG []byte

func faviconDataURL(svg []byte) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)
}

func faviconLinks() string {
	return `<link rel="icon" type="image/svg+xml" media="(prefers-color-scheme: dark)" href="` + faviconDataURL(webFaviconDarkSVG) + `"><link rel="icon" type="image/svg+xml" media="(prefers-color-scheme: light)" href="` + faviconDataURL(webFaviconLightSVG) + `">`
}

// brandIcon renders the favicon artwork inline; the web UI always uses the dark theme, so only that variant is needed.
func brandIcon() string {
	return `<img class="brand-icon" alt="" src="` + faviconDataURL(webFaviconDarkSVG) + `">`
}

package webui

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

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

//go:embed assets/jobs_template.html
var jobsTemplateHTML string

//go:embed assets/web_info_styles.css
var webInfoStylesCSS string

//go:embed assets/web_styles.css
var webStylesCSS string

//go:embed assets/web_sidebar_styles.css
var webSidebarStylesCSS string

func composeWebHTML(executors []string, notifications bool, bootstrap string) string {
	executorJSON, _ := json.Marshal(executors)
	webAppJS := strings.Join([]string{webAppCoreJS, webAppActionsJS, webAppLogsJS, webAppTablesJS, webAppChartsJS, webAppMatrixJS, webAppNotificationsJS, webAppBootstrapJS}, "\n")
	template := strings.Replace(webTemplateHTML, "__ROTARI_WEB_APP__", webAppJS, 1)
	template = strings.Replace(template, "__ROTARI_EXECUTORS__", string(executorJSON), 1)
	template = strings.ReplaceAll(template, "__ROTARI_BRAND_ICON__", brandIcon())
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
	template = strings.Replace(template, "__ROTARI_INFO_STYLES__", webInfoStylesCSS+"\n"+webSidebarStylesCSS, 1)
	template = strings.ReplaceAll(template, "__ROTARI_BRAND_ICON__", brandIcon())
	template = strings.ReplaceAll(template, "__ROTARI_HOME_PATH__", html.EscapeString(homePath))
	template = strings.Replace(template, "__ROTARI_CONTENT__", content, 1)
	return template
}

func jobsHTML(homePath string, projects []string, rows []joblist.Row, since string, canFilter, notifications bool) string {
	var builder strings.Builder
	if canFilter {
		builder.WriteString(`<form class="jobs-filter" method="get"><label for="jobs-since">Since</label><input id="jobs-since" name="since" value="`)
		builder.WriteString(html.EscapeString(since))
		builder.WriteString(`" placeholder="24h" inputmode="text"><button type="submit">Apply</button></form>`)
	}
	var sidebar strings.Builder
	writeJobsSidebarProjects(&sidebar, homePath, projects)
	template := strings.Replace(jobsTemplateHTML, "__ROTARI_JOBS_PROJECTS__", sidebar.String(), 1)
	var toolbar string
	if canFilter {
		toolbar = `<div class="toolbar"><button id="notify-toggle" type="button" onclick="toggleJobsNotifications()">Notification off</button><button type="button" onclick="location.reload()">Refresh</button></div>`
	}
	template = strings.Replace(template, "__ROTARI_JOBS_TOOLBAR__", toolbar, 1)
	template = strings.Replace(template, "__ROTARI_JOBS_LIVE__", strconv.FormatBool(canFilter), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_DEFAULT__", strconv.FormatBool(notifications), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_ICON__", faviconDataURL(webFaviconDarkSVG), 1)
	if len(rows) == 0 {
		builder.WriteString(`<p class="meta">No running or recently finished jobs found.</p>`)
		return composeInfoHTML(template, homePath, builder.String())
	}
	builder.WriteString(`<section><table class="jobs-table"><thead><tr><th data-sort="state">State</th><th data-sort="project">Project</th><th data-sort="job">Job</th><th data-sort="command">Command</th><th data-sort="attempt">Attempt</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th><th data-sort="elapsed">Elapsed</th></tr></thead><tbody>`)
	for _, row := range rows {
		builder.WriteString(`<tr data-notification-key="`)
		builder.WriteString(html.EscapeString(row.Project + "/" + row.RunID + "/" + row.AttemptID))
		builder.WriteString(`" data-project="`)
		builder.WriteString(html.EscapeString(row.Project))
		builder.WriteString(`" data-job="`)
		builder.WriteString(html.EscapeString(row.JobName))
		builder.WriteString(`" data-run-url="`)
		builder.WriteString(html.EscapeString(homePath + "project/" + url.PathEscape(row.Project) + "/run/" + url.PathEscape(row.RunID)))
		builder.WriteString(`"><td class="jobs-state jobs-state-`)
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
		builder.WriteString(`</td><td data-sort-value="`)
		builder.WriteString(html.EscapeString(row.StartedAt.Format(time.RFC3339Nano)))
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(joblist.FormatTimestamp(row.StartedAt)))
		builder.WriteString(`</td><td data-sort-value="`)
		if row.State != "running" {
			builder.WriteString(html.EscapeString(row.FinishedAt.Format(time.RFC3339Nano)))
		}
		builder.WriteString(`">`)
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
	return composeInfoHTML(template, homePath, builder.String())
}

func writeJobsSidebarProjects(builder *strings.Builder, homePath string, projects []string) {
	builder.WriteString(`<div class="sidebar-project expanded" id="sidebar-root"><div class="sidebar-project-row"><button type="button" class="sidebar-toggle" aria-expanded="true" aria-label="Toggle all projects" onclick="toggleJobsSidebar(this)"></button><a class="sidebar-project-link" href="`)
	builder.WriteString(html.EscapeString(homePath))
	builder.WriteString(`">All projects</a></div><div class="sidebar-projects" id="sidebar-projects">`)
	for _, project := range projects {
		builder.WriteString(`<div class="sidebar-project-row"><span class="sidebar-toggle-placeholder" aria-hidden="true"></span><a class="sidebar-project-link" href="`)
		builder.WriteString(html.EscapeString(homePath))
		builder.WriteString(`project/`)
		builder.WriteString(url.PathEscape(project))
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(project))
		builder.WriteString(`</a></div>`)
	}
	builder.WriteString(`</div></div>`)
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

package webui

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/diagnose"
	"github.com/rotari-dev/rotari/internal/joblist"
	"github.com/rotari-dev/rotari/internal/notification"
)

//go:embed assets/web_template.html
var webTemplateHTML string

//go:embed assets/web_app_core.js
var webAppCoreJS string

//go:embed assets/web_app_actions.js
var webAppActionsJS string

//go:embed assets/web_app_logs.js
var webAppLogsJS string

//go:embed assets/web_app_markdown.js
var webAppMarkdownJS string

//go:embed assets/web_app_artifacts.js
var webAppArtifactsJS string

//go:embed assets/web_app_launches.js
var webAppLaunchesJS string

//go:embed assets/web_app_tables.js
var webAppTablesJS string

//go:embed assets/web_app_charts.js
var webAppChartsJS string

//go:embed assets/web_app_matrix.js
var webAppMatrixJS string

//go:embed assets/web_app_notifications.js
var webAppNotificationsJS string

//go:embed assets/web_app_search.js
var webAppSearchJS string

//go:embed assets/web_app_activity.js
var webAppActivityJS string

//go:embed assets/web_app_bootstrap.js
var webAppBootstrapJS string

//go:embed assets/web_static_bootstrap.js
var webStaticBootstrapJS string

//go:embed assets/jobs_template.html
var jobsTemplateHTML string

//go:embed assets/web_info_styles.css
var webInfoStylesCSS string

//go:embed assets/web_shell.js
var webShellJS string

//go:embed assets/web_tokens.css
var webTokensCSS string

//go:embed assets/web_styles.css
var webStylesCSS string

//go:embed assets/web_shell.css
var webShellCSS string

type webBaseDir struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Current bool   `json:"current"`
	// Parent and Name label the basedir in the sidebar and breadcrumbs: its
	// parent directory tells it apart from other basedirs, which usually
	// share a name such as .rotari-state.
	Parent string `json:"parent"`
	Name   string `json:"name"`
}

// newWebBaseDir describes a registered basedir, with its label.
func newWebBaseDir(id, path string, current bool) webBaseDir {
	name := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	if parent == string(filepath.Separator) || parent == "." || parent == name {
		parent = ""
	}
	return webBaseDir{ID: id, Path: path, Current: current, Parent: parent, Name: name}
}

// basedirLabelHTML is a basedir's label: its parent directory in bold and its
// own name. The JS sidebar builds the same markup from Parent and Name.
func basedirLabelHTML(entry webBaseDir) string {
	name := html.EscapeString(entry.Name)
	if entry.Parent == "" {
		return `<span class="basedir-name">` + name + `</span>`
	}
	return `<span class="basedir-parent">` + html.EscapeString(entry.Parent) + `</span><span class="basedir-name">/` + name + `</span>`
}

// basedirBellHTML is the switch that notifies about runs in one basedir. It
// is a checkbox drawn as a bell; onchange names the page's handler.
func basedirBellHTML(entry webBaseDir, extra, onchange string) string {
	return `<label class="basedir-bell" title="Enable notifications"><input class="basedir-notification-toggle" type="checkbox" data-basedir-id="` +
		html.EscapeString(entry.ID) + `"` + extra + ` aria-label="Notify about runs in this basedir" onchange="` + onchange + `(this)" />` + bellIconSVG + `</label>`
}

const bellIconSVG = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 16V11a6 6 0 1 1 12 0v5l1.5 2h-15z"></path><path d="M10 20a2 2 0 0 0 4 0"></path></svg>`

func composeWebHTML(executors []string, notifications bool, bootstrap string, basedirLists ...[]webBaseDir) string {
	return composeWebHTMLWithSession(executors, notifications, bootstrap, "", basedirLists...)
}

func composeWebHTMLWithSession(executors []string, notifications bool, bootstrap, notificationSession string, basedirLists ...[]webBaseDir) string {
	return composeWebHTMLWithNotificationSettings(executors, notifications, notification.Defaults().Browser, bootstrap, notificationSession, basedirLists...)
}

func composeWebHTMLWithNotificationSettings(executors []string, notifications bool, settings notification.ChannelSettings, bootstrap, notificationSession string, basedirLists ...[]webBaseDir) string {
	executorJSON, _ := json.Marshal(executors)
	notificationSettingsJSON, _ := json.Marshal(settings)
	diagnosisGuidanceJSON, _ := json.Marshal(struct {
		NoMatchNext     string `json:"noMatchNext"`
		UnavailableNext string `json:"unavailableNext"`
		OutdatedNote    string `json:"outdatedNote"`
	}{diagnose.NoMatchNext, diagnose.UnavailableNext, diagnose.OutdatedNote})
	basedirs := []webBaseDir{}
	if len(basedirLists) > 0 {
		basedirs = basedirLists[0]
	}
	basedirJSON, _ := json.Marshal(basedirs)
	webAppJS := strings.Join([]string{
		"const diagnosisGuidance = " + string(diagnosisGuidanceJSON) + ";",
		webAppCoreJS, webAppActionsJS, webAppLogsJS, webAppMarkdownJS, webAppArtifactsJS, webAppLaunchesJS, webAppTablesJS, webAppChartsJS, webAppMatrixJS, webAppNotificationsJS, webAppSearchJS, webAppActivityJS, webAppBootstrapJS,
	}, "\n")
	template := strings.Replace(webTemplateHTML, "__ROTARI_SHELL_SCRIPT__", webShellJS, 1)
	template = strings.Replace(template, "__ROTARI_WEB_APP__", webAppJS, 1)
	template = strings.Replace(template, "__ROTARI_BASEDIRS__", string(basedirJSON), 1)
	template = strings.Replace(template, "__ROTARI_EXECUTORS__", string(executorJSON), 1)
	template = strings.Replace(template, "__ROTARI_STATUS_TONES__", statusTonesJSON(), 1)
	template = strings.ReplaceAll(template, "__ROTARI_BRAND_ICON__", brandIcon())
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_ICON__", faviconDataURL(webFaviconDarkSVG), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_DEFAULT__", strconv.FormatBool(notifications), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_SETTINGS__", string(notificationSettingsJSON), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_SESSION__", notificationSession, 1)
	template = strings.Replace(template, "__ROTARI_STATIC_BOOTSTRAP__", bootstrap, 1)
	return template
}

func composeStaticBootstrap(state, logs, reports, configTargets, configs, wordClouds, artifacts, artifactData, activity string) string {
	bootstrap := webStaticBootstrapJS
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_STATE_DATA__", state, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_LOGS_DATA__", logs, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_REPORTS_DATA__", reports, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_CONFIG_TARGETS_DATA__", configTargets, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_CONFIGS_DATA__", configs, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_WORD_CLOUDS_DATA__", wordClouds, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_ARTIFACTS_DATA__", artifacts, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_ARTIFACT_CONTENTS_DATA__", artifactData, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_ACTIVITY_DATA__", activity, 1)
	launchers, _ := json.Marshal(staticLaunchAvailability())
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_LAUNCHERS_DATA__", string(launchers), 1)
	return bootstrap
}

func composeInfoHTML(template, homePath, content string) string {
	template = strings.Replace(template, "__ROTARI_FAVICON_LINKS__", faviconLinks(), 1)
	template = strings.Replace(template, "__ROTARI_SHELL_SCRIPT__", webShellJS, 1)
	styles := fontFaceCSS(homePath+"fonts/") + "\n" + webTokensCSS + "\n" + webInfoStylesCSS + "\n" + webShellCSS
	template = strings.Replace(template, "__ROTARI_INFO_STYLES__", styles, 1)
	template = strings.ReplaceAll(template, "__ROTARI_BRAND_ICON__", brandIcon())
	template = strings.ReplaceAll(template, "__ROTARI_HOME_PATH__", html.EscapeString(homePath))
	searchPath := strings.TrimRight(homePath, "/") + "/search/"
	template = strings.ReplaceAll(template, "__ROTARI_SEARCH_PATH__", html.EscapeString(searchPath))
	activityPath := strings.TrimRight(homePath, "/") + "/activity/"
	template = strings.ReplaceAll(template, "__ROTARI_ACTIVITY_PATH__", html.EscapeString(activityPath))
	template = strings.Replace(template, "__ROTARI_CONTENT__", content, 1)
	return template
}

func jobsHTML(homePath string, projects []string, rows []joblist.Row, since string, canFilter, notifications bool, basedirLists ...[]webBaseDir) string {
	return jobsHTMLWithSession(homePath, projects, rows, since, canFilter, notifications, "", basedirLists...)
}

func jobsHTMLWithSession(homePath string, projects []string, rows []joblist.Row, since string, canFilter, notifications bool, notificationSession string, basedirLists ...[]webBaseDir) string {
	var builder strings.Builder
	if canFilter {
		builder.WriteString(`<form class="jobs-filter" method="get"><label for="jobs-since">Since</label><input id="jobs-since" name="since" value="`)
		builder.WriteString(html.EscapeString(since))
		builder.WriteString(`" placeholder="24h" inputmode="text"><button type="submit">Apply</button></form>`)
	}
	var sidebar strings.Builder
	if len(basedirLists) > 0 {
		writeJobsBasedirSidebar(&sidebar, homePath, projects, basedirLists[0])
	} else {
		writeJobsSidebarProjects(&sidebar, homePath, projects)
	}
	template := strings.Replace(jobsTemplateHTML, "__ROTARI_JOBS_PROJECTS__", sidebar.String(), 1)
	if len(basedirLists) > 0 {
		template = strings.Replace(template, "__ROTARI_BASEDIR_SCROLL_KEY__", currentBasedirScrollID(basedirLists[0]), 1)
	} else {
		template = strings.Replace(template, "__ROTARI_BASEDIR_SCROLL_KEY__", "", 1)
	}
	notificationControl := `<button id="notify-toggle" type="button" onclick="toggleJobsNotifications()">Notification off</button>`
	var toolbar string
	if canFilter {
		toolbar = `<button id="refresh-button" type="button" onclick="location.reload()">Refresh</button>`
	}
	var crumbs strings.Builder
	if len(basedirLists) > 0 {
		activeID := jobsActiveBasedirID(homePath, basedirLists[0])
		for _, entry := range basedirLists[0] {
			if entry.ID == activeID {
				writeBasedirCrumb(&crumbs, homePath, entry)
				crumbs.WriteString(`<span class="crumb-sep">/</span>`)
			}
		}
	}
	crumbs.WriteString(`<span class="crumb-here">Job activity</span>`)
	template = strings.Replace(template, "__ROTARI_JOBS_CRUMBS__", crumbs.String(), 1)
	template = strings.Replace(template, "__ROTARI_JOBS_NOTIFICATION_CONTROL__", notificationControl, 1)
	template = strings.Replace(template, "__ROTARI_JOBS_TOOLBAR__", toolbar, 1)
	template = strings.Replace(template, "__ROTARI_JOBS_LIVE__", strconv.FormatBool(canFilter), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_DEFAULT__", strconv.FormatBool(notifications), 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_SESSION__", notificationSession, 1)
	template = strings.Replace(template, "__ROTARI_NOTIFICATION_ICON__", faviconDataURL(webFaviconDarkSVG), 1)
	if len(rows) == 0 {
		builder.WriteString(`<p class="meta">No unfinished or recently finished jobs found.</p>`)
		return composeInfoHTML(template, homePath, builder.String())
	}
	builder.WriteString(`<section class="jobs-section"><table class="jobs-table"><thead><tr><th data-sort="state">State</th><th data-sort="project">Project</th><th data-sort="job">Job</th><th data-sort="attempt">Attempt</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th><th data-sort="elapsed">Elapsed</th></tr></thead><tbody>`)
	for _, row := range rows {
		// A job that has not started has no attempt ID yet.
		notificationID := row.AttemptID
		if notificationID == "" {
			notificationID = row.JobID
		}
		builder.WriteString(`<tr data-notification-key="`)
		builder.WriteString(html.EscapeString(row.Project + "/" + row.RunID + "/" + notificationID))
		builder.WriteString(`" data-project="`)
		builder.WriteString(html.EscapeString(row.Project))
		builder.WriteString(`" data-job="`)
		builder.WriteString(html.EscapeString(row.JobName))
		builder.WriteString(`" data-run-url="`)
		builder.WriteString(html.EscapeString(homePath + "project/" + url.PathEscape(row.Project) + "/run/" + url.PathEscape(row.RunID)))
		builder.WriteString(`"><td class="jobs-state">`)
		builder.WriteString(statusPillHTML(row.State))
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
		// The command is shown under the job name, as on the run page.
		builder.WriteString(`</a><div class="jobs-command"><code title="`)
		builder.WriteString(html.EscapeString(row.FullCommand))
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(row.Command))
		builder.WriteString(`</code>`)
		writeJobsCopyButton(&builder, row.FullCommand, "command")
		builder.WriteString(`</div>`)
		if row.AttemptID == "" {
			builder.WriteString(`</td><td>-`)
		} else {
			builder.WriteString(`</td><td class="jobs-attempt" data-sort-value="`)
			builder.WriteString(html.EscapeString(row.AttemptID))
			builder.WriteString(`">`)
			builder.WriteString(idTailHTML(row.AttemptID))
			writeJobsCopyButton(&builder, row.AttemptID, "attempt ID")
		}
		builder.WriteString(`</td><td class="jobs-time" data-sort-value="`)
		if !row.StartedAt.IsZero() {
			builder.WriteString(html.EscapeString(row.StartedAt.Format(time.RFC3339Nano)))
		}
		builder.WriteString(`">`)
		if row.StartedAt.IsZero() {
			builder.WriteString(`-`)
		} else {
			builder.WriteString(html.EscapeString(joblist.FormatTimestamp(row.StartedAt)))
		}
		builder.WriteString(`</td><td class="jobs-time" data-sort-value="`)
		if !row.FinishedAt.IsZero() {
			builder.WriteString(html.EscapeString(row.FinishedAt.Format(time.RFC3339Nano)))
		}
		builder.WriteString(`">`)
		if row.FinishedAt.IsZero() {
			builder.WriteString(`-`)
		} else {
			builder.WriteString(html.EscapeString(joblist.FormatTimestamp(row.FinishedAt)))
		}
		builder.WriteString(`</td><td>`)
		builder.WriteString(html.EscapeString(joblist.FormatRunTime(row.RunTime)))
		builder.WriteString(`</td></tr>`)
	}
	builder.WriteString(`</tbody></table></section>`)
	return composeInfoHTML(template, homePath, builder.String())
}

func currentBasedirScrollID(basedirs []webBaseDir) string {
	for _, entry := range basedirs {
		if entry.Current {
			return entry.ID
		}
	}
	return ""
}

func writeJobsBasedirSidebar(builder *strings.Builder, homePath string, projects []string, basedirs []webBaseDir) {
	if len(basedirs) == 0 {
		writeJobsSidebarProjects(builder, homePath, projects)
		return
	}
	activeID := jobsActiveBasedirID(homePath, basedirs)
	for _, entry := range basedirs {
		writeJobsBasedirEntry(builder, homePath, projects, entry, activeID)
	}
}

func jobsActiveBasedirID(homePath string, basedirs []webBaseDir) string {
	if strings.HasPrefix(homePath, basedirRoutePrefix) {
		return strings.Split(strings.TrimPrefix(homePath, basedirRoutePrefix), "/")[0]
	}
	for _, entry := range basedirs {
		if entry.Current {
			return entry.ID
		}
	}
	return ""
}

func writeJobsBasedirEntry(builder *strings.Builder, homePath string, currentProjects []string, entry webBaseDir, activeID string) {
	active := entry.ID == activeID
	basePath := jobsBasedirHomePath(homePath, entry, active)
	projectNames := jobsBasedirProjects(entry, active, currentProjects)
	builder.WriteString(`<div class="sidebar-project basedir-entry`)
	if active {
		builder.WriteString(` expanded`)
	}
	builder.WriteString(`" data-basedir-id="`)
	builder.WriteString(html.EscapeString(entry.ID))
	builder.WriteString(`"><div class="sidebar-project-row basedir-row"><button type="button" class="sidebar-toggle" aria-expanded="`)
	builder.WriteString(strconv.FormatBool(active))
	builder.WriteString(`" aria-label="Toggle projects" onclick="toggleJobsSidebar(this)"></button><a class="sidebar-project-link basedir-path" title="`)
	builder.WriteString(html.EscapeString(entry.Path))
	builder.WriteString(`" href="`)
	builder.WriteString(html.EscapeString(basePath))
	builder.WriteString(`">`)
	builder.WriteString(basedirLabelHTML(entry))
	builder.WriteString(`</a>`)
	builder.WriteString(basedirBellHTML(entry, ` data-jobs-url="`+html.EscapeString(basePath+"jobs/")+`"`, "toggleJobsNotificationBasedir"))
	builder.WriteString(`</div><div class="sidebar-projects basedir-contents"`)
	if !active {
		builder.WriteString(` hidden`)
	}
	builder.WriteString(`><div class="sidebar-project expanded all-projects"><div class="sidebar-project-row"><button type="button" class="sidebar-toggle" aria-expanded="true" aria-label="Toggle project list" onclick="toggleJobsSidebar(this)"></button><a class="sidebar-project-link" href="`)
	builder.WriteString(html.EscapeString(basePath))
	builder.WriteString(`">All projects</a></div><div class="sidebar-projects project-list">`)
	for _, project := range projectNames {
		writeJobsBasedirProject(builder, basePath, project)
	}
	builder.WriteString(`</div></div><a class="sidebar-run`)
	if active {
		builder.WriteString(` active`)
	}
	builder.WriteString(`" href="`)
	builder.WriteString(html.EscapeString(basePath))
	builder.WriteString(`jobs/">Job activity</a></div></div>`)
}

// writeBasedirCrumb writes the breadcrumb that names a page's basedir, with
// a button that copies its full path.
func writeBasedirCrumb(builder *strings.Builder, homePath string, entry webBaseDir) {
	builder.WriteString(`<a class="crumb-basedir" href="`)
	builder.WriteString(html.EscapeString(homePath))
	builder.WriteString(`" title="`)
	builder.WriteString(html.EscapeString(entry.Path))
	builder.WriteString(`">`)
	builder.WriteString(basedirLabelHTML(entry))
	builder.WriteString(`</a>`)
	writeJobsCopyButton(builder, entry.Path, "state directory path")
}

func jobsBasedirHomePath(homePath string, entry webBaseDir, active bool) string {
	if active {
		return homePath
	}
	if entry.Current {
		return "/"
	}
	return basedirRoutePrefix + entry.ID + "/"
}

func jobsBasedirProjects(entry webBaseDir, active bool, currentProjects []string) []string {
	if active {
		return currentProjects
	}
	projects, err := joblist.Projects(entry.Path, "")
	if err != nil {
		return nil
	}
	return projects
}

func writeJobsBasedirProject(builder *strings.Builder, basePath, project string) {
	builder.WriteString(`<div class="sidebar-project-row"><span class="sidebar-toggle-placeholder" aria-hidden="true"></span><a class="sidebar-project-link" href="`)
	builder.WriteString(html.EscapeString(basePath))
	builder.WriteString(`project/`)
	builder.WriteString(url.PathEscape(project))
	builder.WriteString(`">`)
	builder.WriteString(html.EscapeString(project))
	builder.WriteString(`</a></div>`)
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

// idTailHTML shows a long ID by its end, which tells attempts apart, with a
// leading ellipsis (.id-tail in web_shell.css); the text and title keep the
// whole ID. web_app_core.js writes the same markup.
func idTailHTML(id string) string {
	escaped := html.EscapeString(id)
	return `<span class="id-tail" title="` + escaped + `"><bdi>` + escaped + `</bdi></span>`
}

func writeJobsCopyButton(builder *strings.Builder, value, label string) {
	builder.WriteString(`<button class="jobs-copy" type="button" title="Copy `)
	builder.WriteString(html.EscapeString(label))
	builder.WriteString(`" aria-label="Copy `)
	builder.WriteString(html.EscapeString(label))
	builder.WriteString(`" data-copy-value="`)
	builder.WriteString(html.EscapeString(value))
	builder.WriteString(`" onclick="copyJobsValue(this)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 9H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"></path><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg></button>`)
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
// brandIcon embeds both favicons; web_tokens.css shows the one that matches
// the page's theme.
func brandIcon() string {
	return `<img class="brand-icon" alt="" src="` + faviconDataURL(webFaviconLightSVG) + `" data-theme-icon="light">` +
		`<img class="brand-icon" alt="" src="` + faviconDataURL(webFaviconDarkSVG) + `" data-theme-icon="dark">`
}

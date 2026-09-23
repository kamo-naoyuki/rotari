package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"html"
	"strings"
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

//go:embed assets/web_app_bootstrap.js
var webAppBootstrapJS string

//go:embed assets/web_static_bootstrap.js
var webStaticBootstrapJS string

//go:embed assets/cli_docs_template.html
var cliDocsTemplateHTML string

//go:embed assets/environment_template.html
var environmentTemplateHTML string

//go:embed assets/web_info_styles.css
var webInfoStylesCSS string

//go:embed assets/web_styles.css
var webStylesCSS string

func composeWebHTML(executors []string, bootstrap string) string {
	executorJSON, _ := json.Marshal(executors)
	webAppJS := strings.Join([]string{webAppCoreJS, webAppActionsJS, webAppLogsJS, webAppTablesJS, webAppChartsJS, webAppBootstrapJS}, "\n")
	template := strings.Replace(webTemplateHTML, "__ROTARI_WEB_APP__", webAppJS, 1)
	template = strings.Replace(template, "__ROTARI_EXECUTORS__", string(executorJSON), 1)
	template = strings.Replace(template, "__ROTARI_STATIC_BOOTSTRAP__", bootstrap, 1)
	return template
}

func composeStaticBootstrap(state, logs, reports string) string {
	bootstrap := webStaticBootstrapJS
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_STATE_DATA__", state, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_LOGS_DATA__", logs, 1)
	bootstrap = strings.Replace(bootstrap, "__ROTARI_STATIC_REPORTS_DATA__", reports, 1)
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

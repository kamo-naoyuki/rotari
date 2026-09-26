package webui

import (
	"strings"
	"testing"
)

func TestComposeWebHTMLAssemblesAssetBoundaries(t *testing.T) {
	html := composeWebHTML([]string{"local", "slurm"}, true, "window.__BOOTSTRAP__ = true;")
	for _, marker := range []string{"local", "slurm", "window.__BOOTSTRAP__", "__ROTARI_WEB_APP__"} {
		if strings.Contains(html, marker) && marker == "__ROTARI_WEB_APP__" {
			t.Fatalf("template placeholder %q was not replaced", marker)
		}
	}
	if !strings.Contains(html, "local") || !strings.Contains(html, "slurm") || !strings.Contains(html, "window.__BOOTSTRAP__") {
		t.Fatalf("composed HTML omitted embedded asset data")
	}
	if !strings.Contains(html, "Lost connection to the Rotari Web server.") {
		t.Fatal("disconnect banner does not identify the Web server")
	}
	if !strings.Contains(webAppTablesJS, "success (accepted)") {
		t.Fatal("Web job status does not distinguish manually accepted results")
	}
}

func TestComposeStaticBootstrapInjectsData(t *testing.T) {
	bootstrap := composeStaticBootstrap("state", "logs", "reports", "targets", "configs")
	for _, want := range []string{
		"window.__ROTARI_STATIC_STATE__ = state;",
		"window.__ROTARI_STATIC_LOGS__ = logs;",
		"window.__ROTARI_STATIC_REPORTS__ = reports;",
		"window.__ROTARI_STATIC_CONFIG_TARGETS__ = targets;",
		"window.__ROTARI_STATIC_CONFIGS__ = configs;",
	} {
		if !strings.Contains(bootstrap, want) {
			t.Fatalf("static bootstrap does not contain %q", want)
		}
	}
	if strings.Contains(bootstrap, "__ROTARI_STATIC_STATE_DATA__") ||
		strings.Contains(bootstrap, "__ROTARI_STATIC_LOGS_DATA__") ||
		strings.Contains(bootstrap, "__ROTARI_STATIC_REPORTS_DATA__") ||
		strings.Contains(bootstrap, "__ROTARI_STATIC_CONFIG_TARGETS_DATA__") ||
		strings.Contains(bootstrap, "__ROTARI_STATIC_CONFIGS_DATA__") {
		t.Fatal("static bootstrap contains an unreplaced data placeholder")
	}
}

package main

import (
	"strings"
	"testing"
)

func TestComposeWebHTMLAssemblesAssetBoundaries(t *testing.T) {
	html := composeWebHTML([]string{"local", "slurm"}, "window.__BOOTSTRAP__ = true;")
	for _, marker := range []string{"local", "slurm", "window.__BOOTSTRAP__", "__ROTARI_WEB_APP__"} {
		if strings.Contains(html, marker) && marker == "__ROTARI_WEB_APP__" {
			t.Fatalf("template placeholder %q was not replaced", marker)
		}
	}
	if !strings.Contains(html, "local") || !strings.Contains(html, "slurm") || !strings.Contains(html, "window.__BOOTSTRAP__") {
		t.Fatalf("composed HTML omitted embedded asset data")
	}
}

func TestComposeStaticBootstrapInjectsData(t *testing.T) {
	bootstrap := composeStaticBootstrap("state", "logs", "reports")
	for _, want := range []string{
		"window.__ROTARI_STATIC_STATE__ = state;",
		"window.__ROTARI_STATIC_LOGS__ = logs;",
		"window.__ROTARI_STATIC_REPORTS__ = reports;",
	} {
		if !strings.Contains(bootstrap, want) {
			t.Fatalf("static bootstrap does not contain %q", want)
		}
	}
	if strings.Contains(bootstrap, "__ROTARI_STATIC_STATE_DATA__") ||
		strings.Contains(bootstrap, "__ROTARI_STATIC_LOGS_DATA__") ||
		strings.Contains(bootstrap, "__ROTARI_STATIC_REPORTS_DATA__") {
		t.Fatal("static bootstrap contains an unreplaced data placeholder")
	}
}

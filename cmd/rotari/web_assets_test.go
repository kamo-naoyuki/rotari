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

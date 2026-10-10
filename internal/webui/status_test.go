package webui

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// The jobs page (Go) and the other pages (JS) must give a status the same
// tone and pill; each normalises a label before looking it up in statusTones.
func TestStatusToneMatchesInGoAndJS(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	labels := []string{
		"success", "Success", "succeeded", "finished", "success (accepted)", "success (carried)",
		"failed", "failed (carried)", "failed (marked)", "unreadable",
		"running", "running ...", "running (recorded)", "in progress",
		"blocked", "interrupted", "pending", "unfinished", "waiting (recorded)",
		"suspended (recorded)", "incomplete", "cancelled", "unknown", "something new", "-",
	}
	start := strings.Index(webAppCoreJS, "const statusTones = __ROTARI_STATUS_TONES__;")
	end := strings.Index(webAppCoreJS, "function statusPill(")
	if start < 0 || end <= start {
		t.Fatal("status tone functions were not found in web_app_core.js")
	}
	input, _ := json.Marshal(labels)
	script := strings.Replace(webAppCoreJS[start:end], "__ROTARI_STATUS_TONES__", statusTonesJSON(), 1) +
		"\nconsole.log(JSON.stringify(" + string(input) + ".map(label => [statusTone(label), statusPillClass(label)])));"
	output, err := exec.Command("node", "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node failed: %v\n%s", err, output)
	}
	var got [][2]string
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode %q: %v", output, err)
	}
	for index, label := range labels {
		want := [2]string{statusTone(label), statusPillClass(label)}
		if got[index] != want {
			t.Errorf("%q: JS gives %q, Go gives %q", label, got[index], want)
		}
	}
	for label, tone := range map[string]string{
		"success (carried)": "ok", "running (recorded)": "run", "interrupted": "warn",
		"cancelled": "off", "unreadable": "bad", "something new": "off",
	} {
		if statusTone(label) != tone {
			t.Errorf("statusTone(%q) = %q, want %q", label, statusTone(label), tone)
		}
	}
	if !strings.Contains(statusPillClass("success (carried)"), " carried") {
		t.Error("a carried result's pill is not marked carried")
	}
}

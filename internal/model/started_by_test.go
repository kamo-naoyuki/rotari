package model

import (
	"strings"
	"testing"
)

func TestNormalizeActor(t *testing.T) {
	for _, test := range []struct {
		label, want, err string
	}{
		{"", "", ""},
		{"  claude-code ", "claude-code", ""},
		{"エージェント", "エージェント", ""},
		{strings.Repeat("a", MaxActorLength), strings.Repeat("a", MaxActorLength), ""},
		{strings.Repeat("a", MaxActorLength+1), "", "longer than"},
		{"line\nbreak", "", "control character"},
		{"tab\there", "", "control character"},
	} {
		got, err := NormalizeActor(test.label)
		if test.err == "" && (err != nil || got != test.want) {
			t.Errorf("NormalizeActor(%q) = %q, %v; want %q", test.label, got, err, test.want)
		}
		if test.err != "" && (err == nil || !strings.Contains(err.Error(), test.err)) {
			t.Errorf("NormalizeActor(%q) error = %v; want %q", test.label, err, test.err)
		}
	}
	if got := (StartedBy{Interface: StartedByCLI}).Label(); got != "cli" {
		t.Errorf("unlabelled label = %q", got)
	}
	if got := (StartedBy{Interface: StartedByMCP, Actor: "codex"}).Label(); got != "mcp · codex" {
		t.Errorf("labelled label = %q", got)
	}
}

package main

import (
	"strings"
	"testing"
)

func TestCommandHelpRequested(t *testing.T) {
	for _, test := range []struct {
		command string
		args    []string
		want    bool
	}{
		{"run", []string{"--config", "missing.json", "--help"}, true},
		{"run", []string{"-h"}, true},
		{"run", []string{"-help"}, true},
		{"run", []string{"--help=true"}, true},
		{"run", []string{"run-id", "--help"}, true},
		{"run", []string{"--run-name", "--help"}, false},
		{"run", []string{"-r", "--help"}, false},
		{"run", []string{"--", "--help"}, false},
		{"run", []string{"--unknown", "--help"}, false},
		{"add", []string{"--help", "echo"}, true},
		{"add", []string{"echo", "--help"}, false},
		{"change", []string{"echo", "--help"}, false},
		{"server", []string{"status", "--help"}, true},
		{"unknown", []string{"--help"}, false},
	} {
		t.Run(test.command+" "+strings.Join(test.args, " "), func(t *testing.T) {
			if got := commandHelpRequested(test.command, test.args); got != test.want {
				t.Fatalf("commandHelpRequested(%q, %q) = %v, want %v", test.command, test.args, got, test.want)
			}
		})
	}
}

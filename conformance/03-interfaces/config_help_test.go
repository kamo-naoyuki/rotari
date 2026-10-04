package interfaces

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestCommandHelpSurvivesConfigurationErrors(t *testing.T) {
	covers(t, "CLI-17")
	e := support.NewEnv(t)
	for _, source := range []string{"missing", "malformed"} {
		t.Run(source, func(t *testing.T) {
			path := filepath.Join(e.Root, source+".json")
			if source == "malformed" {
				if err := os.WriteFile(path, []byte(`{"run":`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, command := range configCommandCases(t, e, path, "unused", "unused") {
				t.Run(command.name, func(t *testing.T) {
					got := e.Rotari(command.args...)
					if got.Code != 0 || !strings.Contains(got.Stdout, "usage: rotari ") || !strings.Contains(got.Stderr, "WARNING: failed to load config") {
						t.Fatalf("help must remain available with a config warning: %s", got)
					}
				})
			}
		})
	}
}

func TestConfigurationErrorsStillPreventExecution(t *testing.T) {
	covers(t, "CLI-17")
	e := support.NewEnv(t)
	path := filepath.Join(e.Root, "missing.json")
	for _, args := range [][]string{
		{"run", "--config", path},
		{"run", "--config", path, "--", "--help"},
		{"run", "--config", path, "--run-name", "--help"},
		{"add", "--config", path, "--", "echo", "--help"},
		{"add", "--config", path, "echo", "--help"},
		{"change", "--config", path, "echo", "--help"},
	} {
		got := e.Rotari(args...)
		if got.Code == 0 || !strings.Contains(got.Stderr, "failed to load config") || strings.Contains(got.Stderr, "WARNING") || strings.Contains(got.Stdout, "usage: rotari ") {
			t.Errorf("non-help invocation must fail on configuration: %s", got)
		}
	}
}

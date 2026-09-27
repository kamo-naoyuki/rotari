package resolution

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestCompletionScriptsExposeDynamicCompletion(t *testing.T) {
	covers(t, "RES-20")
	e := support.NewEnv(t)
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			script := e.MustRotari("completion", shell).Stdout
			if !strings.Contains(script, "__complete") {
				t.Fatalf("%s completion script has no dynamic completion hook", shell)
			}
		})
	}
}

func TestCompletionMissingStateDirectoryHasNoCandidates(t *testing.T) {
	covers(t, "RES-21")
	e := support.NewEnv(t)
	missing := filepath.Join(e.Root, "missing-state")
	for _, args := range [][]string{
		{"__complete", "project-name", "--basedir", missing},
		{"__complete", "run-id", "--basedir", missing, "--project-name", "missing"},
	} {
		t.Run(strings.Join(args[:2], "/"), func(t *testing.T) {
			r := e.Rotari(args...)
			if r.Code != 0 || strings.TrimSpace(r.Stdout) != "" {
				t.Fatalf("completion for missing state returned %d and %q: %s", r.Code, r.Stdout, r)
			}
		})
	}
}

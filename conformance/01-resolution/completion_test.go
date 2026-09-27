package resolution

import (
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

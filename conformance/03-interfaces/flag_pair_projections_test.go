package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestCLIFlagPairCheckObservability(t *testing.T) {
	covers(t, "CLI-1")
	e := support.NewEnv(t)
	// A missing executable is valid queue data but fails deep validation. A
	// healthy fixture alone cannot demonstrate --deep having an effect.
	e.MustRotari("add", "-p", "missing", "--", "./pair-missing-executable")
	state := pairStateSnapshot(t, e.Base)
	for _, mode := range []string{"--quiet", "--json"} {
		t.Run("deep+"+strings.TrimPrefix(mode, "--"), func(t *testing.T) {
			plain := pairInvoke(t, e, "check", "-p", "missing", mode)
			if plain.Code != 0 {
				t.Fatalf("ordinary check must accept queue definition: %s", plain)
			}
			for _, flags := range [][]string{{"--deep", mode}, {mode, "--deep"}} {
				deep := pairInvoke(t, e, append([]string{"check", "-p", "missing"}, flags...)...)
				if deep.Code != 1 || !strings.Contains(deep.Stderr, "pair-missing-executable") || !strings.Contains(deep.Stderr, "is not available") {
					t.Fatalf("--deep was ignored or failed for another reason: %s", deep)
				}
			}
		})
	}
	t.Run("json+quiet", func(t *testing.T) {
		quiet := pairInvoke(t, e, "check", "-p", "missing", "--quiet")
		machine := pairInvoke(t, e, "check", "-p", "missing", "--json")
		combined := pairInvoke(t, e, "check", "-p", "missing", "--quiet", "--json")
		var result struct {
			Project, State, Lock, RunID, Revision string
			Runnable                              bool
			Queued                                int
		}
		if err := json.Unmarshal([]byte(combined.Stdout), &result); err != nil {
			t.Fatal(err)
		}
		if quiet.Code != 0 || quiet.Stdout != "" || combined.Code != 0 || machine.Stdout != combined.Stdout ||
			result.Project != "missing" || result.State != "ready" || !result.Runnable || result.Queued != 1 || result.Lock != "none" || result.Revision == "" {
			t.Fatalf("check json/quiet projections disagree: %+v\n%s\n%s\n%s", result, quiet, machine, combined)
		}
		// Reasoned mode precedence, not an ignore exception: quiet suppresses
		// success text, while requested machine-readable data remains emitted.
	})
	if !reflect.DeepEqual(state, pairStateSnapshot(t, e.Base)) {
		t.Fatal("check changed fixture state")
	}
}

func TestCLIFlagPairExportSourceObservability(t *testing.T) {
	f := newPairFixture(t)
	f.e.MustRotari("copy", "--run-id", f.run, "--quiet")
	f.e.MustRotari("add", "--job-name", "queue-only", "--", "true")
	state := pairStateSnapshot(t, f.e.Base)
	queue := pairExportJSON(t, f, nil)
	if !strings.Contains(queue, `"queue-only"`) {
		t.Fatal("queue/run fixture has no distinguishing job")
	}
	for _, order := range []string{"source-first", "output-first"} {
		t.Run(order, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "nested", "workflow.json")
			flags := []string{"--run-id", f.run, "--output", file}
			if order == "output-first" {
				flags = []string{"--output", file, "--run-id", f.run}
			}
			result := pairInvoke(t, f.e, append([]string{"export", "--format", "json"}, flags...)...)
			if result.Code != 0 || result.Stdout != "" {
				t.Fatalf("output not routed to file: %s", result)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			selected := pairExportJSON(t, f, []string{"--run-id", f.run})
			if string(data) != selected || strings.Contains(string(data), `"queue-only"`) || selected == queue {
				t.Fatalf("run-id ignored with output:\n%s\nfile: %s", result, data)
			}
		})
	}
	t.Run("template+format", func(t *testing.T) {
		result := pairExportJSON(t, f, []string{"--template"})
		var template struct {
			Jobs []struct {
				Name string `json:"name"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal([]byte(result), &template); err != nil {
			t.Fatal(err)
		}
		if len(template.Jobs) != 1 || template.Jobs[0].Name != "example" || result == queue {
			t.Fatalf("template ignored: %s", result)
		}
	})
	if !reflect.DeepEqual(state, pairStateSnapshot(t, f.e.Base)) {
		t.Fatal("export changed queue/history")
	}
}

func pairExportJSON(t *testing.T, f pairFixture, flags []string) string {
	t.Helper()
	r := pairInvoke(t, f.e, append([]string{"export", "--format", "json"}, flags...)...)
	if r.Code != 0 || !json.Valid([]byte(r.Stdout)) {
		t.Fatalf("invalid export: %s", r)
	}
	return r.Stdout
}

func TestCLIFlagPairConfigNotificationsObservability(t *testing.T) {
	e := support.NewEnv(t)
	for _, order := range []string{"notifications-first", "output-first"} {
		t.Run(order, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", "notifications.toml")
			flags := []string{"--notifications", "--output", path}
			if order == "output-first" {
				flags = []string{"--output", path, "--notifications"}
			}
			written := pairInvoke(t, e, append([]string{"config"}, flags...)...)
			printed := pairInvoke(t, e, "config", "--output", "-", "--notifications", "--format", "toml")
			ordinary := pairInvoke(t, e, "config", "--output", "-", "--format", "toml")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if written.Code != 0 || written.Stdout != "" || printed.Code != 0 || ordinary.Code != 0 || string(data) != printed.Stdout || printed.Stdout == ordinary.Stdout {
				t.Fatalf("notifications/output did not preserve distinct template:\n%s\n%s\n%s", written, printed, ordinary)
			}
		})
	}
}

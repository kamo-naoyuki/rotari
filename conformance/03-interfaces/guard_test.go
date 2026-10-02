package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

var revisionLine = regexp.MustCompile(`(?m)^revision=([0-9a-f]+)$`)

// TestGuardedCommandsPreviewAndCheckTheRevision gives every command that
// changes a project the same three calls: --dry-run must change nothing and
// print the revision `check` reports; --if-revision with that revision must
// apply and print a new one; and the old revision must then be refused
// without changing anything.
func TestGuardedCommandsPreviewAndCheckTheRevision(t *testing.T) {
	covers(t, "CLI-7")
	for _, test := range []struct {
		name string
		// args builds the command from the run and the queued job's ID.
		args func(e *support.Env, run support.FinishedRun, queued string) []string
	}{
		{"add", func(*support.Env, support.FinishedRun, string) []string {
			return []string{"add", "-p", "p1", "--", "true"}
		}},
		{"change", func(_ *support.Env, _ support.FinishedRun, queued string) []string {
			return []string{"change", "-p", "p1", "-j", queued, "--timeout", "1m"}
		}},
		{"remove", func(_ *support.Env, _ support.FinishedRun, queued string) []string {
			return []string{"remove", "-p", "p1", queued}
		}},
		{"copy", func(_ *support.Env, run support.FinishedRun, _ string) []string {
			return []string{"copy", "-p", "p1", "-r", run.RunID, "--append"}
		}},
		{"delete", func(_ *support.Env, run support.FinishedRun, _ string) []string {
			return []string{"delete", "-p", "p1", "-r", run.RunID}
		}},
		{"reset", func(*support.Env, support.FinishedRun, string) []string { return []string{"reset", "p1"} }},
		{"import", func(e *support.Env, _ support.FinishedRun, _ string) []string {
			manifest := filepath.Join(e.Root, "manifest.json")
			if err := os.WriteFile(manifest, []byte(`{"version":1,"jobs":[{"name":"imported","command":["true"]}]}`), 0o600); err != nil {
				e.T.Fatal(err)
			}
			return []string{"import", manifest, "p1", "--overwrite"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			run := e.CreateFinishedRun()
			queued := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--job-name", "queued", "--", "true"))
			args := test.args(e, run, queued)
			before := projectSnapshot(t, e)
			checked := checkRevision(t, e)

			// Options go before the command's own arguments, which for add
			// follow "--" and belong to the job.
			guarded := func(options ...string) []string {
				return append(append([]string{args[0]}, options...), args[1:]...)
			}
			preview := revisionOf(t, e.MustRotari(guarded("--dry-run")...))
			if preview != checked {
				t.Fatalf("--dry-run revision %s, check reports %s", preview, checked)
			}
			if after := projectSnapshot(t, e); after != before {
				t.Fatalf("--dry-run changed the project:\nbefore %s\nafter  %s", before, after)
			}

			applied := revisionOf(t, e.MustRotari(guarded("--if-revision", preview)...))
			if applied == preview || applied != checkRevision(t, e) {
				t.Fatalf("applied revision %s, previewed %s, check reports %s", applied, preview, checkRevision(t, e))
			}
			changed := projectSnapshot(t, e)
			if changed == before {
				t.Fatal("--if-revision with the previewed revision did not apply")
			}

			// The repeated delete names a run that is gone, so it may fail
			// for that reason before the revision is compared.
			result := e.Rotari(guarded("--if-revision", preview)...)
			if result.Code == 0 || (test.name != "delete" && !strings.Contains(result.Stderr, "project changed since the planned revision")) {
				t.Fatalf("stale --if-revision = %s, want refused", result)
			}
			if after := projectSnapshot(t, e); after != changed {
				t.Fatal("a refused --if-revision changed the project")
			}
		})
	}
}

func revisionOf(t *testing.T, result support.Result) string {
	t.Helper()
	match := revisionLine.FindStringSubmatch(result.Stdout)
	if match == nil {
		t.Fatalf("no revision line: %s", result)
	}
	return match[1]
}

func checkRevision(t *testing.T, e *support.Env) string {
	t.Helper()
	var checked struct {
		Revision string `json:"revision"`
	}
	result := e.Rotari("check", "p1", "--json")
	if err := json.Unmarshal([]byte(result.Stdout), &checked); err != nil || checked.Revision == "" {
		t.Fatalf("check --json has no revision: %s", result)
	}
	return checked.Revision
}

// projectSnapshot describes the project's queue, metadata, and runs.
func projectSnapshot(t *testing.T, e *support.Env) string {
	t.Helper()
	dir := filepath.Join(e.Base, "projects", "p1")
	var parts []string
	for _, name := range []string{"queue.json", "meta.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		parts = append(parts, string(data))
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "runs"))
	var runs []string
	for _, entry := range entries {
		runs = append(runs, entry.Name())
	}
	sort.Strings(runs)
	return strings.Join(append(parts, strings.Join(runs, ",")), "\n")
}

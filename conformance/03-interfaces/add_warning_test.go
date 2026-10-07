package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestAddWarnsOnDuplicateFingerprints(t *testing.T) {
	covers(t, "CLI-18")
	for _, test := range []struct {
		name      string
		first     []string
		second    []string
		warnings  int
		unchanged bool
		quiet     bool
	}{
		{name: "plain", warnings: 1},
		{name: "quiet", second: []string{"--quiet"}, warnings: 1, quiet: true},
		{name: "preview", second: []string{"--dry-run"}, warnings: 1, unchanged: true},
		{name: "execution-settings-excluded", first: []string{"--executor", "local", "--timeout", "1s", "--retry", "0"}, second: []string{"--timeout", "2s", "--retry", "1"}, warnings: 1},
		{name: "different-environment", first: []string{"--env", "VALUE=one"}, second: []string{"--env", "VALUE=two"}},
		{name: "normalized-environment", first: []string{"--env", "A=1", "--env", "B=2"}, second: []string{"--env", "B=2", "--env", "A=1"}, warnings: 1},
		{name: "different-directory", first: []string{"--working-directory", "one"}, second: []string{"--working-directory", "two"}},
		{name: "normalized-directory", first: []string{"--working-directory", "one/../two"}, second: []string{"--working-directory", "two"}, warnings: 1},
		{name: "overlapping-array", first: []string{"--array", "1,3,5"}, second: []string{"--array", "3,5,7"}, warnings: 2},
		{name: "disjoint-array", first: []string{"--array", "1,3"}, second: []string{"--array", "2,4"}},
		{name: "plain-versus-array", second: []string{"--array", "1-2"}},
		{name: "overlapping-matrix", first: []string{"--matrix", "VALUE=one,two"}, second: []string{"--matrix", "VALUE=two,three"}, warnings: 1},
		{name: "matrix-array", first: []string{"--matrix", "VALUE=one,two", "--array", "1,3"}, second: []string{"--matrix", "VALUE=two,three", "--array", "3,5"}, warnings: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			add := func(name string, options []string) support.Result {
				args := append([]string{"add", "-p", "duplicates", "--job-name", name}, options...)
				return e.MustRotari(append(args, "--", "true")...)
			}
			first := add("first", test.first)
			if first.Stderr != "" {
				t.Fatalf("first add warned: %s", first)
			}
			queuePath := filepath.Join(e.Base, "projects", "duplicates", "queue.json")
			before, err := os.ReadFile(queuePath)
			if err != nil {
				t.Fatal(err)
			}
			second := add("second", test.second)
			assertFingerprintWarnings(t, second, test.warnings)
			if test.quiet && second.Stdout != "" {
				t.Fatalf("quiet add wrote stdout: %s", second)
			}
			want := 2
			if strings.Contains(test.name, "matrix") {
				want = 4
			}
			assertFingerprintQueue(t, queuePath, before, test.unchanged, want)
			// Existing duplicates must not produce warnings on an unrelated add.
			unrelated := e.MustRotari("add", "-p", "duplicates", "--", "echo", "unrelated")
			if unrelated.Stderr != "" {
				t.Fatalf("unrelated add warned: %s", unrelated)
			}
		})
	}
}

func assertFingerprintWarnings(t *testing.T, result support.Result, want int) {
	t.Helper()
	if got := strings.Count(result.Stderr, "warning: jobs have the same fingerprint:"); got != want {
		t.Fatalf("got %d warnings, want %d: %s", got, want, result)
	}
	if want == 0 {
		if result.Stderr != "" {
			t.Fatalf("unexpected stderr: %s", result)
		}
		return
	}
	for _, label := range []string{"job_id=", "first", "second"} {
		if !strings.Contains(result.Stderr, label) {
			t.Fatalf("warning omitted %q: %s", label, result)
		}
	}
}

func assertFingerprintQueue(t *testing.T, path string, before []byte, unchanged bool, want int) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged {
		if string(after) != string(before) {
			t.Fatal("dry run changed the queue")
		}
		return
	}
	var queue struct {
		Commands []json.RawMessage `json:"commands"`
	}
	if err := json.Unmarshal(after, &queue); err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != want {
		t.Fatalf("queue has %d commands, want %d", len(queue.Commands), want)
	}
}

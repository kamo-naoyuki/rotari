package interfaces

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type shownArtifacts struct {
	Jobs []struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
		Artifacts struct {
			Recorded         bool   `json:"recorded"`
			WorkingDirectory string `json:"working_directory"`
			Entries          []struct {
				Path   string `json:"path"`
				Type   string `json:"type"`
				Origin string `json:"origin"`
			} `json:"entries"`
		} `json:"artifacts"`
	} `json:"jobs"`
}

func lastAttemptID(t *testing.T, e *support.Env, project, jobID string) string {
	t.Helper()
	var shown struct {
		Summary struct {
			Results []struct {
				ID        string `json:"id"`
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", project, "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	for _, result := range shown.Summary.Results {
		if result.ID == jobID {
			return result.AttemptID
		}
	}
	t.Fatalf("no result for %s", jobID)
	return ""
}

func TestShowListsArtifactCandidates(t *testing.T) {
	covers(t, "CLI-16")
	e := support.NewEnv(t)
	if err := os.MkdirAll(filepath.Join(e.Root, "results"), 0o755); err != nil {
		t.Fatal(err)
	}
	job := support.AddedJobID(t, e.MustRotari("add", "-p", "art", "--", "sh", "-c", "echo x > out.txt", "sh", "results/", "missing.yaml"))
	many := []string{"add", "-p", "art", "--", "true"}
	for index := range 25 {
		many = append(many, fmt.Sprintf("data/%02d.csv", index))
	}
	manyJob := support.AddedJobID(t, e.MustRotari(many...))
	failing := support.AddedJobID(t, e.MustRotari("add", "-p", "art", "--", "sh", "-c", "exit 3", "sh", "fail.csv"))
	e.Rotari("run", "-p", "art", "--quiet")
	attempt := lastAttemptID(t, e, "art", job)

	shown := e.MustRotari("show", "-p", "art", "-j", job, "--no-pager").Stdout
	for _, want := range []string{
		"Artifacts: relative to ",
		"  directory  results  (argument)\n",
		"  missing    missing.yaml  (argument)\n",
		"  file       out.txt  (> (shell code 1:10))\n",
	} {
		if !strings.Contains(shown, want) {
			t.Fatalf("show -j lacks %q:\n%s", want, shown)
		}
	}
	if strings.Index(shown, "Artifacts:") > strings.Index(shown, "OUTPUT:") || strings.Index(shown, "Artifacts:") < strings.Index(shown, "Command:") {
		t.Fatalf("artifacts are not between the command and the logs:\n%s", shown)
	}
	if strings.Contains(shown, "Discovery notes:") {
		t.Fatalf("show -j prints discovery notes:\n%s", shown)
	}

	limited := e.MustRotari("show", "-p", "art", "-j", manyJob, "--no-pager").Stdout
	manyAttempt := lastAttemptID(t, e, "art", manyJob)
	if strings.Count(limited, "missing    data/") != 20 || !strings.Contains(limited, "  ... and 5 more: rotari show -j "+manyAttempt+" --artifacts\n") {
		t.Fatalf("limited listing:\n%s", limited)
	}
	full := e.MustRotari("show", "-j", manyAttempt, "--artifacts", "--no-pager").Stdout
	if strings.Count(full, "missing    data/") != 25 || strings.Contains(full, "OUTPUT:") {
		t.Fatalf("--artifacts listing:\n%s", full)
	}
	notes := e.MustRotari("show", "-j", attempt, "--artifacts", "--no-pager").Stdout
	if !regexp.MustCompile(`Discovery notes:\n  .*missing\.yaml: not inspected: `).MatchString(notes) {
		t.Fatalf("--artifacts lacks the discovery notes:\n%s", notes)
	}

	var parsed shownArtifacts
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "art", "-j", job, "--json").Stdout), &parsed); err != nil {
		t.Fatal(err)
	}
	listing := parsed.Jobs[0].Artifacts
	if !listing.Recorded || len(listing.Entries) != 3 || listing.Entries[0].Type != "file" ||
		listing.Entries[0].Path != filepath.Join(listing.WorkingDirectory, "out.txt") || listing.Entries[0].Origin != "> (shell code 1:10)" {
		t.Fatalf("show --json artifacts = %+v", listing)
	}

	// A filtered rerun carries the successful job: its listing is the one
	// of the attempt that produced the result.
	e.Rotari("run", "-p", "art", "--failed", "--quiet")
	carried := e.MustRotari("show", "-p", "art", "-j", job, "--artifacts", "--no-pager").Stdout
	if !strings.Contains(carried, "  file       out.txt  (> (shell code 1:10))\n") {
		t.Fatalf("carried job's listing:\n%s", carried)
	}
	if fail := e.MustRotari("show", "-p", "art", "-j", failing, "--artifacts", "--no-pager").Stdout; !strings.Contains(fail, "fail.csv") {
		t.Fatalf("rerun job's listing:\n%s", fail)
	}

	for _, args := range [][]string{
		{"show", "-p", "art", "--artifacts"},
		{"show", "-p", "art", "-j", job, "--artifacts", "--logs"},
		{"show", "-p", "art", "-j", job, "--artifacts", "--json"},
	} {
		if r := e.Rotari(args...); r.Code != 1 || !(strings.Contains(r.Stderr, "requires --") || strings.Contains(r.Stderr, "cannot be combined")) {
			t.Fatalf("%v was not rejected: %s", args, r)
		}
	}
}

package lifecycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

type conformanceSummary struct {
	RunID   string `json:"run_id"`
	Results []struct {
		ID        string `json:"id"`
		AttemptID string `json:"attempt_id"`
		ExitCode  int    `json:"exit_code"`
	} `json:"results"`
}

func readSummary(t *testing.T, e *support.Env, project string) conformanceSummary {
	t.Helper()
	var shown struct {
		RunID   string             `json:"run_id"`
		Summary conformanceSummary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", project, "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	shown.Summary.RunID = shown.RunID
	return shown.Summary
}

func summaryResult(t *testing.T, summary conformanceSummary, jobID string) (string, int) {
	t.Helper()
	for _, result := range summary.Results {
		if result.ID == jobID {
			return result.AttemptID, result.ExitCode
		}
	}
	t.Fatalf("summary has no result for job %s: %#v", jobID, summary.Results)
	return "", 0
}

func TestFilteredRerunCarriesCompletedResults(t *testing.T) {
	covers(t, "CORE-3", "CORE-6", "RUN-1")
	e := support.NewEnv(t)
	support.RequireUnixSockets(t)
	okJob := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--job-name", "ok", "--", "sh", "-c", "echo hello"))
	badJob := support.AddedJobID(t, e.MustRotari("add", "-p", "p1", "--job-name", "bad", "--", "sh", "-c", "exit 3"))
	if r := e.Rotari("run", "-p", "p1", "--quiet"); r.Code == 0 {
		t.Fatalf("run with a failing job should exit 1: %s", r)
	}
	first := readSummary(t, e, "p1")
	firstAttempt, firstExit := summaryResult(t, first, okJob)
	if firstExit != 0 {
		t.Fatalf("source success job exit code = %d", firstExit)
	}
	sourceSummaryPath := filepath.Join(e.Base, "projects", "p1", "runs", first.RunID, "summary.json")
	sourceSummary, err := os.ReadFile(sourceSummaryPath)
	if err != nil {
		t.Fatal(err)
	}

	if r := e.Rotari("run", "-p", "p1", "--failed", "--quiet"); r.Code == 0 {
		t.Fatalf("filtered rerun unexpectedly succeeded: %s", r)
	}
	second := readSummary(t, e, "p1")
	if second.RunID == first.RunID {
		t.Fatalf("filtered rerun reused source run %q", first.RunID)
	}
	if after, err := os.ReadFile(sourceSummaryPath); err != nil || !bytes.Equal(after, sourceSummary) {
		t.Fatalf("filtered rerun changed source summary: %v", err)
	}
	secondAttempt, secondExit := summaryResult(t, second, okJob)
	if secondExit != 0 || secondAttempt != firstAttempt {
		t.Fatalf("carried result = attempt %q, exit %d; want source attempt %q, exit 0", secondAttempt, secondExit, firstAttempt)
	}
	if _, exit := summaryResult(t, second, badJob); exit == 0 {
		t.Fatal("failed job was not rerun in the filtered run")
	}
}

func TestRunRetrySucceedsWithinOneRun(t *testing.T) {
	covers(t, "CORE-7", "RUN-2")
	support.RequireUnixSockets(t)
	e := support.NewEnv(t)
	marker := filepath.Join(e.Root, "retry-count")
	command := fmt.Sprintf("printf x >> %q; test $(wc -c < %q) -ge 3", marker, marker)
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "retry", "--", "sh", "-c", command))
	if r := e.Rotari("run", "-p", "retry", "--retry", "2", "--quiet"); r.Code != 0 {
		t.Fatalf("run with a successful retry exited %d: %s", r.Code, r)
	}
	data, err := os.ReadFile(marker)
	if err != nil || len(data) != 3 {
		t.Fatalf("job ran %d times, want three: %v", len(data), err)
	}
	summary := readSummary(t, e, "retry")
	if _, exit := summaryResult(t, summary, jobID); exit != 0 {
		t.Fatalf("retried job result exit code = %d, want 0", exit)
	}
	attempts, err := filepath.Glob(filepath.Join(e.Base, "projects", "retry", "runs", summary.RunID, jobID, "attempts", "*"))
	if err != nil || len(attempts) != 3 {
		t.Fatalf("attempt directories = %d, want three: %v", len(attempts), err)
	}
}

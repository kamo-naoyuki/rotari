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

// callerRecord is what a job observed of its working directory and
// environment, and what its run recorded as the caller's directory.
type callerRecord struct {
	pwd, foo, rotariCWD, contextCWD string
}

// recordingJob adds a job to project that writes its working directory,
// $FOO, and $ROTARI_CWD to a file under e.Root, and returns that file.
func recordingJob(t *testing.T, e *support.Env, project string, addArgs ...string) string {
	t.Helper()
	out := filepath.Join(e.Root, project+".out")
	args := append([]string{"add", "-p", project}, addArgs...)
	args = append(args, "--", "sh", "-c", `printf '%s\n%s\n%s\n' "$(pwd -P)" "$FOO" "$ROTARI_CWD" > "$0"`, out)
	e.MustRotari(args...)
	return out
}

func readCallerRecord(t *testing.T, e *support.Env, project, out string) callerRecord {
	t.Helper()
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("job of %s left no record: %v", project, err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("record of %s = %q", project, data)
	}
	runID := readSummary(t, e, project).RunID
	var context struct {
		CWD string `json:"cwd"`
	}
	contextData, err := os.ReadFile(filepath.Join(e.Base, "projects", project, "runs", runID, "context.json"))
	if err != nil || json.Unmarshal(contextData, &context) != nil {
		t.Fatalf("context.json of %s: %v %s", project, err, contextData)
	}
	return callerRecord{pwd: string(lines[0]), foo: string(lines[1]), rotariCWD: string(lines[2]), contextCWD: context.CWD}
}

// callerDir creates a directory under e.Root and returns its resolved path.
func callerDir(t *testing.T, e *support.Env, name string) string {
	t.Helper()
	dir := filepath.Join(e.Root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestRunUsesCallersDirectoryAndEnvironment(t *testing.T) {
	covers(t, "RUN-3")
	e := support.NewEnv(t)
	support.RequireUnixSockets(t)
	want := func(t *testing.T, project string, got callerRecord, dir, foo string) {
		t.Helper()
		if got != (callerRecord{pwd: dir, foo: foo, rotariCWD: dir, contextCWD: dir}) {
			t.Errorf("%s: job ran in %q with FOO=%q, ROTARI_CWD=%q, context cwd %q; want %q and FOO=%q everywhere",
				project, got.pwd, got.foo, got.rotariCWD, got.contextCWD, dir, foo)
		}
	}

	// A run started while another project's run is active must not take
	// that run's directory and environment.
	dirA, dirB, dirC := callerDir(t, e, "a"), callerDir(t, e, "b"), callerDir(t, e, "c")
	e.In(dirA).WithVar("FOO", "from-a").StartRun("hold", 1, true)

	syncOut := recordingJob(t, e, "sync")
	e.In(dirB).WithVar("FOO", "from-b").MustRotari("run", "-p", "sync", "--quiet")
	want(t, "sync", readCallerRecord(t, e, "sync", syncOut), dirB, "from-b")

	asyncOut := recordingJob(t, e, "async")
	e.In(dirC).WithVar("FOO", "from-c").MustRotari("run", "-p", "async", "--async", "--quiet")
	e.MustRotari("wait", "-p", "async", "--timeout", "30s")
	want(t, "async", readCallerRecord(t, e, "async", asyncOut), dirC, "from-c")

	// A job's own --env still overrides the caller's environment.
	explicitOut := recordingJob(t, e, "explicit", "--env", "FOO=from-job")
	e.In(dirB).WithVar("FOO", "from-b").MustRotari("run", "-p", "explicit", "--quiet")
	want(t, "explicit", readCallerRecord(t, e, "explicit", explicitOut), dirB, "from-job")
}

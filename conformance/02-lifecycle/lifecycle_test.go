package lifecycle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

func TestFingerprintMatchingUsesIDsAndRejectsCountMismatches(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)

	support.AddedJobID(t, e.MustRotari("add", "-p", "fingerprint", "--", "true"))
	e.MustRotari("run", "-p", "fingerprint", "--quiet")

	changedID := support.AddedJobID(t, e.MustRotari("add", "-p", "fingerprint", "--", "true"))
	e.MustRotari("run", "-p", "fingerprint", "--match-by", "fingerprint", "--quiet")
	matchedRun := readSummary(t, e, "fingerprint")
	matchedCommands := readCommandSnapshot(t, e, "fingerprint", matchedRun.RunID)
	matched := commandByID(t, matchedCommands, changedID)
	if matched.Origin == nil || matched.Origin.JobID == changedID {
		t.Fatalf("changed job was not matched to the historical fingerprint: %#v", matched)
	}

	e.MustRotari("add", "-p", "mismatch", "--", "true")
	e.MustRotari("run", "-p", "mismatch", "--quiet")
	firstExtra := support.AddedJobID(t, e.MustRotari("add", "-p", "mismatch", "--", "true"))
	secondExtra := support.AddedJobID(t, e.MustRotari("add", "-p", "mismatch", "--", "true"))
	e.MustRotari("run", "-p", "mismatch", "--match-by", "fingerprint", "--quiet")
	mismatchRun := readSummary(t, e, "mismatch")
	mismatchCommands := readCommandSnapshot(t, e, "mismatch", mismatchRun.RunID)
	for _, jobID := range []string{firstExtra, secondExtra} {
		command := commandByID(t, mismatchCommands, jobID)
		if command.Origin != nil {
			t.Fatalf("count-mismatched job %s was matched: %#v", jobID, command)
		}
	}
}

func TestFingerprintMatchingPrioritizesIDsAndQueueOccurrence(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)

	stableID := support.AddedJobID(t, e.MustRotari("add", "-p", "priority", "--", "true"))
	otherID := support.AddedJobID(t, e.MustRotari("add", "-p", "priority", "--", "echo", "other"))
	e.MustRotari("run", "-p", "priority", "--quiet")
	prioritySource := readSummary(t, e, "priority")
	e.MustRotari("copy", "-p", "priority", "--run-id", prioritySource.RunID, "--overwrite", "--quiet")
	e.MustRotari("change", "-p", "priority", "--job-id", stableID, "--quiet", "echo", "other")
	e.MustRotari("remove", "-p", "priority", "--quiet", otherID)
	e.MustRotari("run", "-p", "priority", "--match-by", "id-and-fingerprint", "--quiet")
	priorityRun := readSummary(t, e, "priority")
	priorityCommands := readCommandSnapshot(t, e, "priority", priorityRun.RunID)
	prioritized := commandByID(t, priorityCommands, stableID)
	if prioritized.Origin == nil || prioritized.Origin.JobID != stableID {
		t.Fatalf("ID match did not take priority over the matching fingerprint: %#v", prioritized)
	}

	firstOld := support.AddedJobID(t, e.MustRotari("add", "-p", "occurrence", "--", "true"))
	secondOld := support.AddedJobID(t, e.MustRotari("add", "-p", "occurrence", "--", "true"))
	e.MustRotari("run", "-p", "occurrence", "--quiet")
	occurrenceSource := readSummary(t, e, "occurrence")
	e.MustRotari("copy", "-p", "occurrence", "--run-id", occurrenceSource.RunID, "--overwrite", "--quiet")
	e.MustRotari("remove", "-p", "occurrence", "--quiet", firstOld)
	e.MustRotari("remove", "-p", "occurrence", "--quiet", secondOld)
	firstNew := support.AddedJobID(t, e.MustRotari("add", "-p", "occurrence", "--", "true"))
	secondNew := support.AddedJobID(t, e.MustRotari("add", "-p", "occurrence", "--", "true"))
	e.MustRotari("run", "-p", "occurrence", "--match-by", "fingerprint", "--quiet")
	occurrenceRun := readSummary(t, e, "occurrence")
	occurrenceCommands := readCommandSnapshot(t, e, "occurrence", occurrenceRun.RunID)
	for currentID, sourceID := range map[string]string{firstNew: firstOld, secondNew: secondOld} {
		command := commandByID(t, occurrenceCommands, currentID)
		if command.Origin == nil || command.Origin.JobID != sourceID {
			t.Fatalf("queue occurrence for %s matched origin %#v, want source job %s", currentID, command.Origin, sourceID)
		}
	}
}

func TestFingerprintMatchingPreservesArrayTasksAndMatrixLeaves(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)

	e.MustRotari("add", "-p", "expanded", "--array", "1-2", "--", "true")
	e.MustRotari("add", "-p", "expanded", "--matrix", "SEED=1,2", "--", "true")
	e.MustRotari("run", "-p", "expanded", "--quiet")

	e.MustRotari("add", "-p", "expanded", "--array", "1-2", "--", "true")
	e.MustRotari("add", "-p", "expanded", "--matrix", "SEED=1,2", "--", "true")
	e.MustRotari("run", "-p", "expanded", "--match-by", "fingerprint", "--quiet")
	run := readSummary(t, e, "expanded")
	snapshot := readCommandSnapshot(t, e, "expanded", run.RunID)
	var arrayTasks, matrixLeaves int
	for _, command := range snapshot.Commands {
		switch {
		case command.Array != nil:
			if len(command.TaskOrigins) != 2 {
				t.Fatalf("array task origins = %#v, want 2 tasks", command.TaskOrigins)
			}
			arrayTasks += len(command.TaskOrigins)
		case command.Matrix != nil:
			if command.Origin == nil {
				t.Fatalf("matrix leaf has no origin: %#v", command)
			}
			matrixLeaves++
		}
	}
	if arrayTasks != 2 || matrixLeaves != 2 {
		t.Fatalf("expanded origins = array tasks %d, matrix leaves %d; want 2, 2", arrayTasks, matrixLeaves)
	}
}

func TestFingerprintMatchingRejectsChangedExplicitInputs(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)
	firstDir := filepath.Join(e.Root, "first")
	secondDir := filepath.Join(e.Root, "second")
	if err := os.MkdirAll(firstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}

	e.MustRotari("add", "-p", "inputs", "--env", "MODE=one", "--working-directory", firstDir, "--", "true")
	e.MustRotari("run", "-p", "inputs", "--quiet")
	e.MustRotari("add", "-p", "inputs", "--env", "MODE=two", "--working-directory", firstDir, "--", "true")
	e.MustRotari("run", "-p", "inputs", "--match-by", "fingerprint", "--quiet")
	firstChanged := readSummary(t, e, "inputs")
	firstSnapshot := readCommandSnapshot(t, e, "inputs", firstChanged.RunID)
	if len(firstSnapshot.Commands) != 1 || firstSnapshot.Commands[0].Origin != nil {
		t.Fatalf("environment change unexpectedly matched: %#v", firstSnapshot.Commands)
	}

	e.MustRotari("add", "-p", "inputs", "--env", "MODE=two", "--working-directory", secondDir, "--", "true")
	e.MustRotari("run", "-p", "inputs", "--match-by", "fingerprint", "--quiet")
	secondChanged := readSummary(t, e, "inputs")
	secondSnapshot := readCommandSnapshot(t, e, "inputs", secondChanged.RunID)
	if len(secondSnapshot.Commands) != 1 || secondSnapshot.Commands[0].Origin != nil {
		t.Fatalf("working-directory change unexpectedly matched: %#v", secondSnapshot.Commands)
	}
}

type commandSnapshot struct {
	Commands []snapshotCommand `json:"commands"`
}

type snapshotCommand struct {
	ID     string `json:"id"`
	Origin *struct {
		JobID string `json:"job_id"`
	} `json:"origin,omitempty"`
	Array *struct {
		Tasks []int `json:"tasks"`
	} `json:"array,omitempty"`
	TaskOrigins map[string]struct {
		JobID string `json:"job_id"`
	} `json:"task_origins,omitempty"`
	Matrix *struct {
		Values []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"values"`
	} `json:"matrix,omitempty"`
}

func readCommandSnapshot(t *testing.T, e *support.Env, project, runID string) commandSnapshot {
	t.Helper()
	path := filepath.Join(e.Base, "projects", project, "runs", runID, "commands.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot commandSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func commandByID(t *testing.T, snapshot commandSnapshot, jobID string) snapshotCommand {
	t.Helper()
	for _, command := range snapshot.Commands {
		if command.ID == jobID {
			return command
		}
	}
	t.Fatalf("command snapshot has no job %s: %#v", jobID, snapshot.Commands)
	return snapshotCommand{}
}

func TestRunRetrySucceedsWithinOneRun(t *testing.T) {
	covers(t, "CORE-7", "RUN-2")
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

func TestImportedWorkflowRunsFreshJobs(t *testing.T) {
	covers(t, "RUN-5")
	e := support.NewEnv(t)
	manifestPath := filepath.Join(e.Root, "fresh.json")
	manifest := `{"version":1,"jobs":[{"name":"fresh","command":["touch","imported-job-ran"]}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("import", manifestPath, "imported")
	e.MustRotari("run", "-p", "imported", "--quiet")
	if _, err := os.Stat(filepath.Join(e.Root, "imported-job-ran")); err != nil {
		t.Fatalf("imported fresh job did not execute: %v", err)
	}
	summary := readSummary(t, e, "imported")
	if len(summary.Results) != 1 {
		t.Fatalf("imported run results = %#v, want one result", summary.Results)
	}
	if _, exit := summaryResult(t, summary, summary.Results[0].ID); exit != 0 {
		t.Fatalf("imported job exit code = %d, want 0", exit)
	}
}

func TestJobStreamsPersistSeparately(t *testing.T) {
	covers(t, "LOG-1")
	e := support.NewEnv(t)
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "streams", "--log-mode", "separate", "--", "sh", "-c", "printf OUT_MARKER; printf ERR_MARKER >&2"))
	mergedJobID := support.AddedJobID(t, e.MustRotari("add", "-p", "streams", "--", "sh", "-c", "printf DEFAULT_OUT; printf DEFAULT_ERR >&2"))
	e.MustRotari("run", "-p", "streams", "--quiet")
	summary := readSummary(t, e, "streams")
	attemptID, _ := summaryResult(t, summary, jobID)
	attemptDir := filepath.Join(e.Base, "projects", "streams", "runs", summary.RunID, jobID, "attempts", attemptID)
	for _, stream := range []struct {
		name string
		want string
	}{{name: "stdout", want: "OUT_MARKER"}, {name: "stderr", want: "ERR_MARKER"}} {
		data, err := os.ReadFile(filepath.Join(attemptDir, stream.name))
		if err != nil || string(data) != stream.want {
			t.Fatalf("%s = %q, err=%v; want %q", stream.name, data, err, stream.want)
		}
	}
	if _, err := os.Stat(filepath.Join(attemptDir, "output")); !os.IsNotExist(err) {
		t.Fatalf("combined output file exists or could not be checked: %v", err)
	}
	mergedAttemptID, _ := summaryResult(t, summary, mergedJobID)
	mergedDir := filepath.Join(e.Base, "projects", "streams", "runs", summary.RunID, mergedJobID, "attempts", mergedAttemptID)
	merged, err := os.ReadFile(filepath.Join(mergedDir, "output"))
	if err != nil || !strings.Contains(string(merged), "DEFAULT_OUT") || !strings.Contains(string(merged), "DEFAULT_ERR") {
		t.Fatalf("default merged output = %q, err=%v", merged, err)
	}
	if _, err := os.Stat(filepath.Join(mergedDir, "stdout")); !os.IsNotExist(err) {
		t.Fatalf("default merged attempt unexpectedly has stdout file: %v", err)
	}

	shown := e.MustRotari("show", "-p", "streams", "--run-id", summary.RunID, "--job-id", jobID, "--stream", "stderr", "--no-pager")
	stderrStart := strings.LastIndex(shown.Stdout, "STDERR:")
	if stderrStart < 0 || !strings.Contains(shown.Stdout[stderrStart:], "ERR_MARKER") || strings.Contains(shown.Stdout[stderrStart:], "OUT_MARKER") {
		t.Fatalf("stderr-only CLI view mixed streams: %s", shown)
	}
	query := url.Values{"project_name": {"streams"}, "run_id": {summary.RunID}, "job_id": {jobID}, "stream": {"stderr"}}
	response := e.HTTPGet(e.StartWeb() + "/api/log?" + query.Encode())
	if response.Status != 200 || response.Body != "ERR_MARKER" {
		t.Fatalf("stderr Web API response = (%d, %q)", response.Status, response.Body)
	}
}

func TestExternalLogDestinations(t *testing.T) {
	covers(t, "LOG-2", "LOG-3", "LOG-4")
	e := support.NewEnv(t)

	if err := os.MkdirAll(filepath.Join(e.Root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	mergedPath := filepath.Join(e.Root, "logs", "merged.log")
	if err := os.WriteFile(mergedPath, []byte("prior:"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("add", "-p", "external-merge", "--output", "logs/merged.log", "--output", "logs/copy.log", "--", "sh", "-c", "printf out; printf err >&2")
	e.MustRotari("run", "-p", "external-merge", "--quiet")
	for _, path := range []string{mergedPath, filepath.Join(e.Root, "logs", "copy.log")} {
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "out") || !strings.Contains(string(data), "err") {
			t.Fatalf("output-only destination %s = %q, err=%v; stderr should follow --output", path, data, err)
		}
	}

	separatedJobID := support.AddedJobID(t, e.MustRotari("add", "-p", "external-separated", "--output", "nested/a/out.log", "--output", "nested/b/out.log", "--output", "nested/shared.log", "--error", "nested/errors/err.log", "--error", "nested/shared.log", "--log-mode", "merge", "--", "sh", "-c", "printf only-out; printf only-err >&2"))
	e.MustRotari("run", "-p", "external-separated", "--quiet")
	separatedSummary := readSummary(t, e, "external-separated")
	separatedAttemptID, _ := summaryResult(t, separatedSummary, separatedJobID)
	separatedAttemptDir := filepath.Join(e.Base, "projects", "external-separated", "runs", separatedSummary.RunID, separatedJobID, "attempts", separatedAttemptID)
	internalMerged, err := os.ReadFile(filepath.Join(separatedAttemptDir, "output"))
	if err != nil || !strings.Contains(string(internalMerged), "only-out") || !strings.Contains(string(internalMerged), "only-err") {
		t.Fatalf("internal merged log with separate external sinks = %q, err=%v", internalMerged, err)
	}
	for _, path := range []string{"nested/a/out.log", "nested/b/out.log"} {
		data, err := os.ReadFile(filepath.Join(e.Root, path))
		if err != nil || string(data) != "only-out" {
			t.Fatalf("stdout destination %s = %q, err=%v", path, data, err)
		}
	}
	stderr, err := os.ReadFile(filepath.Join(e.Root, "nested/errors/err.log"))
	if err != nil || string(stderr) != "only-err" {
		t.Fatalf("stderr destination = %q, err=%v", stderr, err)
	}
	shared, err := os.ReadFile(filepath.Join(e.Root, "nested/shared.log"))
	if err != nil || !strings.Contains(string(shared), "only-out") || !strings.Contains(string(shared), "only-err") {
		t.Fatalf("shared stdout/stderr destination = %q, err=%v", shared, err)
	}

	truncatePath := filepath.Join(e.Root, "logs", "truncate.log")
	if err := os.WriteFile(truncatePath, []byte("old-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("add", "-p", "external-truncate", "--output", "logs/truncate.log", "--open-mode", "truncate", "--", "printf", "new-data")
	e.MustRotari("run", "-p", "external-truncate", "--quiet")
	truncated, err := os.ReadFile(truncatePath)
	if err != nil || string(truncated) != "new-data" {
		t.Fatalf("truncated destination = %q, err=%v", truncated, err)
	}

	blocker := filepath.Join(e.Root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(e.Root, "must-not-run")
	e.MustRotari("add", "-p", "bad-destination", "--output", "not-a-directory/output", "--", "touch", marker)
	if result := e.Rotari("run", "-p", "bad-destination", "--quiet"); result.Code == 0 {
		t.Fatalf("run succeeded despite destination setup failure: %s", result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command ran despite destination setup failure: stat error=%v", err)
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

	noneOut := recordingJob(t, e, "none")
	e.In(dirB).WithVar("FOO", "from-b").MustRotari("run", "-p", "none", "--env=NONE", "--quiet")
	none := readCallerRecord(t, e, "none", noneOut)
	if none.pwd != dirB || none.foo != "" || none.rotariCWD != dirB || none.contextCWD != dirB {
		t.Fatalf("NONE env run record = %#v; want cwd %q, empty FOO, and caller cwd metadata", none, dirB)
	}

	noneJobEnvOut := recordingJob(t, e, "none-job-env", "--env", "FOO=from-job")
	e.In(dirB).WithVar("FOO", "from-b").MustRotari("run", "-p", "none-job-env", "--env=NONE", "--quiet")
	noneJobEnv := readCallerRecord(t, e, "none-job-env", noneJobEnvOut)
	if noneJobEnv.foo != "from-job" || noneJobEnv.rotariCWD != dirB {
		t.Fatalf("NONE env with job override = %#v; want FOO from-job and caller cwd metadata", noneJobEnv)
	}
	relativeDir := filepath.Join(dirB, "relative-work")
	if err := os.Mkdir(relativeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	relativeOut := recordingJob(t, e, "relative-working-directory", "--working-directory", "relative-work")
	e.In(dirB).WithVar("FOO", "from-b").MustRotari("run", "-p", "relative-working-directory", "--quiet")
	relative := readCallerRecord(t, e, "relative-working-directory", relativeOut)
	if relative.pwd != relativeDir || relative.foo != "from-b" || relative.rotariCWD != dirB || relative.contextCWD != dirB {
		t.Fatalf("relative working-directory record = %#v; want job cwd %q and caller context %q", relative, relativeDir, dirB)
	}
}

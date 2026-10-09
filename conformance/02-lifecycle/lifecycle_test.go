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
	"time"

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
		Error     string `json:"error"`
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
	covers(t, "CORE-3", "CORE-6", "RUN-1", "WEB-1")
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
	response := e.HTTPGet(e.StartWeb() + "/api/run?project_name=" + url.QueryEscape("p1") + "&run_id=" + url.QueryEscape(second.RunID))
	if response.Status != 200 {
		t.Fatalf("GET rerun details: status %d: %s", response.Status, response.Body)
	}
	var detail struct {
		RunID    string `json:"run_id"`
		Timeline []struct {
			Pending  int `json:"pending"`
			Finished int `json:"finished"`
			Success  int `json:"success"`
			Failed   int `json:"failed"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal([]byte(response.Body), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.RunID != second.RunID || len(detail.Timeline) < 2 {
		t.Fatalf("rerun timeline = %#v, want initial and execution-event points", detail)
	}
	initial := detail.Timeline[0]
	if initial.Pending != 1 || initial.Finished != 1 || initial.Success != 1 {
		t.Fatalf("initial timeline point = %+v, want one carried success and one pending rerun", initial)
	}
	final := detail.Timeline[len(detail.Timeline)-1]
	if final.Finished != 2 || final.Success != 1 || final.Failed != 1 {
		t.Fatalf("final timeline point = %+v, want carried success and rerun failure", final)
	}
}

func TestRetryFromSavedRunLeavesNextQueueUntouched(t *testing.T) {
	covers(t, "RUN-13")
	e := support.NewEnv(t)
	sourceJob := support.AddedJobID(t, e.MustRotari("add", "-p", "saved", "--job-name", "source", "--", "sh", "-c", "exit 3"))
	if r := e.Rotari("run", "-p", "saved", "--quiet"); r.Code == 0 {
		t.Fatalf("source run unexpectedly succeeded: %s", r)
	}
	source := readSummary(t, e, "saved")
	nextJob := support.AddedJobID(t, e.MustRotari("add", "-p", "saved", "--job-name", "next", "--", "true"))
	for _, args := range [][]string{
		{"run", "-p", "saved", "--run-id", source.RunID, "--overwrite"},
		{"retry", "-p", "saved", "--run-id", source.RunID, "--overwrite"},
	} {
		if r := e.Rotari(args...); r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "no longer accept --overwrite") {
			t.Errorf("removed overwrite option was not rejected: %s", r)
		}
	}
	queuePath := filepath.Join(e.Base, "projects", "saved", "queue.json")
	queueBefore, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatal(err)
	}

	retry := e.Rotari("retry", "-p", "saved", "--run-id", source.RunID, "--quiet")
	if retry.Code == 0 {
		t.Fatalf("retry of failed source job unexpectedly succeeded: %s", retry)
	}
	if queueAfter, err := os.ReadFile(queuePath); err != nil || !bytes.Equal(queueAfter, queueBefore) {
		t.Fatalf("next queue changed across retry: %v", err)
	}
	var shown struct {
		RunID   string             `json:"run_id"`
		Summary conformanceSummary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "saved", "--run-id", "latest", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.RunID == source.RunID || len(shown.Summary.Results) != 1 {
		t.Fatalf("retry result = %s\nlatest run %s results = %+v; want a new run with one copied source job", retry, shown.RunID, shown.Summary.Results)
	}
	runDir := filepath.Join(e.Base, "projects", "saved", "runs", shown.RunID)
	snapshot, err := os.ReadFile(filepath.Join(runDir, "commands.json"))
	var commands struct {
		Commands []struct {
			ID     string `json:"id"`
			Origin struct {
				RunID string `json:"run_id"`
				JobID string `json:"job_id"`
			} `json:"origin"`
		} `json:"commands"`
	}
	if err == nil {
		err = json.Unmarshal(snapshot, &commands)
	}
	if err != nil || len(commands.Commands) != 1 || commands.Commands[0].Origin.RunID != source.RunID || commands.Commands[0].Origin.JobID != sourceJob {
		t.Fatalf("retry snapshot contains next queue job %s: %v, %s", nextJob, err, snapshot)
	}
}

func TestRetryReportsFailedJobsOmittedByNonEmptyQueue(t *testing.T) {
	covers(t, "RUN-14")
	e := support.NewEnv(t)
	failedA := support.AddedJobID(t, e.MustRotari("add", "-p", "report", "--job-name", "failed-a", "--", "sh", "-c", "exit 2"))
	failedB := support.AddedJobID(t, e.MustRotari("add", "-p", "report", "--job-name", "failed-b", "--", "sh", "-c", "exit 3"))
	if r := e.Rotari("run", "-p", "report", "--quiet"); r.Code == 0 {
		t.Fatalf("source run unexpectedly succeeded: %s", r)
	}
	latest := readSummary(t, e, "report")
	e.MustRotari("add", "-p", "report", "--job-name", "next", "--", "true")
	preview := e.MustRotari("retry", "-p", "report", "--dry-run")
	for _, want := range []string{latest.RunID, failedA, failedB, "2 failed or unfinished job(s) not included", "--failed --unfinished --append"} {
		if !strings.Contains(preview.Stdout, want) {
			t.Errorf("retry preview missing %q:\n%s", want, preview.Stdout)
		}
	}
	retry := e.MustRotari("retry", "-p", "report")
	for _, want := range []string{
		"Retry source: current queue; latest run " + latest.RunID + " has 2 failed or unfinished job(s) not included",
		failedA,
		failedB,
		"--failed --unfinished --append",
	} {
		if !strings.Contains(retry.Stdout, want) {
			t.Errorf("retry output missing %q:\n%s", want, retry.Stdout)
		}
	}
	if strings.Contains(retry.Stdout, "run_id="+latest.RunID) {
		t.Fatalf("source report uses the new run ID label and could confuse the client: %s", retry.Stdout)
	}
}

func TestBlockedOriginJobsDoNotRewindWebTimeline(t *testing.T) {
	covers(t, "WEB-1")
	e := support.NewEnv(t)
	rootJob := support.AddedJobID(t, e.MustRotari("add", "-p", "timeline", "--job-name", "root", "--", "true"))
	support.AddedJobID(t, e.MustRotari("add", "-p", "timeline", "--job-name", "dependent", "--depends-on", "root", "--", "true"))
	e.MustRotari("run", "-p", "timeline", "--quiet")
	source := readSummary(t, e, "timeline")
	e.MustRotari("copy", "-p", "timeline", "--run-id", source.RunID, "--overwrite", "--quiet")
	e.MustRotari("change", "-p", "timeline", "--job-id", rootJob, "--", "false")
	if result := e.Rotari("run", "-p", "timeline", "--quiet"); result.Code == 0 {
		t.Fatalf("rerun with a failed dependency unexpectedly succeeded: %s", result)
	}
	current := readSummary(t, e, "timeline")
	response := e.HTTPGet(e.StartWeb() + "/api/run?project_name=" + url.QueryEscape("timeline") + "&run_id=" + url.QueryEscape(current.RunID))
	if response.Status != 200 {
		t.Fatalf("GET rerun details: status %d: %s", response.Status, response.Body)
	}
	var detail struct {
		Timeline []struct {
			At string `json:"at"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal([]byte(response.Body), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Timeline) < 2 {
		t.Fatalf("timeline = %#v, want start and completion points", detail.Timeline)
	}
	for index := 1; index < len(detail.Timeline); index++ {
		if detail.Timeline[index].At < detail.Timeline[index-1].At {
			t.Fatalf("timeline times move backwards: %#v", detail.Timeline)
		}
	}
}

func TestRunTimelineStartsAtActualRunStart(t *testing.T) {
	covers(t, "WEB-1")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "timeline", "--", "sh", "-c", "sleep 2")
	e.MustRotari("run", "-p", "timeline", "--quiet")

	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			StartedAt string `json:"started_at"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "timeline", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	response := e.HTTPGet(e.StartWeb() + "/api/run?project_name=" + url.QueryEscape("timeline") + "&run_id=" + url.QueryEscape(shown.RunID))
	if response.Status != 200 {
		t.Fatalf("GET run timeline: status %d: %s", response.Status, response.Body)
	}
	var detail struct {
		Timeline []struct {
			At string `json:"at"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal([]byte(response.Body), &detail); err != nil {
		t.Fatal(err)
	}
	if shown.Summary.StartedAt == "" || len(detail.Timeline) < 2 {
		t.Fatalf("run start/timeline = %q / %#v, want a start and job event", shown.Summary.StartedAt, detail.Timeline)
	}
	if detail.Timeline[0].At > shown.Summary.StartedAt {
		t.Fatalf("first timeline point = %q, after run start %q", detail.Timeline[0].At, shown.Summary.StartedAt)
	}
	if detail.Timeline[0].At > detail.Timeline[1].At {
		t.Fatalf("timeline begins at %q after its first job event %q", detail.Timeline[0].At, detail.Timeline[1].At)
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

func TestFingerprintMatchingIgnoresNonInputMetadata(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)

	e.MustRotari("add", "-p", "metadata", "--job-name", "anchor", "--", "true")
	targetID := support.AddedJobID(t, e.MustRotari("add", "-p", "metadata", "--job-name", "target", "--", "true"))
	e.MustRotari("run", "-p", "metadata", "--quiet")
	source := readSummary(t, e, "metadata")
	e.MustRotari("copy", "-p", "metadata", "--run-id", source.RunID, "--overwrite", "--quiet")
	e.MustRotari("change", "-p", "metadata", "--job-id", targetID,
		"--set-job-name", "renamed", "--executor", "local", "--depends-on", "anchor",
		"--timeout", "1h", "--retry", "1", "--retry-delay", "1s", "--retry-backoff", "2", "--retry-max-delay", "1m", "--quiet")
	e.MustRotari("run", "-p", "metadata", "--match-by", "fingerprint", "--quiet")
	run := readSummary(t, e, "metadata")
	snapshot := readCommandSnapshot(t, e, "metadata", run.RunID)
	command := commandByID(t, snapshot, targetID)
	if command.Origin == nil || command.Origin.JobID != targetID {
		t.Fatalf("non-input metadata prevented fingerprint matching: %#v", command)
	}
}

func TestFingerprintMatchingTreatsMissingHistoryAsNewWork(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)

	e.MustRotari("add", "-p", "missing", "--", "true")
	e.MustRotari("run", "-p", "missing", "--quiet")
	source := readSummary(t, e, "missing")
	if err := os.Remove(filepath.Join(e.Base, "projects", "missing", "runs", source.RunID, "commands.json")); err != nil {
		t.Fatal(err)
	}
	currentID := support.AddedJobID(t, e.MustRotari("add", "-p", "missing", "--", "true"))
	e.MustRotari("run", "-p", "missing", "--match-by", "fingerprint", "--quiet")
	run := readSummary(t, e, "missing")
	command := commandByID(t, readCommandSnapshot(t, e, "missing", run.RunID), currentID)
	if command.Origin != nil {
		t.Fatalf("job matched a run without a command snapshot: %#v", command)
	}
}

func TestFingerprintMatchingNormalizesDirectoryAndIgnoresArrayRange(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)
	workDir := filepath.Join(e.Root, "work")
	if err := os.MkdirAll(filepath.Join(workDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	e.MustRotari("add", "-p", "range", "--array", "1-2", "--working-directory", workDir, "--", "true")
	e.MustRotari("run", "-p", "range", "--quiet")
	currentID := support.AddedJobID(t, e.MustRotari("add", "-p", "range", "--array", "1-3", "--working-directory", workDir+"/sub/../.", "--", "true"))
	e.MustRotari("run", "-p", "range", "--match-by", "fingerprint", "--quiet")
	run := readSummary(t, e, "range")
	command := commandByID(t, readCommandSnapshot(t, e, "range", run.RunID), currentID)
	if len(command.TaskOrigins) != 2 {
		t.Fatalf("task origins = %#v, want tasks 1 and 2 matched", command.TaskOrigins)
	}
	for task := range command.TaskOrigins {
		if task == "3" || strings.HasSuffix(task, "-3") {
			t.Fatalf("task 3 has no historical unit but matched: %#v", command.TaskOrigins)
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

func TestFingerprintMatchingRejectsChangedCommand(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)

	e.MustRotari("add", "-p", "command", "--", "true")
	e.MustRotari("run", "-p", "command", "--quiet")
	currentID := support.AddedJobID(t, e.MustRotari("add", "-p", "command", "--", "echo", "changed"))
	e.MustRotari("run", "-p", "command", "--match-by", "fingerprint", "--quiet")
	run := readSummary(t, e, "command")
	command := commandByID(t, readCommandSnapshot(t, e, "command", run.RunID), currentID)
	if command.Origin != nil {
		t.Fatalf("changed command unexpectedly matched its source: %#v", command)
	}
}

func TestFingerprintMatchingRejectsChangedMatrixValue(t *testing.T) {
	covers(t, "RUN-4")
	e := support.NewEnv(t)

	e.MustRotari("add", "-p", "matrix-input", "--matrix", "SEED=1,2", "--", "true")
	e.MustRotari("run", "-p", "matrix-input", "--quiet")
	e.MustRotari("add", "-p", "matrix-input", "--matrix", "SEED=1,3", "--", "true")
	e.MustRotari("run", "-p", "matrix-input", "--match-by", "fingerprint", "--quiet")
	run := readSummary(t, e, "matrix-input")
	snapshot := readCommandSnapshot(t, e, "matrix-input", run.RunID)
	originsByValue := make(map[string]*struct {
		JobID string `json:"job_id"`
	})
	for _, command := range snapshot.Commands {
		if command.Matrix == nil || len(command.Matrix.Values) != 1 || command.Matrix.Values[0].Name != "SEED" {
			t.Fatalf("unexpected matrix execution unit: %#v", command)
		}
		originsByValue[command.Matrix.Values[0].Value] = command.Origin
	}
	if originsByValue["1"] == nil {
		t.Fatalf("unchanged matrix value did not match its source: %#v", snapshot.Commands)
	}
	if origin := originsByValue["3"]; origin != nil {
		t.Fatalf("changed matrix value unexpectedly matched: %#v", origin)
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

func TestWorkflowMatrixExclusionExportImport(t *testing.T) {
	covers(t, "RUN-8")
	e := support.NewEnv(t)
	withoutMatrix := e.Rotari("add", "-p", "invalid", "--matrix-exclude", "SEED=1", "--", "true")
	if withoutMatrix.Code == 0 || !strings.Contains(withoutMatrix.Stderr, "--matrix-exclude requires --matrix") {
		t.Fatalf("add without --matrix result = %s, want a matrix requirement error", withoutMatrix)
	}
	manifestPath := filepath.Join(e.Root, "matrix.yaml")
	manifest := `version: 1
jobs:
  - name: train
    command: [sh, -c, 'test "$SEED:$MODEL" != "2:large"']
    matrix:
      SEED: [1, 2]
      MODEL: [small, large]
    matrix_exclude:
      - SEED: 2
        MODEL: large
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("import", manifestPath, "source")
	exportedPath := filepath.Join(e.Root, "exported.yaml")
	e.MustRotari("export", "source", exportedPath)
	exported, err := os.ReadFile(exportedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(exported), "matrix_exclude:") || !strings.Contains(string(exported), "MODEL: large") {
		t.Fatalf("queue export lost matrix_exclude:\n%s", exported)
	}
	e.MustRotari("import", exportedPath, "copy")
	e.MustRotari("run", "-p", "copy", "--quiet")
	summary := readSummary(t, e, "copy")
	if len(summary.Results) != 3 {
		t.Fatalf("imported matrix produced %d results, want 3 after exclusion: %#v", len(summary.Results), summary.Results)
	}
	runExportPath := filepath.Join(e.Root, "run-exported.yaml")
	e.MustRotari("export", summary.RunID, runExportPath)
	runExported, err := os.ReadFile(runExportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runExported), "matrix_exclude:") {
		t.Fatalf("run export lost matrix_exclude:\n%s", runExported)
	}
	e.MustRotari("import", "--overwrite", runExportPath, "copy")
	e.MustRotari("run", "-p", "copy", "--quiet")
	reusedSummary := readSummary(t, e, "copy")
	if len(reusedSummary.Results) != 3 {
		t.Fatalf("run round trip produced %d results, want 3", len(reusedSummary.Results))
	}
	e.MustRotari(
		"add", "-p", "cli-source", "--job-name", "train",
		"--matrix", "SEED=1,2", "--matrix", "MODEL=small,large",
		"--matrix-exclude", "SEED=2,MODEL=large", "--", "true",
	)
	cliExportPath := filepath.Join(e.Root, "cli-exported.yaml")
	e.MustRotari("export", "cli-source", cliExportPath)
	cliExported, err := os.ReadFile(cliExportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cliExported), "matrix_exclude:") {
		t.Fatalf("CLI matrix exclusion was not retained in export:\n%s", cliExported)
	}
	e.MustRotari("import", cliExportPath, "cli-copy")
	e.MustRotari("run", "-p", "cli-copy", "--quiet")
	cliSummary := readSummary(t, e, "cli-copy")
	if len(cliSummary.Results) != 3 {
		t.Fatalf("CLI matrix export/import produced %d results, want 3", len(cliSummary.Results))
	}
}

// A matrix or array job has no status of its own: export writes statuses
// only under instances, and import rejects an edited job-level status
// rather than ignoring it. An older export's job-level status, the aggregate
// of the source results, still imports, and with every instance removed the
// members keep their source results: retry executes only the failed leaves.
func TestImportedGroupStatusKeepsUnlistedLeafResults(t *testing.T) {
	covers(t, "RUN-10")
	e := support.NewEnv(t)
	logPath := filepath.Join(e.Root, "executed.log")
	e.MustRotari("add", "-p", "groups", "--job-name", "train", "--matrix", "SEED=1,2", "--",
		"sh", "-c", `echo train$SEED >> "$0"; [ "$SEED" = 1 ]`, logPath)
	e.MustRotari("add", "-p", "groups", "--job-name", "tasks", "--array", "1-2", "--",
		"sh", "-c", `echo task$ROTARI_ARRAY_TASK_ID >> "$0"; [ "$ROTARI_ARRAY_TASK_ID" = 1 ]`, logPath)
	e.Rotari("run", "-p", "groups", "--quiet")
	manifestPath := filepath.Join(e.Root, "groups.json")
	e.MustRotari("export", "-p", "groups", "--format", "json", "--output", manifestPath)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	writeGroups := func(edit func(fields map[string]any)) {
		t.Helper()
		var manifest map[string]any
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		for _, job := range manifest["jobs"].([]any) {
			edit(job.(map[string]any))
		}
		edited, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, edited, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeGroups(func(fields map[string]any) {
		if _, ok := fields["status"]; ok || fields["instances"] == nil {
			t.Fatalf("exported group = %v, want instances and no job-level status", fields)
		}
		fields["status"] = "success"
	})
	if rejected := e.Rotari("import", "--overwrite", manifestPath, "groups"); rejected.Code == 0 || !strings.Contains(rejected.Stderr, "instances") {
		t.Fatalf("import with an edited group status = %s, want an error pointing to instances", rejected)
	}
	writeGroups(func(fields map[string]any) {
		fields["status"] = "failed"
		delete(fields, "instances")
	})
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("import", "--overwrite", manifestPath, "groups")
	e.Rotari("retry", "-p", "groups", "--quiet")
	executed, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(executed)); strings.Join(got, ",") != "train2,task2" && strings.Join(got, ",") != "task2,train2" {
		t.Fatalf("executed leaves = %v, want only the failed train2 and task2", got)
	}
}

// A task added by widening an exported array has no source result, so it
// is not accepted as success: retry executes it along with the failed task.
func TestImportedArrayWideningExecutesNewTasks(t *testing.T) {
	covers(t, "RUN-10")
	e := support.NewEnv(t)
	logPath := filepath.Join(e.Root, "executed.log")
	e.MustRotari("add", "-p", "widen", "--job-name", "tasks", "--array", "1-2", "--",
		"sh", "-c", `echo task$ROTARI_ARRAY_TASK_ID >> "$0"; [ "$ROTARI_ARRAY_TASK_ID" != 2 ]`, logPath)
	e.Rotari("run", "-p", "widen", "--quiet")
	manifestPath := filepath.Join(e.Root, "widen.json")
	e.MustRotari("export", "-p", "widen", "--format", "json", "--output", manifestPath)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	widened := strings.Replace(string(data), `"array": "1-2"`, `"array": "1-3"`, 1)
	if widened == string(data) {
		t.Fatalf("exported manifest has no array 1-2:\n%s", data)
	}
	if err := os.WriteFile(manifestPath, []byte(widened), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	e.MustRotari("import", "--overwrite", manifestPath, "widen")
	e.Rotari("retry", "-p", "widen", "--quiet")
	executed, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(executed))
	if len(got) != 2 || !strings.Contains(string(executed), "task2") || !strings.Contains(string(executed), "task3") {
		t.Fatalf("executed tasks = %v, want the failed task2 and the new task3", got)
	}
}

// A rule naming one dimension twice is rejected by the CLI and by every
// manifest format, rather than keeping one of the values.
func TestMatrixExclusionRejectsRepeatedDimension(t *testing.T) {
	covers(t, "RUN-8")
	e := support.NewEnv(t)
	cli := e.Rotari("add", "-p", "cli", "--matrix", "SEED=1,2", "--matrix", "MODEL=small,large", "--matrix-exclude", "SEED=2,SEED=1", "--", "true")
	if cli.Code == 0 || !strings.Contains(cli.Stderr, `"SEED"`) {
		t.Fatalf("add with a repeated exclusion dimension = %s, want an error naming SEED", cli)
	}
	for format, manifest := range map[string]string{
		"json": `{"version":1,"jobs":[{"command":["true"],"matrix":["SEED=1,2","MODEL=small,large"],"matrix_exclude":[{"SEED":"2","SEED":"1"}]}]}`,
		"yaml": "version: 1\njobs:\n  - command: [true]\n    matrix: [\"SEED=1,2\", \"MODEL=small,large\"]\n    matrix_exclude:\n      - {SEED: 2, SEED: 1}\n",
		"toml": "version = 1\n[[jobs]]\ncommand = [\"true\"]\nmatrix = [\"SEED=1,2\", \"MODEL=small,large\"]\nmatrix_exclude = [{ SEED = \"2\", SEED = \"1\" }]\n",
	} {
		manifestPath := filepath.Join(e.Root, "repeated."+format)
		if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
		result := e.Rotari("import", manifestPath, "manifest-"+format)
		if result.Code == 0 || !strings.Contains(result.Stderr, "SEED") {
			t.Errorf("%s import with a repeated exclusion dimension = %s, want an error naming SEED", format, result)
		}
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

func TestArrayNameDependsOnEveryTask(t *testing.T) {
	covers(t, "RUN-7")
	e := support.NewEnv(t)
	// Task 2 fails, so the array as a whole does not succeed.
	e.MustRotari("add", "-p", "deps", "--job-name", "train", "--array", "1-2", "--", "sh", "-c", `test "$ROTARI_ARRAY_TASK_ID" = 1`)
	collect := support.AddedJobID(t, e.MustRotari("add", "-p", "deps", "--job-name", "collect", "--depends-on-finished", "train", "--", "true"))
	deploy := support.AddedJobID(t, e.MustRotari("add", "-p", "deps", "--job-name", "deploy", "--depends-on", "train", "--", "true"))
	e.MustRotari("check", "deps")
	if result := e.Rotari("run", "-p", "deps", "--quiet"); result.Code == 0 {
		t.Fatalf("run with a failing task should exit 1: %s", result)
	}
	summary := readSummary(t, e, "deps")
	results := map[string]string{}
	for _, result := range summary.Results {
		results[result.ID] = fmt.Sprintf("exit %d %s", result.ExitCode, result.Error)
	}
	if got := results[collect]; got != "exit 0 " {
		t.Errorf("collect, after every train task finished: %q, want exit 0", got)
	}
	if got := results[deploy]; !strings.HasPrefix(got, "exit 1 blocked") {
		t.Errorf("deploy, after a failed train task: %q, want blocked", got)
	}
}

// TestRerunOfAnInterruptedRun checks that an interrupted run, which never
// wrote summary.json, can be the source of a filtered rerun after unlock:
// jobs that finished keep their results and jobs cut off stay unfinished.
func TestRerunOfAnInterruptedRun(t *testing.T) {
	covers(t, "RUN-12")
	e := support.NewEnv(t)
	okJob := support.AddedJobID(t, e.MustRotari("add", "-p", "live", "--job-name", "ok", "--", "true"))
	badJob := support.AddedJobID(t, e.MustRotari("add", "-p", "live", "--job-name", "bad", "--", "sh", "-c", "exit 3"))
	run := e.StartRun("live", 1, false)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		listed := e.Rotari("jobs", "--basedir", e.Base, "live", "--format", "%a %s").Stdout
		return strings.Count(listed, " success") == 1 && strings.Count(listed, " failed") == 1, "jobs: " + listed
	})
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	e.MustRotari("unlock", "live", "--run-id", run.RunID)

	for _, test := range []struct {
		args []string
		want []string
	}{
		{[]string{"retry", "-p", "live", "--run-id", run.RunID, "--dry-run"}, []string{badJob, run.Jobs[0]}},
		{[]string{"retry", "-p", "live", "--run-id", run.RunID, "--failed", "--dry-run"}, []string{badJob}},
		{[]string{"retry", "-p", "live", "--run-id", run.RunID, "--unfinished", "--dry-run"}, []string{run.Jobs[0]}},
	} {
		r := e.Rotari(test.args...)
		if r.Code != 0 {
			t.Errorf("%v: %s", test.args, r)
			continue
		}
		executed := map[string]bool{}
		for _, line := range strings.Split(r.Stdout, "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "execute job_id="); ok {
				executed[strings.Fields(rest)[0]] = true
			}
		}
		if executed[okJob] || len(executed) != len(test.want) {
			t.Errorf("%v executes %v, want %v: %s", test.args, executed, test.want, r)
		}
		for _, id := range test.want {
			if !executed[id] {
				t.Errorf("%v does not execute %s: %s", test.args, id, r)
			}
		}
	}
}

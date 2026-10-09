package supervisor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func progressTestOperations(t *testing.T) (Operations, state.ProjectPaths) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o755, 0o644)
	return Operations{
		BaseDir: paths.BaseDir, NewRunID: func() string { return "run-1" },
		Runner: projectrun.Runner{Store: store, Executors: executor.NewRegistry(store, func(string, ...any) {})},
	}, paths
}

// Decode through JSON to verify that the persisted model event retains every
// field and value of the original transport response, not just its message.
func progressResponses(t *testing.T, runDir string) []server.Response {
	t.Helper()
	var cursor state.ProgressCursor
	events, err := cursor.Read(runDir)
	if err != nil {
		t.Fatal(err)
	}
	responses := make([]server.Response, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		var response server.Response
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
	return responses
}

func TestRunProgressJournalSyncAsyncAndQuiet(t *testing.T) {
	var baseline []server.Response
	for _, async := range []bool{false, true} {
		for _, quiet := range []bool{false, true} {
			t.Run(fmt.Sprintf("async=%t/quiet=%t", async, quiet), func(t *testing.T) {
				events := testProgressJournalRun(t, async, quiet)
				if baseline == nil {
					baseline = events
				} else if !reflect.DeepEqual(events, baseline) {
					t.Fatalf("sync/async/quiet stream differs:\n%+v\n%+v", events, baseline)
				}
			})
		}
	}
}

func executeProgressTestRun(t *testing.T, ops Operations, request server.Request, wantCode int) []server.Response {
	t.Helper()
	if request.Async {
		done := make(chan struct{})
		id, message, err := ops.StartRun(request, func() { close(done) })
		if err != nil || id != "run-1" || !strings.Contains(message, "Wait for it:\n  rotari wait -r run-1") ||
			!strings.Contains(message, "Check status:\n  rotari show -r run-1") ||
			!strings.Contains(message, "Cancel run:\n  rotari cancel -p demo run-1") || strings.Contains(message, "--basedir") || strings.Contains(message, "--project-name") {
			t.Fatalf("StartRun = %q, %q, %v", id, message, err)
		}
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatal("async run did not finish")
		}
		return nil
	}
	var forwarded []server.Response
	_, code, err := ops.Run(request, func(response server.Response) { forwarded = append(forwarded, response) })
	if err != nil || code != wantCode {
		t.Fatalf("Run = %d, %v", code, err)
	}
	return forwarded
}

func testProgressJournalRun(t *testing.T, async, quiet bool) []server.Response {
	t.Helper()
	ops, paths := progressTestOperations(t)
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "success", Name: "success", Command: []string{"true"}},
		{ID: "failure", DependsOnFinished: []string{"success"}, Command: []string{"sh", "-c", "exit 7"}},
	}}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	request := server.Request{QueueName: "demo", LocalConcurrency: 1, Retry: 1, Quiet: quiet, Async: async, CWD: t.TempDir()}
	forwarded := executeProgressTestRun(t, ops, request, 1)
	runDir := filepath.Join(paths.RunsDir, "run-1")
	events := progressResponses(t, runDir)
	if !async && !reflect.DeepEqual(events, forwarded) {
		t.Fatalf("journal differs from attached stream:\n%+v\n%+v", events, forwarded)
	}
	assertProgressTestEvents(t, events)
	summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
	if err != nil || summary.ExitCode != 1 || len(summary.Results) != 2 {
		t.Fatalf("summary = %+v, %v", summary, err)
	}
	return events
}

func assertProgressTestEvents(t *testing.T, events []server.Response) {
	t.Helper()
	// Run start + three attempt starts + success result + retry notice +
	// retryable failure result + final failure result.
	if len(events) != 8 {
		t.Fatalf("events = %d: %+v", len(events), events)
	}
	wantStart := server.Response{Progress: true, RunID: "run-1", Total: 2, Message: "=== Run started ===\n  Project: demo\n  Run ID: run-1\n  Submitted: 2\n  Excluded: 0\n  Total: 2"}
	if events[0] != wantStart {
		t.Fatalf("start = %+v, want %+v", events[0], wantStart)
	}
	last := events[len(events)-1]
	if last.Completed != 2 || last.Total != 2 || last.Succeeded != 1 || last.Failed != 1 || !strings.Contains(last.Message, "Job failed after retry:") {
		t.Fatalf("final progress = %+v", last)
	}
	if events[4].Message != "Retrying job: attempt=1 job=failure command=[sh -c exit 7]" {
		t.Fatalf("retry event = %+v", events[4])
	}
	for index, event := range events {
		if !event.Progress || index > 0 && event.RunID != "" || event.PID != 0 {
			t.Fatalf("spurious control event: %+v", event)
		}
	}
}

func TestRunProgressJournalFailureIsBestEffort(t *testing.T) {
	for _, async := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%t", async), func(t *testing.T) {
			testProgressJournalFailure(t, async)
		})
	}
}

func testProgressJournalFailure(t *testing.T, async bool) {
	t.Helper()
	ops, paths := progressTestOperations(t)
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	// RegisterRun executes during Begin, before the first event. A directory
	// at the journal path reliably fails even when root.
	ops.Runner.RegisterRun = func(paths state.ProjectPaths, runID string) error {
		return os.Mkdir(filepath.Join(paths.RunsDir, runID, state.ProgressFileName), 0o700)
	}
	var logs []string
	ops.Logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	request := server.Request{QueueName: "demo", LocalConcurrency: 1, Quiet: true, Async: async, CWD: t.TempDir()}
	forwarded := executeProgressTestRun(t, ops, request, 0)
	if !async && len(forwarded) != 3 {
		t.Fatalf("forwarded events = %+v", forwarded)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "progress journal failed") {
		t.Fatalf("journal failure logs = %v", logs)
	}
	summary, err := state.LoadRunSummary(filepath.Join(paths.RunsDir, "run-1", "summary.json"))
	if err != nil || summary.ExitCode != 0 || len(summary.Results) != 1 || summary.Results[0].ExitCode != 0 {
		t.Fatalf("run did not succeed: %+v, %v", summary, err)
	}
}

func TestRunProgressJournalWithoutAttachedClient(t *testing.T) {
	ops, paths := progressTestOperations(t)
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, code, err := ops.Run(server.Request{QueueName: "demo", LocalConcurrency: 1, CWD: t.TempDir()}, nil); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if events := progressResponses(t, filepath.Join(paths.RunsDir, "run-1")); len(events) != 3 {
		t.Fatalf("journal without attached client = %+v", events)
	}
}

func TestRunProgressJournalJobIDCollision(t *testing.T) {
	for _, async := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%t", async), func(t *testing.T) {
			ops, paths := progressTestOperations(t)
			queue := model.Queue{Commands: []model.QueuedCommand{{ID: state.ProgressFileName, Command: []string{"sh", "-c", "printf collision-job-executed"}}}}
			if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
				t.Fatal(err)
			}
			var logs []string
			ops.Logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
			forwarded := executeProgressTestRun(t, ops, server.Request{QueueName: "demo", LocalConcurrency: 1, Async: async, CWD: t.TempDir()}, 0)
			runDir := filepath.Join(paths.RunsDir, "run-1")
			summary, err := state.LoadRunSummary(filepath.Join(runDir, "summary.json"))
			if err != nil || summary.ExitCode != 0 || len(summary.Results) != 1 || summary.Results[0].ID != state.ProgressFileName || summary.Results[0].ExitCode != 0 {
				t.Fatalf("collision job did not succeed: %+v, %v", summary, err)
			}
			attemptDir, err := state.AttemptJobDir(runDir, model.JobSpec{ID: state.ProgressFileName, AttemptID: summary.Results[0].AttemptID})
			if err != nil {
				t.Fatal(err)
			}
			output, err := os.ReadFile(filepath.Join(attemptDir, "output"))
			if err != nil || string(output) != "collision-job-executed" {
				t.Fatalf("job command did not execute: %q, %v", output, err)
			}
			info, err := os.Stat(filepath.Join(runDir, state.ProgressFileName))
			if err != nil || !info.IsDir() {
				t.Fatalf("job directory replaced by journal: %v, %v", info, err)
			}
			var cursor state.ProgressCursor
			if _, err := cursor.Read(runDir); err == nil {
				t.Fatal("reading job directory as journal should fail")
			}
			if len(logs) != 1 || !strings.Contains(logs[0], "progress journal disabled") || !strings.Contains(logs[0], state.ProgressFileName) {
				t.Fatalf("collision warnings = %v", logs)
			}
			if !async && (len(forwarded) != 3 || forwarded[1].JobID != state.ProgressFileName || forwarded[2].Completed != 1 || forwarded[2].Succeeded != 1 || forwarded[2].Failed != 0) {
				t.Fatalf("sync progress not forwarded: %+v", forwarded)
			}
		})
	}
}

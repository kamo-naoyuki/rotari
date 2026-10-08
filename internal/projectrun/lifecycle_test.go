package projectrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func testRunner(t *testing.T) (Runner, state.ProjectPaths) {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(0o755, 0o644)
	return Runner{Store: store, Executors: executor.NewRegistry(store, nil)}, paths
}

func TestBeginRecordsContextBeforeMarkingRunning(t *testing.T) {
	runner, paths := testRunner(t)
	var contextSeen bool
	runner.RegisterRun = func(paths state.ProjectPaths, runID string) error {
		_, err := state.LoadContext(runner.Store, filepath.Join(paths.RunsDir, runID))
		contextSeen = err == nil
		return nil
	}
	if err := runner.Begin(paths, Start{RunID: "run-1", RunName: "nightly", CWD: "/work"}); err != nil {
		t.Fatal(err)
	}
	if !contextSeen {
		t.Fatal("context.json was not written before the run was registered")
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil || lock.RunID != "run-1" || lock.PID != os.Getpid() || lock.Host == "" {
		t.Fatalf("lock = %+v, err = %v", lock, err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil || meta.Phase != "running" || meta.LastRunID != "run-1" {
		t.Fatalf("meta = %+v, err = %v", meta, err)
	}
	context, err := state.LoadContext(runner.Store, filepath.Join(paths.RunsDir, "run-1"))
	if err != nil || context.CWD != "/work" {
		t.Fatalf("context = %+v, err = %v", context, err)
	}
}

func TestBeginRecordsAttachedClientAndCanDetachIt(t *testing.T) {
	runner, paths := testRunner(t)
	if err := runner.Begin(paths, Start{RunID: "run-1", ClientAttached: true, ClientStatus: model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientAttached}}); err != nil {
		t.Fatal(err)
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil || !lock.ClientAttached {
		t.Fatalf("lock after Begin = %+v, %v; want attached client", lock, err)
	}
	status, err := state.LoadRunClientStatus(runner.Store, filepath.Join(paths.RunsDir, "run-1"))
	if err != nil || status.Mode != model.RunClientModeSync || status.State != model.RunClientAttached {
		t.Fatalf("status after Begin = %+v, %v; want sync attached", status, err)
	}
	if err := runner.SetClientAttached(paths, "run-1", false); err != nil {
		t.Fatal(err)
	}
	lock, err = state.LoadLock(paths.LockFile)
	if err != nil || lock.ClientAttached {
		t.Fatalf("lock after detach = %+v, %v; want detached client", lock, err)
	}
	status, err = state.LoadRunClientStatus(runner.Store, filepath.Join(paths.RunsDir, "run-1"))
	if err != nil || status.Mode != model.RunClientModeSync || status.State != model.RunClientDetached {
		t.Fatalf("status after detach = %+v, %v; want sync detached", status, err)
	}
}

func TestClientMetadataCannotPreventRunOrDetach(t *testing.T) {
	for _, collision := range []bool{false, true} {
		t.Run(fmt.Sprint("collision=", collision), func(t *testing.T) {
			runner, paths := testRunner(t)
			runDir := filepath.Join(paths.RunsDir, "run-1")
			jobID := "job"
			if collision {
				jobID = state.RunClientStatusFileName
			}
			if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: jobID, Command: []string{"true"}}}}); err != nil {
				t.Fatal(err)
			}
			if !collision {
				// An unreadable metadata destination must not prevent starting.
				if err := os.MkdirAll(filepath.Join(runDir, state.RunClientStatusFileName), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := runner.Begin(paths, Start{RunID: "run-1", ClientAttached: true, ClientStatus: model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientAttached}}); err != nil {
				t.Fatal(err)
			}
			if collision {
				if err := os.MkdirAll(filepath.Join(runDir, jobID, "attempts"), 0o700); err != nil {
					t.Fatalf("metadata prevented job directory creation: %v", err)
				}
			}
			_ = runner.SetRunClientStatus(paths, "run-1", model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientDetached, Reason: model.RunClientReasonCtrlD})
			lock, err := state.LoadLock(paths.LockFile)
			if err != nil || lock.ClientAttached {
				t.Fatalf("metadata failure prevented detach: lock=%+v error=%v", lock, err)
			}
		})
	}
}

func TestBeginSnapshotsLoadedConfigWithCanonicalName(t *testing.T) {
	runner, paths := testRunner(t)
	configPath := filepath.Join(t.TempDir(), "chosen.toml")
	notificationPath := filepath.Join(t.TempDir(), "notifications.toml")
	for path, content := range map[string]string{
		configPath:       "[run]\nretry = 2\n",
		notificationPath: "[webhook]\nrun_failure = false\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runner.ConfigPaths = func(_ state.ProjectPaths, loadedConfig string) []string {
		return []string{loadedConfig, notificationPath}
	}
	if err := runner.Begin(paths, Start{RunID: "run-1", ConfigPath: configPath}); err != nil {
		t.Fatal(err)
	}
	context, err := state.LoadContext(runner.Store, filepath.Join(paths.RunsDir, "run-1"))
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{configPath, notificationPath}
	if !reflect.DeepEqual(context.ConfigPaths, wantPaths) {
		t.Fatalf("config paths = %#v, want %#v", context.ConfigPaths, wantPaths)
	}
	wantFiles := []string{"config.toml", "notifications.toml"}
	if !reflect.DeepEqual(context.ConfigSnapshotFiles, wantFiles) {
		t.Fatalf("config snapshot files = %#v, want %#v", context.ConfigSnapshotFiles, wantFiles)
	}
	for name, want := range map[string]string{
		"config.toml":        "[run]\nretry = 2\n",
		"notifications.toml": "[webhook]\nrun_failure = false\n",
	} {
		data, err := os.ReadFile(filepath.Join(paths.RunsDir, "run-1", "configs", name))
		if err != nil || string(data) != want {
			t.Fatalf("snapshot %s = %q, err = %v; want %q", name, data, err, want)
		}
	}
}

func TestBeginRollsBackWhenRegistrationFails(t *testing.T) {
	runner, paths := testRunner(t)
	runner.RegisterRun = func(state.ProjectPaths, string) error { return errors.New("registry full") }
	err := runner.Begin(paths, Start{RunID: "run-1"})
	if err == nil || !strings.Contains(err.Error(), "registry full") {
		t.Fatalf("Begin error = %v", err)
	}
	if _, err := os.Stat(paths.LockFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run lock remains after failed Begin: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run directory remains after failed Begin: %v", err)
	}
	if meta, err := state.LoadMeta(paths.MetaFile); err != nil || meta.Phase == "running" {
		t.Fatalf("meta = %+v, err = %v", meta, err)
	}
}

func TestBeginMovesTheQueueIntoTheRun(t *testing.T) {
	runner, paths := testRunner(t)
	queue := model.Queue{DefaultExecutor: "local", DefaultExecutorOptions: []string{"--x"}, Commands: []model.QueuedCommand{
		{ID: "job-1", Command: []string{"true"}},
		{ID: "job-2", Command: []string{"false"}},
	}}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := runner.Begin(paths, Start{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.ReadQueueFile(filepath.Join(paths.RunsDir, "run-1", "commands.json"))
	if err != nil || !reflect.DeepEqual(snapshot.Commands, queue.Commands) || snapshot.DefaultExecutor != "local" {
		t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
	}
	left, err := state.LoadQueue(paths.QueueFile)
	if err != nil || len(left.Commands) != 0 {
		t.Fatalf("queue after Begin = %+v, err = %v", left, err)
	}
	if left.DefaultExecutor != "local" || !reflect.DeepEqual(left.DefaultExecutorOptions, []string{"--x"}) {
		t.Fatalf("queue defaults after Begin = %+v", left)
	}
}

func TestBeginKeepsTheQueueWhenItFails(t *testing.T) {
	runner, paths := testRunner(t)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	runner.RegisterRun = func(state.ProjectPaths, string) error { return errors.New("registry full") }
	if err := runner.Begin(paths, Start{RunID: "run-1"}); err == nil {
		t.Fatal("Begin succeeded")
	}
	left, err := state.LoadQueue(paths.QueueFile)
	if err != nil || !reflect.DeepEqual(left.Commands, queue.Commands) {
		t.Fatalf("queue after failed Begin = %+v, err = %v", left, err)
	}
}

func TestBeginAndFinishKeepQueueForSourceSnapshot(t *testing.T) {
	runner, paths := testRunner(t)
	queued := model.Queue{DefaultExecutor: "local", Commands: []model.QueuedCommand{{ID: "next", Command: []string{"true"}}}}
	if err := state.WriteJSON(paths.QueueFile, queued); err != nil {
		t.Fatal(err)
	}
	queueBytes, err := os.ReadFile(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Queue{Commands: []model.QueuedCommand{{ID: "saved", Command: []string{"true"}}}}
	if err := runner.Begin(paths, Start{RunID: "run-1", Snapshot: &snapshot}); err != nil {
		t.Fatal(err)
	}
	started, err := state.ReadQueueFile(filepath.Join(paths.RunsDir, "run-1", "commands.json"))
	if err != nil || !reflect.DeepEqual(started.Commands, snapshot.Commands) {
		t.Fatalf("run snapshot = %+v, %v; want %+v", started, err, snapshot)
	}
	if left, err := os.ReadFile(paths.QueueFile); err != nil || !reflect.DeepEqual(left, queueBytes) {
		t.Fatalf("queue bytes after Begin = %q, %v; want unchanged %q", left, err, queueBytes)
	}
	if err := runner.Finish(paths, "run-1", 0); err != nil {
		t.Fatal(err)
	}
	if left, err := os.ReadFile(paths.QueueFile); err != nil || !reflect.DeepEqual(left, queueBytes) {
		t.Fatalf("queue bytes after Finish = %q, %v; want unchanged %q", left, err, queueBytes)
	}
}

// TestRunExecutesOnlyTheJobsItTook checks that jobs queued after a run starts
// are left for the next run: Execute reads the run's snapshot, and Finish no
// longer clears the queue.
func TestRunExecutesOnlyTheJobsItTook(t *testing.T) {
	runner, paths := testRunner(t)
	runner.Executors = executor.NewRegistry(runner.Store, func(string, ...any) {})
	taken := model.Queue{Commands: []model.QueuedCommand{{ID: "taken", Command: []string{"true"}}}}
	if err := state.WriteJSON(paths.QueueFile, taken); err != nil {
		t.Fatal(err)
	}
	if err := runner.Begin(paths, Start{RunID: "run-1", CWD: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	next := model.Queue{Commands: []model.QueuedCommand{{ID: "next", Command: []string{"true"}}}}
	if err := state.WriteJSON(paths.QueueFile, next); err != nil {
		t.Fatal(err)
	}
	if code, err := runner.Run(paths, Options{RunID: "run-1", LocalConcurrency: 1, EnvMode: model.EnvModeNone}, Observer{}); err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if attempts := state.ListAttemptIDs(runDir, "taken"); len(attempts) != 1 {
		t.Fatalf("taken attempts = %v", attempts)
	}
	if attempts := state.ListAttemptIDs(runDir, "next"); len(attempts) != 0 {
		t.Fatalf("job queued after the start ran: %v", attempts)
	}
	left, err := state.LoadQueue(paths.QueueFile)
	if err != nil || !reflect.DeepEqual(left.Commands, next.Commands) {
		t.Fatalf("queue after Finish = %+v, err = %v", left, err)
	}
}

func TestBeginRejectsActiveRun(t *testing.T) {
	runner, paths := testRunner(t)
	if err := runner.Begin(paths, Start{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Begin(paths, Start{RunID: "run-2"}); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second Begin error = %v", err)
	}
}

func TestExecuteSnapshotsCommandsBeforeReplanning(t *testing.T) {
	runner, paths := testRunner(t)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := runner.Begin(paths, Start{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	// The source summary can disappear between the supervisor's preflight
	// plan and Execute's second planning pass.
	_, err := runner.Execute(paths, Options{RunID: "run-1", Selection: "failed", SourceRunID: "missing-run"}, Observer{})
	if err == nil || !strings.Contains(err.Error(), `failed to load run summary for origin run "missing-run"`) {
		t.Fatalf("Execute error = %v", err)
	}
	snapshot, err := state.ReadQueueFile(filepath.Join(paths.RunsDir, "run-1", "commands.json"))
	if err != nil || !reflect.DeepEqual(snapshot.Commands, queue.Commands) {
		t.Fatalf("snapshot = %+v, err = %v", snapshot, err)
	}
}

func TestFinishFinalizesProjectAndRemovesLock(t *testing.T) {
	runner, paths := testRunner(t)
	if err := state.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	var notified []int
	runner.RunFinished = func(_ state.ProjectPaths, runID string, exitCode int) { notified = append(notified, exitCode) }
	if err := runner.Begin(paths, Start{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Finish(paths, "run-1", 2); err != nil {
		t.Fatal(err)
	}
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil || meta.Phase != "finished" || meta.LastRunExitCode != 2 {
		t.Fatalf("meta = %+v, err = %v", meta, err)
	}
	if queue, err := state.LoadQueue(paths.QueueFile); err != nil || len(queue.Commands) != 0 {
		t.Fatalf("queue = %+v, err = %v", queue, err)
	}
	if _, err := os.Stat(paths.LockFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("run lock remains after Finish: %v", err)
	}
	if len(notified) != 1 || notified[0] != 2 {
		t.Fatalf("RunFinished calls = %v", notified)
	}
}

func TestFinishKeepsAnotherRunsLock(t *testing.T) {
	runner, paths := testRunner(t)
	if err := runner.Begin(paths, Start{RunID: "run-2"}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Finish(paths, "run-1", 0); err == nil || !strings.Contains(err.Error(), `belongs to "run-2"`) {
		t.Fatalf("Finish error = %v", err)
	}
	if lock, err := state.LoadLock(paths.LockFile); err != nil || lock.RunID != "run-2" {
		t.Fatalf("lock = %+v, err = %v", lock, err)
	}
	if meta, err := state.LoadMeta(paths.MetaFile); err != nil || meta.Phase != "running" || meta.LastRunID != "run-2" {
		t.Fatalf("meta = %+v, err = %v", meta, err)
	}
}

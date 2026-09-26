package projectrun

import (
	"errors"
	"os"
	"path/filepath"
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

func TestBeginRejectsActiveRun(t *testing.T) {
	runner, paths := testRunner(t)
	if err := runner.Begin(paths, Start{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if err := runner.Begin(paths, Start{RunID: "run-2"}); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second Begin error = %v", err)
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

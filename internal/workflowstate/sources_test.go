package workflowstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

func sourcesFixture(t *testing.T) Sources {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	return Sources{Paths: paths, Store: state.NewStore(0o755, 0o644)}
}

func writeSourceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSourceRun(t *testing.T, paths state.ProjectPaths, runID string) string {
	t.Helper()
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Name: "train", Command: []string{"true"}}}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	summary := model.RunSummary{RunID: runID, Status: "finished", Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}}
	if err := state.WriteJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func TestSourcesRunReadsSnapshotSummaryAndContext(t *testing.T) {
	sources := sourcesFixture(t)
	runDir := writeSourceRun(t, sources.Paths, "run-1")
	writeSourceFile(t, filepath.Join(runDir, "job-1", "submitted_at"), "2026-10-01T00:00:00Z\n")
	writeSourceFile(t, filepath.Join(runDir, "job-1", "finished_at"), "2026-10-01T00:01:00Z\n")

	run, err := sources.Run("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.ID != "run-1" || len(run.Queue.Commands) != 1 || run.Summary.Status != "finished" || run.CWD != "" {
		t.Fatalf("run without context = %+v", run)
	}
	if submitted, finished := run.JobTimestamps("job-1"); submitted != "2026-10-01T00:00:00Z" || finished != "2026-10-01T00:01:00Z" {
		t.Fatalf("JobTimestamps = %q, %q", submitted, finished)
	}
	if err := state.SaveContext(sources.Store, runDir, model.RunContext{CWD: "/work"}); err != nil {
		t.Fatal(err)
	}
	if run, err = sources.Run("run-1"); err != nil || run.CWD != "/work" {
		t.Fatalf("run with context = %+v, %v", run, err)
	}
}

func TestSourcesRunRejectsUnreadableRuns(t *testing.T) {
	sources := sourcesFixture(t)
	runDir := writeSourceRun(t, sources.Paths, "run-1")
	if _, err := sources.Run("../run-1"); err == nil || !strings.Contains(err.Error(), "invalid source run ID") {
		t.Fatalf("Run(unsafe) error = %v", err)
	}
	writeSourceFile(t, filepath.Join(runDir, "summary.json"), "{")
	if _, err := sources.Run("run-1"); err == nil || !strings.Contains(err.Error(), "summary") {
		t.Fatalf("Run(bad summary) error = %v", err)
	}
	writeSourceFile(t, filepath.Join(runDir, "commands.json"), "{")
	if _, err := sources.Run("run-1"); err == nil || !strings.Contains(err.Error(), "commands") {
		t.Fatalf("Run(bad commands) error = %v", err)
	}
}

func TestSourcesAttemptReadsLocalAndWrapperResults(t *testing.T) {
	sources := sourcesFixture(t)
	runDir := writeSourceRun(t, sources.Paths, "run-1")
	attempts := filepath.Join(runDir, "job-1", "attempts")

	// A local attempt records its exit code in status once finished_at exists.
	writeSourceFile(t, filepath.Join(attempts, "local", "status"), "3\n")
	writeSourceFile(t, filepath.Join(attempts, "local", "finished_at"), "2026-10-01T00:01:00Z\n")
	// A scheduler attempt records a wrapper status.json.
	writeWrapper := func(attemptID string, status executor.WrapperStatus) {
		if err := state.WriteJSON(filepath.Join(attempts, attemptID, "status.json"), status); err != nil {
			t.Fatal(err)
		}
	}
	writeWrapper("wrapped", executor.WrapperStatus{Phase: "failed", ExitCode: 4, Error: "boom", Hosts: []string{"node-1"}})
	writeWrapper("running", executor.WrapperStatus{Phase: "running"})
	if err := os.MkdirAll(filepath.Join(attempts, "pending"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		attemptID string
		want      model.JobResult
		found     bool
	}{
		{"local", model.JobResult{ID: "job-1", AttemptID: "local", Command: []string{"cmd"}, ExitCode: 3}, true},
		{"wrapped", model.JobResult{ID: "job-1", AttemptID: "wrapped", ExitCode: 4, Error: "boom", Hosts: []string{"node-1"}}, true},
		{"running", model.JobResult{}, false},
		{"pending", model.JobResult{}, false},
	} {
		result, found, err := sources.Attempt("run-1", "job-1", test.attemptID, []string{"cmd"})
		if err != nil || found != test.found || !reflect.DeepEqual(result, test.want) {
			t.Errorf("Attempt(%s) = %+v, %v, %v; want %+v, %v", test.attemptID, result, found, err, test.want, test.found)
		}
	}
	for _, args := range [][3]string{{"../run", "job-1", "local"}, {"run-1", "../job", "local"}, {"run-1", "job-1", "a/b"}, {"run-1", "job-1", "missing"}} {
		if _, _, err := sources.Attempt(args[0], args[1], args[2], nil); err == nil {
			t.Errorf("Attempt%v accepted", args)
		}
	}
}

func TestSourcesAttemptTimestamps(t *testing.T) {
	sources := sourcesFixture(t)
	runDir := writeSourceRun(t, sources.Paths, "run-1")
	writeSourceFile(t, filepath.Join(runDir, "job-1", "attempts", "first", "submitted_at"), "attempt-submitted\n")
	writeSourceFile(t, filepath.Join(runDir, "job-1", "attempts", "first", "finished_at"), "attempt-finished\n")
	writeSourceFile(t, filepath.Join(runDir, "job-2", "submitted_at"), "job-submitted\n")

	if submitted, finished := sources.AttemptTimestamps("run-1", "job-1", "first"); submitted != "attempt-submitted" || finished != "attempt-finished" {
		t.Fatalf("attempt timestamps = %q, %q", submitted, finished)
	}
	// An invalid attempt ID falls back to the job's latest attempt.
	if submitted, finished := sources.AttemptTimestamps("run-1", "job-2", "a/b"); submitted != "job-submitted" || finished != "" {
		t.Fatalf("job timestamps = %q, %q", submitted, finished)
	}
	if submitted, finished := sources.AttemptTimestamps("../run-1", "job-1", "first"); submitted != "" || finished != "" {
		t.Fatalf("unsafe run timestamps = %q, %q", submitted, finished)
	}
}

func TestReconcileWithoutSourceKeepsQueue(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}
	got, removed, err := Reconcile(state.NewStore(0o755, 0o644), t.TempDir(), workflow.Manifest{}, queue)
	if err != nil || removed != nil || !reflect.DeepEqual(got, queue) {
		t.Fatalf("Reconcile = %+v, %v, %v", got, removed, err)
	}
	manifest := workflow.Manifest{Source: &workflow.Source{Project: "../outside"}}
	if _, _, err := Reconcile(state.NewStore(0o755, 0o644), t.TempDir(), manifest, queue); err == nil {
		t.Fatal("Reconcile accepted an unsafe source project")
	}
}

func TestLoadSettledRunRefusesUnsettledRuns(t *testing.T) {
	sources := sourcesFixture(t)
	paths := sources.Paths
	writeSourceRun(t, paths, "run-1")
	accept := func(model.Queue) error { return nil }

	run, err := LoadSettledRun(paths, "run-1", accept, false)
	if err != nil || run.ID != "run-1" || len(run.Queue.Commands) != 1 {
		t.Fatalf("LoadSettledRun(settled) = %+v, %v", run, err)
	}
	if _, err := LoadSettledRun(paths, "run-1", func(model.Queue) error { return errors.New("bad executor") }, false); err == nil || !strings.Contains(err.Error(), "invalid run run-1 commands: bad executor") {
		t.Fatalf("LoadSettledRun(invalid) error = %v", err)
	}
	if _, err := LoadSettledRun(paths, "missing", accept, false); err == nil || !strings.Contains(err.Error(), "failed to load run missing") {
		t.Fatalf("LoadSettledRun(missing) error = %v", err)
	}
	if _, err := LoadSettledRun(paths, "../run-1", accept, false); err == nil {
		t.Fatal("LoadSettledRun accepted an unsafe run ID")
	}

	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSettledRun(paths, "run-1", accept, false); err == nil || !strings.Contains(err.Error(), "was interrupted; recover it with 'rotari unlock demo'") {
		t.Fatalf("LoadSettledRun(interrupted) error = %v", err)
	}
	if err := RequireSettledRun(paths, "run-0", false); err != nil {
		t.Fatalf("another run of an interrupted project = %v", err)
	}

	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := RequireSettledRun(paths, "", false); err == nil || !strings.Contains(err.Error(), "is still running; wait for it with 'rotari wait demo'") {
		t.Fatalf("RequireSettledRun(running) error = %v", err)
	}

	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "exploded"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.LockFile); err != nil {
		t.Fatal(err)
	}
	if err := RequireSettledRun(paths, "", false); err == nil || !strings.Contains(err.Error(), "failed to check project state") {
		t.Fatalf("RequireSettledRun(bad meta) error = %v", err)
	}
}

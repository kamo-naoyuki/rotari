package resolve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type runLocation = runregistry.Location

func registerRunLocation(location runLocation) error {
	registry, err := runregistry.Default()
	if err != nil {
		return err
	}
	return registry.Register(location)
}

func TestJobSelectionPassesThroughPlainJobIDs(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := JobSelection(baseDir, "demo", []string{"job-1", "job-2"})
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseDir != baseDir || got.ProjectName != "demo" || got.RunID != "" || strings.Join(got.JobIDs, ",") != "job-1,job-2" {
		t.Fatalf("got = %+v", got)
	}
}

func TestJobSelectionResolvesBareRunIDAndStripsIt(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	runID := "20260922-000000-00000000"
	if err := registerRunLocation(runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", runID), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := JobSelection("", "", []string{runID, "job-1"})
	if err != nil {
		t.Fatal(err)
	}
	wantBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseDir != wantBaseDir || got.ProjectName != "demo" || got.RunID != runID || strings.Join(got.JobIDs, ",") != "job-1" {
		t.Fatalf("got = %+v", got)
	}
}

func TestJobSelectionResolvesAttemptID(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	runID := "20260922-000000-00000000"
	if err := registerRunLocation(runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: runID}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", runID), 0o755); err != nil {
		t.Fatal(err)
	}
	attemptID := state.MakeAttemptID(runID, "job-1", 0)

	got, err := JobSelection("", "", []string{attemptID})
	if err != nil {
		t.Fatal(err)
	}
	wantBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.BaseDir != wantBaseDir || got.ProjectName != "demo" || got.RunID != runID || strings.Join(got.JobIDs, ",") != attemptID {
		t.Fatalf("got = %+v", got)
	}
}

func TestJobSelectionRejectsMixedRuns(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	otherRunID := "20260922-000000-11111111"
	attemptID := state.MakeAttemptID("20260922-000000-00000000", "job-1", 0)

	if _, err := JobSelection("", "", []string{otherRunID, attemptID}); err == nil ||
		!strings.Contains(err.Error(), "belongs to run") {
		t.Fatalf("JobSelection() error = %v, want run mismatch error", err)
	}
	if _, err := JobSelection("", "", []string{otherRunID, "20260922-000000-00000000"}); err == nil ||
		!strings.Contains(err.Error(), "selection mixes run") {
		t.Fatalf("JobSelection() error = %v, want mixed-run error", err)
	}
}

// TestJobSelectionSearchesActiveRuns checks that a job ID without a project
// is looked for in the active run of every project.
func TestJobSelectionSearchesActiveRuns(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	activeRun := func(project, runID string, jobIDs ...string) {
		paths, err := state.ResolveProjectPaths(baseDir, project)
		if err != nil {
			t.Fatal(err)
		}
		runDir := filepath.Join(paths.RunsDir, runID)
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			t.Fatal(err)
		}
		queue := model.Queue{}
		for _, jobID := range jobIDs {
			queue.Commands = append(queue.Commands, model.QueuedCommand{ID: jobID, Command: []string{"true"}})
		}
		if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
			t.Fatal(err)
		}
		host, _ := os.Hostname()
		if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: runID, Host: host}); err != nil {
			t.Fatal(err)
		}
	}
	activeRun("alpha", "20260922-000000-aaaaaaaa", "job-1", "job-2")
	activeRun("beta", "20260922-000000-bbbbbbbb", "job-2")
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "idle"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := JobSelection(baseDir, "", []string{"job-1", "job-2"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectName != "alpha" || got.RunID != "20260922-000000-aaaaaaaa" || strings.Join(got.JobIDs, ",") != "job-1,job-2" {
		t.Fatalf("got = %+v, want alpha's active run", got)
	}
	if _, err := JobSelection(baseDir, "", []string{"job-2"}); err == nil ||
		!strings.Contains(err.Error(), "project=alpha run=20260922-000000-aaaaaaaa") || !strings.Contains(err.Error(), "project=beta run=20260922-000000-bbbbbbbb") {
		t.Fatalf("JobSelection() error = %v, want both active runs listed", err)
	}
	if _, err := JobSelection(baseDir, "", []string{"job-3"}); err == nil || !strings.Contains(err.Error(), "no active run") {
		t.Fatalf("JobSelection() error = %v, want no active run", err)
	}
	got, err = JobSelection(baseDir, "beta", []string{"job-1"})
	if err != nil || got.ProjectName != "beta" || got.RunID != "" {
		t.Fatalf("JobSelection() with a project = %+v, %v; want beta without a search", got, err)
	}
}

func TestSplitProjectOrRun(t *testing.T) {
	runID := "20260925-120000-12345678"
	project, runIDs, err := SplitProjectOrRun([]string{"demo", runID, "demo"})
	if err != nil || project != "demo" || len(runIDs) != 1 || runIDs[0] != runID {
		t.Fatalf("split = %q, %q, %v", project, runIDs, err)
	}
	if _, _, err := SplitProjectOrRun([]string{"demo", "other"}); err == nil {
		t.Fatal("two different projects were accepted")
	}
}

func TestExistingRunUsesRegistryAndRejectsConflicts(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	location := runLocation{BaseDir: baseDir, ProjectName: "demo", RunID: "run-1"}
	if err := registerRunLocation(location); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo", "runs", "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}

	gotBaseDir, gotProject, err := ExistingRun("", "", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != baseDir || gotProject != "demo" {
		t.Fatalf("target = %q, %q; want %q, demo", gotBaseDir, gotProject, baseDir)
	}
	if _, _, err := ExistingRun(t.TempDir(), "", "run-1"); err == nil {
		t.Fatal("conflicting basedir was accepted")
	}
	if _, _, err := ExistingRun("", "other", "run-1"); err == nil {
		t.Fatal("conflicting project was accepted")
	}
}

func TestExistingRunRejectsStaleRegistryEntry(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	location := runLocation{BaseDir: t.TempDir(), ProjectName: "demo", RunID: "missing-run"}
	if err := registerRunLocation(location); err != nil {
		t.Fatal(err)
	}

	_, _, err := ExistingRun("", "", location.RunID)
	if err == nil || !strings.Contains(err.Error(), "run \"missing-run\" is registered but its run directory is missing") {
		t.Fatalf("ExistingRun() error = %v, want stale registry error", err)
	}
}

func TestAttemptUsesRunRegistry(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	runID := "20260926-000000-0000abcd"
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := registerRunLocation(runLocation{BaseDir: paths.BaseDir, ProjectName: paths.ProjectName, RunID: runID}); err != nil {
		t.Fatal(err)
	}
	attemptID := state.MakeAttemptID(runID, "job-1", 0)
	gotBaseDir, gotProject, gotRunID, gotJobID, err := Attempt(attemptID, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotBaseDir != baseDir || gotProject != "demo" || gotRunID != runID || gotJobID != "job-1" {
		t.Fatalf("resolved attempt = %q, %q, %q, %q", gotBaseDir, gotProject, gotRunID, gotJobID)
	}
}

func TestRunIDUsesValidMetaLastRun(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-from-meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "newer-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := state.DefaultMeta()
	meta.LastRunID = "run-from-meta"
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := RunID(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run-from-meta" {
		t.Fatalf("run ID = %q, want run-from-meta", runID)
	}
}

func TestRunIDFallsBackToNewestRun(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	olderPath := filepath.Join(paths.RunsDir, "older-run")
	newerPath := filepath.Join(paths.RunsDir, "newer-run")
	if err := os.MkdirAll(olderPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newerPath, 0o755); err != nil {
		t.Fatal(err)
	}
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	if err := os.Chtimes(olderPath, older, older); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newerPath, newer, newer); err != nil {
		t.Fatal(err)
	}
	meta := state.DefaultMeta()
	meta.LastRunID = "missing-run"
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := RunID(paths, "")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "newer-run" {
		t.Fatalf("run ID = %q, want newer-run", runID)
	}
}

func TestRunIDDoesNotFallbackForMissingRequestedRun(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "existing-run"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = RunID(paths, "missing-run")
	if err == nil || !strings.Contains(err.Error(), `run "missing-run" not found`) {
		t.Fatalf("error = %v, want exact missing-run error", err)
	}
}

func TestRunIDResolvesLatestAlias(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "latest-run"), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := state.DefaultMeta()
	meta.LastRunID = "latest-run"
	if err := state.WriteJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	runID, err := RunID(paths, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if runID != "latest-run" {
		t.Fatalf("run ID = %q, want latest-run", runID)
	}
}

func TestJobInQueueMatchesArrayCommandBeforeItsTasks(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "eval", Name: "eval", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 2}},
	}}
	for _, test := range []struct {
		selector string
		byName   bool
		want     string
	}{
		{"eval", false, "eval"},
		{"eval", true, "eval"},
		{"eval-2", false, "eval-2"},
		{"eval[2]", true, "eval-2"},
	} {
		if got, found := JobInQueue(queue, test.selector, test.byName); !found || got != test.want {
			t.Fatalf("JobInQueue(%q, byName=%v) = %q, %v; want %q", test.selector, test.byName, got, found, test.want)
		}
	}
}

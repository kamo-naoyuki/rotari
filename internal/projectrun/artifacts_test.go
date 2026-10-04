package projectrun

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func readArtifactRecord(t *testing.T, runner Runner, runDir, jobID, attemptID string) (artifact.Record, bool) {
	t.Helper()
	var record artifact.Record
	err := runner.Store.ReadJSON(filepath.Join(runDir, jobID, "attempts", attemptID, state.ArtifactsFileName), &record)
	if os.IsNotExist(err) {
		return record, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return record, true
}

func candidatePaths(record artifact.Record) []string {
	var paths []string
	for _, candidate := range record.Candidates {
		paths = append(paths, candidate.Path)
	}
	return paths
}

func TestExecuteRecordsArtifactCandidatesPerAttempt(t *testing.T) {
	runner, paths := testRunner(t)
	runner.Executors = executor.NewRegistry(runner.Store, func(string, ...any) {})
	work := t.TempDir()
	if err := os.MkdirAll(filepath.Join(work, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "conf", "a.yaml"), []byte("out_dir: results\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	retry := 1
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "plain", Command: []string{"cat", "conf/a.yaml"}},
		{ID: "arr", Command: []string{"true", "task.csv"}, Array: &model.ArraySpec{First: 0, Last: 1}},
		{ID: "member-a", Command: []string{"true"}, Environment: []string{"OUT_DIR=runs/a"}},
		{ID: "member-b", Command: []string{"true"}, Environment: []string{"OUT_DIR=runs/b"}},
		{ID: "retried", Command: []string{"false", "r.csv"}, Retry: &retry},
		{ID: "relative", Command: []string{"true", "x.csv"}, WorkingDirectory: "sub"},
	}}
	if err := state.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := runner.Begin(paths, Start{RunID: "run-1", CWD: work}); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Execute(paths, Options{RunID: "run-1", EnvMode: model.EnvModeNone}, Observer{}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")

	want := map[string][]string{
		"plain":    {filepath.Join(work, "conf/a.yaml"), filepath.Join(work, "results")},
		"member-a": {filepath.Join(work, "runs/a")},
		"member-b": {filepath.Join(work, "runs/b")},
		"relative": {filepath.Join(work, "sub/x.csv")},
	}
	for _, job := range model.QueueToJobs(queue.Commands) {
		if job.ArrayTaskID != nil {
			want[job.ID] = []string{filepath.Join(work, "task.csv")}
		}
	}
	if len(want) != 6 {
		t.Fatalf("expected two array tasks, got jobs %v", want)
	}
	for jobID, paths := range want {
		attempts := state.ListAttemptIDs(runDir, jobID)
		if len(attempts) != 1 {
			t.Fatalf("%s: attempts = %v", jobID, attempts)
		}
		record, ok := readArtifactRecord(t, runner, runDir, jobID, attempts[0])
		if !ok {
			t.Fatalf("%s: no artifact record", jobID)
		}
		if record.Version != artifact.DiscoveryVersion || !slices.Equal(candidatePaths(record), paths) {
			t.Fatalf("%s: record = %+v, want candidates %q", jobID, record, paths)
		}
		for _, candidate := range record.Candidates {
			for _, source := range candidate.Sources {
				if strings.HasPrefix(source.Key, "ROTARI_") || source.Key == "PWD" {
					t.Fatalf("%s: run variable %s recorded as a candidate", jobID, source.Key)
				}
			}
		}
	}

	attempts := state.ListAttemptIDs(runDir, "retried")
	if len(attempts) != 2 {
		t.Fatalf("retried attempts = %v, want two", attempts)
	}
	for _, attemptID := range attempts {
		record, ok := readArtifactRecord(t, runner, runDir, "retried", attemptID)
		if !ok || !slices.Equal(candidatePaths(record), []string{filepath.Join(work, "r.csv")}) {
			t.Fatalf("retried %s: record = %+v, %v; every executed attempt has its own record", attemptID, record, ok)
		}
	}
}

func TestArtifactRecorderDoesNotReadSSHSources(t *testing.T) {
	runner, _ := testRunner(t)
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "a.yaml"), []byte("out_dir: results\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", AttemptID: state.MakeAttemptID("20260101-000000-aaaaaaaa", "job", 0), Executor: "ssh", Command: []string{"train", "a.yaml"}, WorkingDirectory: work}
	recorder := newArtifactRecorder(runDir, []model.JobSpec{job}, runner.Store, func(string, ...any) {})
	recorder.prepare(job)
	attemptDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recorder.started(job)
	record, ok := readArtifactRecord(t, runner, runDir, "job", job.AttemptID)
	if !ok || !slices.Equal(candidatePaths(record), []string{filepath.Join(work, "a.yaml")}) {
		t.Fatalf("record = %+v, %v; want the argument only", record, ok)
	}
	if len(record.Diagnostics) != 1 || !strings.Contains(record.Diagnostics[0].Message, "SSH execution host") {
		t.Fatalf("diagnostics = %+v", record.Diagnostics)
	}
}

func TestArtifactRecorderSkipsAttemptsThatNeverStarted(t *testing.T) {
	runner, _ := testRunner(t)
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", AttemptID: state.MakeAttemptID("20260101-000000-aaaaaaaa", "job", 0), Command: []string{"true", "a.csv"}}
	recorder := newArtifactRecorder(runDir, []model.JobSpec{job}, runner.Store, func(string, ...any) {})
	recorder.started(job)
	if _, err := os.Stat(filepath.Join(runDir, "job")); !os.IsNotExist(err) {
		t.Fatalf("an attempt that was never prepared got a directory: %v", err)
	}
}

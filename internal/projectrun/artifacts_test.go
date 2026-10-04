package projectrun

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/run"
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

// TestArtifactRecorderReadsSourcesForEveryExecutor checks that remote
// executors read configuration files through the shared filesystem, and that
// a file the supervisor cannot see is skipped with a diagnostic.
func TestArtifactRecorderReadsSourcesForEveryExecutor(t *testing.T) {
	for _, executorName := range []string{"local", "ssh", "slurm"} {
		t.Run(executorName, func(t *testing.T) {
			runner, _ := testRunner(t)
			work := t.TempDir()
			if err := os.WriteFile(filepath.Join(work, "a.yaml"), []byte("out_dir: results\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runDir := t.TempDir()
			job := model.JobSpec{ID: "job", AttemptID: state.MakeAttemptID("20260101-000000-aaaaaaaa", "job", 0), Executor: executorName, Command: []string{"train", "a.yaml", "missing.json"}, WorkingDirectory: work}
			recorder := newArtifactRecorder(runDir, []model.JobSpec{job}, nil, runner.Store, func(string, ...any) {})
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
			want := []string{filepath.Join(work, "a.yaml"), filepath.Join(work, "missing.json"), filepath.Join(work, "results")}
			if !ok || !slices.Equal(candidatePaths(record), want) {
				t.Fatalf("record = %+v, %v; want candidates %q", record, ok, want)
			}
			if len(record.Diagnostics) != 1 || record.Diagnostics[0].Source != filepath.Join(work, "missing.json") {
				t.Fatalf("diagnostics = %+v, want one for the missing file", record.Diagnostics)
			}
		})
	}
}

func TestArtifactRecorderSkipsAttemptsThatNeverStarted(t *testing.T) {
	runner, _ := testRunner(t)
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", AttemptID: state.MakeAttemptID("20260101-000000-aaaaaaaa", "job", 0), Command: []string{"true", "a.csv"}}
	recorder := newArtifactRecorder(runDir, []model.JobSpec{job}, nil, runner.Store, func(string, ...any) {})
	recorder.started(job)
	if _, err := os.Stat(filepath.Join(runDir, "job")); !os.IsNotExist(err) {
		t.Fatalf("an attempt that was never prepared got a directory: %v", err)
	}
}

// TestArtifactRecorderParsesAConfigOncePerRun checks that the attempts of a
// run, such as array tasks, share one parse of an unchanged configuration
// file: a second attempt sees the first parse even though the bytes changed
// without changing the file's size or modification time.
func TestArtifactRecorderParsesAConfigOncePerRun(t *testing.T) {
	runner, _ := testRunner(t)
	work := t.TempDir()
	config := filepath.Join(work, "a.yaml")
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(config, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(config, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	runDir := t.TempDir()
	var jobs []model.JobSpec
	for _, id := range []string{"task-0", "task-1"} {
		jobs = append(jobs, model.JobSpec{ID: id, AttemptID: state.MakeAttemptID("20260101-000000-aaaaaaaa", id, 0), Command: []string{"train", "a.yaml"}, WorkingDirectory: work})
	}
	recorder := newArtifactRecorder(runDir, jobs, nil, runner.Store, func(string, ...any) {})
	write("out_dir: aaa\n")
	recorder.prepare(jobs[0])
	write("out_dir: bbb\n")
	recorder.prepare(jobs[1])
	for _, job := range jobs {
		attemptDir, err := state.AttemptJobDir(runDir, job)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(attemptDir, 0o755); err != nil {
			t.Fatal(err)
		}
		recorder.started(job)
		record, ok := readArtifactRecord(t, runner, runDir, job.ID, job.AttemptID)
		want := []string{config, filepath.Join(work, "aaa")}
		if !ok || !slices.Equal(candidatePaths(record), want) {
			t.Fatalf("%s: candidates = %q, want %q from the run's one parse", job.ID, candidatePaths(record), want)
		}
	}
}

// TestExecuteExpandsTaskVariablesInShellSource checks PATH-E1: each array
// task and each member that differs by environment records its own path.
func TestExecuteExpandsTaskVariablesInShellSource(t *testing.T) {
	runner, paths := testRunner(t)
	runner.Executors = executor.NewRegistry(runner.Store, func(string, ...any) {})
	runner.Environment = run.EnvironmentNames{ArrayTaskID: "ROTARI_ARRAY_TASK_ID", JobDir: "ROTARI_JOB_DIR", RunDir: "ROTARI_RUN_DIR"}
	work := t.TempDir()
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "arr", Command: []string{"bash", "-c", `true > "out/$ROTARI_ARRAY_TASK_ID.log"`}, Array: &model.ArraySpec{First: 0, Last: 1}},
		{ID: "member-a", Command: []string{"bash", "-c", `true > "res/$LR.csv"`}, Environment: []string{"LR=0.1"}},
		{ID: "member-b", Command: []string{"bash", "-c", `true > "res/$LR.csv"`}, Environment: []string{"LR=0.2"}},
		{ID: "attempt", Command: []string{"bash", "-c", `true > "$ROTARI_JOB_DIR/result.txt"`}},
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
	want := map[string]string{
		"member-a": filepath.Join(work, "res/0.1.csv"),
		"member-b": filepath.Join(work, "res/0.2.csv"),
	}
	for _, job := range model.QueueToJobs(queue.Commands) {
		if job.ArrayTaskID != nil {
			want[job.ID] = filepath.Join(work, "out", strconv.Itoa(*job.ArrayTaskID)+".log")
		}
	}
	attemptID := state.ListAttemptIDs(runDir, "attempt")[0]
	want["attempt"] = filepath.Join(runDir, "attempt", "attempts", attemptID, "result.txt")
	if len(want) != 5 {
		t.Fatalf("want = %v, expected two array tasks", want)
	}
	for jobID, path := range want {
		attempts := state.ListAttemptIDs(runDir, jobID)
		if len(attempts) != 1 {
			t.Fatalf("%s: attempts = %v", jobID, attempts)
		}
		record, ok := readArtifactRecord(t, runner, runDir, jobID, attempts[0])
		if !ok || !slices.Equal(candidatePaths(record), []string{path}) {
			t.Fatalf("%s: record = %+v, %v; want %s", jobID, record, ok, path)
		}
		if source := record.Candidates[0].Sources[0]; source.Rule != artifact.RuleRedirection || !source.Expanded {
			t.Fatalf("%s: source = %+v, want an expanded PATH-R1 redirection", jobID, source)
		}
	}
}

package runlist

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeListsRun(t *testing.T, baseDir, projectName, runID string) state.ProjectPaths {
	t.Helper()
	return writeListsRunAt(t, baseDir, projectName, runID, time.Date(2026, 10, 8, 10, 0, 1, 0, time.UTC))
}

func writeListsRunAt(t *testing.T, baseDir, projectName, runID string, finishedAt time.Time) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{
		RunID: runID, RunName: "saved run", Status: "finished", ExitCode: 0,
		StartedAt: finishedAt.Add(-time.Second).Format(time.RFC3339Nano), FinishedAt: finishedAt.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}

func writeTestFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func TestCollectShowClientModeAndDetachReason(t *testing.T) {
	paths := writeListsRun(t, t.TempDir(), "demo", "run-async")
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "run-async", state.RunClientStatusFileName), model.RunClientStatus{Mode: model.RunClientModeAsync, State: model.RunClientCompleted, Reason: model.RunClientReasonAsync}); err != nil {
		t.Fatal(err)
	}
	paths = writeListsRun(t, paths.BaseDir, "demo", "run-ctrl-d")
	if err := state.WriteJSON(filepath.Join(paths.RunsDir, "run-ctrl-d", state.RunClientStatusFileName), model.RunClientStatus{Mode: model.RunClientModeSync, State: model.RunClientCompleted, Reason: model.RunClientReasonCtrlD}); err != nil {
		t.Fatal(err)
	}
	rows, err := Collect([]string{paths.BaseDir}, "demo")
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]string{}
	for _, row := range rows {
		labels[row.RunID] = row.ClientLabel
	}
	if labels["run-async"] != "async (completed)" || labels["run-ctrl-d"] != "sync (completed; Ctrl-D detached)" {
		t.Fatalf("client labels = %#v", labels)
	}
}

func TestCollectReadsOnlySelectedProject(t *testing.T) {
	baseDir := t.TempDir()
	selected := writeListsRun(t, baseDir, "selected", "run-selected")
	broken := writeListsRun(t, baseDir, "aaa-broken", "run-broken")
	for _, file := range []string{selected.QueueFile, broken.QueueFile, broken.MetaFile,
		filepath.Join(broken.RunsDir, "run-broken", "summary.json")} {
		if err := writeTestFile(file, []byte("{broken")); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := Collect([]string{baseDir}, "selected")
	if err != nil || len(rows) != 1 || rows[0].RunID != "run-selected" {
		t.Fatalf("rows = %#v, err = %v", rows, err)
	}
}

func TestCollectDoesNotReadQueues(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeListsRun(t, baseDir, "selected", "run-selected")
	if err := writeTestFile(paths.QueueFile, []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	rows, err := Collect([]string{baseDir}, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %#v, err = %v", rows, err)
	}
}

func TestCollectSummaryErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{"malformed", "{broken", ""},
		{"newer-version", `{"state_version":999999,"run_id":"run-bad"}`, "upgrade"},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeListsRun(t, baseDir, "selected", "run-bad")
			if err := writeTestFile(filepath.Join(paths.RunsDir, "run-bad", "summary.json"), []byte(test.data)); err != nil {
				t.Fatal(err)
			}
			for _, active := range []bool{false, true} {
				if active {
					if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-bad"}); err != nil {
						t.Fatal(err)
					}
				}
				_, err := Collect([]string{baseDir}, "selected")
				if err == nil || !strings.Contains(err.Error(), "run-bad") || !strings.Contains(err.Error(), "selected") || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("active = %v, err = %v", active, err)
				}
			}
		})
	}
}

func TestCollectPhaseOverridesSummary(t *testing.T) {
	for _, test := range []struct {
		name string
		pid  int
		want string
	}{
		{"active", os.Getpid(), "running"},
		{"interrupted", -1, "interrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeListsRun(t, baseDir, "selected", "run-current")
			writeListsRun(t, baseDir, "selected", "run-old")
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-current"}); err != nil {
				t.Fatal(err)
			}
			if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: test.pid, RunID: "run-current", Host: host}); err != nil {
				t.Fatal(err)
			}
			for _, missing := range []bool{false, true} {
				if missing {
					if err := os.Remove(filepath.Join(paths.RunsDir, "run-current", "summary.json")); err != nil {
						t.Fatal(err)
					}
				}
				rows, err := Collect([]string{baseDir}, "selected")
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					want := "finished"
					if row.RunID == "run-current" {
						want = test.want
						if !missing && (row.RunName != "saved run" || row.ExitCode == nil || *row.ExitCode != 0) {
							t.Errorf("summary details lost: %#v", row)
						}
					}
					if row.Lifecycle != want {
						t.Errorf("missing summary = %v, row = %#v, want status %q", missing, row, want)
					}
				}
			}
		})
	}
}

func TestCollectEmptyAndMissingScopes(t *testing.T) {
	baseDir := t.TempDir()
	for _, filter := range []string{"", "selected"} {
		if rows, err := Collect([]string{baseDir}, filter); err != nil || len(rows) != 0 {
			t.Fatalf("no projects: rows = %#v, err = %v", rows, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "selected"), 0700); err != nil {
		t.Fatal(err)
	}
	if rows, err := Collect([]string{baseDir}, "selected"); err != nil || len(rows) != 0 {
		t.Fatalf("no runs: rows = %#v, err = %v", rows, err)
	}
	if _, err := Collect([]string{baseDir}, "missing"); err == nil || !strings.Contains(err.Error(), `project "missing" not found`) {
		t.Fatalf("missing named project: %v", err)
	}
	otherBase := t.TempDir()
	paths := writeListsRun(t, otherBase, "elsewhere", "run-incomplete")
	if err := os.Remove(filepath.Join(paths.RunsDir, "run-incomplete", "summary.json")); err != nil {
		t.Fatal(err)
	}
	rows, err := Collect([]string{baseDir, otherBase}, "elsewhere")
	if err != nil || len(rows) != 1 || rows[0].Lifecycle != "incomplete" {
		t.Fatalf("named project in another basedir: rows = %#v, err = %v", rows, err)
	}
}

func TestCollectOrderTiesByRunID(t *testing.T) {
	baseDir := t.TempDir()
	for _, runID := range []string{"run-z", "run-a", "run-m"} {
		writeListsRun(t, baseDir, "selected", runID)
	}
	// Repeating a scope prevents ReadDir's alphabetical order from masking
	// a missing RunID tie-breaker in the final sort.
	rows, err := Collect([]string{baseDir, baseDir}, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"run-a", "run-a", "run-m", "run-m", "run-z", "run-z"} {
		if rows[i].RunID != want {
			t.Fatalf("rows = %#v", rows)
		}
	}
}

func TestRecentKeepsActiveAndRecentRuns(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	rows := []Run{
		{RunID: "recent", Lifecycle: "finished", Finished: now.Add(-23 * time.Hour)},
		{RunID: "old", Lifecycle: "failed", Finished: now.Add(-25 * time.Hour)},
		{RunID: "running-old", Lifecycle: "running", Finished: now.Add(-48 * time.Hour)},
		{RunID: "interrupted-old", Lifecycle: "interrupted", Finished: now.Add(-48 * time.Hour)},
		{RunID: "incomplete", Lifecycle: "incomplete"},
	}
	filtered := Recent(rows, now.Add(-24*time.Hour))
	var got []string
	for _, row := range filtered {
		got = append(got, row.RunID)
	}
	if !reflect.DeepEqual(got, []string{"recent", "running-old", "interrupted-old", "incomplete"}) {
		t.Fatalf("filtered run IDs = %v", got)
	}
}

func TestCollectActiveRunWithoutSummaryUsesLockDetails(t *testing.T) {
	for _, test := range []struct {
		name string
		pid  int
		want string
	}{
		{"active", os.Getpid(), "running"},
		{"interrupted", -1, "interrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, err := state.ResolveProjectPaths(baseDir, "selected")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-current"), 0o755); err != nil {
				t.Fatal(err)
			}
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-current"}); err != nil {
				t.Fatal(err)
			}
			startedAt := time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)
			lock := model.LockInfo{PID: test.pid, RunID: "run-current", RunName: "sweep", Host: host, StartedAt: startedAt.Format(time.RFC3339Nano)}
			if err := state.WriteJSON(paths.LockFile, lock); err != nil {
				t.Fatal(err)
			}
			rows, err := Collect([]string{baseDir}, "selected")
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 {
				t.Fatalf("rows = %#v", rows)
			}
			row := rows[0]
			if row.Lifecycle != test.want || row.RunName != "sweep" || row.StartedAt != lock.StartedAt || row.Order != startedAt.UnixNano() {
				t.Fatalf("row = %#v, want status %q, name sweep, and the lock start time", row, test.want)
			}
		})
	}
}

// A run's details come from its context, sources, and its own notes; a run
// that wrote none of those files has empty details, and an unreadable file
// is an error rather than an empty fact.
func TestReadDetails(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeListsRun(t, baseDir, "demo", "run-full")
	writeListsRun(t, baseDir, "demo", "run-bare")
	runDir := filepath.Join(paths.RunsDir, "run-full")
	if err := state.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{CWD: "/work/repo", Hostname: "node7"}); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveRunSources(runDir, model.RunSources{Sources: []model.SourceRevision{
		{Root: "/work/repo", VCS: "git", CommitID: "0123456789abcdef"},
		{Root: "/work/lib", VCS: "jj", CommitID: "fedcba9876543210", ChangeID: "zyxwvutsrqponmlk"},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, note := range []model.RunNote{
		{At: "1", JobID: "job-1", Text: "a job's note"},
		{At: "2", Text: "Raise the beam\nto 8"},
		{At: "3", Text: "second run note"},
	} {
		if err := state.AppendRunNote(runDir, note); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := Collect([]string{baseDir}, "demo")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Run{}
	for _, run := range runs {
		byID[run.RunID] = run
	}
	details, err := ReadDetails(byID["run-full"])
	if err != nil {
		t.Fatal(err)
	}
	want := Details{
		WorkingDirectory: "/work/repo", Host: "node7",
		Sources:   []string{"git 0123456789ab (clean) in /work/repo", "jj fedcba987654 (change zyxwvutsrqpo) in /work/lib"},
		NoteCount: 2, FirstNote: "Raise the beam",
	}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("details = %#v, want %#v", details, want)
	}
	if details, err := ReadDetails(byID["run-bare"]); err != nil || !reflect.DeepEqual(details, Details{}) {
		t.Fatalf("bare run details = %#v, %v; want empty", details, err)
	}
	if err := writeTestFile(filepath.Join(runDir, "context.json"), []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDetails(byID["run-full"]); err == nil || !strings.Contains(err.Error(), "run-full") {
		t.Fatalf("broken context error = %v, want one naming the run", err)
	}
}

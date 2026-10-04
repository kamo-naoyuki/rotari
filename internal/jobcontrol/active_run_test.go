package jobcontrol

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// suspendableExecutor records the jobs it suspends, resumes, and cancels.
type suspendableExecutor struct {
	fakeExecutor
	suspended, resumed []string
}

func (fake *suspendableExecutor) Suspend(jobDir string) error {
	fake.suspended = append(fake.suspended, filepath.Base(jobDir))
	return nil
}

func (fake *suspendableExecutor) Resume(jobDir string) error {
	fake.resumed = append(fake.resumed, filepath.Base(jobDir))
	return nil
}

// activeRunFixture is a project whose run testRunID holds the lock of this
// process. Its snapshot has a job on each executor, a finished job, a job
// that was never submitted, and a three-task array with one task finished
// and one never submitted.
type activeRunFixture struct {
	baseDir    string
	paths      state.ProjectPaths
	runDir     string
	controller Controller
	slurm      *fakeExecutor
	pbs        *suspendableExecutor
}

func newActiveRunFixture(t *testing.T) activeRunFixture {
	t.Helper()
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, testRunID)
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "on-pbs", Name: "train", Command: []string{"true"}},
		{ID: "on-slurm", Name: "eval", Command: []string{"true"}},
		{ID: "done", Name: "done", Command: []string{"true"}},
		{ID: "waiting", Name: "waiting", Command: []string{"true"}},
		{ID: "grid", Name: "grid", Command: []string{"true"}, Array: &model.ArraySpec{First: 1, Last: 3}},
	}}
	if err := state.WriteJSON(filepath.Join(runDir, "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	writeJobJSON(t, filepath.Join(runDir, "on-pbs"), "pbs")
	writeJobJSON(t, filepath.Join(runDir, "on-slurm"), "slurm")
	writeJobJSON(t, filepath.Join(runDir, "done"), "pbs")
	writeJobJSON(t, filepath.Join(runDir, "grid-1"), "pbs")
	writeJobJSON(t, filepath.Join(runDir, "grid-2"), "pbs")
	for _, jobID := range []string{"done", "grid-2"} {
		if err := os.WriteFile(filepath.Join(runDir, jobID, "finished_at"), []byte(now()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: testRunID, Host: host}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: testRunID}); err != nil {
		t.Fatal(err)
	}
	slurm := &fakeExecutor{name: "slurm"}
	pbs := &suspendableExecutor{fakeExecutor: fakeExecutor{name: "pbs"}}
	controller := Controller{Store: state.NewStore(0o700, 0o600), Executors: executor.Registry{"slurm": slurm, "pbs": pbs}}
	return activeRunFixture{baseDir: baseDir, paths: paths, runDir: runDir, controller: controller, slurm: slurm, pbs: pbs}
}

func sorted(values []string) []string {
	values = append([]string(nil), values...)
	sort.Strings(values)
	return values
}

func TestControlSuspendsAndResumesSelectedJobs(t *testing.T) {
	fixture := newActiveRunFixture(t)
	message, err := fixture.controller.Control(fixture.baseDir, "demo", testRunID, []string{"on-pbs", "grid"}, "suspend")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "Suspend requested") || !strings.Contains(message, "Jobs: 2") {
		t.Fatalf("message = %q", message)
	}
	// The array selects only its submitted, unfinished task.
	if want := []string{"grid-1", "on-pbs"}; !reflect.DeepEqual(sorted(fixture.pbs.suspended), want) {
		t.Fatalf("suspended = %v, want %v", fixture.pbs.suspended, want)
	}
	if status := executor.LoadSchedulerStatus(fixture.controller.Store, filepath.Join(fixture.runDir, "on-pbs")); status != "suspended" {
		t.Fatalf("scheduler status = %q, want suspended", status)
	}
	if _, err := fixture.controller.Control(fixture.baseDir, "demo", "", []string{"on-pbs"}, "resume"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fixture.pbs.resumed, []string{"on-pbs"}) {
		t.Fatalf("resumed = %v", fixture.pbs.resumed)
	}
	if status := executor.LoadSchedulerStatus(fixture.controller.Store, filepath.Join(fixture.runDir, "on-pbs")); status != "running" {
		t.Fatalf("scheduler status = %q, want running", status)
	}
}

func TestControlRejectsJobsItCannotSignal(t *testing.T) {
	fixture := newActiveRunFixture(t)
	for _, test := range []struct {
		runID  string
		jobIDs []string
		want   string
	}{
		{"", []string{"done"}, `job "done" is not running`},
		{"", []string{"waiting"}, `job "waiting" is not running`},
		{"", []string{"on-slurm"}, `executor "slurm" does not support suspend`},
		{"other-run", []string{"on-pbs"}, `run "other-run" is not running; the active run of project "demo" is "` + testRunID + `"`},
	} {
		if _, err := fixture.controller.Control(fixture.baseDir, "demo", test.runID, test.jobIDs, "suspend"); err == nil || err.Error() != test.want {
			t.Errorf("Control(%v) error = %v, want %q", test.jobIDs, err, test.want)
		}
	}
	// grid-3 was never submitted and grid-2 is finished: no task is running.
	if err := os.WriteFile(filepath.Join(fixture.runDir, "grid-1", "finished_at"), []byte(now()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.controller.Control(fixture.baseDir, "demo", "", []string{"grid"}, "suspend"); err == nil || err.Error() != `job "grid" is not running` {
		t.Fatalf("Control(finished array) error = %v", err)
	}
	if len(fixture.pbs.suspended) != 0 {
		t.Fatalf("rejected requests suspended %v", fixture.pbs.suspended)
	}
}

func TestControlAllJobsFailsAtUnsupportedExecutor(t *testing.T) {
	fixture := newActiveRunFixture(t)
	if _, err := fixture.controller.Control(fixture.baseDir, "demo", "", nil, "suspend"); err == nil || err.Error() != `executor "slurm" does not support suspend` {
		t.Fatalf("Control(all) error = %v", err)
	}
	// Jobs signalled before the error stay suspended; see development/ISSUES.md.
	fixture.pbs.suspended = nil
	if err := os.WriteFile(filepath.Join(fixture.runDir, "on-slurm", "finished_at"), []byte(now()), 0o600); err != nil {
		t.Fatal(err)
	}
	message, err := fixture.controller.Control(fixture.baseDir, "demo", "", nil, "suspend")
	if err != nil || !strings.Contains(message, "Jobs: 2") {
		t.Fatalf("Control(all) = %q, %v", message, err)
	}
	if want := []string{"grid-1", "on-pbs"}; !reflect.DeepEqual(sorted(fixture.pbs.suspended), want) {
		t.Fatalf("suspended = %v, want %v", fixture.pbs.suspended, want)
	}
}

func TestCancelWholeRunCancelsUnfinishedJobs(t *testing.T) {
	fixture := newActiveRunFixture(t)
	message, err := fixture.controller.Cancel(fixture.baseDir, "demo", testRunID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "Jobs: 5") || !strings.Contains(message, "rotari show --run-id "+testRunID) {
		t.Fatalf("message = %q", message)
	}
	if want := []string{"grid-1", "on-pbs"}; !reflect.DeepEqual(sorted(fixture.pbs.cancelled), want) {
		t.Fatalf("pbs cancelled = %v, want %v", fixture.pbs.cancelled, want)
	}
	if !reflect.DeepEqual(fixture.slurm.cancelled, []string{"on-slurm"}) {
		t.Fatalf("slurm cancelled = %v", fixture.slurm.cancelled)
	}
	for _, jobID := range []string{"waiting", "grid-3"} {
		if _, err := os.Stat(filepath.Join(fixture.runDir, jobID, "cancelled")); err != nil {
			t.Errorf("unsubmitted job %s was not marked cancelled: %v", jobID, err)
		}
	}
	meta, err := state.LoadMeta(fixture.paths.MetaFile)
	if err != nil || meta.Phase != "cancelling" {
		t.Fatalf("meta = %#v, %v; want cancelling", meta, err)
	}
}

func TestCancelArrayCancelsItsUnfinishedTasks(t *testing.T) {
	fixture := newActiveRunFixture(t)
	message, err := fixture.controller.Cancel(fixture.baseDir, "demo", "", []string{"grid"}, false)
	if err != nil || !strings.Contains(message, "Jobs: 2") {
		t.Fatalf("Cancel(grid) = %q, %v", message, err)
	}
	if !reflect.DeepEqual(fixture.pbs.cancelled, []string{"grid-1"}) {
		t.Fatalf("pbs cancelled = %v, want [grid-1]", fixture.pbs.cancelled)
	}
	if _, err := os.Stat(filepath.Join(fixture.runDir, "grid-3", "cancelled")); err != nil {
		t.Fatalf("pending task was not marked cancelled: %v", err)
	}
	if meta, err := state.LoadMeta(fixture.paths.MetaFile); err != nil || meta.Phase != "running" {
		t.Fatalf("a job cancel changed the run phase: %#v, %v", meta, err)
	}
}

func TestCancelRefusesRunOfAnotherHost(t *testing.T) {
	fixture := newActiveRunFixture(t)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	// A remote lock is not checked for liveness, so it stays the active run.
	if err := state.WriteJSON(fixture.paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: testRunID, Host: host + "-other"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.controller.Cancel(fixture.baseDir, "demo", "", nil, false); err == nil || !strings.Contains(err.Error(), "run cancel from that host") {
		t.Fatalf("Cancel(remote run) error = %v", err)
	}
	if meta, err := state.LoadMeta(fixture.paths.MetaFile); err != nil || meta.Phase != "running" {
		t.Fatalf("a refused cancel changed the run phase: %#v, %v", meta, err)
	}
}

func TestSelectResolvesTheActiveRun(t *testing.T) {
	fixture := newActiveRunFixture(t)
	runID, jobIDs, err := fixture.controller.Select(fixture.baseDir, "demo", "", Selection{States: States["suspend"]}, time.Now())
	if err != nil || runID != testRunID {
		t.Fatalf("Select = %q, %v, %v", runID, jobIDs, err)
	}
	if want := []string{"grid-1", "on-pbs", "on-slurm"}; !reflect.DeepEqual(jobIDs, want) {
		t.Fatalf("Select = %v, want %v", jobIDs, want)
	}
	if _, _, err := fixture.controller.Select(fixture.baseDir, "demo", "", Selection{Names: []string{"done"}}, time.Now()); err == nil || err.Error() != "run "+testRunID+": no unfinished jobs match the selection" {
		t.Fatalf("Select(finished) error = %v", err)
	}
	if _, _, err := fixture.controller.Select(t.TempDir(), "demo", "run-1", Selection{}, time.Now()); err == nil || err.Error() != `run "run-1" is not running; project "demo" has no active run` {
		t.Fatalf("Select(idle) error = %v", err)
	}
	if _, _, err := fixture.controller.Select(fixture.baseDir, "../demo", "", Selection{}, time.Now()); err == nil {
		t.Fatal("Select accepted an unsafe project name")
	}
}

func TestActiveLockRejectsMalformedLock(t *testing.T) {
	fixture := newActiveRunFixture(t)
	if err := os.WriteFile(fixture.paths.LockFile, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.controller.Cancel(fixture.baseDir, "demo", "", nil, false); err == nil || !strings.Contains(err.Error(), "invalid running lock") {
		t.Fatalf("Cancel(malformed lock) error = %v", err)
	}
}

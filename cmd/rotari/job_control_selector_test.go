package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// jobControlCase is one row of the cancel, suspend, and resume selectors in
// docs/contracts/06-selectors.md, run against newSelectorFixture with run
// "live" of project sweep active: array job hold (tasks 1 and 2) and job
// idle, each sleeping. args use the placeholders of selectorCase, plus
// {run:live} and {att:idle/live} for the active run and idle's attempt in it.
type jobControlCase struct {
	name string
	args string
	// suspended suspends every live job before the command, for resume.
	suspended bool
	// jobs are the live jobs the command acted on: cancelled, suspended, or
	// resumed. Array tasks are hold-1 and hold-2.
	jobs []string
	// err is a substring of the expected error; no job may be affected.
	err string
}

var jobControlCases = []jobControlCase{
	// cancel
	{name: "project", args: "cancel -b {B} -p sweep", jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "job ID", args: "cancel -b {B} -p sweep {job:idle}", jobs: []string{"idle"}},
	{name: "job ID option", args: "cancel -b {B} -p sweep -j {job:idle}", jobs: []string{"idle"}},
	{name: "job ID in any project's active run", args: "cancel -b {B} {job:idle}", jobs: []string{"idle"}},
	{name: "job ID in no active run", args: "cancel -b {B} {job:other-prep}", err: "no active run in state directory \"{B}\" holds job {job:other-prep}"},
	{name: "job ID not in the active run", args: "cancel -b {B} -p sweep {job:prep}", err: `job "{job:prep}" is not found`},
	{name: "array command ID", args: "cancel -b {B} -p sweep {job:hold}", jobs: []string{"hold-1", "hold-2"}},
	{name: "array task ID", args: "cancel -b {B} -p sweep {job:hold}-1", jobs: []string{"hold-1"}},
	{name: "attempt ID", args: "cancel {att:idle/live}", jobs: []string{"idle"}},
	{name: "attempt ID of a finished run", args: "cancel {att:train-SEED2/0}", err: `run "{run:sweep-first}" is not running; the active run of project "sweep" is "{run:live}"`},
	{name: "run ID", args: "cancel {run:live}", jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "run ID and job ID", args: "cancel {run:live} {job:idle}", jobs: []string{"idle"}},
	{name: "finished run ID", args: "cancel {run:sweep-first}", err: `run "{run:sweep-first}" is not running; the active run of project "sweep" is "{run:live}"`},
	{name: "run ID of a project without an active run", args: "cancel {run:other-first}", err: `run "{run:other-first}" is not running; project "other" has no active run`},
	{name: "job IDs both ways", args: "cancel -b {B} -p sweep -j {job:idle} {job:hold}", err: "usage"},
	{name: "job ID and wait", args: "cancel -b {B} -p sweep --wait {job:idle}", err: "--wait may not be used with a job selection"},
	{name: "latest is not a run", args: "cancel -b {B} -p sweep latest", err: `job "latest" is not found`},

	// suspend
	{name: "project", args: "suspend -b {B} -p sweep", jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "job ID", args: "suspend -b {B} -p sweep {job:idle}", jobs: []string{"idle"}},
	{name: "job ID in any project's active run", args: "suspend -b {B} {job:idle}", jobs: []string{"idle"}},
	{name: "array command ID", args: "suspend -b {B} -p sweep {job:hold}", jobs: []string{"hold-1", "hold-2"}},
	{name: "array task ID", args: "suspend -b {B} -p sweep {job:hold}-2", jobs: []string{"hold-2"}},
	{name: "attempt ID", args: "suspend {att:idle/live}", jobs: []string{"idle"}},
	{name: "run ID", args: "suspend {run:live}", jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "finished run ID", args: "suspend {run:sweep-first}", err: `run "{run:sweep-first}" is not running; the active run of project "sweep" is "{run:live}"`},
	{name: "job ID not in the active run", args: "suspend -b {B} -p sweep {job:prep}", err: `job "{job:prep}" is not running`},
	{name: "job IDs both ways", args: "suspend -b {B} -p sweep -j {job:idle} {job:hold}", err: "usage"},

	// resume
	{name: "project", args: "resume -b {B} -p sweep", suspended: true, jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "job ID", args: "resume -b {B} -p sweep {job:idle}", suspended: true, jobs: []string{"idle"}},
	{name: "job ID in any project's active run", args: "resume -b {B} {job:idle}", suspended: true, jobs: []string{"idle"}},
	{name: "array command ID", args: "resume -b {B} -p sweep {job:hold}", suspended: true, jobs: []string{"hold-1", "hold-2"}},
	{name: "attempt ID", args: "resume {att:idle/live}", suspended: true, jobs: []string{"idle"}},
	{name: "run ID", args: "resume {run:live}", suspended: true, jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "finished run ID", args: "resume {run:sweep-first}", suspended: true, err: `run "{run:sweep-first}" is not running; the active run of project "sweep" is "{run:live}"`},
}

func TestJobControlSelectors(t *testing.T) {
	for _, tc := range jobControlCases {
		command := strings.Fields(tc.args)[0]
		t.Run(command+"/"+tc.name, func(t *testing.T) {
			fixture := newSelectorFixture(t)
			live := fixture.startLiveRun(t)
			if tc.suspended {
				if _, err := jobController().Control(fixture.BaseDir, "sweep", "", nil, "suspend"); err != nil {
					t.Fatal(err)
				}
			}
			var args []string
			for _, arg := range fixture.expand(tc.args) {
				arg = strings.ReplaceAll(arg, "{run:live}", fixture.Runs["live"])
				arg = strings.ReplaceAll(arg, "{att:idle/live}", fixture.Attempts["idle/live"])
				args = append(args, arg)
			}
			code, output := captureSelectorOutput(func() int { return run(args) })
			output = strings.ReplaceAll(fixture.symbolic(output), fixture.Runs["live"], "{run:live}")
			if tc.err != "" {
				if code == 0 || !strings.Contains(output, tc.err) {
					t.Fatalf("rotari %s: exit %d, output:\n%s\nwant an error containing %q", tc.args, code, output, tc.err)
				}
			} else if code != 0 {
				t.Fatalf("rotari %s: exit %d, output:\n%s", tc.args, code, output)
			}
			want := append([]string(nil), tc.jobs...)
			sort.Strings(want)
			if got := live.affected(t, command, tc.suspended, want); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("rotari %s acted on [%s], want [%s]; output:\n%s", tc.args, strings.Join(got, ","), strings.Join(want, ","), output)
			}
		})
	}
}

// liveRun is run "live" of project sweep, running in process.
type liveRun struct {
	runDir string
	// jobs maps hold-1, hold-2, and idle to their job IDs in the run.
	jobs map[string]string
}

// startLiveRun queues array job hold and job idle in project sweep, starts
// run "live" of them, and returns once every job has started. The run's
// supervisor is started too, and the run is cancelled when the test ends.
func (fixture selectorFixture) startLiveRun(t *testing.T) liveRun {
	t.Helper()
	fixture.add(t, fixture.BaseDir, "sweep", "--job-name", "hold", "--array", "1-2", "--", "sleep", "30")
	fixture.add(t, fixture.BaseDir, "sweep", "--job-name", "idle", "--", "sleep", "30")
	fixture.recordJobs(t, fixture.BaseDir, "sweep", map[string]string{"hold": "hold", "idle": "idle"}, "")
	paths := fixture.paths(t, fixture.BaseDir, "sweep")
	runID := makeRunID()
	fixture.Runs["live"] = runID
	runner := projectRunner()
	if err := runner.Begin(paths, projectrun.Start{RunID: runID, RunName: "live", CWD: fixture.BaseDir}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = runner.Run(paths, projectrun.Options{RunID: runID, RunName: "live", LocalConcurrency: 4, BatchMaxActive: 1, PartialArray: true}, projectrun.Observer{})
	}()
	t.Cleanup(func() {
		_, _ = jobController().Control(fixture.BaseDir, "sweep", runID, nil, "resume")
		_, _ = jobController().Cancel(fixture.BaseDir, "sweep", runID, nil, false)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("run live did not end after cancellation")
		}
	})
	live := liveRun{
		runDir: filepath.Join(paths.RunsDir, runID),
		jobs:   map[string]string{"hold-1": fixture.Jobs["hold"] + "-1", "hold-2": fixture.Jobs["hold"] + "-2", "idle": fixture.Jobs["idle"]},
	}
	deadline := time.Now().Add(5 * time.Second)
	for !live.allStarted(t) {
		if time.Now().After(deadline) {
			t.Fatal("jobs of run live did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	attemptID, err := state.LatestAttemptID(live.runDir, fixture.Jobs["idle"])
	if err != nil {
		t.Fatal(err)
	}
	fixture.Attempts["idle/live"] = attemptID
	fixture.startServer(t)
	return live
}

func (live liveRun) jobDir(t *testing.T, key string) string {
	t.Helper()
	dir, err := state.LatestAttemptJobDir(live.runDir, live.jobs[key])
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func (live liveRun) allStarted(t *testing.T) bool {
	t.Helper()
	for key := range live.jobs {
		if _, err := os.Stat(filepath.Join(live.jobDir(t, key), "pid")); err != nil {
			return false
		}
	}
	return true
}

// affected reports the live jobs that command acted on: the finished jobs
// for cancel, and the jobs whose scheduler state changed for suspend and
// resume. A cancelled job finishes asynchronously, so it waits briefly for
// the finished set to reach want.
func (live liveRun) affected(t *testing.T, command string, suspended bool, want []string) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got []string
		for key := range live.jobs {
			dir := live.jobDir(t, key)
			switch command {
			case "cancel":
				if _, err := os.Stat(filepath.Join(dir, "finished_at")); err == nil {
					got = append(got, key)
				}
			case "suspend":
				if executor.LoadSchedulerStatus(jsonStore(), dir) == "suspended" {
					got = append(got, key)
				}
			case "resume":
				if executor.LoadSchedulerStatus(jsonStore(), dir) == "running" {
					got = append(got, key)
				}
			}
		}
		sort.Strings(got)
		if command != "cancel" || strings.Join(got, ",") == strings.Join(want, ",") || time.Now().After(deadline) {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
}

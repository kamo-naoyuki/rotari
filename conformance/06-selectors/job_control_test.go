package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// Contract SEL-9: cancel, suspend, and resume resolve each form as "Job
// control" in contracts/06-selectors.md says.

// jobControlCase is one row of the cancel, suspend, and resume selectors in
// contracts/06-selectors.md, run against newSelectorFixture with run "live"
// of project sweep active: array job hold (tasks 1 and 2) and job idle, each
// sleeping. args use the placeholders of selectorCase, plus {run:live} and
// {att:idle/live} for the active run and idle's attempt in it.
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
	{name: "job name", args: "cancel -b {B} -p sweep --job-name idle", jobs: []string{"idle"}},
	{name: "repeated job names", args: "cancel -b {B} -p sweep --job-name hold --job-name idle", jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "stage scope", args: "cancel -b {B} -p sweep --stage single --yes", jobs: []string{"idle"}},
	{name: "stage filter", args: "cancel -b {B} -p sweep --filter-stage single --yes", jobs: []string{"idle"}},
	{name: "command filter", args: "cancel -b {B} -p sweep --filter-command=sleep.*301 --yes", jobs: []string{"idle"}},
	{name: "state filter", args: "cancel -b {B} -p sweep --filter-state running --yes", jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "negated stage filter", args: "cancel -b {B} -p sweep --filter-not-stage batch --yes", jobs: []string{"idle"}},
	{name: "filter matching no jobs", args: "cancel -b {B} -p sweep --filter-stage missing --yes", err: `no jobs in stage "missing"`},
	{name: "unknown job name", args: "cancel -b {B} -p sweep --job-name missing", err: `job name "missing" not found`},
	{name: "filter and wait", args: "cancel -b {B} -p sweep --filter-state running --wait --yes", err: "--wait may not be used with a job selection"},
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
	{name: "job name", args: "suspend -b {B} -p sweep --job-name idle", jobs: []string{"idle"}},
	{name: "stage scope", args: "suspend -b {B} -p sweep --stage single --yes", jobs: []string{"idle"}},
	{name: "stage filter", args: "suspend -b {B} -p sweep --filter-stage single --yes", jobs: []string{"idle"}},
	{name: "state filter", args: "suspend -b {B} -p sweep --filter-state running --yes", jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "negated stage filter", args: "suspend -b {B} -p sweep --filter-not-stage batch --yes", jobs: []string{"idle"}},
	{name: "pending state rejected", args: "suspend -b {B} -p sweep --filter-state pending --yes", err: `invalid choice "pending" (choose from running)`},
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
	{name: "job name", args: "resume -b {B} -p sweep --job-name idle", suspended: true, jobs: []string{"idle"}},
	{name: "stage scope", args: "resume -b {B} -p sweep --stage single --yes", suspended: true, jobs: []string{"idle"}},
	{name: "stage filter", args: "resume -b {B} -p sweep --filter-stage single --yes", suspended: true, jobs: []string{"idle"}},
	{name: "negated stage filter", args: "resume -b {B} -p sweep --filter-not-stage batch --yes", suspended: true, jobs: []string{"idle"}},
	{name: "pending state rejected", args: "resume -b {B} -p sweep --filter-state pending --yes", suspended: true, err: `invalid choice "pending" (choose from running)`},
	{name: "job ID", args: "resume -b {B} -p sweep {job:idle}", suspended: true, jobs: []string{"idle"}},
	{name: "job ID in any project's active run", args: "resume -b {B} {job:idle}", suspended: true, jobs: []string{"idle"}},
	{name: "array command ID", args: "resume -b {B} -p sweep {job:hold}", suspended: true, jobs: []string{"hold-1", "hold-2"}},
	{name: "attempt ID", args: "resume {att:idle/live}", suspended: true, jobs: []string{"idle"}},
	{name: "run ID", args: "resume {run:live}", suspended: true, jobs: []string{"hold-1", "hold-2", "idle"}},
	{name: "finished run ID", args: "resume {run:sweep-first}", suspended: true, err: `run "{run:sweep-first}" is not running; the active run of project "sweep" is "{run:live}"`},
}

func TestJobControlSelectors(t *testing.T) {
	covers(t, "RES-18", "SEL-9", "SEL-12")
	for _, tc := range jobControlCases {
		command := strings.Fields(tc.args)[0]
		t.Run(command+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSelectorFixture(t)
			live := f.startJobControlRun()
			if tc.suspended {
				f.e.mustRotari("suspend", "-b", f.base, "-p", "sweep")
			}
			r := f.e.rotari(f.expand(tc.args)...)
			output := f.symbolic(r.stdout + r.stderr)
			if tc.err != "" {
				if r.code == 0 || !strings.Contains(output, tc.err) {
					t.Fatalf("rotari %s: exit %d, output:\n%s\nwant an error containing %q", tc.args, r.code, output, tc.err)
				}
			} else if r.code != 0 {
				t.Fatalf("rotari %s: exit %d, output:\n%s", tc.args, r.code, output)
			}
			want := append([]string(nil), tc.jobs...)
			sort.Strings(want)
			if got := live.affected(t, command, want); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("rotari %s acted on [%s], want [%s]; output:\n%s", tc.args, strings.Join(got, ","), strings.Join(want, ","), output)
			}
		})
	}
}

// jobControlRun is run "live" of project sweep, with the attempt directory
// of each of its jobs by key: hold-1, hold-2, and idle.
type jobControlRun struct {
	attemptDirs map[string]string
}

// startJobControlRun queues array job hold and job idle in project sweep,
// starts run "live" of them in the background, records {run:live},
// {job:hold}, {job:idle}, and {att:idle/live}, and returns once every job
// runs. The run is cancelled when the test ends.
func (f selectorFixture) startJobControlRun() jobControlRun {
	f.e.t.Helper()
	f.add(f.base, "sweep", "--job-name", "hold", "--stage", "batch", "--array", "1-2", "--", "sleep", "300")
	f.add(f.base, "sweep", "--job-name", "idle", "--stage", "single", "--", "sleep", "301")
	f.recordJobs(f.base, "sweep", map[string]string{"hold": "hold", "idle": "idle"}, "")
	client := f.e.command("run", "-b", f.base, "-p", "sweep", "--run-name", "live", "--quiet")
	if err := client.Start(); err != nil {
		f.e.t.Fatal(err)
	}
	f.e.t.Cleanup(func() {
		// Resume first: a suspended job cannot act on the cancel.
		_ = f.e.command("resume", "-b", f.base, "-p", "sweep").Run()
		// Disconnecting a synchronous client cancels its run.
		_ = client.Process.Kill()
		_ = client.Wait()
	})
	keys := map[string]string{"hold-1": f.jobs["hold"] + "-1", "hold-2": f.jobs["hold"] + "-2", "idle": f.jobs["idle"]}
	var liveRunID string
	waitUntil(f.e.t, 15*time.Second, func() (bool, string) {
		runID := f.lastRunID(f.base, "sweep")
		running := strings.Count(f.e.rotari("jobs", "-b", f.base, "sweep", "--format", "%a %s").stdout, " running")
		if runID != f.runs["sweep-second"] && running == len(keys) {
			liveRunID = runID
			return true, "live run is not ready"
		}
		return false, fmt.Sprintf("run=%s running=%d, want run different from %s with %d running jobs", runID, running, f.runs["sweep-second"], len(keys))
	})
	f.runs["live"] = liveRunID
	live := jobControlRun{attemptDirs: map[string]string{}}
	for key, jobID := range keys {
		dirs, err := filepath.Glob(filepath.Join(f.base, "projects", "sweep", "runs", f.runs["live"], jobID, "attempts", "*"))
		if err != nil || len(dirs) != 1 {
			f.e.t.Fatalf("attempts of %s in run live: %q, %v", key, dirs, err)
		}
		live.attemptDirs[key] = dirs[0]
	}
	f.attempts["idle/live"] = filepath.Base(live.attemptDirs["idle"])
	return live
}

// affected reports the live jobs that command acted on: the finished jobs
// for cancel, and the jobs in the scheduler state it sets for suspend and
// resume. The files that show this can lag the command, most for a cancelled
// job, which finishes asynchronously, so it waits briefly for the set to
// reach want.
func (live jobControlRun) affected(t *testing.T, command string, want []string) []string {
	t.Helper()
	var got []string
	waitUntil(t, 5*time.Second, func() (bool, string) {
		got = nil
		for key, dir := range live.attemptDirs {
			switch command {
			case "cancel":
				if _, err := os.Stat(filepath.Join(dir, "finished_at")); err == nil {
					got = append(got, key)
				}
			case "suspend":
				if schedulerState(dir) == "suspended" {
					got = append(got, key)
				}
			case "resume":
				if schedulerState(dir) == "running" {
					got = append(got, key)
				}
			}
		}
		sort.Strings(got)
		return strings.Join(got, ",") == strings.Join(want, ","), fmt.Sprintf("affected=%v, want=%v", got, want)
	})
	return got
}

// schedulerState reads the state an attempt's scheduler_status.json records.
func schedulerState(attemptDir string) string {
	var status struct {
		State string `json:"state"`
	}
	data, err := os.ReadFile(filepath.Join(attemptDir, "scheduler_status.json"))
	if err != nil || json.Unmarshal(data, &status) != nil {
		return ""
	}
	return status.State
}

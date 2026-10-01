package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Contract SEL-10: each command takes the positional arguments of its row in
// "Positional arguments" of contracts/06-selectors.md, with that meaning and
// those exclusions, and the general rules above the table hold.

// positionalCase is one positional-argument row of
// contracts/06-selectors.md. args start with the command and use the
// placeholders of selectorCase, plus {T} for a temporary directory, {M} for
// the master directory, and {run:live} for the run that setup starts.
type positionalCase struct {
	name  string
	args  string
	setup string
	// fail reports that the command must exit non-zero.
	fail bool
	// want is a substring of the command's symbolic output.
	want string
	// check, when set, inspects the state after the command.
	check func(t *testing.T, f selectorFixture, tmp string)
}

// Setups for positionalCase.
const (
	// setupActive starts run "live" of project sweep and leaves it running.
	setupActive = "active"
	// setupInterrupted leaves run "live" of project sweep interrupted.
	setupInterrupted = "interrupted"
	// setupManifest exports run sweep-first to {T}/m.yaml.
	setupManifest = "manifest"
	// setupQueued restores project sweep's latest run into its queue.
	setupQueued = "queued"
	// setupQueuedAll restores the latest run of both sweep and other.
	setupQueuedAll = "queued-all"
)

var positionalCases = []positionalCase{
	// General rules.
	{name: "option after a positional", args: "copy {run:remote-run} --overwrite", want: "copied jobs=1 from run={run:remote-run} to queue=remote"},
	{name: "option between positionals", args: "lineage {run:sweep-first} --json {run:sweep-second}", want: `"run_id": "{run:sweep-second}"`},
	{name: "positional after --", args: "show -b {B} -p sweep -- --job-id", fail: true, want: `selector "--job-id" not found`},
	{name: "job command after its first word", args: "add -b {B} -p other echo --retry 3", check: hasQueuedCommand("other", "echo --retry 3")},
	{name: "two runs", args: "run -b {B} -p sweep {run:sweep-first} {run:sweep-second}", fail: true, want: "usage"},
	{name: "run ID and option", args: "retry -b {B} -p sweep --run-id {run:sweep-first} {run:sweep-first}", fail: true, want: "usage"},
	{name: "command without positionals", args: "web -b {B} extra", fail: true, want: "usage"},
	{name: "command without positionals", args: "config extra", fail: true, want: "usage"},
	{name: "command without positionals", args: "env extra", fail: true},

	// add and change: the job command keeps its own options.
	{name: "job command options", args: "add -b {B} -p other echo --flag", check: hasQueuedCommand("other", "echo --flag")},
	{name: "job command options", args: "change -b {B} -p sweep --job-name prep echo --flag", setup: setupQueued, check: hasQueuedCommand("sweep", "echo --flag")},

	// A project.
	{name: "project", args: "check -b {B} sweep", fail: true, want: "project=sweep state=empty"},
	{name: "project and option", args: "check -b {B} -p sweep sweep", fail: true, want: "usage"},
	{name: "project", args: "reset -b {B} other", want: "reset project=other"},
	{name: "project and option", args: "reset -b {B} -p other other", fail: true, want: "usage"},
	{name: "project", args: "jobs -b {B} other", want: "{job:other-prep}"},
	{name: "project and option", args: "jobs -b {B} -p sweep sweep", fail: true, want: "usage"},
	{name: "project", args: "unlock -b {B} sweep", setup: setupInterrupted, want: "recovered queue project=sweep run_id={run:live}"},
	{name: "project and run ID option", args: "unlock -b {B} --run-id {run:live} sweep", setup: setupInterrupted, want: "recovered queue project=sweep run_id={run:live}"},
	{name: "project and option", args: "unlock -b {B} -p sweep {run:live}", setup: setupInterrupted, fail: true, want: "pass the run ID as --run-id {run:live}"},
	{name: "project and both options", args: "unlock -b {B} -p sweep --run-id {run:live} sweep", setup: setupInterrupted, fail: true, want: "cannot be combined with --project-name"},

	// show: a project, then run and job selectors.
	{name: "project", args: "show -b {B} sweep", want: "Project: sweep"},
	{name: "project name with project option", args: "show -b {B} -p other sweep", fail: true, want: `selector "sweep" not found`},
	{name: "active run name", args: "show -b {B} live", setup: setupActive, want: "Run: live ({run:live})"},
	{name: "run-only option with a queue", args: "show -b {B} -p sweep --failed", setup: setupQueued, want: "Run: second ({run:sweep-second})"},
	{name: "success filter", args: "show -b {B} -p sweep --success", want: "{job:prep}"},
	{name: "success filter with a report", args: "show -b {B} -p sweep --success --report", fail: true, want: "take --failed only"},
	{name: "view with a queue", args: "show -b {B} -p sweep --stage training", setup: setupQueued, want: "SHOW MODE: PROJECT / QUEUE"},

	// export: a queue, else the latest run; never an unsettled run.
	{name: "project with an empty queue", args: "export -b {B} other", want: "exported run {run:other-first} (project other has no queued jobs)"},
	{name: "project with a queue", args: "export -b {B} sweep", setup: setupQueued, want: "name: prep"},
	{name: "project with an active run", args: "export -b {B} sweep", setup: setupActive, fail: true, want: "is still running; wait for it with 'rotari wait sweep'"},
	{name: "active run", args: "export {run:live}", setup: setupActive, fail: true, want: "is still running"},
	{name: "project with an interrupted run", args: "export -b {B} sweep", setup: setupInterrupted, fail: true, want: "was interrupted; recover it with 'rotari unlock sweep' first"},
	{name: "selector and job option", args: "show -b {B} -p sweep prep --job-id {job:prep}", fail: true, want: "cannot be combined"},

	// wait: a project, an active run name, or a run ID.
	{name: "project", args: "wait -b {B} --timeout 50ms sweep", setup: setupActive, fail: true, want: "timed out waiting for run {run:live}"},
	{name: "active run name", args: "wait -b {B} --timeout 50ms live", setup: setupActive, fail: true, want: "timed out waiting for run {run:live}"},
	{name: "run ID through registry", args: "wait --timeout 50ms {run:live}", setup: setupActive, fail: true, want: "timed out waiting for run {run:live}"},
	{name: "finished run ID", args: "wait {run:sweep-first}", fail: true, want: "Run: first ({run:sweep-first})"},
	{name: "several run IDs", args: "wait {run:other-first} {run:remote-run}", want: "Run: remote ({run:remote-run})"},
	{name: "missing project", args: "wait -b {B} nothing"},
	{name: "unknown run ID", args: "wait -b {B} 20990101-000000-deadbeef", fail: true, want: `no project, run name, or run ID matches "20990101-000000-deadbeef"`},
	{name: "project without an active run", args: "wait -b {B} sweep", fail: true, want: "Run: second ({run:sweep-second})"},
	{name: "finished run name", args: "wait -b {B} second", fail: true, want: "Run: second ({run:sweep-second})"},
	{name: "finished run name in two projects", args: "wait -b {B} first", fail: true, want: "project=sweep run={run:sweep-first}"},
	{name: "finished run name in a project", args: "wait -b {B} -p other first", want: "Run: first ({run:other-first})"},

	// A run.
	{name: "run ID through registry", args: "delete {run:sweep-first}", want: "cleared logs project=sweep run={run:sweep-first}"},
	{name: "run ID and option", args: "delete -b {B} -p sweep --run-id {run:sweep-first} {run:sweep-first}", fail: true, want: "usage"},
	{name: "no run", args: "delete -b {B} -p other", fail: true, want: "or --all to delete every run"},
	{name: "every run", args: "delete -b {B} -p other --all", want: "cleared logs project=other"},
	{name: "run ID and every run", args: "delete -b {B} -p sweep --all {run:sweep-first}", fail: true, want: "or --all to delete every run"},
	{name: "run ID and option", args: "copy -b {B} -p sweep --run-id {run:sweep-first} {run:sweep-first}", fail: true, want: "usage"},

	// lineage: none, one, or two runs.
	{name: "no run", args: "lineage -b {B} -p sweep", want: "first ({run:sweep-first})"},
	{name: "one run", args: "lineage {run:sweep-second}", want: "Run: second ({run:sweep-second})"},
	{name: "two runs", args: "lineage {run:sweep-first} {run:sweep-second}", want: "first ({run:sweep-first}) -> second ({run:sweep-second})"},
	{name: "runs of two projects", args: "lineage {run:other-first} {run:sweep-second}", fail: true, want: `run "{run:sweep-second}" not found`},
	{name: "three runs of two projects", args: "lineage {run:sweep-first} {run:sweep-second} {run:other-first}", fail: true, want: `run "{run:other-first}" not found`},
	{name: "old --all", args: "lineage {run:sweep-second} --all", fail: true, want: "flag provided but not defined: -all"},
	{name: "old --all", args: "jobs -b {B} --all", fail: true, want: "flag provided but not defined: -all"},

	// export: a run or a project, and a file.
	{name: "run ID", args: "export {run:sweep-first}", want: "- {run:sweep-first}"},
	{name: "project and option", args: "export -b {B} -p sweep other", fail: true, want: "cannot be combined with --project-name"},
	{name: "latest run", args: "export -b {B} -p sweep latest", want: "- {run:sweep-second}"},
	{name: "run ID and file", args: "export -b {B} -p sweep {run:sweep-first} {T}/out.yaml", check: fileExists("out.yaml")},
	{name: "file and output option", args: "export {run:sweep-first} {T}/out.yaml --output {T}/other.yaml", fail: true, want: "usage"},

	// import: a file and a project.
	{name: "file and project", args: "import -b {B} {T}/m.yaml sweep", setup: setupManifest, check: queueLength("sweep", 5)},
	{name: "project other than the run's", args: "import -b {B} {T}/m.yaml other", setup: setupManifest, fail: true, want: `source project "sweep" does not match destination project "other"`},
	{name: "project and option", args: "import -b {B} -p sweep {T}/m.yaml sweep", setup: setupManifest, fail: true, want: "usage"},

	// diagnose: a job or attempt.
	{name: "job ID", args: "diagnose -b {B} -p sweep --rules {job:train-SEED2}", want: "No known rule-based diagnosis"},
	{name: "job ID in any project", args: "diagnose -b {B} --rules {job:train-SEED2}", want: "No known rule-based diagnosis"},
	{name: "job name", args: "diagnose -b {B} --rules --job-name train-SEED2", want: "No known rule-based diagnosis"},
	{name: "job name in a given run", args: "diagnose -b {B} -p sweep --run-id {run:sweep-first} --rules --job-name train-SEED2", want: "No known rule-based diagnosis"},
	{name: "job name shared by projects", args: "diagnose -b {B} --rules --job-name prep", fail: true, want: "project=other run={run:other-first}"},
	{name: "job name and ID", args: "diagnose -b {B} --rules --job-name prep {job:prep}", fail: true, want: "cannot be combined"},
	{name: "attempt ID through registry", args: "diagnose --rules {att:train-SEED2/0}", want: "No known rule-based diagnosis"},
	{name: "job ID and option", args: "diagnose -b {B} -p sweep --rules --job-id {job:train-SEED2} {job:train-SEED2}", fail: true, want: "usage"},

	// gc: a master directory.
	{name: "master directory", args: "gc {M}", want: "found 0 orphan run registry entries"},
	{name: "master directory and option", args: "gc --masterdir {M} {M}", fail: true, want: "usage"},

	// latest: the latest run wherever a run can be given, and a reserved name.
	{name: "latest run", args: "copy -b {B} -p sweep latest", want: "copied jobs=6 from run={run:sweep-second}"},
	{name: "latest run", args: "delete -b {B} -p sweep latest", want: "cleared logs project=sweep run={run:sweep-second}"},
	{name: "latest run", args: "wait -b {B} -p sweep latest", fail: true, want: "Run: second ({run:sweep-second})"},
	{name: "latest run option", args: "wait -b {B} -p sweep --run-id latest", fail: true, want: "Run: second ({run:sweep-second})"},
	{name: "latest run", args: "show -b {B} -p sweep latest", want: "Run: second ({run:sweep-second})"},
	{name: "latest run", args: "lineage -b {B} -p sweep latest", want: "Run: second ({run:sweep-second})"},
	{name: "reserved job name", args: "add -b {B} -p other --job-name latest true", fail: true, want: `job name "latest" is reserved`},
	{name: "reserved stage", args: "add -b {B} -p other --stage latest true", fail: true, want: `stage "latest" is reserved`},
	{name: "reserved matrix name", args: "add -b {B} -p other --job-name latest --matrix X=1,2 true", fail: true, want: `"latest" is reserved`},
	{name: "reserved project", args: "add -b {B} -p latest true", fail: true, want: `project "latest" is reserved`},
	{name: "reserved run name", args: "run -b {B} -p sweep --run-name latest", fail: true, want: `run name "latest" is reserved`},
	{name: "reserved rename", args: "change -b {B} -p sweep --job-name prep --set-job-name latest", setup: setupQueued, fail: true, want: `job name "latest" is reserved`},

	// A run ID or attempt ID alone resolves the base directory, project, and
	// run. The fixture's base directories are not the default one, so each
	// row shows, by the project and run in its output, that the command
	// found them through the registry; what the command then does is its own.
	{name: "complete run ID", args: "show {run:remote-run}", want: "Run: remote ({run:remote-run})"},
	{name: "complete attempt ID", args: "show {att:train-SEED2/0}", want: "Attempt ID: {att:train-SEED2/0}"},
	{name: "complete run ID", args: "copy {run:remote-run}", want: "copied jobs=1 from run={run:remote-run} to queue=remote"},
	{name: "complete attempt ID", args: "copy -j {att:train-SEED2/0}", want: "copied jobs=1 from run={run:sweep-first} to queue=sweep"},
	{name: "complete run ID", args: "change --run-id {run:remote-run} --all --timeout 1m", want: "changed queue=remote"},
	{name: "complete run ID", args: "remove --run-id {run:remote-run} --all", want: "removed 1 job(s) from queue=remote"},
	{name: "complete run ID", args: "delete {run:remote-run}", want: "cleared logs project=remote run={run:remote-run}"},
	{name: "complete run ID", args: "lineage {run:sweep-second}", want: "Run: second ({run:sweep-second})"},
	{name: "complete run ID", args: "diagnose --rules --run-id {run:sweep-first} --job-name train-SEED2", want: "No known rule-based diagnosis"},
	{name: "complete attempt ID", args: "diagnose --rules {att:train-SEED2/0}", want: "No known rule-based diagnosis"},
	{name: "complete run ID", args: "export {run:remote-run}", want: "- {run:remote-run}"},
	{name: "complete run ID", args: "wait {run:remote-run}", want: "Run: remote ({run:remote-run})"},
	{name: "complete run ID", args: "unlock --run-id {run:live}", setup: setupInterrupted, want: "recovered queue project=sweep run_id={run:live}"},

	// A project that does not exist, for commands that read or edit one.
	{name: "project that does not exist", args: "show -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "empty missing project", args: "check -b {B} nope", fail: true, want: "project=nope state=empty runnable=false queued=0 lock=none"},
	{name: "new project", args: "reset -b {B} nope", want: "reset project=nope cleared=0 job(s)"},
	{name: "missing project", args: "unlock -b {B} nope", want: "project=nope already unlocked"},
	{name: "project that does not exist", args: "copy -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "change -b {B} -p nope --all --timeout 1m", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "remove -b {B} -p nope --all", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "delete -b {B} -p nope --all", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "run -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "lineage -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "export -b {B} nope", fail: true, want: `project "nope" does not exist`},
	{name: "missing project", args: "wait -b {B} -p nope"},
	{name: "project that does not exist", args: "wait -b {B} -p nope --run-id latest", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "show -b {B} -p nope --job-name prep", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "jobs -b {B} nope", fail: true, want: `project "nope" does not exist`},
	{name: "new project", args: "add -b {B} -p fresh true", check: queueLength("fresh", 1)},

	// A job name queued in two projects is ambiguous for queue edits.
	{name: "job name queued in two projects", args: "change -b {B} --job-name prep --timeout 1m", setup: setupQueuedAll, fail: true, want: "project=other queue job={job:other-prep}"},

	// run's positional is a run ID, not the old config alias.
	{name: "no config alias", args: "run -b {B} -p sweep config", fail: true, want: `run "config" not found`},
}

func TestPositionalArguments(t *testing.T) {
	covers(t, "SEL-1", "SEL-10")
	for _, tc := range positionalCases {
		t.Run(strings.Fields(tc.args)[0]+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSelectorFixture(t)
			tmp := t.TempDir()
			f.setUp(tc.setup, tmp)
			var args []string
			for _, arg := range f.expand(tc.args) {
				arg = strings.ReplaceAll(arg, "{T}", tmp)
				arg = strings.ReplaceAll(arg, "{M}", f.e.master)
				args = append(args, arg)
			}
			r := f.e.rotari(args...)
			output := strings.ReplaceAll(f.symbolic(r.stdout+r.stderr), tmp, "{T}")
			if (r.code != 0) != tc.fail {
				t.Fatalf("rotari %s exit code = %d, want failure %v; output:\n%s", tc.args, r.code, tc.fail, output)
			}
			if !strings.Contains(output, tc.want) {
				t.Fatalf("rotari %s output does not contain %q:\n%s", tc.args, tc.want, output)
			}
			if tc.check != nil {
				tc.check(t, f, tmp)
			}
		})
	}
}

func (f selectorFixture) setUp(setup, tmp string) {
	f.e.t.Helper()
	switch setup {
	case "":
	case setupQueued:
		f.restore("sweep-second")
	case setupQueuedAll:
		f.restore("sweep-second")
		f.e.mustRotari("copy", "-b", f.base, "-p", "other", "--run-id", f.runs["other-first"], "--overwrite", "--quiet")
	case setupManifest:
		f.e.mustRotari("export", f.runs["sweep-first"], filepath.Join(tmp, "m.yaml"))
	case setupActive:
		f.startLive()
	case setupInterrupted:
		// Killing the run's supervisor, and its job, leaves the run lock
		// naming a process that is gone.
		f.startLive()
		killStrays(f.e.t, f.e.root)
		waitUntil(f.e.t, 15*time.Second, func() (bool, string) {
			out := f.e.rotari("check", "-b", f.base, "sweep").stdout
			return strings.Contains(out, "state=interrupted"), "check: " + out
		})
	default:
		f.e.t.Fatalf("unknown setup %q", setup)
	}
}

// startLive starts run "live" of project sweep, with one job that sleeps,
// records its ID as {run:live}, and returns once the job runs. The run is
// cancelled when the test ends.
func (f selectorFixture) startLive() {
	f.e.t.Helper()
	f.add(f.base, "sweep", "--job-name", "hold", "--", "sleep", "300")
	client := f.e.command("run", "-b", f.base, "-p", "sweep", "--run-name", "live", "--quiet")
	if err := client.Start(); err != nil {
		f.e.t.Fatal(err)
	}
	f.e.t.Cleanup(func() {
		// Disconnecting a synchronous client cancels its run.
		_ = client.Process.Kill()
		_ = client.Wait()
	})
	waitUntil(f.e.t, 15*time.Second, func() (bool, string) {
		runID := f.lastRunID(f.base, "sweep")
		running := strings.Count(f.e.rotari("jobs", "-b", f.base, "sweep", "--format", "%a %s").stdout, " running")
		if runID != f.runs["sweep-second"] && running == 1 {
			f.runs["live"] = runID
			return true, "live run is not ready"
		}
		return false, fmt.Sprintf("run=%s running=%d, want a new run with one running job", runID, running)
	})
}

// hasQueuedCommand checks that project's queue holds a job with command.
func hasQueuedCommand(project, command string) func(*testing.T, selectorFixture, string) {
	return func(t *testing.T, f selectorFixture, _ string) {
		t.Helper()
		for _, queued := range queueCommands(t, f, project) {
			if strings.Join(queued.Command, " ") == command {
				return
			}
		}
		t.Fatalf("project %s queue has no job with command %q", project, command)
	}
}

// queueLength checks the number of jobs in project's queue.
func queueLength(project string, want int) func(*testing.T, selectorFixture, string) {
	return func(t *testing.T, f selectorFixture, _ string) {
		t.Helper()
		if got := len(queueCommands(t, f, project)); got != want {
			t.Fatalf("project %s queue has %d jobs, want %d", project, got, want)
		}
	}
}

// fileExists checks that the command wrote name in the temporary directory.
func fileExists(name string) func(*testing.T, selectorFixture, string) {
	return func(t *testing.T, _ selectorFixture, tmp string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(tmp, name)); err != nil {
			t.Fatal(err)
		}
	}
}

// queueCommands returns the commands of project's queue in the base
// directory, with the job command each runs.
func queueCommands(t *testing.T, f selectorFixture, project string) []struct {
	Command []string `json:"command"`
} {
	t.Helper()
	var queue struct {
		Commands []struct {
			Command []string `json:"command"`
		} `json:"commands"`
	}
	data, err := os.ReadFile(filepath.Join(f.base, "projects", project, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &queue); err != nil {
		t.Fatal(err)
	}
	return queue.Commands
}

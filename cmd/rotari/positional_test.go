package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// positionalCase is one positional-argument row of
// docs/contracts/06-selectors.md. args start with the command and use the
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
	check func(t *testing.T, fixture selectorFixture, tmp string)
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
	{name: "option between positionals", args: "diff {run:sweep-first} --all {run:sweep-second}", want: "train-SEED1"},
	{name: "positional after --", args: "show -b {B} -p sweep -- --job-id", fail: true, want: `selector "--job-id" not found`},
	{name: "job command after its first word", args: "add -b {B} -p other echo --retry 3", check: queuedCommand("other", "echo --retry 3")},
	{name: "command without positionals", args: "run -b {B} -p sweep extra", fail: true, want: "usage"},
	{name: "command without positionals", args: "retry -b {B} -p sweep extra", fail: true, want: "usage"},
	{name: "command without positionals", args: "web -b {B} extra", fail: true, want: "usage"},
	{name: "command without positionals", args: "config extra", fail: true, want: "usage"},
	{name: "command without positionals", args: "env extra", fail: true},

	// add and change: the job command keeps its own options.
	{name: "job command options", args: "add -b {B} -p other echo --flag", check: queuedCommand("other", "echo --flag")},
	{name: "job command options", args: "change -b {B} -p sweep --job-name prep echo --flag", setup: setupQueued, check: queuedCommand("sweep", "echo --flag")},

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
	{name: "selector and job option", args: "show -b {B} -p sweep prep --job-id {job:prep}", fail: true, want: "cannot be combined"},

	// wait: a project, an active run name, or a run ID.
	{name: "project", args: "wait -b {B} --timeout 50ms sweep", setup: setupActive, fail: true, want: "timed out waiting for run {run:live}"},
	{name: "active run name", args: "wait -b {B} --timeout 50ms live", setup: setupActive, fail: true, want: "timed out waiting for run {run:live}"},
	{name: "run ID through registry", args: "wait --timeout 50ms {run:live}", setup: setupActive, fail: true, want: "timed out waiting for run {run:live}"},
	{name: "finished run ID", args: "wait {run:sweep-first}", fail: true, want: "Run: first ({run:sweep-first})"},
	{name: "several run IDs", args: "wait {run:other-first} {run:remote-run}", want: "Run: remote ({run:remote-run})"},
	{name: "unknown selector", args: "wait -b {B} nothing", fail: true, want: `no project, active run name, or run ID matches "nothing"`},

	// A run.
	{name: "run ID through registry", args: "delete {run:sweep-first}", want: "cleared logs project=sweep run={run:sweep-first}"},
	{name: "run ID and option", args: "delete -b {B} -p sweep --run-id {run:sweep-first} {run:sweep-first}", fail: true, want: "usage"},
	{name: "no run", args: "delete -b {B} -p other", fail: true, want: "or --all to delete every run"},
	{name: "every run", args: "delete -b {B} -p other --all", want: "cleared logs project=other"},
	{name: "run ID and every run", args: "delete -b {B} -p sweep --all {run:sweep-first}", fail: true, want: "or --all to delete every run"},
	{name: "run ID and option", args: "copy -b {B} -p sweep --run-id {run:sweep-first} {run:sweep-first}", fail: true, want: "usage"},

	// diff: none, one, or two runs.
	{name: "no run", args: "diff -b {B} -p sweep", want: "first ({run:sweep-first}) -> second ({run:sweep-second})"},
	{name: "one run", args: "diff {run:sweep-second}", want: "first ({run:sweep-first}) -> second ({run:sweep-second})"},
	{name: "one run without an earlier one", args: "diff {run:sweep-first}", fail: true, want: "no earlier run"},
	{name: "two runs", args: "diff {run:sweep-first} {run:sweep-second}", want: "first ({run:sweep-first}) -> second ({run:sweep-second})"},
	{name: "runs of two projects", args: "diff {run:other-first} {run:sweep-second}", fail: true, want: "runs {run:other-first} and {run:sweep-second} belong to different projects (other and sweep)"},
	{name: "three runs", args: "diff {run:sweep-first} {run:sweep-second} {run:other-first}", fail: true, want: "usage"},

	// export: a run or a project, and a file.
	{name: "run ID", args: "export {run:sweep-first}", want: "- {run:sweep-first}"},
	{name: "project exports its queue", args: "export -b {B} sweep", fail: true, want: "queue has no jobs"},
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
	{name: "attempt ID through registry", args: "diagnose --rules {att:train-SEED2/0}", want: "No known rule-based diagnosis"},
	{name: "job ID and option", args: "diagnose -b {B} -p sweep --rules --job-id {job:train-SEED2} {job:train-SEED2}", fail: true, want: "usage"},

	// gc: a master directory.
	{name: "master directory", args: "gc {M}", want: "found 0 orphan run registry entries"},
	{name: "master directory and option", args: "gc --masterdir {M} {M}", fail: true, want: "usage"},

	// latest: the latest run wherever a run can be given, and a reserved name.
	{name: "latest run", args: "copy -b {B} -p sweep latest", want: "copied jobs=5 from run={run:sweep-second}"},
	{name: "latest run", args: "delete -b {B} -p sweep latest", want: "cleared logs project=sweep run={run:sweep-second}"},
	{name: "latest run", args: "wait -b {B} -p sweep latest", fail: true, want: "Run: second ({run:sweep-second})"},
	{name: "latest run option", args: "wait -b {B} -p sweep --run-id latest", fail: true, want: "Run: second ({run:sweep-second})"},
	{name: "latest run", args: "show -b {B} -p sweep latest", want: "Run: second ({run:sweep-second})"},
	{name: "latest run", args: "diff -b {B} -p sweep latest", want: "first ({run:sweep-first}) -> second ({run:sweep-second})"},
	{name: "reserved job name", args: "add -b {B} -p other --job-name latest true", fail: true, want: `job name "latest" is reserved`},
	{name: "reserved stage", args: "add -b {B} -p other --stage latest true", fail: true, want: `stage "latest" is reserved`},
	{name: "reserved matrix name", args: "add -b {B} -p other --job-name latest --matrix X=1,2 true", fail: true, want: `"latest" is reserved`},
	{name: "reserved project", args: "add -b {B} -p latest true", fail: true, want: `project "latest" is reserved`},
	{name: "reserved run name", args: "run -b {B} -p sweep --run-name latest", fail: true, want: `run name "latest" is reserved`},
	{name: "reserved rename", args: "change -b {B} -p sweep --job-name prep --set-job-name latest", setup: setupQueued, fail: true, want: `job name "latest" is reserved`},

	// A project that does not exist, for commands that read or edit one.
	{name: "project that does not exist", args: "show -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "check -b {B} nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "reset -b {B} nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "unlock -b {B} nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "copy -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "change -b {B} -p nope --all --timeout 1m", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "remove -b {B} -p nope --all", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "delete -b {B} -p nope --all", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "run -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "diff -b {B} -p nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "export -b {B} nope", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "wait -b {B} -p nope --run-id latest", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "show -b {B} -p nope --job-name prep", fail: true, want: `project "nope" does not exist`},
	{name: "project that does not exist", args: "jobs -b {B} nope", fail: true, want: `project "nope" does not exist`},
	{name: "new project", args: "add -b {B} -p fresh true", check: queueLength("fresh", 1)},

	// A job name queued in two projects is ambiguous for queue edits.
	{name: "job name queued in two projects", args: "change -b {B} --job-name prep --timeout 1m", setup: setupQueuedAll, fail: true, want: "project=other queue job={job:other-prep}"},

	// run takes no positionals, not even config.
	{name: "no config alias", args: "run config", fail: true, want: "usage"},
}

func TestPositionalArguments(t *testing.T) {
	for _, tc := range positionalCases {
		t.Run(strings.Fields(tc.args)[0]+"/"+tc.name, func(t *testing.T) {
			fixture := newSelectorFixture(t)
			tmp := t.TempDir()
			fixture.setUp(t, tc.setup, tmp)
			var args []string
			for _, arg := range fixture.expand(tc.args) {
				arg = strings.ReplaceAll(arg, "{T}", tmp)
				arg = strings.ReplaceAll(arg, "{M}", fixture.MasterDir)
				arg = strings.ReplaceAll(arg, "{run:live}", fixture.Runs["live"])
				args = append(args, arg)
			}
			code, output := captureSelectorOutput(func() int { return run(args) })
			output = fixture.symbolic(output)
			output = strings.ReplaceAll(output, tmp, "{T}")
			if (code != 0) != tc.fail {
				t.Fatalf("rotari %s exit code = %d, want failure %v; output:\n%s", tc.args, code, tc.fail, output)
			}
			if !strings.Contains(output, tc.want) {
				t.Fatalf("rotari %s output does not contain %q:\n%s", tc.args, tc.want, output)
			}
			if tc.check != nil {
				tc.check(t, fixture, tmp)
			}
		})
	}
}

func (fixture selectorFixture) setUp(t *testing.T, setup, tmp string) {
	t.Helper()
	sweep := fixture.paths(t, fixture.BaseDir, "sweep")
	switch setup {
	case "":
	case setupQueued:
		fixture.restore(t, fixture.BaseDir, "sweep", fixture.Runs["sweep-second"])
	case setupQueuedAll:
		fixture.restore(t, fixture.BaseDir, "sweep", fixture.Runs["sweep-second"])
		fixture.restore(t, fixture.BaseDir, "other", fixture.Runs["other-first"])
	case setupManifest:
		if code, output := captureSelectorOutput(func() int {
			return cmdExport([]string{fixture.Runs["sweep-first"], filepath.Join(tmp, "m.yaml")})
		}); code != 0 {
			t.Fatalf("export failed:\n%s", output)
		}
	case setupActive, setupInterrupted:
		// The lock records this test process, which is alive, so the run
		// counts as active until the lock names a process that is gone.
		fixture.Runs["live"] = makeRunID()
		if err := projectRunner().Begin(sweep, projectrun.Start{RunID: fixture.Runs["live"], RunName: "live", CWD: tmp}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(sweep.LockFile) })
		if setup == setupInterrupted {
			lock, err := state.LoadLock(sweep.LockFile)
			if err != nil {
				t.Fatal(err)
			}
			lock.PID = 1 << 30
			if err := writeJSON(sweep.LockFile, lock); err != nil {
				t.Fatal(err)
			}
		}
	default:
		t.Fatalf("unknown setup %q", setup)
	}
}

// queuedCommand checks that project's queue holds a job with command.
func queuedCommand(project, command string) func(*testing.T, selectorFixture, string) {
	return func(t *testing.T, fixture selectorFixture, _ string) {
		t.Helper()
		queue, err := state.LoadQueue(fixture.paths(t, fixture.BaseDir, project).QueueFile)
		if err != nil {
			t.Fatal(err)
		}
		for _, queued := range queue.Commands {
			if strings.Join(queued.Command, " ") == command {
				return
			}
		}
		t.Fatalf("project %s queue has no job with command %q", project, command)
	}
}

// queueLength checks the number of jobs in project's queue.
func queueLength(project string, want int) func(*testing.T, selectorFixture, string) {
	return func(t *testing.T, fixture selectorFixture, _ string) {
		t.Helper()
		queue, err := state.LoadQueue(fixture.paths(t, fixture.BaseDir, project).QueueFile)
		if err != nil {
			t.Fatal(err)
		}
		if len(queue.Commands) != want {
			t.Fatalf("project %s queue has %d jobs, want %d", project, len(queue.Commands), want)
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

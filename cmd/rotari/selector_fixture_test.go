package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/queueedit"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// selectorFixture is shared state for testing how commands resolve their
// project, run, and job selectors (docs/contracts/06-selectors.md). It is
// built through the real add and run paths, so run IDs, attempt IDs, the run
// registry, and run names are laid out exactly as a user would see them.
//
// Layout, with the symbolic keys of Runs, Jobs, and Attempts in brackets:
//
//	BaseDir
//	  project "sweep"
//	    prep            stage setup, succeeds                          [prep]
//	    train-SEED1/2   matrix "train", stage training, --retry 1;
//	                    SEED 2 fails both attempts                     [train-SEED1] [train-SEED2]
//	    eval            array 1-3, stage evaluation; task 2 fails            [eval]
//	    (unnamed)       stage report, succeeds                         [report]
//	    late            added after run "first", never runs            [late]
//	    run "first":  every job but late                               [sweep-first]
//	    run "second": failed jobs only; the others carry forward, and
//	                  late, with no result to select, stays unfinished [sweep-second]
//	  project "other"
//	    prep            also named prep, succeeds                      [other-prep]
//	    run "first"                                                     [other-first]
//	OtherBaseDir, reachable only through the run registry
//	  project "remote"
//	    solo            succeeds                                        [solo]
//	    run "remote"                                                    [remote-run]
//
// Attempts: [train-SEED2/0] and [train-SEED2/1] in sweep-first, and
// [train-SEED2/second] in sweep-second. After the runs every queue is empty.
type selectorFixture struct {
	BaseDir      string
	OtherBaseDir string
	MasterDir    string
	Runs         map[string]string
	Jobs         map[string]string
	Attempts     map[string]string
}

// newSelectorFixture builds a fresh fixture. Selector resolution must not
// depend on the caller's environment, so location variables and user config
// are cleared for the test.
func newSelectorFixture(t *testing.T) selectorFixture {
	t.Helper()
	fixture := selectorFixture{
		BaseDir:      t.TempDir(),
		OtherBaseDir: t.TempDir(),
		MasterDir:    t.TempDir(),
		Runs:         make(map[string]string),
		Jobs:         make(map[string]string),
		Attempts:     make(map[string]string),
	}
	t.Setenv("ROTARI_MASTERDIR", fixture.MasterDir)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, name := range []string{envBaseDir, envProjectName, envRunID, envJobID, envJobName, envExecutor, envExecutorOpts} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}

	fixture.add(t, fixture.BaseDir, "sweep", "--job-name", "prep", "--stage", "setup", "--", "true")
	fixture.add(t, fixture.BaseDir, "sweep", "--job-name", "train", "--matrix", "SEED=1,2", "--stage", "training", "--retry", "1", "--", "sh", "-c", `test "$SEED" = 1`)
	fixture.add(t, fixture.BaseDir, "sweep", "--job-name", "eval", "--array", "1-3", "--stage", "evaluation", "--", "sh", "-c", `test "$ROTARI_ARRAY_TASK_ID" != 2`)
	fixture.add(t, fixture.BaseDir, "sweep", "--stage", "report", "--", "true")
	fixture.recordJobs(t, fixture.BaseDir, "sweep", map[string]string{"prep": "prep", "train-SEED1": "train-SEED1", "train-SEED2": "train-SEED2", "eval": "eval"}, "report")
	fixture.Runs["sweep-first"] = fixture.run(t, fixture.BaseDir, "sweep", "first", "")
	fixture.Attempts["train-SEED2/0"] = state.MakeAttemptID(fixture.Runs["sweep-first"], fixture.Jobs["train-SEED2"], 0)
	fixture.Attempts["train-SEED2/1"] = state.MakeAttemptID(fixture.Runs["sweep-first"], fixture.Jobs["train-SEED2"], 1)
	fixture.restore(t, fixture.BaseDir, "sweep", fixture.Runs["sweep-first"])
	fixture.add(t, fixture.BaseDir, "sweep", "--job-name", "late", "--", "true")
	fixture.recordJobs(t, fixture.BaseDir, "sweep", map[string]string{"late": "late"}, "")
	fixture.Runs["sweep-second"] = fixture.run(t, fixture.BaseDir, "sweep", "second", "failed")
	fixture.Attempts["train-SEED2/second"] = state.MakeAttemptID(fixture.Runs["sweep-second"], fixture.Jobs["train-SEED2"], 0)

	fixture.add(t, fixture.BaseDir, "other", "--job-name", "prep", "--", "true")
	fixture.recordJobs(t, fixture.BaseDir, "other", map[string]string{"prep": "other-prep"}, "")
	fixture.Runs["other-first"] = fixture.run(t, fixture.BaseDir, "other", "first", "")

	fixture.add(t, fixture.OtherBaseDir, "remote", "--job-name", "solo", "--", "true")
	fixture.recordJobs(t, fixture.OtherBaseDir, "remote", map[string]string{"solo": "solo"}, "")
	fixture.Runs["remote-run"] = fixture.run(t, fixture.OtherBaseDir, "remote", "remote", "")
	return fixture
}

func (fixture selectorFixture) add(t *testing.T, baseDir, project string, args ...string) {
	t.Helper()
	full := append([]string{"--basedir", baseDir, "--project-name", project, "--quiet"}, args...)
	if code := cmdAdd(full); code != 0 {
		t.Fatalf("rotari add %v exit code = %d", args, code)
	}
}

// recordJobs stores the queued job IDs of project under symbolic keys: named
// jobs by their name, and the one unnamed job, if any, under unnamedKey.
func (fixture selectorFixture) recordJobs(t *testing.T, baseDir, project string, keysByName map[string]string, unnamedKey string) {
	t.Helper()
	paths := fixture.paths(t, baseDir, project)
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range queue.Commands {
		key, ok := keysByName[command.Name]
		if command.Name == "" {
			key, ok = unnamedKey, unnamedKey != ""
		}
		if !ok {
			continue
		}
		if _, exists := fixture.Jobs[key]; exists {
			t.Fatalf("fixture job key %q recorded twice", key)
		}
		fixture.Jobs[key] = command.ID
	}
	for _, key := range keysByName {
		if fixture.Jobs[key] == "" {
			t.Fatalf("fixture job %q was not queued", key)
		}
	}
}

// run executes project's queue in process, as the run worker does, and
// returns the run ID. A selection re-executes only matching jobs.
func (fixture selectorFixture) run(t *testing.T, baseDir, project, runName, selection string) string {
	t.Helper()
	paths := fixture.paths(t, baseDir, project)
	runID := makeRunID()
	// Like cmdRun, a selection reads jobs without an origin from the run
	// that was latest before this one begins.
	sourceRunID := ""
	if selection != "" {
		meta, err := state.LoadMeta(paths.MetaFile)
		if err != nil {
			t.Fatal(err)
		}
		sourceRunID = meta.LastRunID
	}
	runner := projectRunner()
	if err := runner.Begin(paths, projectrun.Start{RunID: runID, RunName: runName, CWD: baseDir}); err != nil {
		t.Fatal(err)
	}
	options := projectrun.Options{RunID: runID, RunName: runName, LocalConcurrency: 4, BatchMaxActive: 1, Selection: selection, SourceRunID: sourceRunID, PartialArray: true}
	if _, err := runner.Run(paths, options, projectrun.Observer{}); err != nil {
		t.Fatalf("run %s of %s failed: %v", runName, project, err)
	}
	return runID
}

// restore copies every job of runID back into the empty queue, as retry does
// before planning a filtered run.
func (fixture selectorFixture) restore(t *testing.T, baseDir, project, runID string) {
	t.Helper()
	if _, err := queueEditor().Copy(baseDir, project, runID, queueedit.CopyRequest{Selection: "all", Overwrite: true}); err != nil {
		t.Fatal(err)
	}
}

func (fixture selectorFixture) paths(t *testing.T, baseDir, project string) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

// TestSelectorFixtureLayout checks that the fixture has the layout its
// documentation promises, so selector tests can rely on it.
func TestSelectorFixtureLayout(t *testing.T) {
	fixture := newSelectorFixture(t)
	sweep := fixture.paths(t, fixture.BaseDir, "sweep")
	for _, key := range []string{"sweep-first", "sweep-second"} {
		if _, err := os.Stat(filepath.Join(sweep.RunsDir, fixture.Runs[key])); err != nil {
			t.Fatalf("run %s missing: %v", key, err)
		}
	}
	for key, attemptID := range fixture.Attempts {
		payload, err := state.DecodeAttemptID(attemptID)
		if err != nil {
			t.Fatal(err)
		}
		dir, err := state.SpecificAttemptJobDir(filepath.Join(sweep.RunsDir, payload.RunID), payload.JobID, attemptID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("attempt %s (%s) missing: %v", key, attemptID, err)
		}
	}
	for _, location := range [][2]string{{fixture.BaseDir, "sweep"}, {fixture.BaseDir, "other"}, {fixture.OtherBaseDir, "remote"}} {
		queue, err := state.LoadQueue(fixture.paths(t, location[0], location[1]).QueueFile)
		if err != nil || len(queue.Commands) != 0 {
			t.Fatalf("project %s queue = %d jobs, err %v; want empty", location[1], len(queue.Commands), err)
		}
	}
	registry, err := runregistry.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"sweep-first", "sweep-second", "other-first", "remote-run"} {
		location, found, err := registry.Lookup(fixture.Runs[key])
		if err != nil || !found {
			t.Fatalf("run %s not registered: found=%v err=%v", key, found, err)
		}
		wantBase := fixture.BaseDir
		if key == "remote-run" {
			wantBase = fixture.OtherBaseDir
		}
		if location.BaseDir != wantBase {
			t.Fatalf("run %s registered under %q, want %q", key, location.BaseDir, wantBase)
		}
	}
}

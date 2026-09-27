package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// selectorFixture is the state that the selector tests run against
// (contracts/06-selectors.md, "Fixture"). It is built with the binary, so run
// IDs, attempt IDs, the run registry, and run names are laid out exactly as a
// user would see them.
//
// Layout, with the symbolic keys of runs, jobs, and attempts in brackets:
//
//	base
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
//	otherBase, reachable only through the run registry
//	  project "remote"
//	    solo            succeeds                                        [solo]
//	    run "remote"                                                    [remote-run]
//
// Attempts: [train-SEED2/0] and [train-SEED2/1] in sweep-first, and
// [train-SEED2/second] in sweep-second. After the runs every queue is empty.
//
// The environment has no ROTARI_BASEDIR, so commands find a base directory
// only through -b or the run registry.
type selectorFixture struct {
	e         *env
	base      string
	otherBase string
	runs      map[string]string
	jobs      map[string]string
	attempts  map[string]string
}

func newSelectorFixture(t *testing.T) selectorFixture {
	t.Helper()
	requireUnixSockets(t)
	e := newEnv(t).in(t).without("ROTARI_BASEDIR")
	f := selectorFixture{
		e:         e,
		base:      filepath.Join(e.root, "base"),
		otherBase: filepath.Join(e.root, "other-base"),
		runs:      map[string]string{},
		jobs:      map[string]string{},
		attempts:  map[string]string{},
	}

	f.add(f.base, "sweep", "--job-name", "prep", "--stage", "setup", "--", "true")
	f.add(f.base, "sweep", "--job-name", "train", "--matrix", "SEED=1,2", "--stage", "training", "--retry", "1", "--", "sh", "-c", `test "$SEED" = 1`)
	f.add(f.base, "sweep", "--job-name", "eval", "--array", "1-3", "--stage", "evaluation", "--", "sh", "-c", `test "$ROTARI_ARRAY_TASK_ID" != 2`)
	f.add(f.base, "sweep", "--stage", "report", "--", "true")
	f.recordJobs(f.base, "sweep", map[string]string{"prep": "prep", "train-SEED1": "train-SEED1", "train-SEED2": "train-SEED2", "eval": "eval"}, "report")
	f.runs["sweep-first"] = f.run(f.base, "sweep", "first")
	f.attempts["train-SEED2/0"], f.attempts["train-SEED2/1"] = f.attempt("sweep-first", "train-SEED2", 0), f.attempt("sweep-first", "train-SEED2", 1)
	f.restore("sweep-first")
	f.add(f.base, "sweep", "--job-name", "late", "--", "true")
	f.recordJobs(f.base, "sweep", map[string]string{"late": "late"}, "")
	f.runs["sweep-second"] = f.run(f.base, "sweep", "second", "--failed")
	f.attempts["train-SEED2/second"] = f.attempt("sweep-second", "train-SEED2", 0)

	f.add(f.base, "other", "--job-name", "prep", "--", "true")
	f.recordJobs(f.base, "other", map[string]string{"prep": "other-prep"}, "")
	f.runs["other-first"] = f.run(f.base, "other", "first")

	f.add(f.otherBase, "remote", "--job-name", "solo", "--", "true")
	f.recordJobs(f.otherBase, "remote", map[string]string{"solo": "solo"}, "")
	f.runs["remote-run"] = f.run(f.otherBase, "remote", "remote")
	return f
}

func (f selectorFixture) add(baseDir, project string, args ...string) {
	f.e.t.Helper()
	f.e.mustRotari(append([]string{"add", "-b", baseDir, "-p", project, "--quiet"}, args...)...)
}

// queuedCommand is the part of a queue.json command the tests read.
type queuedCommand struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Timeout string `json:"timeout"`
	Origin  *struct {
		RunID string `json:"run_id"`
	} `json:"origin"`
	Array *struct {
		Tasks []int `json:"tasks"`
	} `json:"array"`
}

func (f selectorFixture) queue(baseDir, project string) []queuedCommand {
	f.e.t.Helper()
	var queue struct {
		Commands []queuedCommand `json:"commands"`
	}
	data, err := os.ReadFile(filepath.Join(baseDir, "projects", project, "queue.json"))
	if err != nil {
		f.e.t.Fatal(err)
	}
	if err := json.Unmarshal(data, &queue); err != nil {
		f.e.t.Fatal(err)
	}
	return queue.Commands
}

// recordJobs stores the queued job IDs of project under symbolic keys: named
// jobs by their name, and the one unnamed job, if any, under unnamedKey.
func (f selectorFixture) recordJobs(baseDir, project string, keysByName map[string]string, unnamedKey string) {
	f.e.t.Helper()
	for _, command := range f.queue(baseDir, project) {
		key, ok := keysByName[command.Name]
		if command.Name == "" {
			key, ok = unnamedKey, unnamedKey != ""
		}
		if ok {
			f.jobs[key] = command.ID
		}
	}
	for _, key := range keysByName {
		if f.jobs[key] == "" {
			f.e.t.Fatalf("fixture job %q was not queued", key)
		}
	}
}

// run runs project's queue under runName and returns the run ID. A run whose
// jobs fail exits non-zero, which the fixture expects.
func (f selectorFixture) run(baseDir, project, runName string, args ...string) string {
	f.e.t.Helper()
	f.e.rotari(append([]string{"run", "-b", baseDir, "-p", project, "--run-name", runName, "--quiet"}, args...)...)
	runID := f.lastRunID(baseDir, project)
	if runID == "" {
		f.e.t.Fatalf("run %s of %s did not start", runName, project)
	}
	return runID
}

func (f selectorFixture) lastRunID(baseDir, project string) string {
	f.e.t.Helper()
	var meta struct {
		LastRunID string `json:"last_run_id"`
	}
	data, err := os.ReadFile(filepath.Join(baseDir, "projects", project, "meta.json"))
	if err != nil {
		f.e.t.Fatal(err)
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		f.e.t.Fatal(err)
	}
	return meta.LastRunID
}

// attempt returns the ID of job's attempt number n in run, from the run's
// attempt directories.
func (f selectorFixture) attempt(run, job string, n int) string {
	f.e.t.Helper()
	dirs, err := filepath.Glob(filepath.Join(f.base, "projects", "sweep", "runs", f.runs[run], f.jobs[job], "attempts", "*-"+strconv.Itoa(n)))
	if err != nil || len(dirs) != 1 {
		f.e.t.Fatalf("attempt %d of %s in %s: %q, %v", n, job, run, dirs, err)
	}
	return filepath.Base(dirs[0])
}

// restore copies every job of project sweep's run back into its queue.
func (f selectorFixture) restore(run string) {
	f.e.t.Helper()
	f.e.mustRotari("copy", "-b", f.base, "-p", "sweep", "--run-id", f.runs[run], "--overwrite", "--quiet")
}

// expand replaces {B}, {OB}, {run:KEY}, {job:KEY}, and {att:KEY} in args.
func (f selectorFixture) expand(args string) []string {
	var expanded []string
	for _, arg := range strings.Fields(args) {
		arg = strings.ReplaceAll(arg, "{B}", f.base)
		arg = strings.ReplaceAll(arg, "{OB}", f.otherBase)
		for key, value := range f.runs {
			arg = strings.ReplaceAll(arg, "{run:"+key+"}", value)
		}
		for key, value := range f.attempts {
			arg = strings.ReplaceAll(arg, "{att:"+key+"}", value)
		}
		for key, value := range f.jobs {
			arg = strings.ReplaceAll(arg, "{job:"+key+"}", value)
		}
		expanded = append(expanded, arg)
	}
	return expanded
}

// symbolic replaces the fixture's directories and generated IDs in text with
// their keys.
func (f selectorFixture) symbolic(text string) string {
	text = strings.ReplaceAll(text, f.otherBase, "{OB}")
	text = strings.ReplaceAll(text, f.base, "{B}")
	for key, value := range f.attempts {
		text = strings.ReplaceAll(text, value, "{att:"+key+"}")
	}
	for key, value := range f.runs {
		text = strings.ReplaceAll(text, value, "{run:"+key+"}")
	}
	for key, value := range f.jobs {
		text = strings.ReplaceAll(text, value, "{job:"+key+"}")
	}
	return text
}

// jobKey returns the key of a job or array task ID.
func (f selectorFixture) jobKey(id string) string {
	for key, value := range f.jobs {
		if id == value {
			return key
		}
		if task, ok := strings.CutPrefix(id, value+"-"); ok {
			return key + "-" + task
		}
	}
	return "?" + id
}

func (f selectorFixture) runKey(id string) string {
	for key, value := range f.runs {
		if id == value {
			return key
		}
	}
	return "?" + id
}

// commandKeys returns a queued command's key, or its task keys when it is an
// array narrowed to some of its tasks.
func (f selectorFixture) commandKeys(command queuedCommand) []string {
	key := f.jobKey(command.ID)
	if command.Array == nil || len(command.Array.Tasks) == 0 {
		return []string{key}
	}
	keys := make([]string, 0, len(command.Array.Tasks))
	for _, task := range command.Array.Tasks {
		keys = append(keys, f.jobKey(command.ID+"-"+strconv.Itoa(task)))
	}
	return keys
}

// TestSelectorFixtureLayout checks that the fixture has the layout its
// documentation promises, so the selector tests can rely on it.
func TestSelectorFixtureLayout(t *testing.T) {
	t.Parallel()
	f := newSelectorFixture(t)
	for key, attemptID := range f.attempts {
		if !strings.HasPrefix(attemptID, "att_") {
			t.Errorf("attempt %s = %q, want an attempt ID", key, attemptID)
		}
	}
	for _, location := range [][2]string{{f.base, "sweep"}, {f.base, "other"}, {f.otherBase, "remote"}} {
		if queue := f.queue(location[0], location[1]); len(queue) != 0 {
			t.Errorf("project %s queue has %d jobs; want it empty", location[1], len(queue))
		}
	}
	// Each run is registered: its ID alone finds it.
	for _, key := range []string{"sweep-first", "sweep-second", "other-first", "remote-run"} {
		if r := f.e.rotari("show", f.runs[key]); r.code != 0 || !strings.Contains(r.stdout, f.runs[key]) {
			t.Errorf("run %s is not found through the registry: %s", key, r)
		}
	}
	jobs := make([]string, 0, len(f.jobs))
	for key := range f.jobs {
		jobs = append(jobs, key)
	}
	sort.Strings(jobs)
	if strings.Join(jobs, ",") != "eval,late,other-prep,prep,report,solo,train-SEED1,train-SEED2" {
		t.Errorf("fixture jobs = %v", jobs)
	}
}

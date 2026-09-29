package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Contracts SEL-1 to SEL-8: which run and jobs each command's selectors
// name. See contracts/06-selectors.md.

// selectorCase is one row of the selector tables in
// contracts/06-selectors.md, run against newSelectorFixture.
//
// Arguments may use {B} and {OB} for the fixture's base directories, and
// {run:KEY}, {job:KEY}, and {att:KEY} for its generated IDs. Results are
// reported with the same symbolic keys; an array task is KEY-N.
type selectorCase struct {
	name string
	cmd  string
	args string
	// queued restores project sweep's latest run into its queue first, for
	// queue edits.
	queued bool
	// change, when set, are the arguments of a change run on the restored
	// queue before the command, to edit a job first.
	change string
	// table reads show's jobs from the rows of a job table instead of a
	// single job's details.
	table bool
	// jobs are the jobs the command acted on: the job shown, the commands
	// copied, changed, or removed, or the jobs a run executed.
	jobs []string
	// run is the run the command read: the run shown, or the source run of
	// copied jobs.
	run string
	// err is a substring of the expected error; jobs and run are then empty.
	err string
}

// selectorResult is what a selector case observed.
type selectorResult struct {
	jobs []string
	run  string
	err  string
}

func TestSelectorTable(t *testing.T) {
	covers(t, "SEL-1", "SEL-2", "SEL-3", "SEL-4", "SEL-5", "SEL-6", "SEL-7", "SEL-8", "SEL-11")
	for _, tc := range selectorCases {
		t.Run(tc.cmd+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSelectorFixture(t)
			if mismatch := f.runSelectorCase(tc).mismatch(tc); mismatch != "" {
				t.Fatalf("rotari %s %s: %s", tc.cmd, tc.args, mismatch)
			}
		})
	}
}

func (got selectorResult) mismatch(tc selectorCase) string {
	if tc.err != "" {
		if !strings.Contains(got.err, tc.err) {
			return "error = " + quoteOrNone(got.err) + ", want one containing " + quoteOrNone(tc.err)
		}
		return ""
	}
	if got.err != "" {
		return "error = " + quoteOrNone(got.err)
	}
	want := append([]string(nil), tc.jobs...)
	sort.Strings(want)
	if strings.Join(got.jobs, ",") != strings.Join(want, ",") {
		return "jobs = [" + strings.Join(got.jobs, ",") + "], want [" + strings.Join(want, ",") + "]"
	}
	if tc.run != "" && got.run != tc.run {
		return "run = " + quoteOrNone(got.run) + ", want " + quoteOrNone(tc.run)
	}
	return ""
}

func quoteOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return `"` + value + `"`
}

func (f selectorFixture) runSelectorCase(tc selectorCase) selectorResult {
	f.e.t.Helper()
	if tc.queued {
		f.restore("sweep-second")
	}
	if tc.change != "" {
		f.e.mustRotari(append(append([]string{"change"}, f.expand(tc.change)...), "--quiet")...)
	}
	before := f.queue(f.base, "sweep")
	args := append([]string{tc.cmd}, f.expand(tc.args)...)
	if tc.cmd == "change" {
		args = append(args, "--timeout", "7m")
	}
	r := f.e.rotari(args...)
	if tc.cmd == "run" || tc.cmd == "retry" {
		// A run whose jobs fail exits non-zero; it still executed them.
		if result, ran := f.observeRun(); ran {
			return result
		}
	}
	if r.code != 0 {
		return selectorResult{err: f.symbolic(strings.TrimSpace(r.stdout + r.stderr))}
	}
	switch tc.cmd {
	case "show":
		return f.observeShow(r.stdout, tc.table)
	case "copy":
		return f.observeQueue(func(queuedCommand) bool { return true })
	case "change":
		return f.observeQueue(func(command queuedCommand) bool { return command.Timeout == "7m" })
	case "remove":
		return f.observeRemoved(before)
	default:
		return selectorResult{err: "no new run"}
	}
}

var (
	showRunLine     = regexp.MustCompile(`(?m)^Run: .*?(\d{8}-\d{6}-[0-9a-f]{8})`)
	showAttemptLine = regexp.MustCompile(`(?m)^Attempt ID: (att_\S+)`)
)

// observeShow reads the run and job that a show view displays, or with table
// the jobs of its job table.
func (f selectorFixture) observeShow(output string, table bool) selectorResult {
	var result selectorResult
	if match := showRunLine.FindStringSubmatch(output); match != nil {
		result.run = f.runKey(match[1])
	}
	if table {
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if key := f.jobKey(fields[0]); !strings.HasPrefix(key, "?") {
				result.jobs = append(result.jobs, key)
			}
		}
		sort.Strings(result.jobs)
		return result
	}
	if match := showAttemptLine.FindStringSubmatch(output); match != nil {
		if jobID, ok := attemptJobID(match[1]); ok {
			result.jobs = []string{f.jobKey(jobID)}
		}
	}
	return result
}

// runIDLength is the length of a run ID such as 20260927-052653-765c4b90.
const runIDLength = len("20060102-150405-0123abcd")

// attemptJobID returns the job ID in an attempt ID, which has the form
// att_RUNID-JOBID-N.
func attemptJobID(attemptID string) (string, bool) {
	value, ok := strings.CutPrefix(attemptID, "att_")
	separator := strings.LastIndexByte(value, '-')
	if !ok || separator <= runIDLength+1 {
		return "", false
	}
	return value[runIDLength+1 : separator], true
}

// observeQueue reports the queued commands that keep reports true, in
// project sweep and in the projects that stay empty unless the arguments
// name them.
func (f selectorFixture) observeQueue(keep func(queuedCommand) bool) selectorResult {
	var result selectorResult
	for _, command := range f.queue(f.base, "sweep") {
		if !keep(command) {
			continue
		}
		result.jobs = append(result.jobs, f.commandKeys(command)...)
		if command.Origin != nil && result.run == "" {
			result.run = f.runKey(command.Origin.RunID)
		}
	}
	for _, project := range []struct{ baseDir, name string }{{f.base, "other"}, {f.otherBase, "remote"}} {
		for _, command := range f.queue(project.baseDir, project.name) {
			if !keep(command) {
				continue
			}
			result.jobs = append(result.jobs, project.name+":"+f.jobKey(command.ID))
			if command.Origin != nil && result.run == "" {
				result.run = f.runKey(command.Origin.RunID)
			}
		}
	}
	sort.Strings(result.jobs)
	return result
}

// observeRemoved reports the commands of project sweep's queue that are gone
// after the command, compared with before or, when the queue was empty and
// the command worked on the latest run, with that run's snapshot.
func (f selectorFixture) observeRemoved(before []queuedCommand) selectorResult {
	f.e.t.Helper()
	if len(before) == 0 {
		before = f.snapshot("sweep-second")
	}
	remaining := map[string]bool{}
	for _, command := range f.queue(f.base, "sweep") {
		remaining[command.ID] = true
	}
	var result selectorResult
	for _, command := range before {
		if !remaining[command.ID] {
			result.jobs = append(result.jobs, f.jobKey(command.ID))
		}
	}
	sort.Strings(result.jobs)
	return result
}

// snapshot returns the commands that run of project sweep ran with.
func (f selectorFixture) snapshot(run string) []queuedCommand {
	f.e.t.Helper()
	var queue struct {
		Commands []queuedCommand `json:"commands"`
	}
	data, err := os.ReadFile(filepath.Join(f.base, "projects", "sweep", "runs", f.runs[run], "commands.json"))
	if err != nil {
		f.e.t.Fatal(err)
	}
	if err := json.Unmarshal(data, &queue); err != nil {
		f.e.t.Fatal(err)
	}
	return queue.Commands
}

// observeRun reports the jobs that project sweep's newest run executed: the
// job directories in it. It reports false when no run started.
func (f selectorFixture) observeRun() (selectorResult, bool) {
	f.e.t.Helper()
	runID := f.lastRunID(f.base, "sweep")
	if runID == f.runs["sweep-second"] {
		return selectorResult{}, false
	}
	entries, err := os.ReadDir(filepath.Join(f.base, "projects", "sweep", "runs", runID))
	if err != nil {
		f.e.t.Fatal(err)
	}
	var result selectorResult
	for _, entry := range entries {
		if entry.IsDir() {
			result.jobs = append(result.jobs, f.jobKey(entry.Name()))
		}
	}
	sort.Strings(result.jobs)
	return result, true
}

// TestRunJobIDRunsEditedQueue checks the fix-and-retry loop: a job changed
// in the queue runs with its edit, instead of the queue being replaced by
// the latest run.
func TestRunJobIDRunsEditedQueue(t *testing.T) {
	covers(t, "SEL-5")
	t.Parallel()
	f := newSelectorFixture(t)
	f.restore("sweep-second")
	f.e.mustRotari("change", "-b", f.base, "-p", "sweep", "--job-name", "prep", "--quiet", "echo", "edited")
	// The run fails: it carries the failures of the jobs it does not run.
	r := f.e.rotari("run", "-b", f.base, "-p", "sweep", "--job-id", f.jobs["prep"])
	runID := f.lastRunID(f.base, "sweep")
	if runID == f.runs["sweep-second"] {
		t.Fatalf("run --job-id did not start a run: %s", r)
	}
	var snapshot struct {
		Commands []struct {
			ID      string   `json:"id"`
			Command []string `json:"command"`
		} `json:"commands"`
	}
	data, err := os.ReadFile(filepath.Join(f.base, "projects", "sweep", "runs", runID, "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, command := range snapshot.Commands {
		if command.ID == f.jobs["prep"] && strings.Join(command.Command, " ") != "echo edited" {
			t.Fatalf("run executed prep as %q, want the edited command", strings.Join(command.Command, " "))
		}
	}
}

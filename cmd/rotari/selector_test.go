package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// selectorCase is one row of the selector table in
// docs/contracts/06-selectors.md, run against newSelectorFixture.
//
// Arguments may use {B} and {OB} for the fixture's base directories,
// {run:KEY}, {job:KEY}, and {att:KEY} for its generated IDs. Results are
// reported with the same symbolic keys; an array task is KEY-N.
type selectorCase struct {
	name string
	cmd  string
	args string
	// queued restores project sweep's latest run into its queue first, for
	// queue edits.
	queued bool
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
	// known names the ISSUES.md entry for a row whose current behavior
	// differs from the table. The row must fail until the issue is fixed,
	// and then the mark must be removed.
	known string
}

// selectorResult is what a selector case observed.
type selectorResult struct {
	jobs []string
	run  string
	err  string
}

func TestSelectorTable(t *testing.T) {
	for _, tc := range selectorCases {
		t.Run(tc.cmd+"/"+tc.name, func(t *testing.T) {
			fixture := newSelectorFixture(t)
			got := fixture.runSelectorCase(t, tc)
			mismatch := got.mismatch(tc)
			switch {
			case tc.known == "" && mismatch != "":
				t.Fatalf("rotari %s %s: %s", tc.cmd, tc.args, mismatch)
			case tc.known != "" && mismatch == "":
				t.Fatalf("rotari %s %s now matches the table; remove its known mark (%s) and the ISSUES.md entry", tc.cmd, tc.args, tc.known)
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
	return "\"" + value + "\""
}

func (fixture selectorFixture) runSelectorCase(t *testing.T, tc selectorCase) selectorResult {
	t.Helper()
	sweep := fixture.paths(t, fixture.BaseDir, "sweep")
	if tc.queued {
		fixture.restore(t, fixture.BaseDir, "sweep", fixture.Runs["sweep-second"])
	}
	before, err := state.LoadQueue(sweep.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	args := fixture.expand(tc.args)
	var run func([]string) int
	switch tc.cmd {
	case "show":
		run = cmdShow
	case "copy":
		run = cmdCopy
	case "change":
		run, args = cmdChange, append(args, "--timeout", "7m")
	case "remove":
		run = cmdRemove
	case "run", "retry":
		run = cmdRun
		if tc.cmd == "retry" {
			run = cmdRetry
		}
		fixture.startServer(t)
	default:
		t.Fatalf("unknown command %q", tc.cmd)
	}
	code, output := captureSelectorOutput(func() int { return run(args) })
	if tc.cmd == "run" || tc.cmd == "retry" {
		// A run whose jobs fail exits non-zero; it still executed them.
		if result, ran := fixture.observeRun(t, sweep); ran {
			return result
		}
	}
	if code != 0 {
		return selectorResult{err: fixture.symbolic(strings.TrimSpace(output))}
	}
	var result selectorResult
	switch tc.cmd {
	case "show":
		result = fixture.observeShow(output, tc.table)
	case "copy":
		result = fixture.observeQueue(t, sweep, tc.args, func(model.QueuedCommand) bool { return true })
	case "change":
		result = fixture.observeQueue(t, sweep, tc.args, func(command model.QueuedCommand) bool { return command.Timeout == "7m" })
	case "remove":
		result = fixture.observeRemoved(t, sweep, before)
	default:
		result = selectorResult{err: "no new run"}
	}
	return result
}

func (fixture selectorFixture) expand(args string) []string {
	var expanded []string
	for _, arg := range strings.Fields(args) {
		arg = strings.ReplaceAll(arg, "{B}", fixture.BaseDir)
		arg = strings.ReplaceAll(arg, "{OB}", fixture.OtherBaseDir)
		for key, value := range fixture.Runs {
			arg = strings.ReplaceAll(arg, "{run:"+key+"}", value)
		}
		for key, value := range fixture.Attempts {
			arg = strings.ReplaceAll(arg, "{att:"+key+"}", value)
		}
		for key, value := range fixture.Jobs {
			arg = strings.ReplaceAll(arg, "{job:"+key+"}", value)
		}
		expanded = append(expanded, arg)
	}
	return expanded
}

// symbolic replaces the fixture's generated IDs and directories in text with
// their keys.
func (fixture selectorFixture) symbolic(text string) string {
	text = strings.ReplaceAll(text, fixture.BaseDir, "{B}")
	text = strings.ReplaceAll(text, fixture.OtherBaseDir, "{OB}")
	for key, value := range fixture.Attempts {
		text = strings.ReplaceAll(text, value, "{att:"+key+"}")
	}
	for key, value := range fixture.Runs {
		text = strings.ReplaceAll(text, value, "{run:"+key+"}")
	}
	for key, value := range fixture.Jobs {
		text = strings.ReplaceAll(text, value, "{job:"+key+"}")
	}
	return text
}

// jobKey returns the key of a job or array task ID.
func (fixture selectorFixture) jobKey(id string) string {
	for key, value := range fixture.Jobs {
		if id == value {
			return key
		}
		if task, ok := strings.CutPrefix(id, value+"-"); ok {
			return key + "-" + task
		}
	}
	return "?" + id
}

func (fixture selectorFixture) runKey(id string) string {
	for key, value := range fixture.Runs {
		if id == value {
			return key
		}
	}
	return "?" + id
}

var (
	showRunLine     = regexp.MustCompile(`(?m)^Run: .*?(\d{8}-\d{6}-[0-9a-f]{8})`)
	showAttemptLine = regexp.MustCompile(`(?m)^Attempt ID: (att_\S+)`)
)

// observeShow reads the run and job that a show view displays, or with
// table the jobs of its job table.
func (fixture selectorFixture) observeShow(output string, table bool) selectorResult {
	var result selectorResult
	if match := showRunLine.FindStringSubmatch(output); match != nil {
		result.run = fixture.runKey(match[1])
	}
	if table {
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if key := fixture.jobKey(fields[0]); !strings.HasPrefix(key, "?") {
				result.jobs = append(result.jobs, key)
			}
		}
		sort.Strings(result.jobs)
		return result
	}
	if match := showAttemptLine.FindStringSubmatch(output); match != nil {
		if payload, err := state.DecodeAttemptID(match[1]); err == nil {
			result.jobs = []string{fixture.jobKey(payload.JobID)}
		}
	}
	return result
}

// observeQueue reports the commands of project sweep's queue that keep
// reports true. Queues of the other projects must stay empty unless args
// name them.
func (fixture selectorFixture) observeQueue(t *testing.T, sweep state.ProjectPaths, args string, keep func(model.QueuedCommand) bool) selectorResult {
	t.Helper()
	queue, err := state.LoadQueue(sweep.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	var result selectorResult
	for _, command := range queue.Commands {
		if !keep(command) {
			continue
		}
		result.jobs = append(result.jobs, fixture.commandKeys(command)...)
		if command.Origin != nil && result.run == "" {
			result.run = fixture.runKey(command.Origin.RunID)
		}
	}
	for _, project := range []struct{ baseDir, name string }{{fixture.BaseDir, "other"}, {fixture.OtherBaseDir, "remote"}} {
		other, err := state.LoadQueue(fixture.paths(t, project.baseDir, project.name).QueueFile)
		if err != nil {
			t.Fatal(err)
		}
		for _, command := range other.Commands {
			if !keep(command) {
				continue
			}
			result.jobs = append(result.jobs, project.name+":"+fixture.jobKey(command.ID))
			if command.Origin != nil && result.run == "" {
				result.run = fixture.runKey(command.Origin.RunID)
			}
		}
	}
	sort.Strings(result.jobs)
	return result
}

// commandKeys returns a command's key, or its task keys when it is an array
// narrowed to some of its tasks.
func (fixture selectorFixture) commandKeys(command model.QueuedCommand) []string {
	key := fixture.jobKey(command.ID)
	if command.Array == nil || len(command.Array.Tasks) == 0 {
		return []string{key}
	}
	keys := make([]string, 0, len(command.Array.Tasks))
	for _, task := range model.ArrayTaskIDs(command.Array) {
		keys = append(keys, fixture.jobKey(command.ID+"-"+strconv.Itoa(task)))
	}
	return keys
}

// observeRemoved reports the commands of project sweep's queue that are gone
// after the command, compared with before or, when the queue was replaced by
// a run, with that run's snapshot.
func (fixture selectorFixture) observeRemoved(t *testing.T, sweep state.ProjectPaths, before model.Queue) selectorResult {
	t.Helper()
	after, err := state.LoadQueue(sweep.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Commands) == 0 {
		snapshot, err := state.LoadQueue(filepath.Join(sweep.RunsDir, fixture.Runs["sweep-second"], "commands.json"))
		if err != nil {
			t.Fatal(err)
		}
		before = snapshot
	}
	remaining := make(map[string]bool, len(after.Commands))
	for _, command := range after.Commands {
		remaining[command.ID] = true
	}
	var result selectorResult
	for _, command := range before.Commands {
		if !remaining[command.ID] {
			result.jobs = append(result.jobs, fixture.jobKey(command.ID))
		}
	}
	sort.Strings(result.jobs)
	return result
}

// observeRun reports the jobs that project sweep's newest run executed: the
// jobs with an output directory in it. It reports false when no run started.
func (fixture selectorFixture) observeRun(t *testing.T, sweep state.ProjectPaths) (selectorResult, bool) {
	t.Helper()
	meta, err := state.LoadMeta(sweep.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastRunID == fixture.Runs["sweep-second"] {
		return selectorResult{}, false
	}
	entries, err := os.ReadDir(filepath.Join(sweep.RunsDir, meta.LastRunID))
	if err != nil {
		t.Fatal(err)
	}
	var result selectorResult
	for _, entry := range entries {
		if entry.IsDir() {
			result.jobs = append(result.jobs, fixture.jobKey(entry.Name()))
		}
	}
	sort.Strings(result.jobs)
	return result, true
}

// startServer runs the base directory's supervisor in process for a run
// case and stops it when the test ends.
func (fixture selectorFixture) startServer(t *testing.T) {
	t.Helper()
	done := make(chan int, 1)
	go func() { done <- runServer(fixture.BaseDir) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if response, err := serverinternal.SendRequest(fixture.BaseDir, serverinternal.Request{Op: serverinternal.OpPing}); err == nil && response.OK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Cleanup(func() {
		_, _ = serverinternal.SendRequest(fixture.BaseDir, serverinternal.Request{Op: serverinternal.OpShutdown})
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Log("server did not stop")
		}
	})
}

func captureSelectorOutput(run func() int) (int, string) {
	stdout, stderr := os.Stdout, os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout, os.Stderr = writer, writer
	var output bytes.Buffer
	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(&output, reader)
		close(copied)
	}()
	code := run()
	writer.Close()
	<-copied
	os.Stdout, os.Stderr = stdout, stderr
	return code, output.String()
}

// TestRunJobIDKeepsEditedQueue checks the fix-and-retry loop: a job changed
// in the queue runs with its edit, instead of the queue being replaced by
// the latest run.
func TestRunJobIDKeepsEditedQueue(t *testing.T) {
	fixture := newSelectorFixture(t)
	fixture.restore(t, fixture.BaseDir, "sweep", fixture.Runs["sweep-second"])
	if code := cmdChange([]string{"-b", fixture.BaseDir, "-p", "sweep", "--job-name", "prep", "--quiet", "echo", "edited"}); code != 0 {
		t.Fatalf("change exit code = %d", code)
	}
	fixture.startServer(t)
	// The run fails: it carries the failures of the jobs it does not run.
	_, output := captureSelectorOutput(func() int {
		return cmdRun([]string{"-b", fixture.BaseDir, "-p", "sweep", "--job-id", fixture.Jobs["prep"]})
	})
	sweep := fixture.paths(t, fixture.BaseDir, "sweep")
	meta, err := state.LoadMeta(sweep.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastRunID == fixture.Runs["sweep-second"] {
		t.Fatalf("run --job-id did not start a run:\n%s", output)
	}
	snapshot, err := state.LoadQueue(filepath.Join(sweep.RunsDir, meta.LastRunID, "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range snapshot.Commands {
		if command.ID == fixture.Jobs["prep"] && strings.Join(command.Command, " ") != "echo edited" {
			t.Fatalf("run executed prep as %q, want the edited command", strings.Join(command.Command, " "))
		}
	}
}

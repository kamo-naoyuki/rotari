package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Contracts CAN-1 to CAN-4: cancellation stops the jobs it names and lets
// the run finish normally, for synchronous and asynchronous runs and from
// the CLI and the Web UI alike. See "Cancellation" in
// contracts/02-run-lifecycle-and-execution.md.

type cancelCase struct {
	name  string
	async bool
	web   bool
}

var wholeRunCancelCases = []cancelCase{
	{"sync run, CLI", false, false},
	{"sync run, Web", false, true},
	{"async run, CLI", true, false},
	{"async run, Web", true, true},
}

func TestWholeRunCancelFinishesRun(t *testing.T) {
	covers(t, "CAN-1", "CAN-2")
	for _, c := range wholeRunCancelCases {
		t.Run(c.name, func(t *testing.T) {
			if c.async || c.web {
				knownDeviation(t, "CAN-1")
			}
			e := newEnv(t).in(t)
			run := e.startRun("live", 2, c.async)
			if c.web {
				base := e.startWeb()
				if got := e.httpPostJSON(base+"/api/cancel-run", map[string]any{"project_name": run.project, "run_id": run.runID}); got.status != 200 {
					t.Fatalf("cancel-run: status %d: %s", got.status, got.body)
				}
			} else {
				e.mustRotari("cancel", "-p", run.project)
			}

			// CAN-1: every job stops.
			waitUntil(t, 30*time.Second, func() (bool, string) {
				alive := jobProcesses(t, e.root, "")
				return alive == 0, fmt.Sprintf("%d job processes still running", alive)
			})
			// CAN-2: the run finishes and the project is idle again.
			waitUntil(t, 30*time.Second, func() (bool, string) {
				check := e.rotari("check", run.project).stdout
				return projectFinished(check), "check: " + check
			})
			results := runResults(t, e, run)
			if len(results) != len(run.jobs) {
				t.Errorf("summary records %d results, want one for each of %d jobs", len(results), len(run.jobs))
			}
			e.mustRotari("add", "-p", run.project, "--", "true")
		})
	}
}

func TestCancelWaitReturnsAfterRunFinishes(t *testing.T) {
	covers(t, "CAN-3")
	for _, async := range []bool{false, true} {
		name := "sync run"
		if async {
			name = "async run"
		}
		t.Run(name, func(t *testing.T) {
			knownDeviation(t, "CAN-3")
			e := newEnv(t).in(t)
			run := e.startRun("live", 1, async)
			r := e.rotari("cancel", "-p", run.project, "--wait")
			if r.code != 0 {
				t.Fatalf("cancel --wait failed: %s", r)
			}
			if check := e.rotari("check", run.project).stdout; !projectFinished(check) {
				t.Errorf("run not finished when cancel --wait returned; check: %s", check)
			}
		})
	}
}

func TestCancelJobStopsOnlyThatJob(t *testing.T) {
	covers(t, "CAN-4")
	for _, web := range []bool{false, true} {
		name := "CLI"
		if web {
			name = "Web"
		}
		t.Run(name, func(t *testing.T) {
			e := newEnv(t).in(t)
			run := e.startRun("live", 2, false, "--retry", "1")
			cancelled, other := run.jobs[0], run.jobs[1]
			if web {
				base := e.startWeb()
				if got := e.httpPostJSON(base+"/api/cancel-job", map[string]any{"project_name": run.project, "run_id": run.runID, "job_id": cancelled}); got.status != 200 {
					t.Fatalf("cancel-job: status %d: %s", got.status, got.body)
				}
			} else {
				e.mustRotari("cancel", "-p", run.project, cancelled)
			}

			waitUntil(t, 30*time.Second, func() (bool, string) {
				return jobProcesses(t, e.root, cancelled) == 0, "the cancelled job is still running"
			})
			// Give a retry the time it would need to start.
			time.Sleep(2 * time.Second)
			if alive := jobProcesses(t, e.root, other); alive == 0 {
				t.Errorf("job %s stopped too; only %s was cancelled", other, cancelled)
			}
			if attempts := jobAttempts(t, e, run, cancelled); attempts != 1 {
				t.Errorf("job %s has %d attempts, want 1: the run retried a cancelled job", cancelled, attempts)
			}
			if check := e.rotari("check", run.project).stdout; !strings.Contains(check, "state=running") {
				t.Errorf("run is no longer running after one job was cancelled; check: %s", check)
			}
		})
	}
}

// projectFinished reports whether `rotari check` output shows no active or
// interrupted run.
func projectFinished(check string) bool {
	return strings.Contains(check, "lock=none") && !strings.Contains(check, "state=interrupted") && !strings.Contains(check, "state=running")
}

// runResults returns the job results in the run's summary.
func runResults(t *testing.T, e *env, run activeRun) []json.RawMessage {
	t.Helper()
	var shown struct {
		Summary *struct {
			Results []json.RawMessage `json:"results"`
		} `json:"summary"`
	}
	r := e.mustRotari("show", "-p", run.project, "--run-id", run.runID, "--json")
	if err := json.Unmarshal([]byte(r.stdout), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.Summary == nil {
		t.Fatalf("run %s has no summary: %s", run.runID, r)
	}
	return shown.Summary.Results
}

// jobAttempts counts the attempts of job that `rotari jobs` lists.
func jobAttempts(t *testing.T, e *env, run activeRun, job string) int {
	t.Helper()
	return strings.Count(e.mustRotari("jobs", run.project, "--format", "%a").stdout, "-"+job+"-")
}

// jobProcesses counts the local job wrappers running under root, only those
// of job when it is not empty. It needs /proc and skips the test without it.
func jobProcesses(t *testing.T, root, job string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skipf("cannot list processes: %v", err)
	}
	marker := []byte("local-wrapper.sh")
	count := 0
	for _, entry := range entries {
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || !bytes.Contains(cmdline, []byte(root+string(filepath.Separator))) || !bytes.Contains(cmdline, marker) {
			continue
		}
		if job == "" || bytes.Contains(cmdline, []byte(string(filepath.Separator)+job+string(filepath.Separator))) {
			count++
		}
	}
	return count
}

// waitUntil polls done until it reports true, failing the test with its
// last message after timeout.
func waitUntil(t *testing.T, timeout time.Duration, done func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ok, message := done()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("after %s: %s", timeout, message)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

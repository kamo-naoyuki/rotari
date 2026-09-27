package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Contracts DUR-3, DUR-4, and DUR-6: a local job outlives its killed
// supervisor and records its own result, and neither a restart nor recovery
// touches it. See "Job execution durability" in
// contracts/04-coordination-and-safety.md.

// orphanRun starts a run of project with one job that runs script, waits
// until the job runs, and kills the run's supervisor with SIGKILL, leaving
// the job running on its own. It returns the job's ID.
func (e *env) orphanRun(project, script string) string {
	e.t.Helper()
	requireUnixSockets(e.t)
	jobID := addedJobID(e.t, e.mustRotari("add", "-p", project, "--", "sh", "-c", script))
	client := e.command("run", "-p", project, "--quiet")
	if err := client.Start(); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = client.Wait() })
	waitUntil(e.t, 15*time.Second, func() (bool, string) {
		return jobProcesses(e.t, e.root, jobID) == 1, "the job did not start"
	})
	var lock struct {
		PID int `json:"pid"`
	}
	data, err := os.ReadFile(filepath.Join(e.base, "projects", project, "running.lock"))
	if err != nil || json.Unmarshal(data, &lock) != nil || lock.PID <= 0 {
		e.t.Fatalf("running.lock does not name the supervisor: %s, %v", data, err)
	}
	if err := syscall.Kill(lock.PID, syscall.SIGKILL); err != nil {
		e.t.Fatal(err)
	}
	return jobID
}

// jobExitStatus waits for show to report job's result and returns its status
// line.
func jobExitStatus(e *env, project, jobID string) string {
	e.t.Helper()
	var status string
	waitUntil(e.t, 30*time.Second, func() (bool, string) {
		out := e.rotari("show", "-p", project, "--run-id", "latest", "--job-id", jobID).stdout
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Status:") {
				status = line
			}
		}
		return strings.Contains(status, "7"), "show reports " + status
	})
	return status
}

func TestJobOutlivesKilledSupervisor(t *testing.T) {
	covers(t, "DUR-3", "DUR-4")
	e := newEnv(t)
	jobID := e.orphanRun("p", "sleep 2; exit 7")

	// DUR-4: nothing restarts the supervisor; the run is interrupted.
	if state := checkState(e, "p"); state != "interrupted" {
		t.Errorf("after the supervisor was killed: state %q, want interrupted", state)
	}
	// DUR-3: the job finishes on its own and its result is readable.
	jobExitStatus(e, "p", jobID)
	if rows := e.mustRotari("jobs", "p", "--format", "%s").stdout; !strings.Contains(rows, "failed") {
		t.Errorf("jobs does not report the orphaned job's failure:\n%s", rows)
	}
	if state := checkState(e, "p"); state != "interrupted" {
		t.Errorf("after the job finished: state %q, want interrupted until recovered", state)
	}
}

func TestRecoveryLeavesJobsRunning(t *testing.T) {
	covers(t, "DUR-6")
	e := newEnv(t)
	jobID := e.orphanRun("p", "sleep 3; exit 7")
	e.mustRotari("unlock", "p")
	if alive := jobProcesses(t, e.root, jobID); alive != 1 {
		t.Fatalf("recovery stopped the job: %d processes left", alive)
	}
	jobExitStatus(e, "p", jobID)
}

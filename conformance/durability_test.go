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

func (e *env) orphanRun(project, script string) string {
	e.t.Helper()
	requireUnixSockets(e.t)
	jobID := addedJobID(e.t, e.mustRotari("add", "-p", project, "--", "sh", "-c", script))
	client := e.command("run", "-p", project, "--quiet")
	if err := client.Start(); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = client.Wait() })
	waitUntil(e.t, 15*time.Second, func() (bool, string) { return jobProcesses(e.t, e.root, jobID) == 1, "the job did not start" })
	var lock struct {
		PID int `json:"pid"`
	}
	data, err := os.ReadFile(filepath.Join(e.base, "projects", project, "running.lock"))
	if err != nil || json.Unmarshal(data, &lock) != nil || lock.PID <= 0 {
		e.t.Fatalf("running.lock does not name supervisor: %s", data)
	}
	if err := syscall.Kill(lock.PID, syscall.SIGKILL); err != nil {
		e.t.Fatal(err)
	}
	waitForInterrupted(e, project)
	return jobID
}

func waitForInterrupted(e *env, project string) {
	e.t.Helper()
	waitUntil(e.t, 15*time.Second, func() (bool, string) {
		state := checkState(e, project)
		return state == "interrupted", "state " + state + ", want interrupted"
	})
}

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
	jobExitStatus(e, "p", jobID)
	if rows := e.mustRotari("jobs", "p", "--format", "%s").stdout; !strings.Contains(rows, "failed") {
		t.Errorf("jobs does not report failure: %s", rows)
	}
	if state := checkState(e, "p"); state != "interrupted" {
		t.Errorf("state %q, want interrupted", state)
	}
}

func TestRecoveryLeavesJobsRunning(t *testing.T) {
	covers(t, "DUR-6")
	e := newEnv(t)
	jobID := e.orphanRun("p", "sleep 3; exit 7")
	e.mustRotari("unlock", "p")
	if alive := jobProcesses(t, e.root, jobID); alive != 1 {
		t.Fatalf("recovery stopped job: %d processes", alive)
	}
	jobExitStatus(e, "p", jobID)
}

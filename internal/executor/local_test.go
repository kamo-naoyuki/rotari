package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
	"github.com/rotari-dev/rotari/internal/state"
)

func TestLocalCancelAcceptsAnExitedJob(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), []byte(fmt.Sprintf("%d\n", command.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := NewLocal(testStore(), testLogf).Cancel(jobDir); err != nil {
		t.Fatalf("cancel of a job that already ended: %v", err)
	}
	marker, err := os.ReadFile(filepath.Join(jobDir, "cancelled"))
	if err != nil || strings.TrimSpace(string(marker)) == "" {
		t.Fatalf("cancellation marker: %v %q", err, marker)
	}
}

// The run checks for a cancel once Submit returns, so a local job must be
// cancellable by then.
func TestLocalSubmitStartsCancellableJob(t *testing.T) {
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", AttemptID: "att_20260101-000000-00000000-job-0", Command: []string{"sleep", "30"}}
	local := NewLocal(testStore(), testLogf)
	handle, err := local.Submit(runDir, job, nil)
	if err != nil {
		t.Fatal(err)
	}
	jobDir, err := state.AttemptJobDir(runDir, job)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewRegistry(testStore(), testLogf).Owner(testStore(), jobDir)
	if err != nil || owner.Name() != "local" {
		_ = local.Cancel(jobDir)
		local.Wait(runDir, handle)
		t.Fatalf("owner after Submit = %v, %v; want the local executor", owner, err)
	}
	if err := local.Cancel(jobDir); err != nil {
		t.Fatal(err)
	}
	done := make(chan model.JobResult, 1)
	go func() { done <- local.Wait(runDir, handle) }()
	select {
	case result := <-done:
		if result.ExitCode == 0 {
			t.Fatalf("result = %+v, want the cancelled job to fail", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled job did not end")
	}
}

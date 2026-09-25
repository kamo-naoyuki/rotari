package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func runLocalWithTimeout(t *testing.T, command []string, timeout string) (model.JobResult, string, time.Duration) {
	t.Helper()
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", Command: command, Timeout: timeout}
	started := time.Now()
	result := RunLocalJob(runDir, job, testStore(), func(string, ...any) {})
	elapsed := time.Since(started)
	output, _ := os.ReadFile(filepath.Join(runDir, "job", "output"))
	return result, string(output), elapsed
}

func TestLocalJobTimeoutStopsCommand(t *testing.T) {
	result, output, elapsed := runLocalWithTimeout(t, []string{"sleep", "30"}, "1s")
	if result.ExitCode != TimeoutExitCode || result.Error != "timed out after 1s" {
		t.Fatalf("result = %+v, want a timeout", result)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("timed-out job took %s", elapsed)
	}
	if !strings.Contains(output, "rotari: job timed out after 1s") {
		t.Fatalf("output = %q, want the timeout message", output)
	}
}

func TestLocalJobTimeoutKillsCommandIgnoringTerm(t *testing.T) {
	old := timeoutGraceSeconds
	timeoutGraceSeconds = 1
	t.Cleanup(func() { timeoutGraceSeconds = old })
	result, _, elapsed := runLocalWithTimeout(t, []string{"sh", "-c", `trap "" TERM; sleep 30 & wait $!; sleep 30`}, "1s")
	if result.ExitCode != TimeoutExitCode || !strings.Contains(result.Error, "timed out") {
		t.Fatalf("result = %+v, want a timeout", result)
	}
	if elapsed > 15*time.Second {
		t.Fatalf("job ignoring SIGTERM was not killed after the grace period: %s", elapsed)
	}
}

func TestLocalJobFinishingBeforeTimeoutIsUnaffected(t *testing.T) {
	result, output, elapsed := runLocalWithTimeout(t, []string{"sh", "-c", "echo done; exit 3"}, "1h")
	if result.ExitCode != 3 || result.Error != "" || !strings.Contains(output, "done") {
		t.Fatalf("result = %+v, output = %q", result, output)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("a finished job waited for its watchdog: %s", elapsed)
	}
}

func TestSSHWrapperTimeoutStopsRemoteCommand(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid is not installed")
	}
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	command := exec.Command("sh", "-s")
	command.Stdin = strings.NewReader(sshWrapperScript([]string{"sleep", "30"}, nil, "", "0123456789abcdef0123456789abcdef", "1s"))
	started := time.Now()
	output, err := command.CombinedOutput()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != TimeoutExitCode {
		t.Fatalf("SSH wrapper error = %v, output = %q, want exit %d", err, output, TimeoutExitCode)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("SSH wrapper took %s", elapsed)
	}
	if !strings.Contains(string(output), "timed out after 1s") {
		t.Fatalf("output = %q", output)
	}
}

func TestSchedulerArrayWrapperEnforcesTimeout(t *testing.T) {
	runDir := t.TempDir()
	task := 1
	jobDir := filepath.Join(runDir, "array-1")
	job := model.JobSpec{ID: "array-1", ArrayTaskID: &task, Command: []string{"sleep", "30"}, Timeout: "1s",
		Environment: []string{model.EnvJobDir + "=" + jobDir}}
	wrapper := filepath.Join(runDir, "wrapper.sh")
	if err := os.WriteFile(wrapper, []byte(schedulerArrayWrapperScript([]model.JobSpec{job}, "TASK")), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", wrapper)
	command.Env = append(os.Environ(), "TASK=1")
	err := command.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != TimeoutExitCode {
		t.Fatalf("array wrapper error = %v, want exit %d", err, TimeoutExitCode)
	}
	status, _ := os.ReadFile(filepath.Join(jobDir, "status.json"))
	if !strings.Contains(string(status), `"exit_code":124`) || !strings.Contains(string(status), fmt.Sprintf(`"error":"%s"`, TimeoutMessage(1))) {
		t.Fatalf("status.json = %s", status)
	}
}

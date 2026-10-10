package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func runLocalWithTimeout(t *testing.T, command []string, timeout string) (model.JobResult, string, string, time.Duration) {
	t.Helper()
	runDir := t.TempDir()
	t.Cleanup(func() {
		pidData, err := os.ReadFile(filepath.Join(runDir, "job", "pid"))
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if err != nil {
			return
		}
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			err := syscall.Kill(-pid, 0)
			if err == syscall.ESRCH {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	job := model.JobSpec{ID: "job", Command: command, Timeout: timeout, LogMode: model.LogModeSeparate}
	started := time.Now()
	result := RunLocalJob(runDir, job, testStore(), func(string, ...any) {})
	elapsed := time.Since(started)
	stdout, _ := os.ReadFile(filepath.Join(runDir, "job", "stdout"))
	stderr, _ := os.ReadFile(filepath.Join(runDir, "job", "stderr"))
	return result, string(stdout), string(stderr), elapsed
}

func TestLocalJobKeepsStdoutAndStderrSeparate(t *testing.T) {
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job", LogMode: model.LogModeSeparate, Command: []string{"sh", "-c", "printf out; printf err >&2"}}
	result := RunLocalJob(runDir, job, testStore(), func(string, ...any) {})
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	for _, testCase := range []struct {
		name string
		want string
	}{{name: "stdout", want: "out"}, {name: "stderr", want: "err"}} {
		got, err := os.ReadFile(filepath.Join(runDir, "job", testCase.name))
		if err != nil || string(got) != testCase.want {
			t.Errorf("%s = %q, err=%v; want %q", testCase.name, got, err, testCase.want)
		}
	}
}

func TestLocalJobMergesLogsByDefaultAndFansOutDestinations(t *testing.T) {
	runDir := t.TempDir()
	externalRoot := t.TempDir()
	externalOutput := filepath.Join(externalRoot, "nested", "output.log")
	externalError := filepath.Join(t.TempDir(), "nested", "error.log")
	if err := os.MkdirAll(filepath.Dir(externalOutput), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalOutput, []byte("prior:"), 0o600); err != nil {
		t.Fatal(err)
	}
	job := model.JobSpec{ID: "job", Command: []string{"sh", "-c", "printf out; printf err >&2"}, Output: []string{externalOutput}, Error: []string{externalError}}
	result := RunLocalJob(runDir, job, testStore(), func(string, ...any) {})
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	merged, err := os.ReadFile(filepath.Join(runDir, "job", "output"))
	if err != nil || !strings.Contains(string(merged), "out") || !strings.Contains(string(merged), "err") {
		t.Fatalf("merged log = %q, err=%v", merged, err)
	}
	stdout, err := os.ReadFile(externalOutput)
	if err != nil || string(stdout) != "prior:out" {
		t.Fatalf("external stdout = %q, err=%v; want append", stdout, err)
	}
	stderr, err := os.ReadFile(externalError)
	if err != nil || string(stderr) != "err" {
		t.Fatalf("external stderr = %q, err=%v", stderr, err)
	}
}

func TestLocalOutputDestinationMergesStderrUnlessErrorIsSet(t *testing.T) {
	runDir := t.TempDir()
	destination := filepath.Join(t.TempDir(), "combined.log")
	job := model.JobSpec{ID: "job", Output: []string{destination}, Command: []string{"sh", "-c", "printf out; printf err >&2"}}
	result := RunLocalJob(runDir, job, testStore(), func(string, ...any) {})
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	data, err := os.ReadFile(destination)
	if err != nil || !strings.Contains(string(data), "out") || !strings.Contains(string(data), "err") {
		t.Fatalf("output-only destination = %q, err=%v; stderr should follow stdout destination", data, err)
	}
}

func TestLocalLogsAndExternalDestinationsAreReadableWhileRunning(t *testing.T) {
	runDir := t.TempDir()
	external := filepath.Join(t.TempDir(), "live.log")
	job := model.JobSpec{
		ID: "job", Output: []string{external},
		Command: []string{"sh", "-c", "printf LIVE_OUT; printf LIVE_ERR >&2; sleep 2"},
	}
	resultCh := make(chan model.JobResult, 1)
	go func() {
		resultCh <- RunLocalJob(runDir, job, testStore(), func(string, ...any) {})
	}()
	mergedPath := filepath.Join(runDir, "job", "output")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		merged, _ := os.ReadFile(mergedPath)
		externalData, _ := os.ReadFile(external)
		if strings.Contains(string(merged), "LIVE_OUT") && strings.Contains(string(merged), "LIVE_ERR") &&
			strings.Contains(string(externalData), "LIVE_OUT") && strings.Contains(string(externalData), "LIVE_ERR") {
			select {
			case result := <-resultCh:
				t.Fatalf("job finished before the live-output assertion: %#v", result)
			default:
			}
			result := <-resultCh
			if result.ExitCode != 0 {
				t.Fatalf("job result = %#v", result)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("internal and external logs were not readable before the job finished")
}

func TestLocalDestinationSetupFailurePreventsCommandStart(t *testing.T) {
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "work")
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(workingDirectory, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "command-ran")
	job := model.JobSpec{ID: "job", WorkingDirectory: workingDirectory, Output: []string{"not-a-directory/output"}, Command: []string{"sh", "-c", "touch \"$1\"", "sh", marker}}
	result := RunLocalJob(filepath.Join(root, "runs"), job, testStore(), func(string, ...any) {})
	if result.ExitCode == 0 {
		t.Fatalf("RunLocalJob unexpectedly succeeded: %#v", result)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("command ran despite destination setup failure: stat error=%v", err)
	}
}

func TestLocalJobSeparateLogsCanShareExternalDestination(t *testing.T) {
	runDir := t.TempDir()
	destination := filepath.Join(t.TempDir(), "combined.log")
	job := model.JobSpec{ID: "job", LogMode: model.LogModeSeparate, Output: []string{destination}, Error: []string{destination}, Command: []string{"sh", "-c", "printf out; printf err >&2"}}
	result := RunLocalJob(runDir, job, testStore(), func(string, ...any) {})
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	stdout, err := os.ReadFile(filepath.Join(runDir, "job", state.StdoutFileName))
	if err != nil || string(stdout) != "out" {
		t.Fatalf("internal stdout = %q, err=%v", stdout, err)
	}
	stderr, err := os.ReadFile(filepath.Join(runDir, "job", state.StderrFileName))
	if err != nil || string(stderr) != "err" {
		t.Fatalf("internal stderr = %q, err=%v", stderr, err)
	}
	combined, err := os.ReadFile(destination)
	if err != nil || !strings.Contains(string(combined), "out") || !strings.Contains(string(combined), "err") {
		t.Fatalf("external combined = %q, err=%v", combined, err)
	}
}

func TestWrapperCreatesRelativeDestinationAndMergesErrorByDefault(t *testing.T) {
	t.Setenv("PATH", installTestRotari(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "work")
	jobDir := filepath.Join(root, "attempt")
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	preexisting := filepath.Join(workingDirectory, "nested", "result.log")
	if err := os.MkdirAll(filepath.Dir(preexisting), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preexisting, []byte("old-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := StatusWrapperScriptWithDestinations(
		[]string{"sh", "-c", "printf out; printf err >&2"}, jobDir, nil,
		workingDirectory, "", model.LogModeMerge, model.OpenModeTruncate,
		[]string{"nested/result.log"}, nil,
	)
	path := filepath.Join(jobDir, "wrapper.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", path)
	stdoutFile, err := os.Create(filepath.Join(jobDir, state.StdoutFileName))
	if err != nil {
		t.Fatal(err)
	}
	stderrFile, err := os.Create(filepath.Join(jobDir, state.StderrFileName))
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = stdoutFile, stderrFile
	err = command.Run()
	_ = stdoutFile.Close()
	_ = stderrFile.Close()
	if err != nil {
		t.Fatalf("wrapper failed: %v\nscript:\n%s", err, script)
	}
	data, err := os.ReadFile(preexisting)
	if err != nil || strings.Contains(string(data), "old-data") || !strings.Contains(string(data), "out") || !strings.Contains(string(data), "err") {
		t.Fatalf("external merged destination = %q, err=%v", data, err)
	}
}

func TestWrapperSeparatesExternalOutputAndErrorWhenBothSpecified(t *testing.T) {
	t.Setenv("PATH", installTestRotari(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "work")
	jobDir := filepath.Join(root, "attempt")
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := StatusWrapperScriptWithDestinations(
		[]string{"sh", "-c", "printf out; printf err >&2"}, jobDir, nil,
		workingDirectory, "", model.LogModeMerge, model.OpenModeAppend,
		[]string{"nested/stdout.log"}, []string{"nested/stderr.log"},
	)
	path := filepath.Join(jobDir, "wrapper.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", path)
	stdoutFile, err := os.Create(filepath.Join(jobDir, state.StdoutFileName))
	if err != nil {
		t.Fatal(err)
	}
	stderrFile, err := os.Create(filepath.Join(jobDir, state.StderrFileName))
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = stdoutFile, stderrFile
	err = command.Run()
	_ = stdoutFile.Close()
	_ = stderrFile.Close()
	if err != nil {
		t.Fatalf("wrapper failed: %v\nscript:\n%s", err, script)
	}
	for name, want := range map[string]string{"stdout.log": "out", "stderr.log": "err"} {
		data, err := os.ReadFile(filepath.Join(workingDirectory, "nested", name))
		if err != nil || string(data) != want {
			t.Errorf("%s = %q, err=%v; want %q", name, data, err, want)
		}
	}
}

func TestLocalJobTimeoutStopsCommand(t *testing.T) {
	result, _, stderr, elapsed := runLocalWithTimeout(t, []string{"sleep", "30"}, "1s")
	if result.ExitCode != TimeoutExitCode || result.Error != "timed out after 1s" {
		t.Fatalf("result = %+v, want a timeout", result)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("timed-out job took %s", elapsed)
	}
	if !strings.Contains(stderr, "rotari: job timed out after 1s") {
		t.Fatalf("stderr = %q, want the timeout message", stderr)
	}
}

func TestLocalJobTimeoutKillsCommandIgnoringTerm(t *testing.T) {
	old := timeoutGraceSeconds
	timeoutGraceSeconds = 1
	t.Cleanup(func() { timeoutGraceSeconds = old })
	result, _, _, elapsed := runLocalWithTimeout(t, []string{"sh", "-c", `trap "" TERM; sleep 30 & wait $!; sleep 30`}, "1s")
	if result.ExitCode != TimeoutExitCode || !strings.Contains(result.Error, "timed out") {
		t.Fatalf("result = %+v, want a timeout", result)
	}
	if elapsed > 15*time.Second {
		t.Fatalf("job ignoring SIGTERM was not killed after the grace period: %s", elapsed)
	}
}

func TestLocalCancelBeforeWrapperStarts(t *testing.T) {
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrapperPath := filepath.Join(jobDir, "local-wrapper.sh")
	script := StatusWrapperScript([]string{"sh", "-c", "sleep 0.25; exec sleep 30"}, jobDir, nil, "", "")
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte("cancelled\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", wrapperPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	start := time.Now()
	err := cmd.Run()
	if err == nil {
		t.Fatal("wrapper exited successfully even though cancellation was already recorded")
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("cancelled wrapper took %s; it waited for the delayed exec before aborting", d)
	}
	status, err := os.ReadFile(filepath.Join(jobDir, "status.json"))
	if err != nil {
		t.Fatalf("status.json not written: %v", err)
	}
	if !strings.Contains(string(status), `"phase":"cancelled"`) || !strings.Contains(string(status), `"exit_code":143`) {
		t.Fatalf("status.json = %s, want a cancelled exit", status)
	}
}

func TestLocalJobCancelStopsCommandBeforeForegroundExec(t *testing.T) {
	for i := 0; i < 25; i++ {
		runDir := t.TempDir()
		wrapperPath := filepath.Join(runDir, "job", "local-wrapper.sh")
		if err := os.MkdirAll(filepath.Dir(wrapperPath), 0o755); err != nil {
			t.Fatal(err)
		}
		script := StatusWrapperScript([]string{"sh", "-c", "sleep 0.25; exec sleep 30"}, filepath.Dir(wrapperPath), nil, "", "")
		if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command("/bin/sh", wrapperPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		// Trigger cancellation before the wrapped command has had time to exec;
		// the shell must abort promptly instead of waiting for the delayed child.
		time.Sleep(10 * time.Millisecond)
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
			t.Fatalf("signal = %v", err)
		}

		start := time.Now()
		err := cmd.Wait()
		elapsed := time.Since(start)
		if err == nil {
			t.Fatalf("job exited successfully after SIGTERM before the delayed exec")
		}
		if elapsed > 500*time.Millisecond {
			t.Fatalf("killed job took %s; cancellation was deferred until after the delayed child started", elapsed)
		}
		// A signal that reached the command while it was still the
		// wrapper's forked copy must not leave it running.
		deadline := time.Now().Add(5 * time.Second)
		for syscall.Kill(-cmd.Process.Pid, 0) == nil {
			if time.Now().After(deadline) {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				t.Fatalf("iteration %d: the command outlived the cancel", i)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestLocalJobFinishingBeforeTimeoutIsUnaffected(t *testing.T) {
	result, stdout, _, elapsed := runLocalWithTimeout(t, []string{"sh", "-c", "echo done; exit 3"}, "1h")
	if result.ExitCode != 3 || result.Error != "" || !strings.Contains(stdout, "done") {
		t.Fatalf("result = %+v, stdout = %q", result, stdout)
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

func TestLocalJobTimeoutKillsLogForwarderAndCommand(t *testing.T) {
	old := timeoutGraceSeconds
	timeoutGraceSeconds = 1
	t.Cleanup(func() { timeoutGraceSeconds = old })
	job := model.JobSpec{
		ID: "job", Timeout: "1s", Output: []string{filepath.Join(t.TempDir(), "live.log")},
		Command: []string{"sh", "-c", `trap "" TERM; sleep 30 & wait $!; sleep 30`},
	}
	started := time.Now()
	result := RunLocalJob(t.TempDir(), job, testStore(), func(string, ...any) {})
	if result.ExitCode != TimeoutExitCode || !strings.Contains(result.Error, "timed out") {
		t.Fatalf("result = %+v, want a timeout", result)
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("timed-out helper job was not killed after the grace period: %s", elapsed)
	}
}

func TestLocalJobTimeoutWorksWithoutUsablePs(t *testing.T) {
	// BusyBox ps, as in some scheduler images, rejects -p; the wrapper must
	// still recognize that it leads its process group.
	fakeBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeBin, "ps"), []byte("#!/bin/sh\necho 'ps: unrecognized option: p' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, _, _, elapsed := runLocalWithTimeout(t, []string{"sleep", "30"}, "1s")
	if result.ExitCode != TimeoutExitCode || elapsed > 10*time.Second {
		t.Fatalf("result = %+v after %s; want the timeout enforced", result, elapsed)
	}
}

// startCancelTestWrapper starts the local wrapper around the command that
// command returns for the job directory, in its own process group, and
// returns it with that directory.
func startCancelTestWrapper(t *testing.T, command func(jobDir string) string) (*exec.Cmd, string) {
	t.Helper()
	jobDir := filepath.Join(t.TempDir(), "job")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrapperPath := filepath.Join(jobDir, "local-wrapper.sh")
	if err := os.WriteFile(wrapperPath, []byte(StatusWrapperScript([]string{"sh", "-c", command(jobDir)}, jobDir, nil, "", "")), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", wrapperPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, err := os.ReadFile(filepath.Join(jobDir, "status.json")); err == nil && strings.Contains(string(data), `"phase":"running"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("wrapper did not record the running phase")
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	return cmd, jobDir
}

// A cancel records the job cancelled only after its command has exited, so
// a command that cleans up on SIGTERM is not reported stopped while it runs.
func TestLocalCancelWaitsForCommandToExit(t *testing.T) {
	cmd, jobDir := startCancelTestWrapper(t, func(jobDir string) string {
		return `trap 'sleep 1; : > ` + ShellQuote(filepath.Join(jobDir, "cleaned")) + `; exit 143' TERM; while true; do sleep 0.1; done`
	})
	cleaned := filepath.Join(jobDir, "cleaned")
	start := time.Now()
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("wrapper exited after %s, before the command finished its cleanup", elapsed)
	}
	status, err := os.ReadFile(filepath.Join(jobDir, "status.json"))
	if err != nil || !strings.Contains(string(status), `"phase":"cancelled"`) || !strings.Contains(string(status), `"exit_code":143`) {
		t.Fatalf("status.json = %s, %v; want a cancelled exit", status, err)
	}
	if _, err := os.Stat(cleaned); err != nil {
		t.Fatalf("command had not finished its cleanup when the wrapper recorded the cancel: %v", err)
	}
	// Nothing of the job, including the wrapper's grace timer, is left.
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(-cmd.Process.Pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("processes of the cancelled job are still running after the wrapper exited")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A command that ignores SIGTERM is killed after the grace period, and the
// cancel is still recorded.
func TestLocalCancelKillsCommandIgnoringTerm(t *testing.T) {
	old := timeoutGraceSeconds
	timeoutGraceSeconds = 1
	t.Cleanup(func() { timeoutGraceSeconds = old })
	cmd, jobDir := startCancelTestWrapper(t, func(string) string { return `trap '' TERM; while true; do sleep 0.1; done` })
	start := time.Now()
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("job ignoring SIGTERM was not killed after the grace period: %s", elapsed)
	}
	status, err := os.ReadFile(filepath.Join(jobDir, "status.json"))
	if err != nil || !strings.Contains(string(status), `"phase":"cancelled"`) {
		t.Fatalf("status.json = %s, %v; want a cancelled phase", status, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(-cmd.Process.Pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("processes of the cancelled job are still running")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

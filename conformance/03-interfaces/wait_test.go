package interfaces

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestWaitReturnsCompletedRunExitCode(t *testing.T) {
	covers(t, "RES-16")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	r := e.Rotari("wait", "-p", run.Project, "--run-id", run.RunID)
	// wait returns the run's overall exit code, not the failed job's.
	if r.Code != 1 {
		t.Fatalf("wait exit code = %d, want 1: %s", r.Code, r)
	}
	for _, want := range []string{"=== Run failed ===", "Success: 1", "Failed: 1"} {
		if !strings.Contains(r.Stdout, want) {
			t.Fatalf("wait output does not contain %q:\n%s", want, r.Stdout)
		}
	}
}

func TestWaitPrintsSameCompletionMessageAsRun(t *testing.T) {
	covers(t, "CLI-14")
	e := support.NewEnv(t)
	project := "completion-message"
	e.MustRotari("add", "-p", project, "--", "true")
	run := e.Rotari("run", "-p", project)
	if run.Code != 0 {
		t.Fatalf("run exit code = %d, want 0: %s", run.Code, run)
	}
	start := strings.LastIndex(run.Stdout, "=== Run finished ===")
	if start < 0 {
		t.Fatalf("run output has no completion message:\n%s", run.Stdout)
	}
	completion := run.Stdout[start:]
	wait := e.Rotari("wait", "-p", project)
	if wait.Code != 0 {
		t.Fatalf("wait exit code = %d, want 0: %s", wait.Code, wait)
	}
	if wait.Stdout != completion {
		t.Fatalf("wait completion message differs from run\nrun:  %q\nwait: %q", completion, wait.Stdout)
	}
}

func TestWaitLiveProgress(t *testing.T) {
	covers(t, "CLI-19")
	e := support.NewEnv(t)
	project := "live-progress"
	gateFirst := filepath.Join(e.Root, "release-first")
	gateSecond := filepath.Join(e.Root, "release-second")
	t.Cleanup(func() {
		_ = os.WriteFile(gateFirst, nil, 0o600)
		_ = os.WriteFile(gateSecond, nil, 0o600)
		_ = e.Rotari("cancel", "-p", project, "--wait")
	})
	// The first attempt is already running when wait attaches and must not be
	// replayed. The retry starts afterward, so its progress must be visible live.
	e.MustRotari("add", "-p", project, "--", "sh", "-c",
		"if [ ! -f attempted ]; then touch attempted; while [ ! -f release-first ]; do sleep 0.05; done; exit 7; fi; while [ ! -f release-second ]; do sleep 0.05; done")
	e.MustRotari("run", "-p", project, "--async", "--quiet", "--retry", "1")
	support.WaitUntil(t, 10*time.Second, func() (bool, string) {
		jobs := e.MustRotari("jobs", "--basedir", e.Base, project, "--since", "0").Stdout
		return strings.Contains(jobs, "running"), jobs
	})
	cmd := e.Command("wait", "-p", project, "--timeout", "10s")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	live := make(chan struct{}, 1)
	snapshot := make(chan struct{}, 1)
	done := make(chan string, 1)
	scanErrors := make(chan error, 1)
	go func() {
		var output strings.Builder
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			output.WriteString(line)
			output.WriteByte('\n')
			if strings.Contains(line, "Job running:") {
				select {
				case live <- struct{}{}:
				default:
				}
			}
			if strings.Contains(line, "progress: 0/1 succeeded=0 failed=0") {
				select {
				case snapshot <- struct{}{}:
				default:
				}
			}
		}
		scanErrors <- scanner.Err()
		done <- output.String()
	}()
	select {
	case <-snapshot:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("wait did not show the current progress snapshot on attach")
	}
	if err := os.WriteFile(gateFirst, nil, 0o600); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	select {
	case <-live:
	case <-time.After(12 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("wait did not print live progress before the gated job finished")
	}
	if err := os.WriteFile(gateSecond, nil, 0o600); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	output := <-done
	if err := <-scanErrors; err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil || stderr.Len() != 0 {
		t.Fatalf("wait: %v; stderr: %s; stdout: %s", err, stderr.String(), output)
	}
	for _, want := range []string{"=== Run attached ===", "progress: 0/1 succeeded=0 failed=0", "Job running:", "Command:", "Retrying job", "progress: 1/1 succeeded=1 failed=0", "=== Run finished ==="} {
		if !strings.Contains(output, want) {
			t.Errorf("wait missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "=== Run started ===") || strings.Contains(output, "Submitted: 1") || strings.Contains(output, "Job running:") && strings.Index(output, "=== Run attached ===") > strings.Index(output, "Job running:") {
		t.Fatalf("wait replayed events from before attach: %s", output)
	}
	if !strings.Contains(output, "Press Ctrl-D to stop waiting; Ctrl-C to cancel the run.") {
		t.Fatalf("wait omitted its control hint: %s", output)
	}
	if strings.Count(output, "=== Run finished ===") != 1 || strings.LastIndex(output, "progress: 1/1") > strings.Index(output, "=== Run finished ===") {
		t.Fatalf("final progress must precede exactly one completion: %s", output)
	}
}

func TestWaitInterruptCancelsRun(t *testing.T) {
	covers(t, "CLI-19")
	e := support.NewEnv(t)
	project := "interrupt-wait"
	gate := filepath.Join(e.Root, "release")
	t.Cleanup(func() {
		_ = os.WriteFile(gate, nil, 0o600)
		_ = e.Rotari("cancel", "-p", project, "--wait")
	})
	e.MustRotari("add", "-p", project, "--", "sh", "-c", "while [ ! -f release ]; do sleep 0.05; done")
	e.MustRotari("run", "-p", project, "--async", "--quiet")
	cmd := e.Command("wait", "-p", project, "--timeout", "10s")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "Run attached") {
				ready <- nil
				return
			}
		}
		if err := scanner.Err(); err != nil {
			ready <- err
		} else {
			ready <- io.EOF
		}
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatal(err)
		}
	case <-time.After(12 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("wait did not attach to the active run")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("Ctrl-C unexpectedly returned success")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 130 {
		t.Fatalf("Ctrl-C exit = %v, want 130", err)
	}
	result := e.Rotari("wait", "-p", project, "--quiet", "--json", "--timeout", "10s")
	var summary struct {
		Status string `json:"status"`
	}
	if result.Code == 0 || strings.Contains(result.Stderr, "timed out waiting") {
		t.Fatalf("cancelled run did not finish after Ctrl-C: %s", result)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &summary); err != nil || summary.Status == "running" {
		t.Fatalf("Ctrl-C did not cancel run or polluted JSON: %v; %s", err, result)
	}
}

func TestRunClientDisconnectDetachesByDefaultAndCanCancel(t *testing.T) {
	covers(t, "CLI-19")
	for _, test := range []struct {
		name   string
		flag   bool
		env    bool
		cancel bool
	}{
		{name: "default detaches"},
		{name: "option cancels", flag: true, cancel: true},
		{name: "environment cancels", env: true, cancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			project := "disconnect-policy"
			gate := filepath.Join(e.Root, "release")
			t.Cleanup(func() {
				_ = os.WriteFile(gate, nil, 0o600)
				_ = e.Rotari("cancel", "-p", project, "--wait")
			})
			e.MustRotari("add", "-p", project, "--", "sh", "-c", "while [ ! -f "+gate+" ]; do sleep 0.05; done")
			clientEnv := e
			args := []string{"run", "-p", project, "--quiet"}
			if test.flag {
				args = append(args, "--disconnect-action", "cancel")
			}
			if test.env {
				clientEnv = e.WithVar("ROTARI_DISCONNECT_ACTION", "cancel")
			}
			cmd := clientEnv.Command(args...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			runID := ""
			support.WaitUntil(t, 10*time.Second, func() (bool, string) {
				jobs := e.MustRotari("jobs", "--basedir", e.Base, project, "--since", "0").Stdout
				if strings.Contains(jobs, "\nrunning ") {
					var shown struct {
						RunID string `json:"run_id"`
					}
					output := e.MustRotari("show", "-p", project, "--json").Stdout
					_ = json.Unmarshal([]byte(output), &shown)
					runID = shown.RunID
				}
				return runID != "", jobs
			})
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			support.WaitUntil(t, 5*time.Second, func() (bool, string) {
				data, err := os.ReadFile(filepath.Join(e.Base, "projects", project, "running.lock"))
				if errors.Is(err, os.ErrNotExist) {
					// A cancelled run may already have finished.
					return test.cancel, "run lock removed"
				}
				if err != nil {
					return false, err.Error()
				}
				var lock struct {
					ClientAttached bool `json:"client_attached"`
				}
				if err := json.Unmarshal(data, &lock); err != nil {
					return false, err.Error()
				}
				return !lock.ClientAttached, string(data)
			})

			if test.cancel {
				waited := e.Rotari("wait", "--basedir", e.Base, "--run-id", runID, "--quiet", "--json", "--timeout", "5s")
				var summary struct {
					Status string `json:"status"`
				}
				if waited.Code == 0 || strings.Contains(waited.Stderr, "timed out waiting") || json.Unmarshal([]byte(waited.Stdout), &summary) != nil || summary.Status == "running" {
					t.Fatalf("disconnect policy did not cancel run: %s", waited)
				}
				return
			}

			implicit := e.Rotari("wait", "--basedir", e.Base, "--timeout", "50ms")
			if implicit.Code == 0 || !strings.Contains(implicit.Stderr, "timed out waiting for run "+runID) {
				t.Fatalf("default disconnect was not discoverable as a detached run: %s", implicit)
			}
			if err := os.WriteFile(gate, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if completed := e.Rotari("wait", "--basedir", e.Base, "--timeout", "5s"); completed.Code != 0 {
				t.Fatalf("wait after releasing detached run: %s", completed)
			}
		})
	}
}

func TestWaitClientDisconnectDetachesByDefaultAndCanCancel(t *testing.T) {
	covers(t, "CLI-19")
	for _, test := range []struct {
		name   string
		args   []string
		env    bool
		cancel bool
	}{
		{name: "default detaches"},
		{name: "option cancels", args: []string{"--disconnect-action", "cancel"}, cancel: true},
		{name: "environment cancels", env: true, cancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			project := "wait-disconnect"
			gate := filepath.Join(e.Root, "release")
			t.Cleanup(func() {
				_ = os.WriteFile(gate, nil, 0o600)
				_ = e.Rotari("cancel", "-p", project, "--wait")
			})
			e.MustRotari("add", "-p", project, "--", "sh", "-c", "while [ ! -f "+gate+" ]; do sleep 0.05; done")
			e.MustRotari("run", "-p", project, "--async", "--quiet")
			waiterEnv := e
			if test.env {
				waiterEnv = e.WithVar("ROTARI_DISCONNECT_ACTION", "cancel")
			}
			cmd := waiterEnv.Command(append([]string{"wait", "-p", project, "--timeout", "30s"}, test.args...)...)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			attached := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					if strings.Contains(scanner.Text(), "Run attached") {
						attached <- true
						break
					}
				}
				_, _ = io.Copy(io.Discard, stdout)
			}()
			select {
			case <-attached:
			case <-time.After(12 * time.Second):
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("wait did not attach")
			}
			// A closed terminal or a tool timeout terminates the waiter.
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()

			result := e.Rotari("wait", "-p", project, "--quiet", "--json", "--timeout", "3s")
			stillRunning := strings.Contains(result.Stderr, "timed out waiting")
			if stillRunning == test.cancel {
				t.Fatalf("waiter disconnect cancel=%v left run running=%v: %s", test.cancel, stillRunning, result)
			}
		})
	}
}

func TestWaitInterruptCancelsAllSelectedRuns(t *testing.T) {
	covers(t, "CLI-19")
	e := support.NewEnv(t)
	projects := []string{"wait-alpha", "wait-beta"}
	runIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		e.MustRotari("add", "--basedir", e.Base, "--project-name", project, "--", "sh", "-c", "sleep 30")
		started := e.MustRotari("run", "--basedir", e.Base, "--project-name", project, "--async").Stdout
		runID := ""
		for _, line := range strings.Split(started, "\n") {
			if strings.HasPrefix(line, "  Run: ") {
				runID = strings.TrimPrefix(line, "  Run: ")
				break
			}
		}
		if runID == "" {
			t.Fatalf("async output has no run ID for %s: %s", project, started)
		}
		runIDs = append(runIDs, runID)
		t.Cleanup(func() { _ = e.Rotari("cancel", "--basedir", e.Base, "--project-name", project, "--wait") })
	}
	args := []string{"wait", "--basedir", e.Base, "--run-id", runIDs[0], "--run-id", runIDs[1], "--timeout", "10s"}
	quietArgs := []string{"wait", "--basedir", e.Base, "--run-id", runIDs[0], "--run-id", runIDs[1], "--quiet", "--timeout", "1ns"}
	quiet := e.Rotari(quietArgs...)
	if quiet.Code != 1 || quiet.Stdout != "" || !strings.Contains(quiet.Stderr, "timed out waiting") {
		t.Fatalf("multi-run quiet wait printed normal output: %s", quiet)
	}
	cmd := e.Command(args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	attached := make(chan struct{}, 2)
	scanDone := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "Run attached") {
				attached <- struct{}{}
			}
		}
		scanDone <- scanner.Err()
	}()
	killAndReap := func() {
		_ = cmd.Process.Kill()
		<-scanDone
		_ = cmd.Wait()
	}
	for range 2 {
		select {
		case <-attached:
		case <-time.After(12 * time.Second):
			killAndReap()
			t.Fatal("multi-run wait did not attach to both active runs")
		}
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		killAndReap()
		t.Fatal(err)
	}
	var scanErr error
	select {
	case scanErr = <-scanDone:
	case <-time.After(12 * time.Second):
		killAndReap()
		t.Fatal("multi-run wait stdout did not close after Ctrl-C")
	}
	if scanErr != nil {
		_ = cmd.Wait()
		t.Fatalf("scan multi-run wait output: %v", scanErr)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("Ctrl-C unexpectedly returned success")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 130 {
		t.Fatalf("Ctrl-C exit = %v, want 130", err)
	}
	for _, runID := range runIDs {
		result := e.Rotari("wait", "--basedir", e.Base, "--run-id", runID, "--quiet", "--json", "--timeout", "10s")
		if strings.Contains(result.Stderr, "timed out waiting") {
			t.Fatalf("Ctrl-C left run %s active: %s", runID, result)
		}
		var summary struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &summary); err != nil || summary.Status == "running" {
			t.Fatalf("run %s was not cancelled or JSON was polluted: %v; %s", runID, err, result)
		}
	}
}

func TestWaitQuietAndJSON(t *testing.T) {
	covers(t, "CLI-19")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	for _, args := range [][]string{{"--quiet"}, {"--quiet", "--json"}} {
		result := e.Rotari(append([]string{"wait", "--run-id", run.RunID}, args...)...)
		if result.Code != 1 || result.Stderr != "" {
			t.Fatalf("quiet changed the result: %s", result)
		}
		if len(args) == 1 {
			if result.Stdout != "" {
				t.Fatalf("quiet printed completion: %s", result)
			}
		} else {
			var summary struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal([]byte(result.Stdout), &summary); err != nil || summary.RunID != run.RunID {
				t.Fatalf("quiet suppressed or polluted JSON: %v; %s", err, result)
			}
		}
	}
	config := filepath.Join(e.Root, "wait.yaml")
	if err := os.WriteFile(config, []byte("quiet: false\nwait:\n  quiet: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		env   *support.Env
		args  []string
		quiet bool
	}{
		{name: "config", env: e, args: []string{"--config", config}, quiet: true},
		{name: "environment", env: e.WithVar("ROTARI_QUIET", "true"), quiet: true},
		{name: "environment-over-config", env: e.WithVar("ROTARI_QUIET", "false"), args: []string{"--config", config}},
		{name: "cli-over-environment", env: e.WithVar("ROTARI_QUIET", "true"), args: []string{"--quiet=false"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := test.env.Rotari(append([]string{"wait", "--run-id", run.RunID}, test.args...)...)
			if result.Code != 1 || result.Stderr != "" || (result.Stdout == "") != test.quiet {
				t.Fatalf("quiet precedence: %s", result)
			}
		})
	}

	// Reconstruct an active read-side fixture with actual persisted failure
	// events and results, but no summary. No coordinator can be cancelled.
	projectDir := filepath.Join(e.Base, "projects", run.Project)
	lockPath := filepath.Join(projectDir, "running.lock")
	lock := []byte(`{"pid":1,"host":"wait-remote.invalid","run_id":"` + run.RunID + `"}`)
	if err := os.WriteFile(lockPath, lock, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(projectDir, "runs", run.RunID, "summary.json")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--quiet"}, {"--json"}, {"--quiet", "--json"}} {
		result := e.Rotari(append([]string{"wait", "--run-id", run.RunID, "--until-failure", "--timeout", "2s"}, args...)...)
		if result.Code != 1 || result.Stderr != "" {
			t.Fatalf("early failure: %s", result)
		}
		if args[len(args)-1] == "--json" {
			var failure struct {
				RunID  string `json:"run_id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal([]byte(result.Stdout), &failure); err != nil || failure.RunID != run.RunID || failure.Status != "running" {
				t.Fatalf("early-failure JSON: %v; %s", err, result)
			}
		} else if !strings.Contains(result.Stdout, "Failures by cause:") || !strings.Contains(result.Stdout, "jobs have failed") || strings.Contains(result.Stdout, "Job running:") || strings.Contains(result.Stdout, "progress:") || strings.Contains(result.Stdout, "=== Run") {
			t.Fatalf("quiet hid diagnostics or showed normal progress: %s", result)
		}
	}
	result := e.Rotari("wait", "--run-id", run.RunID, "--quiet", "--timeout", "1ns")
	if result.Code != 1 || !strings.Contains(result.Stderr, "timed out waiting") || strings.Contains(result.Stdout, "progress:") {
		t.Fatalf("quiet hid timeout: %s", result)
	}
	after, err := os.ReadFile(lockPath)
	if err != nil || !bytes.Equal(after, lock) {
		t.Fatalf("wait changed active run lock: %s; %v", after, err)
	}
}

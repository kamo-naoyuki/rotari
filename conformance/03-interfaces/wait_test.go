package interfaces

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	gate := filepath.Join(e.Root, "release")
	t.Cleanup(func() {
		_ = os.WriteFile(gate, nil, 0o600)
		_ = e.Rotari("cancel", "-p", project, "--wait")
	})
	// The second attempt cannot finish until wait has visibly reported a
	// job start. This proves live output, not just a replay after completion.
	e.MustRotari("add", "-p", project, "--", "sh", "-c",
		"if [ ! -f attempted ]; then touch attempted; exit 7; fi; while [ ! -f release ]; do sleep 0.05; done")
	e.MustRotari("run", "-p", project, "--async", "--quiet", "--retry", "1")
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
		}
		scanErrors <- scanner.Err()
		done <- output.String()
	}()
	select {
	case <-live:
	case <-time.After(12 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("wait did not print live progress before the gated job finished")
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
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
	for _, want := range []string{"=== Run started ===", "Submitted: 1", "Job running:", "Command:", "Retrying job", "progress: 1/1 succeeded=1 failed=0", "=== Run finished ==="} {
		if !strings.Contains(output, want) {
			t.Errorf("wait missing %q: %s", want, output)
		}
	}
	if strings.Contains(output, "Ctrl-C to cancel") || strings.Contains(output, "Ctrl-D") {
		t.Fatalf("wait offers synchronous controls: %s", output)
	}
	if strings.Count(output, "=== Run finished ===") != 1 || strings.LastIndex(output, "progress: 1/1") > strings.Index(output, "=== Run finished ===") {
		t.Fatalf("final progress must precede exactly one completion: %s", output)
	}
}

func TestWaitInterruptDoesNotCancelRun(t *testing.T) {
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
			if strings.Contains(scanner.Text(), "Job running:") {
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
		t.Fatal("interrupted wait unexpectedly succeeded")
	}
	// A new quiet JSON waiter can still observe successful completion. If
	// interrupting the first waiter had cancelled the run, this would fail.
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	result := e.MustRotari("wait", "-p", project, "--quiet", "--json", "--timeout", "10s")
	var summary struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &summary); err != nil || summary.Status != "finished" || result.Stderr != "" {
		t.Fatalf("wait interruption cancelled or polluted run: %v; %s", err, result)
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
		} else if !strings.Contains(result.Stdout, "Job failed:") || !strings.Contains(result.Stdout, "jobs have failed") || strings.Contains(result.Stdout, "Job running:") || strings.Contains(result.Stdout, "progress:") || strings.Contains(result.Stdout, "=== Run") {
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

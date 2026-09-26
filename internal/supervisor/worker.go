package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/run"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// launchWorker records a new run and starts a detached worker that executes
// it. The caller holds the state lock and has checked that the project is
// idle.
func (ops Operations) launchWorker(paths state.ProjectPaths, options run.Options) error {
	if err := ops.Runner.Begin(paths, projectrun.Start{RunID: options.RunID, RunName: options.RunName, CWD: options.CWD}); err != nil {
		return err
	}
	// A worker that never starts leaves the project interrupted, not running.
	abandon := func(format string, err error) error {
		_ = os.Remove(paths.LockFile)
		return fmt.Errorf(format, err)
	}
	executable := ops.Executable
	if executable == nil {
		executable = os.Executable
	}
	exe, err := executable()
	if err != nil {
		return abandon("failed to detect executable path: %w", err)
	}
	childOptions := options
	childOptions.BaseDir = paths.BaseDir
	cmd := exec.Command(exe, run.WorkerArgs(childOptions, paths.BaseDirExplicit, executor.RunSettingNames)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return abandon("failed to launch async runner: %w", err)
	}
	host, err := os.Hostname()
	if err != nil {
		_ = cmd.Process.Kill()
		return abandon("failed to determine lock host: %w", err)
	}
	startedAt := time.Now().UTC().Format(time.RFC3339)
	if err := state.WriteJSON(paths.LockFile, model.LockInfo{PID: cmd.Process.Pid, RunID: options.RunID, RunName: options.RunName, StartedAt: startedAt, Host: host}); err != nil {
		_ = cmd.Process.Kill()
		return abandon("failed to update lock with child pid: %w", err)
	}
	waitForWorker(cmd, options.OnDone)

	ops.printf("submitted project=%s run_id=%s pid=%d\n", options.QueueName, options.RunID, cmd.Process.Pid)
	return nil
}

// waitForWorker reaps a started worker in the background and then calls
// onDone, if not nil.
func waitForWorker(cmd *exec.Cmd, onDone func()) {
	go func() {
		_ = cmd.Wait()
		if onDone != nil {
			onDone()
		}
	}()
}

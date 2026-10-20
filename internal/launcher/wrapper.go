package launcher

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// Descriptors a wrapper inherits from Start.
const (
	lifetimeFD = 3
	statusFD   = 4
)

// groupPoll is how often a wrapper checks whether a process group is gone.
const groupPoll = 50 * time.Millisecond

// groupKillWait bounds the wait for a process group to disappear after
// SIGKILL, which only an uninterruptible process outlives.
const groupKillWait = 5 * time.Second

// RunWrapper is the hidden `rotari __launch [--grace D] -- COMMAND...`. It
// starts COMMAND in a new process group and reports on descriptor 4:
//
//	started PGID     once COMMAND is running
//	error MESSAGE    instead, if COMMAND could not start
//	exit CODE        after COMMAND and its group have ended
//	signal NUMBER    instead, if a signal ended COMMAND
//
// It stops the whole group (SIGTERM, then SIGKILL after the grace period)
// when it receives SIGTERM, SIGINT, or SIGHUP, or when descriptor 3 (the read
// end of a Lifetime) reports EOF. When COMMAND exits by itself, members it
// left in its group are stopped the same way. Processes that leave the group
// (setsid, setpgid) are not tracked.
func RunWrapper(args []string) int {
	flags := flag.NewFlagSet(WrapperCommand, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	grace := flags.Duration("grace", 5*time.Second, "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	command := flags.Args()
	if len(command) == 0 || *grace < 0 {
		fmt.Fprintln(os.Stderr, "rotari "+WrapperCommand+": usage: [--grace DURATION] -- COMMAND...")
		return 2
	}
	for _, fd := range []int{lifetimeFD, statusFD} {
		var stat syscall.Stat_t
		if err := syscall.Fstat(fd, &stat); err != nil {
			fmt.Fprintf(os.Stderr, "rotari %s: descriptor %d is not open; this command is started only by rotari\n", WrapperCommand, fd)
			return 2
		}
		// The command must not inherit them: a service holding the status
		// pipe would delay the final report until the service's own
		// children exit.
		syscall.CloseOnExec(fd)
	}
	lifetime := os.NewFile(lifetimeFD, "launcher-lifetime")
	status := os.NewFile(statusFD, "launcher-status")

	// Handled, not ignored: an ignored disposition would be inherited by
	// the command.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	child := exec.Command(command[0], command[1:]...)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		fmt.Fprintf(status, "error %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
		return 127
	}
	pgid := child.Process.Pid
	fmt.Fprintf(status, "started %d\n", pgid)

	// Reaping the leader promptly matters: an unreaped leader keeps the
	// group alive, so stopGroup would wait for the full grace period.
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	ownerGone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, lifetime)
		close(ownerGone)
	}()

	var waitErr error
	select {
	case waitErr = <-exited:
		stopGroup(pgid, *grace)
	case <-signals:
		stopGroup(pgid, *grace)
		waitErr = <-exited
	case <-ownerGone:
		stopGroup(pgid, *grace)
		waitErr = <-exited
	}
	fmt.Fprintln(status, resultLine(waitErr))
	return 0
}

func resultLine(waitErr error) string {
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
			return fmt.Sprintf("signal %d", int(waitStatus.Signal()))
		}
		return fmt.Sprintf("exit %d", exitErr.ExitCode())
	}
	if waitErr != nil {
		return "exit -1"
	}
	return "exit 0"
}

// stopGroup sends SIGTERM to the process group, waits up to grace for it to
// disappear, and then sends SIGKILL.
func stopGroup(pgid int, grace time.Duration) {
	if !signalGroup(pgid, syscall.SIGTERM) {
		return
	}
	if waitGroupGone(pgid, grace) {
		return
	}
	if signalGroup(pgid, syscall.SIGKILL) {
		waitGroupGone(pgid, groupKillWait)
	}
}

// signalGroup reports whether the group still existed.
func signalGroup(pgid int, sig syscall.Signal) bool {
	return !errors.Is(syscall.Kill(-pgid, sig), syscall.ESRCH)
}

func waitGroupGone(pgid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !signalGroup(pgid, 0) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(groupPoll)
	}
}

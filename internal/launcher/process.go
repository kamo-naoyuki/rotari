// Package launcher runs the commands a user registered for artifacts and
// owns their processes. It knows nothing about jobs, runs, or HTTP: callers
// supply a command, its working directory, and its environment.
//
// Every launch runs under a wrapper process (RunWrapper) that puts the
// command in a process group of its own and stops that group when the
// command is stopped, when the command exits, or when the process that holds
// the Lifetime exits for any reason, including SIGKILL.
package launcher

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// WrapperCommand is the hidden rotari subcommand that runs RunWrapper.
const WrapperCommand = "__launch"

// Shell runs every launched command as `Shell -c COMMAND`.
const Shell = "/bin/sh"

// stopMargin is how long Stop waits beyond the grace period for the wrapper
// to finish its own cleanup before killing the group itself.
const stopMargin = 5 * time.Second

// Lifetime is a pipe whose write end only its creator holds. Wrappers inherit
// the read end and stop their command when it reports EOF, which the kernel
// delivers when the creator exits, however it exits.
type Lifetime struct {
	reader, writer *os.File
}

// NewLifetime creates the pipe. Both ends are close-on-exec, so only the
// wrappers started by Start receive the read end and nothing receives the
// write end.
func NewLifetime() (*Lifetime, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create launcher lifetime pipe: %w", err)
	}
	return &Lifetime{reader: reader, writer: writer}, nil
}

// Close closes both ends. Every wrapper still running then stops its command.
func (lifetime *Lifetime) Close() error {
	return errors.Join(lifetime.writer.Close(), lifetime.reader.Close())
}

// Spec describes one command to start.
type Spec struct {
	// Wrapper is the argv that runs RunWrapper, for example the rotari
	// executable followed by WrapperCommand.
	Wrapper []string
	// Command is shell source, run as `Shell -c Command`.
	Command string
	// Dir is the command's working directory.
	Dir string
	// Env is the complete environment of the wrapper and the command.
	Env []string
	// Output receives the command's standard output and standard error.
	// Standard input is always empty.
	Output io.Writer
	// Grace is how long the command's process group has to exit after
	// SIGTERM before it receives SIGKILL.
	Grace time.Duration
}

// Result is how a command ended.
type Result struct {
	// ExitCode is the command's exit status, or -1 when a signal ended it.
	ExitCode int
	// Signal is the signal that ended the command, or 0.
	Signal syscall.Signal
}

// Process is a started command.
type Process struct {
	wrapper *exec.Cmd
	pgid    int
	grace   time.Duration
	done    chan struct{}
	result  Result
	err     error
}

// Start starts spec's command under its wrapper and returns once the command
// is running in its own process group.
func Start(lifetime *Lifetime, spec Spec) (*Process, error) {
	if len(spec.Wrapper) == 0 {
		return nil, errors.New("launcher wrapper command is empty")
	}
	if spec.Command == "" {
		return nil, errors.New("launch command is empty")
	}
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create launcher status pipe: %w", err)
	}
	args := append(append([]string{}, spec.Wrapper[1:]...),
		"--grace", spec.Grace.String(), "--", Shell, "-c", spec.Command)
	wrapper := exec.Command(spec.Wrapper[0], args...)
	wrapper.Dir = spec.Dir
	wrapper.Env = spec.Env
	wrapper.Stdout, wrapper.Stderr = spec.Output, spec.Output
	// Descriptors 3 and 4 in the wrapper; see RunWrapper.
	wrapper.ExtraFiles = []*os.File{lifetime.reader, statusWriter}
	// A group of its own keeps terminal signals sent to the caller's group
	// away from the wrapper; the caller stops launches explicitly.
	wrapper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	err = wrapper.Start()
	statusWriter.Close()
	if err != nil {
		statusReader.Close()
		return nil, fmt.Errorf("start launcher wrapper: %w", err)
	}

	status := bufio.NewReader(statusReader)
	kind, value, readErr := readStatus(status)
	if readErr != nil || kind != "started" {
		statusReader.Close()
		_ = wrapper.Wait()
		switch {
		case kind == "error":
			return nil, fmt.Errorf("start launch command: %s", value)
		case readErr != nil:
			return nil, fmt.Errorf("launcher wrapper ended before starting the command: %w", readErr)
		default:
			return nil, fmt.Errorf("launcher wrapper reported %q before starting the command", kind)
		}
	}
	pgid, err := strconv.Atoi(value)
	if err != nil || pgid <= 1 {
		statusReader.Close()
		_ = wrapper.Process.Kill()
		_ = wrapper.Wait()
		return nil, fmt.Errorf("launcher wrapper reported an invalid process group %q", value)
	}

	process := &Process{wrapper: wrapper, pgid: pgid, grace: spec.Grace, done: make(chan struct{})}
	go process.finish(statusReader, status)
	return process, nil
}

// finish reads the wrapper's final status line, which it writes after the
// command's process group has ended, and then reaps the wrapper.
func (process *Process) finish(statusReader *os.File, status *bufio.Reader) {
	defer close(process.done)
	kind, value, readErr := readStatus(status)
	statusReader.Close()
	waitErr := process.wrapper.Wait()
	if readErr != nil {
		process.err = fmt.Errorf("launcher wrapper ended without reporting the command's result: %w", errors.Join(readErr, waitErr))
		return
	}
	code, err := strconv.Atoi(value)
	switch {
	case err != nil:
		process.err = fmt.Errorf("launcher wrapper reported an invalid result %q", kind+" "+value)
	case kind == "exit":
		process.result = Result{ExitCode: code}
	case kind == "signal":
		process.result = Result{ExitCode: -1, Signal: syscall.Signal(code)}
	default:
		process.err = fmt.Errorf("launcher wrapper reported an invalid result %q", kind+" "+value)
	}
}

// readStatus reads one "KIND VALUE" line written by RunWrapper.
func readStatus(status *bufio.Reader) (kind, value string, err error) {
	line, err := status.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return "", "", err
	}
	kind, value, _ = strings.Cut(strings.TrimSuffix(line, "\n"), " ")
	return kind, value, nil
}

// PGID is the command's process group. The group's leader is the shell.
func (process *Process) PGID() int { return process.pgid }

// Done is closed after the command and its process group have ended.
func (process *Process) Done() <-chan struct{} { return process.done }

// Wait waits for the command and its process group to end.
func (process *Process) Wait() (Result, error) {
	<-process.done
	return process.result, process.err
}

// Stop asks the wrapper to stop the command's process group (SIGTERM, then
// SIGKILL after the grace period) and waits for it. If the wrapper does not
// finish in time, Stop kills the group and the wrapper itself.
func (process *Process) Stop() (Result, error) {
	if err := process.wrapper.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return Result{}, fmt.Errorf("signal launcher wrapper: %w", err)
	}
	select {
	case <-process.done:
	case <-time.After(process.grace + stopMargin):
		_ = syscall.Kill(-process.pgid, syscall.SIGKILL)
		_ = process.wrapper.Process.Kill()
		<-process.done
	}
	return process.result, process.err
}

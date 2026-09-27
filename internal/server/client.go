package server

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// SendRequest sends one request to the supervisor of dir, a project
// directory, and returns its answer.
func SendRequest(dir string, request Request) (Response, error) {
	conn, err := net.DialTimeout("unix", SocketPath(dir), time.Second)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return Response{}, err
	}
	return response, nil
}

// RunOutcome says how a synchronous run request ended on the client side.
type RunOutcome int

const (
	// RunFinished means the server sent the run's final response.
	RunFinished RunOutcome = iota
	// RunDetached means the client detached and the run continues.
	RunDetached
	// RunInterrupted means the client disconnected, which asks the server to
	// cancel the run.
	RunInterrupted
)

// StreamRun sends a synchronous run request and passes each progress response
// to progress until the final response arrives. A value from detach sends
// DetachControl before disconnecting; a value from interrupt disconnects
// without it.
func StreamRun(dir string, request Request, detach <-chan struct{}, interrupt <-chan os.Signal, progress func(Response)) (Response, RunOutcome, error) {
	conn, err := net.DialTimeout("unix", SocketPath(dir), time.Second)
	if err != nil {
		return Response{}, RunFinished, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return Response{}, RunFinished, err
	}
	decoder := json.NewDecoder(conn)
	responses := make(chan Response, 1)
	decodeErrors := make(chan error, 1)
	go func() {
		for {
			var response Response
			if err := decoder.Decode(&response); err != nil {
				decodeErrors <- err
				return
			}
			responses <- response
			if !response.Progress {
				return
			}
		}
	}()
	for {
		select {
		case <-detach:
			if _, err := conn.Write([]byte{DetachControl}); err != nil {
				return Response{}, RunDetached, err
			}
			return Response{OK: true}, RunDetached, nil
		case <-interrupt:
			return Response{OK: true, ExitCode: 130}, RunInterrupted, nil
		case err := <-decodeErrors:
			return Response{}, RunFinished, err
		case response := <-responses:
			if !response.Progress {
				return response, RunFinished, nil
			}
			progress(response)
		}
	}
}

// Start starts command as the supervisor of dir, a project directory, and
// returns its PID once it answers. The supervisor is detached from the
// caller's terminal session but inherits the caller's working directory and
// environment, which its run's jobs then inherit in turn. Start never reuses
// a running supervisor: it fails with ErrAlreadyRunning when another one
// serves dir. logPath names the log that explains a failed start.
func Start(dir string, command []string, logPath string) (int, error) {
	child := exec.Command(command[0], command[1:]...)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to detach server stdio: %w", err)
	}
	defer devNull.Close()
	child.Stdin = devNull
	child.Stdout = devNull
	child.Stderr = devNull
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return 0, fmt.Errorf("failed to start server: %w", err)
	}
	pid := child.Process.Pid
	exited := make(chan struct{})
	go func() {
		_ = child.Wait()
		close(exited)
	}()
	for range 100 {
		// A supervisor that answers with another PID holds the lease, and
		// the child exits once it fails to take it.
		if response, err := SendRequest(dir, Request{Op: OpPing}); err == nil && response.OK {
			if response.PID == pid {
				return pid, nil
			}
			return 0, ErrAlreadyRunning
		}
		select {
		case <-exited:
			if response, err := SendRequest(dir, Request{Op: OpPing}); err == nil && response.OK {
				return 0, ErrAlreadyRunning
			}
			return 0, fmt.Errorf("server exited before it became ready (pid=%d); see %s for the cause", pid, logPath)
		case <-time.After(50 * time.Millisecond):
		}
	}
	_ = child.Process.Kill()
	return 0, fmt.Errorf("server did not become ready (pid=%d); see %s for the cause", pid, logPath)
}

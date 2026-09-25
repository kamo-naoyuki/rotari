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

func SendRequest(baseDir string, request Request) (Response, error) {
	conn, err := net.DialTimeout("unix", SocketPath(baseDir), time.Second)
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
func StreamRun(baseDir string, request Request, detach <-chan struct{}, interrupt <-chan os.Signal, progress func(Response)) (Response, RunOutcome, error) {
	conn, err := net.DialTimeout("unix", SocketPath(baseDir), time.Second)
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

// Ensure makes sure a server with the current protocol serves baseDir. It
// stops a server with another protocol version and otherwise starts command,
// detached from the caller's terminal session.
func Ensure(baseDir string, command []string) error {
	if response, err := SendRequest(baseDir, Request{Op: OpPing}); err == nil && response.OK {
		if response.Protocol == ProtocolVersion {
			return nil
		}
		if _, shutdownErr := SendRequest(baseDir, Request{Op: OpShutdown}); shutdownErr != nil {
			return fmt.Errorf("server protocol mismatch (got %d, want %d); failed to stop old server: %w", response.Protocol, ProtocolVersion, shutdownErr)
		}
		for range 40 {
			if _, pingErr := SendRequest(baseDir, Request{Op: OpPing}); pingErr != nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	child := exec.Command(command[0], command[1:]...)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to detach server stdio: %w", err)
	}
	defer devNull.Close()
	child.Stdin = devNull
	child.Stdout = devNull
	child.Stderr = devNull
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	for range 40 {
		if response, err := SendRequest(baseDir, Request{Op: OpPing}); err == nil && response.OK {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready (pid=%d)", child.Process.Pid)
}

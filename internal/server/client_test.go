package server

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSendRequest(t *testing.T) {
	baseDir, err := os.MkdirTemp("/tmp", "rotari-server-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	listener, err := net.Listen("unix", SocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer connection.Close()
		var request Request
		if err := json.NewDecoder(connection).Decode(&request); err != nil {
			serverDone <- err
			return
		}
		if request.Op != OpRun || request.QueueName != "demo" {
			serverDone <- &unexpectedRequestError{request: request}
			return
		}
		serverDone <- json.NewEncoder(connection).Encode(Response{OK: true, JobID: "job-1"})
	}()

	response, err := SendRequest(baseDir, Request{Op: OpRun, QueueName: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.JobID != "job-1" {
		t.Fatalf("SendRequest() = %#v", response)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestSendRequestReturnsConnectionError(t *testing.T) {
	if _, err := SendRequest(t.TempDir(), Request{Op: OpPing}); err == nil {
		t.Fatal("SendRequest() returned nil error without a server")
	}
}

func TestSendRequestReturnsInvalidResponseError(t *testing.T) {
	baseDir, err := os.MkdirTemp("/tmp", "rotari-server-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	listener, err := net.Listen("unix", SocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = connection.Write([]byte("not json\n"))
	}()

	if _, err := SendRequest(baseDir, Request{Op: OpPing}); err == nil {
		t.Fatal("SendRequest() accepted an invalid response")
	}
}

func TestSendRequestTimesOutWhenServerDoesNotRespond(t *testing.T) {
	baseDir := t.TempDir()
	listener, err := net.Listen("unix", SocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			defer connection.Close()
			<-serverDone
		}
	}()
	defer close(serverDone)

	result := make(chan error, 1)
	go func() {
		_, err := SendRequest(baseDir, Request{Op: OpPing})
		result <- err
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("SendRequest() returned nil error without a response")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendRequest() did not time out")
	}
}

type unexpectedRequestError struct {
	request Request
}

func (err *unexpectedRequestError) Error() string {
	return "unexpected request"
}

func TestStartReportsSupervisorThatExitsEarly(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "rotari-server-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	_, err = Start(dir, []string{"sh", "-c", "exit 1"}, "LOG")
	if err == nil || !strings.Contains(err.Error(), "exited before it became ready") || !strings.Contains(err.Error(), "LOG") {
		t.Fatalf("Start() error = %v, want the early exit and the log to read", err)
	}
}

func TestStartDoesNotReuseRunningSupervisor(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "rotari-server-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	listener, release, err := Listen(dir, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	server := New(listener, &fakeOperations{}, nil)
	go server.Serve()
	defer server.Stop()
	// The started command never listens; the answer comes from the running
	// supervisor, whose PID is not the child's.
	if _, err := Start(dir, []string{"sleep", "5"}, "LOG"); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Start() error = %v, want ErrAlreadyRunning", err)
	}
}

package server

import (
	"encoding/json"
	"net"
	"os"
	"testing"
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

type unexpectedRequestError struct {
	request Request
}

func (err *unexpectedRequestError) Error() string {
	return "unexpected request"
}

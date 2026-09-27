package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// helperEnv makes the test binary act as a supervisor started by Start.
const helperEnv = "ROTARI_SERVER_TEST_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnv) == "spawn-job" {
		os.Exit(helperSpawnJob())
	}
	os.Exit(m.Run())
}

// helperSpawnJob announces readiness, starts a long-running job the way a
// supervisor does, and exits without answering.
func helperSpawnJob() int {
	conn, err := InheritedConn()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := Ready(conn, nil); err != nil {
		return 1
	}
	job := exec.Command("sleep", "5")
	if err := job.Start(); err != nil {
		return 1
	}
	return 0
}

func TestClientSend(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		if err := Ready(serverConn, nil); err != nil {
			serverDone <- err
			return
		}
		var request Request
		if err := json.NewDecoder(serverConn).Decode(&request); err != nil {
			serverDone <- err
			return
		}
		if request.Op != OpRun || request.QueueName != "demo" {
			serverDone <- &unexpectedRequestError{request: request}
			return
		}
		serverDone <- json.NewEncoder(serverConn).Encode(Response{OK: true, JobID: "job-1"})
	}()
	client, err := Connect(clientConn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Send(Request{Op: OpRun, QueueName: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.JobID != "job-1" {
		t.Fatalf("Send() = %#v", response)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestClientSendReturnsInvalidResponseError(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	go func() {
		defer serverConn.Close()
		_ = Ready(serverConn, nil)
		var request Request
		_ = json.NewDecoder(serverConn).Decode(&request)
		_, _ = serverConn.Write([]byte("not json\n"))
	}()
	client, err := Connect(clientConn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Send(Request{Op: OpRun}); err == nil {
		t.Fatal("Send() accepted an invalid response")
	}
}

func TestConnectFailures(t *testing.T) {
	for name, test := range map[string]struct {
		serve func(net.Conn)
		want  func(error) bool
	}{
		"already running": {
			serve: func(conn net.Conn) { _ = Ready(conn, ErrAlreadyRunning) },
			want:  func(err error) bool { return errors.Is(err, ErrAlreadyRunning) },
		},
		"start failed": {
			serve: func(conn net.Conn) { _ = Ready(conn, errors.New("failed to register server")) },
			want:  func(err error) bool { return err != nil && err.Error() == "failed to register server" },
		},
		"closed": {
			serve: func(net.Conn) {},
			want:  func(err error) bool { return errors.Is(err, errNotReady) },
		},
	} {
		t.Run(name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			go func() {
				defer serverConn.Close()
				test.serve(serverConn)
			}()
			if _, err := Connect(clientConn, time.Second); !test.want(err) {
				t.Fatalf("Connect() error = %v", err)
			}
		})
	}
}

func TestConnectTimesOutWhenSupervisorIsSilent(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()
	result := make(chan error, 1)
	go func() {
		_, err := Connect(clientConn, 50*time.Millisecond)
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("Connect() error = %v, want a timeout", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Connect() did not time out")
	}
}

type unexpectedRequestError struct {
	request Request
}

func (err *unexpectedRequestError) Error() string {
	return "unexpected request"
}

func TestStartReportsSupervisorThatExitsEarly(t *testing.T) {
	_, err := Start([]string{"sh", "-c", "exit 1"}, "LOG")
	if err == nil || !strings.Contains(err.Error(), "exited before it became ready") || !strings.Contains(err.Error(), "LOG") {
		t.Fatalf("Start() error = %v, want the early exit and the log to read", err)
	}
}

func TestStartReportsAnotherSupervisor(t *testing.T) {
	// The started command stands in for a supervisor that finds the lease
	// taken and says so before exiting.
	command := `printf '{"message":"server is already running"}\n' >&4`
	if _, err := Start([]string{"sh", "-c", command}, "LOG"); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Start() error = %v, want ErrAlreadyRunning", err)
	}
}

func TestStartConnectsThroughInheritedPipes(t *testing.T) {
	command := `printf '{"ok":true,"pid":%d}\n' $$ >&4; read request <&3; case $request in *'"op":"run"'*) printf '{"ok":true,"message":"got run"}\n' >&4;; esac`
	client, err := Start([]string{"sh", "-c", command}, "LOG")
	if err != nil {
		t.Fatal(err)
	}
	if client.PID <= 0 || client.PID == os.Getpid() {
		t.Fatalf("PID = %d, want the child's", client.PID)
	}
	response, err := client.Send(Request{Op: OpRun})
	if err != nil || !response.OK || response.Message != "got run" {
		t.Fatalf("Send() = %#v, %v, want the child to answer the request", response, err)
	}
}

func TestJobsDoNotHoldTheClientPipes(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(helperEnv, "spawn-job")
	client, err := Start([]string{executable}, "LOG")
	if err != nil {
		t.Fatal(err)
	}
	// The supervisor exited; a job that inherited its pipes would keep the
	// connection open until the job ends.
	done := make(chan error, 1)
	go func() {
		_, err := client.Send(Request{Op: OpRun})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Send() succeeded after the supervisor exited")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("connection stayed open after the supervisor exited; a job inherited its pipes")
	}
}

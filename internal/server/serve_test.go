package server

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

type fakeOperations struct {
	mu         sync.Mutex
	cancelled  []Request
	runStart   chan struct{}
	runFinish  chan struct{}
	runEntered chan struct{}
	runGate    chan struct{}
	startErr   error
}

func (ops *fakeOperations) StartRun(_ Request, _ func()) (string, string, error) {
	return "", "", ops.startErr
}

func (ops *fakeOperations) Run(_ Request, progress func(Response)) (string, int, error) {
	if ops.runEntered != nil {
		close(ops.runEntered)
	}
	if ops.runGate != nil {
		<-ops.runGate
	}
	progress(Response{OK: true, Progress: true, Message: "started"})
	if ops.runStart != nil {
		close(ops.runStart)
	}
	if ops.runFinish != nil {
		<-ops.runFinish
	}
	return "finished", 3, nil
}

func roundTrip(t *testing.T, server *Server, request Request) Response {
	t.Helper()
	client, serverConn := net.Pipe()
	defer client.Close()
	go server.Handle(serverConn)
	if err := json.NewEncoder(client).Encode(request); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(client)
	for {
		var response Response
		if err := decoder.Decode(&response); err != nil {
			t.Fatal(err)
		}
		if !response.Progress {
			return response
		}
	}
}

func TestHandleDispatchesOperations(t *testing.T) {
	ops := &fakeOperations{}
	server := New(ops, nil)
	// Only the run's own client can reach a supervisor, so liveness and
	// shutdown go through the lease and signals instead of requests.
	for _, op := range []string{"ping", "shutdown"} {
		if response := roundTrip(t, server, Request{Op: op}); response.OK || response.Message != "unknown server operation: "+op {
			t.Fatalf("%s = %#v, want unknown operation", op, response)
		}
	}
	for _, op := range []string{"cancel", "suspend", "resume"} {
		if response := roundTrip(t, server, Request{Op: op}); response.OK || response.Message != "unknown server operation: "+op {
			t.Fatalf("%s = %#v, want unknown operation; job control does not go through the server", op, response)
		}
	}
	if response := roundTrip(t, server, Request{Op: "unknown"}); response.OK || response.Message != "unknown server operation: unknown" {
		t.Fatalf("unknown = %#v", response)
	}
	if response := roundTrip(t, server, Request{Op: "submit"}); response.OK || response.Message != "unknown server operation: submit" {
		t.Fatalf("submit = %#v, want unknown operation", response)
	}
	if server.Stopped() {
		t.Fatal("Handle stopped the server without a run")
	}
}

func TestServeStopsWithoutRun(t *testing.T) {
	for name, send := range map[string]func(net.Conn){
		"client gone": func(client net.Conn) { _ = client.Close() },
		"unknown op": func(client net.Conn) {
			_ = json.NewEncoder(client).Encode(Request{Op: "ping"})
			_, _ = io.Copy(io.Discard, client)
		},
		"start failed": func(client net.Conn) {
			_ = json.NewEncoder(client).Encode(Request{Op: OpRun, Async: true})
			_, _ = io.Copy(io.Discard, client)
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := New(&fakeOperations{startErr: errors.New("no commands")}, nil)
			client, serverConn := net.Pipe()
			defer client.Close()
			go send(client)
			server.Serve(serverConn)
			select {
			case <-server.Done():
			default:
				t.Fatal("server kept running after a request that started no run")
			}
		})
	}
}

func TestHandleSyncRunStreamsProgressAndEndsRun(t *testing.T) {
	server := New(&fakeOperations{}, nil)
	response := roundTrip(t, server, Request{Op: OpRun})
	if !response.OK || !response.Accepted || response.RunID != "" {
		t.Fatalf("run = %#v", response)
	}
	// The test operation emits no RunID; acceptance only confirms Begin/Run entered.
	waitIdle(server)
	if server.Busy() || !server.Stopped() {
		t.Fatal("server did not end its only run")
	}
}

func TestHandleRejectsSecondRun(t *testing.T) {
	ops := &fakeOperations{runStart: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(ops, nil)
	client, serverConn := net.Pipe()
	defer client.Close()
	go server.Handle(serverConn)
	if err := json.NewEncoder(client).Encode(Request{Op: OpRun}); err != nil {
		t.Fatal(err)
	}
	final := make(chan Response, 1)
	go func() {
		decoder := json.NewDecoder(client)
		for {
			var response Response
			if err := decoder.Decode(&response); err != nil || !response.Accepted {
				final <- response
				return
			}
		}
	}()
	<-ops.runStart
	// A supervisor belongs to the run that started it, so another run of
	// the project, even one sent to it directly, is rejected.
	if response := roundTrip(t, server, Request{Op: OpRun}); response.OK || response.Message != ErrRunAlreadyStarted.Error() {
		t.Fatalf("second run = %#v, want rejection", response)
	}
	close(ops.runFinish)
	if response := <-final; !response.OK || response.Message != "finished" {
		t.Fatalf("first run = %#v, want it to finish", response)
	}
}

func TestHandleAsyncRunStartFailureEndsRun(t *testing.T) {
	server := New(&fakeOperations{startErr: errors.New("no commands")}, nil)
	response := roundTrip(t, server, Request{Op: OpRun, Async: true})
	if response.OK || response.Message != "no commands" || server.Busy() {
		t.Fatalf("run = %#v, busy = %v", response, server.Busy())
	}
}

func TestHandleSyncStartupPipeCloseDoesNotEndRun(t *testing.T) {
	ops := &fakeOperations{runStart: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(ops, nil)
	client, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Handle(serverConn)
	}()
	if err := json.NewEncoder(client).Encode(Request{Op: OpRun, QueueName: "demo"}); err != nil {
		t.Fatal(err)
	}
	var accepted Response
	if err := json.NewDecoder(client).Decode(&accepted); err != nil || !accepted.Accepted {
		t.Fatalf("acceptance = %#v, %v", accepted, err)
	}
	<-ops.runStart
	_ = client.Close()
	select {
	case <-done:
		t.Fatal("startup pipe close ended the active run")
	case <-time.After(20 * time.Millisecond):
	}
	if !server.Busy() {
		t.Fatal("closing the startup pipe ended the run")
	}
	close(ops.runFinish)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not finish after its execution gate opened")
	}
	waitIdle(server)
}

func TestHandleSyncStartupPipeCloseDoesNotApplyDisconnectPolicy(t *testing.T) {
	ops := &fakeOperations{runStart: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(ops, nil)
	client, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Handle(serverConn)
	}()
	if err := json.NewEncoder(client).Encode(Request{Op: OpRun, QueueName: "demo", DisconnectAction: DisconnectActionCancel}); err != nil {
		t.Fatal(err)
	}
	var accepted Response
	if err := json.NewDecoder(client).Decode(&accepted); err != nil || !accepted.Accepted {
		t.Fatalf("acceptance = %#v, %v", accepted, err)
	}
	<-ops.runStart
	_ = client.Close()
	if !server.Busy() {
		t.Fatal("closing the startup pipe ended the run")
	}
	close(ops.runFinish)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not finish after its execution gate opened")
	}
}

func TestHandleSyncPipeCloseDuringStartupDoesNotControlRun(t *testing.T) {
	ops := &fakeOperations{runEntered: make(chan struct{}), runGate: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(ops, nil)
	client, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Handle(serverConn)
	}()
	if err := json.NewEncoder(client).Encode(Request{Op: OpRun, QueueName: "demo"}); err != nil {
		t.Fatal(err)
	}
	<-ops.runEntered
	_ = client.Close()
	close(ops.runGate)
	select {
	case <-done:
		t.Fatal("startup pipe close ended the run before execution finished")
	case <-time.After(20 * time.Millisecond):
	}
	if !server.Busy() {
		t.Fatal("startup handoff altered run state")
	}
	close(ops.runFinish)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not finish")
	}
	waitIdle(server)
}

func TestHandleSyncRunFinishesAfterStartupPipeCloses(t *testing.T) {
	ops := &fakeOperations{runStart: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(ops, nil)
	client, serverConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Handle(serverConn)
	}()
	if err := json.NewEncoder(client).Encode(Request{Op: OpRun}); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(client)
	var progress Response
	if err := decoder.Decode(&progress); err != nil {
		t.Fatal(err)
	}
	<-ops.runStart
	_ = client.Close()
	if !server.Busy() {
		t.Fatal("closing the startup connection ended the run")
	}
	close(ops.runFinish)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not complete after startup connection closed")
	}
	waitIdle(server)
	if server.Busy() || !server.Stopped() {
		t.Fatalf("busy = %v, stopped = %v", server.Busy(), server.Stopped())
	}
}

func TestBeginAndEndRunTrackActiveRuns(t *testing.T) {
	server := New(&fakeOperations{}, nil)
	if server.Busy() {
		t.Fatal("server should be idle before any run begins")
	}
	server.BeginRun()
	server.BeginRun()
	server.EndRun()
	if !server.Busy() || server.Stopped() {
		t.Fatal("server should stay busy while a run is active")
	}
	server.EndRun()
	if server.Busy() || !server.Stopped() {
		t.Fatal("server should stop after its last run ends")
	}
	select {
	case <-server.Done():
	default:
		t.Fatal("Done is open after the server stopped")
	}
}

func TestAcquireHoldsLease(t *testing.T) {
	dir := t.TempDir()
	if _, running := Running(dir); running {
		t.Fatal("Running() = true before any supervisor")
	}
	if _, err := os.Stat(LockPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("Running() created the lease file: %v", err)
	}
	release, err := Acquire(dir, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if pid, running := Running(dir); !running || pid != os.Getpid() {
		t.Fatalf("Running() = %d, %v, want this process", pid, running)
	}
	if _, err := Acquire(dir, 0o600); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Acquire() error = %v, want ErrAlreadyRunning", err)
	}
	release()
	if _, err := os.Stat(PIDPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("pid file after release: %v", err)
	}
	if _, running := Running(dir); running {
		t.Fatal("Running() = true after release")
	}
	release, err = Acquire(dir, 0o600)
	if err != nil {
		t.Fatalf("Acquire() after release: %v", err)
	}
	release()
}

func TestAcquireWaitsOutRunningProbe(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(LockPath(dir), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// A probe's shared lock is momentary; hold one briefly as Running does.
	probe, err := os.Open(LockPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(probe.Fd()), syscall.LOCK_SH); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(3 * leaseRetryDelay)
		_ = probe.Close()
	}()
	release, err := Acquire(dir, 0o600)
	if err != nil {
		t.Fatalf("Acquire() during a probe: %v", err)
	}
	release()
}

// fakeSupervisor serves one connection like a supervisor: Ready, then one
// request, which it answers according to the request's RunName, and returns
// what it read after the request.
func fakeSupervisor(conn net.Conn) <-chan []byte {
	received := make(chan []byte, 1)
	go func() {
		defer conn.Close()
		if Ready(conn, nil) != nil {
			return
		}
		decoder := json.NewDecoder(conn)
		var request Request
		if decoder.Decode(&request) != nil {
			return
		}
		encoder := json.NewEncoder(conn)
		_ = encoder.Encode(Response{OK: true, Accepted: true, RunID: "run-1"})
		if request.RunName == "finish" {
			_ = encoder.Encode(Response{OK: true, Message: "done", ExitCode: 2})
			return
		}
		reader := io.MultiReader(decoder.Buffered(), conn)
		var buffer [1]byte
		for {
			n, _ := reader.Read(buffer[:])
			if n == 0 || buffer[0] != '\n' {
				received <- buffer[:n]
				return
			}
		}
	}()
	return received
}

func connectFake(t *testing.T) (*Client, <-chan []byte) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	received := fakeSupervisor(serverConn)
	client, err := Connect(clientConn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if client.PID != os.Getpid() {
		t.Fatalf("PID = %d, want the supervisor's", client.PID)
	}
	return client, received
}

func TestAcceptRunUsesStartupHandshake(t *testing.T) {
	client, received := connectFake(t)
	response, err := client.AcceptRun(Request{Op: OpRun})
	if err != nil || !response.OK || !response.Accepted || response.RunID != "run-1" {
		t.Fatalf("accept = %#v, %v, want accepted run-1", response, err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if got := <-received; len(got) != 0 {
		t.Fatalf("startup pipe carried a steady-state control byte: %v", got)
	}
}

func waitIdle(server *Server) {
	deadline := time.Now().Add(2 * time.Second)
	for server.Busy() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

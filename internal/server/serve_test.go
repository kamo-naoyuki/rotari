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
	mu        sync.Mutex
	cancelled []Request
	detached  []Request
	runStart  chan struct{}
	runFinish chan struct{}
	startErr  error
}

func (ops *fakeOperations) StartRun(_ Request, _ func()) (string, string, error) {
	return "", "", ops.startErr
}

func (ops *fakeOperations) Run(_ Request, progress func(Response)) (string, int, error) {
	progress(Response{OK: true, Progress: true, Message: "started"})
	if ops.runStart != nil {
		close(ops.runStart)
	}
	if ops.runFinish != nil {
		<-ops.runFinish
	}
	return "finished", 3, nil
}

func (ops *fakeOperations) CancelRun(request Request) {
	ops.mu.Lock()
	ops.cancelled = append(ops.cancelled, request)
	ops.mu.Unlock()
	if ops.runFinish != nil {
		close(ops.runFinish)
	}
}

func (ops *fakeOperations) DetachRun(request Request) {
	ops.mu.Lock()
	ops.detached = append(ops.detached, request)
	ops.mu.Unlock()
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
	if !response.OK || response.Message != "finished" || response.ExitCode != 3 {
		t.Fatalf("run = %#v", response)
	}
	// The run ends after its final response is written.
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
			if err := decoder.Decode(&response); err != nil || !response.Progress {
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

func TestHandleSyncRunDisconnectDetachesByDefault(t *testing.T) {
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
	var progress Response
	if err := json.NewDecoder(client).Decode(&progress); err != nil || !progress.Progress {
		t.Fatalf("progress = %#v, %v", progress, err)
	}
	<-ops.runStart
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect did not end the request")
	}
	if len(ops.cancelled) != 0 {
		t.Fatalf("disconnect cancelled the run: %#v", ops.cancelled)
	}
	if len(ops.detached) != 1 {
		t.Fatalf("detached = %#v, want the disconnected run detached", ops.detached)
	}
	if !server.Busy() {
		t.Fatal("detached run is no longer active")
	}
	close(ops.runFinish)
	waitIdle(server)
}

func TestHandleSyncRunDisconnectCanCancel(t *testing.T) {
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
	var progress Response
	if err := json.NewDecoder(client).Decode(&progress); err != nil || !progress.Progress {
		t.Fatalf("progress = %#v, %v", progress, err)
	}
	<-ops.runStart
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run was not cancelled after disconnect")
	}
	if len(ops.cancelled) != 1 || ops.cancelled[0].QueueName != "demo" {
		t.Fatalf("cancelled = %#v, want the disconnected run", ops.cancelled)
	}
	if server.Busy() {
		t.Fatal("cancelled run is still active")
	}
}

func TestHandleSyncRunDetachEndsRunAfterCompletion(t *testing.T) {
	ops := &fakeOperations{runStart: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(ops, nil)
	client, serverConn := net.Pipe()
	defer client.Close()
	go server.Handle(serverConn)
	if err := json.NewEncoder(client).Encode(Request{Op: OpRun}); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(client)
	var progress Response
	if err := decoder.Decode(&progress); err != nil {
		t.Fatal(err)
	}
	<-ops.runStart
	if _, err := client.Write([]byte{DetachControl}); err != nil {
		t.Fatal(err)
	}
	var final Response
	if err := decoder.Decode(&final); err != nil || !final.OK || final.Message != DetachedMessage {
		t.Fatalf("final = %#v, %v, want detached", final, err)
	}
	if !server.Busy() {
		t.Fatal("detached run ended before it completed")
	}
	close(ops.runFinish)
	waitIdle(server)
	if server.Busy() || !server.Stopped() || len(ops.cancelled) != 0 {
		t.Fatalf("busy = %v, stopped = %v, cancelled = %#v", server.Busy(), server.Stopped(), ops.cancelled)
	}
	if len(ops.detached) != 1 {
		t.Fatalf("detached = %#v, want one detach notification", ops.detached)
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
		_ = encoder.Encode(Response{Progress: true, Message: "started"})
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

func TestStreamRunOutcomes(t *testing.T) {
	var progress []string
	collect := func(response Response) { progress = append(progress, response.Message) }
	client, _ := connectFake(t)
	response, outcome, err := client.StreamRun(Request{Op: OpRun, RunName: "finish"}, nil, nil, collect)
	if err != nil || outcome != RunFinished || response.Message != "done" || response.ExitCode != 2 || len(progress) != 1 {
		t.Fatalf("finish = %#v, %v, %v, progress %#v", response, outcome, err, progress)
	}

	client, received := connectFake(t)
	detach := make(chan struct{}, 1)
	detach <- struct{}{}
	response, outcome, err = client.StreamRun(Request{Op: OpRun}, detach, nil, collect)
	if err != nil || outcome != RunDetached || !response.OK {
		t.Fatalf("detach = %#v, %v, %v", response, outcome, err)
	}
	if got := <-received; len(got) != 1 || got[0] != DetachControl {
		t.Fatalf("server received %v, want detach control", got)
	}

	client, received = connectFake(t)
	interrupt := make(chan os.Signal, 1)
	interrupt <- os.Interrupt
	response, outcome, err = client.StreamRun(Request{Op: OpRun}, nil, interrupt, collect)
	if err != nil || outcome != RunInterrupted || response.ExitCode != 130 {
		t.Fatalf("interrupt = %#v, %v, %v", response, outcome, err)
	}
	if got := <-received; len(got) != 1 || got[0] != CancelControl {
		t.Fatalf("server received %v on interrupt, want cancel control", got)
	}
}

func waitIdle(server *Server) {
	deadline := time.Now().Add(2 * time.Second)
	for server.Busy() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

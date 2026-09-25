package server

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

type fakeOperations struct {
	mu        sync.Mutex
	submitted []Request
	cancelled []Request
	runStart  chan struct{}
	runFinish chan struct{}
	startErr  error
}

func (ops *fakeOperations) Submit(request Request) (string, error) {
	ops.mu.Lock()
	defer ops.mu.Unlock()
	ops.submitted = append(ops.submitted, request)
	return "added", nil
}

func (ops *fakeOperations) Cancel(Request) (string, error) { return "", errors.New("not running") }

func (ops *fakeOperations) Control(request Request) (string, error) {
	return request.Op + " requested", nil
}

func (ops *fakeOperations) StartRun(_ Request, _ func()) (string, error) {
	return "", ops.startErr
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
	server := New(nil, ops, nil)
	if response := roundTrip(t, server, Request{Op: OpPing}); !response.OK || response.Protocol != ProtocolVersion || response.PID != os.Getpid() {
		t.Fatalf("ping = %#v", response)
	}
	if response := roundTrip(t, server, Request{Op: OpSubmit, QueueName: "demo"}); !response.OK || response.Message != "added" || len(ops.submitted) != 1 {
		t.Fatalf("submit = %#v, submitted = %#v", response, ops.submitted)
	}
	if response := roundTrip(t, server, Request{Op: OpCancel}); response.OK || response.Message != "not running" {
		t.Fatalf("cancel = %#v, want operation error", response)
	}
	if response := roundTrip(t, server, Request{Op: OpSuspend}); !response.OK || response.Message != "suspend requested" {
		t.Fatalf("suspend = %#v", response)
	}
	if response := roundTrip(t, server, Request{Op: "unknown"}); response.OK || response.Message != "unknown server operation: unknown" {
		t.Fatalf("unknown = %#v", response)
	}
	if response := roundTrip(t, server, Request{Op: OpCopy}); response.OK || response.Message != "unsupported server operation: copy" {
		t.Fatalf("copy = %#v", response)
	}
	if server.Stopped() {
		t.Fatal("server stopped without a run or shutdown")
	}
	if response := roundTrip(t, server, Request{Op: OpShutdown}); !response.OK || !server.Stopped() {
		t.Fatalf("shutdown = %#v, stopped = %v", response, server.Stopped())
	}
}

func TestHandleSyncRunStreamsProgressAndEndsRun(t *testing.T) {
	server := New(nil, &fakeOperations{}, nil)
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

func TestHandleAsyncRunStartFailureEndsRun(t *testing.T) {
	server := New(nil, &fakeOperations{startErr: errors.New("no commands")}, nil)
	response := roundTrip(t, server, Request{Op: OpRun, Async: true})
	if response.OK || response.Message != "no commands" || server.Busy() {
		t.Fatalf("run = %#v, busy = %v", response, server.Busy())
	}
}

func TestHandleSyncRunDisconnectCancelsRun(t *testing.T) {
	ops := &fakeOperations{runStart: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(nil, ops, nil)
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
	if len(ops.cancelled) != 1 || ops.cancelled[0].QueueName != "demo" {
		t.Fatalf("cancelled = %#v, want the disconnected run", ops.cancelled)
	}
	if server.Busy() {
		t.Fatal("cancelled run is still active")
	}
}

func TestHandleSyncRunDetachEndsRunAfterCompletion(t *testing.T) {
	ops := &fakeOperations{runStart: make(chan struct{}), runFinish: make(chan struct{})}
	server := New(nil, ops, nil)
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
}

func TestBeginAndEndRunTrackActiveRuns(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := New(listener, &fakeOperations{}, nil)
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
	if _, err := listener.Accept(); err == nil {
		t.Fatal("stopped server left its listener open")
	}
}

func TestListenHoldsLease(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-listen-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	listener, release, err := Listen(baseDir, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(SocketPath(baseDir))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket = %v, %v, want owner-only", info, err)
	}
	if _, _, err := Listen(baseDir, 0o600); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Listen() error = %v, want ErrAlreadyRunning", err)
	}
	_ = listener
	release()
	if _, err := os.Stat(PIDPath(baseDir)); !os.IsNotExist(err) {
		t.Fatalf("pid file after release: %v", err)
	}
	listener, release, err = Listen(baseDir, 0o600)
	if err != nil {
		t.Fatalf("Listen() after release: %v", err)
	}
	_ = listener
	release()
}

func TestStreamRunOutcomes(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-stream-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	listener, err := net.Listen("unix", SocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan []byte, 3)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
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
			}(conn)
		}
	}()

	var progress []string
	collect := func(response Response) { progress = append(progress, response.Message) }
	response, outcome, err := StreamRun(baseDir, Request{Op: OpRun, RunName: "finish"}, nil, nil, collect)
	if err != nil || outcome != RunFinished || response.Message != "done" || response.ExitCode != 2 || len(progress) != 1 {
		t.Fatalf("finish = %#v, %v, %v, progress %#v", response, outcome, err, progress)
	}

	detach := make(chan struct{}, 1)
	detach <- struct{}{}
	response, outcome, err = StreamRun(baseDir, Request{Op: OpRun}, detach, nil, collect)
	if err != nil || outcome != RunDetached || !response.OK {
		t.Fatalf("detach = %#v, %v, %v", response, outcome, err)
	}
	if got := <-received; len(got) != 1 || got[0] != DetachControl {
		t.Fatalf("server received %v, want detach control", got)
	}

	interrupt := make(chan os.Signal, 1)
	interrupt <- os.Interrupt
	response, outcome, err = StreamRun(baseDir, Request{Op: OpRun}, nil, interrupt, collect)
	if err != nil || outcome != RunInterrupted || response.ExitCode != 130 {
		t.Fatalf("interrupt = %#v, %v, %v", response, outcome, err)
	}
	if got := <-received; len(got) != 0 {
		t.Fatalf("server received %v on interrupt, want plain disconnect", got)
	}
}

func waitIdle(server *Server) {
	deadline := time.Now().Add(2 * time.Second)
	for server.Busy() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

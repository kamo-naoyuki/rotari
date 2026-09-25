package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// ProtocolVersion is reported by ping. A client replaces a running server
// that reports a different version.
const ProtocolVersion = 3

// DetachControl is the byte a synchronous run client sends before
// disconnecting to leave the run going in the background.
const DetachControl byte = 0x04

// DetachedMessage reports a synchronous run that was detached from its client.
const DetachedMessage = "Run detached; it continues in the background."

// ErrAlreadyRunning reports that another server holds the base directory's
// lease.
var ErrAlreadyRunning = errors.New("server is already running")

// Operations performs the project work behind server requests. The server
// owns the transport, active-run bookkeeping, and its own lifetime.
type Operations interface {
	Submit(request Request) (string, error)
	Cancel(request Request) (string, error)
	// Control suspends or resumes jobs, as named by request.Op.
	Control(request Request) (string, error)
	// StartRun starts an asynchronous run and calls onDone once it ends.
	StartRun(request Request, onDone func()) (string, error)
	// Run executes a synchronous run, reporting progress as it goes.
	Run(request Request, progress func(Response)) (message string, exitCode int, err error)
	// CancelRun cancels a synchronous run whose client disconnected without
	// detaching.
	CancelRun(request Request)
}

// Server serves one base directory's requests and stops once it is idle or
// its last run ends.
type Server struct {
	ops        Operations
	logger     *Logger
	listener   net.Listener
	stopped    chan struct{}
	stopOnce   sync.Once
	accessMu   sync.Mutex
	lastAccess time.Time
	activeRuns int
}

// New returns a server that accepts from listener. A nil listener is allowed
// when connections are passed to Handle directly.
func New(listener net.Listener, ops Operations, logger *Logger) *Server {
	return &Server{ops: ops, logger: logger, listener: listener, stopped: make(chan struct{}), lastAccess: time.Now()}
}

// Listen takes the base directory's server lease, listens on its owner-only
// socket, and records the server PID. The returned function releases them.
func Listen(baseDir string, fileMode os.FileMode) (net.Listener, func(), error) {
	lease, err := os.OpenFile(LockPath(baseDir), os.O_CREATE|os.O_RDWR, fileMode)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open server lock: %w", err)
	}
	if err := syscall.Flock(int(lease.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lease.Close()
		return nil, nil, ErrAlreadyRunning
	}
	socketPath := SocketPath(baseDir)
	if err := prepareSocketDir(socketPath); err != nil {
		_ = lease.Close()
		return nil, nil, err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_ = lease.Close()
		return nil, nil, fmt.Errorf("failed to listen on server socket %s: %w", socketPath, err)
	}
	release := func() {
		_ = listener.Close()
		_ = os.Remove(PIDPath(baseDir))
		_ = os.Remove(socketPath)
		_ = lease.Close()
	}
	// net.Listen applies the process umask; enforce owner-only access explicitly
	// regardless of --shared-state, since this is the control-plane socket.
	if err := os.Chmod(socketPath, 0o600); err != nil {
		release()
		return nil, nil, fmt.Errorf("failed to set server socket permissions: %w", err)
	}
	if err := os.WriteFile(PIDPath(baseDir), []byte(strconv.Itoa(os.Getpid())+"\n"), fileMode); err != nil {
		release()
		return nil, nil, fmt.Errorf("failed to write server pid: %w", err)
	}
	return listener, release, nil
}

// Serve accepts connections from verified peers until the server stops.
func (server *Server) Serve() {
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			if server.Stopped() {
				return
			}
			continue
		}
		if err := VerifyPeerCredential(conn); err != nil {
			_ = conn.Close()
			continue
		}
		go server.Handle(conn)
	}
}

// WatchIdle stops the server once it has no active runs and no requests for
// timeout. heartbeat, if set, runs on every check.
func (server *Server) WatchIdle(interval, timeout time.Duration, heartbeat func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if heartbeat != nil {
			heartbeat()
		}
		server.accessMu.Lock()
		idle := time.Since(server.lastAccess) >= timeout && server.activeRuns == 0
		server.accessMu.Unlock()
		if idle {
			server.Stop()
			return
		}
		if server.Stopped() {
			return
		}
	}
}

// Stopped reports whether Stop has been called.
func (server *Server) Stopped() bool {
	select {
	case <-server.stopped:
		return true
	default:
		return false
	}
}

// Stop stops accepting connections.
func (server *Server) Stop() {
	server.stopOnce.Do(func() {
		server.logger.Writef("stopping")
		close(server.stopped)
		if server.listener != nil {
			_ = server.listener.Close()
		}
	})
}

// Busy reports whether any run is active.
func (server *Server) Busy() bool {
	server.accessMu.Lock()
	defer server.accessMu.Unlock()
	return server.activeRuns > 0
}

// BeginRun records a started run.
func (server *Server) BeginRun() {
	server.accessMu.Lock()
	server.activeRuns++
	server.lastAccess = time.Now()
	server.accessMu.Unlock()
}

// EndRun records a finished run and stops the server after its last run.
func (server *Server) EndRun() {
	server.accessMu.Lock()
	server.activeRuns--
	server.lastAccess = time.Now()
	shouldStop := server.activeRuns == 0
	server.accessMu.Unlock()
	if shouldStop {
		server.Stop()
	}
}

func (server *Server) touch() {
	server.accessMu.Lock()
	server.lastAccess = time.Now()
	server.accessMu.Unlock()
}

// Handle serves one connection's request.
func (server *Server) Handle(conn net.Conn) {
	defer conn.Close()
	server.touch()
	var request Request
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		server.logger.Writef("request decode failed: %v", err)
		_ = json.NewEncoder(conn).Encode(Response{Message: err.Error()})
		return
	}
	server.logger.Writef("request op=%s", request.Op)
	encoder := json.NewEncoder(conn)
	if !IsKnownOperation(request.Op) {
		_ = encoder.Encode(Response{Message: "unknown server operation: " + request.Op})
		return
	}
	var response Response
	switch request.Op {
	case OpPing:
		response = Response{OK: true, PID: os.Getpid(), Protocol: ProtocolVersion}
	case OpSubmit:
		response = messageResponse(server.ops.Submit(request))
	case OpCancel:
		response = messageResponse(server.ops.Cancel(request))
	case OpSuspend, OpResume:
		response = messageResponse(server.ops.Control(request))
	case OpRun:
		server.BeginRun()
		if request.Async {
			message, err := server.ops.StartRun(request, server.EndRun)
			if err != nil {
				server.EndRun()
			}
			response = messageResponse(message, err)
		} else {
			message, exitCode, err, detached := server.runAttached(conn, request, func(progress Response) {
				_ = encoder.Encode(progress)
			})
			if !detached {
				defer server.EndRun()
			}
			response = messageResponse(message, err)
			response.ExitCode = exitCode
		}
	case OpShutdown:
		response = Response{OK: true, Message: "server stopped"}
		server.Stop()
	default:
		response.Message = "unsupported server operation: " + request.Op
	}
	_ = encoder.Encode(response)
}

func messageResponse(message string, err error) Response {
	if err != nil {
		return Response{Message: err.Error()}
	}
	return Response{OK: true, Message: message}
}

// runAttached runs a synchronous run while watching its client. A detach
// leaves the run going and moves EndRun to its completion; any other
// disconnect cancels the run and waits for it to finish.
func (server *Server) runAttached(conn net.Conn, request Request, progress func(Response)) (string, int, error, bool) {
	type result struct {
		message  string
		exitCode int
		err      error
	}
	done := make(chan result, 1)
	go func() {
		message, exitCode, err := server.ops.Run(request, progress)
		done <- result{message: message, exitCode: exitCode, err: err}
	}()
	disconnected := make(chan bool, 1)
	go func() {
		var buffer [1]byte
		n, err := conn.Read(buffer[:])
		if n > 0 && buffer[0] == DetachControl {
			disconnected <- true
			return
		}
		if err != nil && (err == io.EOF || !errors.Is(err, os.ErrDeadlineExceeded)) {
			disconnected <- false
		}
	}()
	select {
	case result := <-done:
		return result.message, result.exitCode, result.err, false
	case detached := <-disconnected:
		if detached {
			go func() {
				<-done
				server.EndRun()
			}()
			return DetachedMessage, 0, nil, true
		}
		server.ops.CancelRun(request)
		result := <-done
		return result.message, result.exitCode, result.err, false
	}
}

package server

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
	"time"
)

// ProtocolVersion is reported by Ready. A `run` client talks only to the
// supervisor it started from its own executable, so the version is for
// diagnosis rather than negotiation.
const ProtocolVersion = 12

const (
	DisconnectActionDetach = "detach"
	DisconnectActionCancel = "cancel"
)

// DetachedMessage reports a synchronous run that was detached from its client.
const DetachedMessage = "Run detached; it continues in the background."

// ErrAlreadyRunning reports that another supervisor holds the project's
// lease.
var ErrAlreadyRunning = errors.New("server is already running")

// ErrRunAlreadyStarted rejects a second run request to one supervisor.
var ErrRunAlreadyStarted = errors.New("this supervisor has already started a run")

// Operations performs the project work behind server requests. The server
// owns the transport, active-run bookkeeping, and its own lifetime.
type Operations interface {
	// StartRun starts an asynchronous run and calls onDone once it ends. It
	// returns the new run's ID and the message for the client.
	StartRun(request Request, onDone func()) (runID, message string, err error)
	// Run executes a synchronous run, reporting progress as it goes.
	Run(request Request, progress func(Response)) (message string, exitCode int, err error)
}

// Server is the supervisor of one run of one project. It accepts a single run
// request and stops once that run ends, or when it is idle before one
// arrives. A new run starts a new supervisor, so the run's jobs inherit the
// working directory and environment of the command that started it.
type Server struct {
	ops        Operations
	logger     *Logger
	stopped    chan struct{}
	stopOnce   sync.Once
	accessMu   sync.Mutex
	lastAccess time.Time
	activeRuns int
	// runClaimed is set by the first run request.
	runClaimed bool
}

// New returns a server whose connection is passed to Serve or Handle.
func New(ops Operations, logger *Logger) *Server {
	return &Server{ops: ops, logger: logger, stopped: make(chan struct{}), lastAccess: time.Now()}
}

// Ready tells the run client on conn that the supervisor holds its project's
// lease and waits for the run request, or why it could not start. It is the
// first message of every connection.
func Ready(conn io.Writer, err error) error {
	response := Response{OK: true, PID: os.Getpid(), Protocol: ProtocolVersion}
	if err != nil {
		response = Response{Message: err.Error()}
	}
	return json.NewEncoder(conn).Encode(response)
}

// Serve serves the run request on conn, the supervisor's only connection,
// and stops the server when the request leaves no run active.
func (server *Server) Serve(conn io.ReadWriteCloser) {
	server.Handle(conn)
	if !server.Busy() {
		server.Stop()
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

// Done is closed when the server stops.
func (server *Server) Done() <-chan struct{} {
	return server.stopped
}

// Stop stops the server; the supervisor process exits once it has.
func (server *Server) Stop() {
	server.stopOnce.Do(func() {
		server.logger.Writef("stopping")
		close(server.stopped)
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

// claimRun reports whether this is the server's first run request.
func (server *Server) claimRun() bool {
	server.accessMu.Lock()
	defer server.accessMu.Unlock()
	if server.runClaimed {
		return false
	}
	server.runClaimed = true
	return true
}

// EndRun records a finished run and stops the server after its last run.
func (server *Server) EndRun() {
	server.accessMu.Lock()
	defer server.accessMu.Unlock()
	server.activeRuns--
	server.lastAccess = time.Now()
	// Stop under the lock so Busy never reports idle before Stopped is true.
	if server.activeRuns == 0 {
		server.Stop()
	}
}

func (server *Server) touch() {
	server.accessMu.Lock()
	server.lastAccess = time.Now()
	server.accessMu.Unlock()
}

// Handle serves one connection's request.
func (server *Server) Handle(conn io.ReadWriteCloser) {
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
	var responseMu sync.Mutex
	encode := func(response Response) {
		// Detached runs can still emit progress while Handle sends its final
		// response. Neither the encoder nor the connection allows interleaving.
		responseMu.Lock()
		defer responseMu.Unlock()
		_ = encoder.Encode(response)
	}
	if !IsKnownOperation(request.Op) {
		encode(Response{Message: "unknown server operation: " + request.Op})
		return
	}
	var response Response
	switch request.Op {
	case OpRun:
		if !server.claimRun() {
			response.Message = ErrRunAlreadyStarted.Error()
			break
		}
		server.BeginRun()
		if request.Async {
			runID, message, err := server.ops.StartRun(request, server.EndRun)
			if err != nil {
				server.EndRun()
			}
			response = messageResponse(message, err)
			response.RunID = runID
		} else {
			defer server.EndRun()
			message, exitCode, err := server.runAttached(request, encode)
			response = messageResponse(message, err)
			response.ExitCode = exitCode
		}
	default:
		response.Message = "unsupported server operation: " + request.Op
	}
	encode(response)
}

func messageResponse(message string, err error) Response {
	if err != nil {
		return Response{Message: err.Error()}
	}
	return Response{OK: true, Message: message}
}

// runAttached executes the run while using the connection only for startup
// acceptance. Progress and client control are handled by the filesystem
// follower after this handoff.
func (server *Server) runAttached(request Request, progress func(Response)) (string, int, error) {
	type result struct {
		message  string
		exitCode int
		err      error
	}
	done := make(chan result, 1)
	accepted := make(chan struct{})
	var startOnce sync.Once
	go func() {
		message, exitCode, err := server.ops.Run(request, func(response Response) {
			startOnce.Do(func() {
				progress(Response{OK: true, Accepted: true, RunID: response.RunID, Notice: response.Notice})
				close(accepted)
			})
		})
		done <- result{message: message, exitCode: exitCode, err: err}
	}()
	select {
	case <-accepted:
		result := <-done
		return result.message, result.exitCode, result.err
	case result := <-done:
		return result.message, result.exitCode, result.err
	}
}

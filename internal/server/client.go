package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// The supervisor reads requests from requestFD and writes responses to
// responseFD, the pipes Start passes as its first extra files. They are not
// stdin and stdout, so writing after the client has gone fails with EPIPE
// instead of killing the supervisor with SIGPIPE.
const (
	requestFD  = 3
	responseFD = 4
)

// readyTimeout bounds how long a client waits for its supervisor's Ready.
const readyTimeout = 5 * time.Second

// errNotReady reports a connection that closed before Ready arrived.
var errNotReady = errors.New("connection closed before the supervisor was ready")

// pipeConn joins the two pipes of a connection.
type pipeConn struct {
	io.Reader
	io.Writer
	closers []io.Closer
}

func (conn pipeConn) Close() error {
	var err error
	for _, closer := range conn.closers {
		err = errors.Join(err, closer.Close())
	}
	return err
}

// Pipe returns the two ends of a connection within one process, for a
// supervisor served in the same process as its client.
func Pipe() (client, server io.ReadWriteCloser, err error) {
	requestRead, requestWrite, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	responseRead, responseWrite, err := os.Pipe()
	if err != nil {
		_ = requestRead.Close()
		_ = requestWrite.Close()
		return nil, nil, err
	}
	client = pipeConn{Reader: responseRead, Writer: requestWrite, closers: []io.Closer{responseRead, requestWrite}}
	server = pipeConn{Reader: requestRead, Writer: responseWrite, closers: []io.Closer{requestRead, responseWrite}}
	return client, server, nil
}

// InheritedConn returns the connection to the run client that started this
// supervisor with Start. The descriptors are closed on exec so that jobs do
// not hold the client's pipes open.
func InheritedConn() (io.ReadWriteCloser, error) {
	for _, fd := range []int{requestFD, responseFD} {
		var stat syscall.Stat_t
		if err := syscall.Fstat(fd, &stat); err != nil {
			return nil, fmt.Errorf("supervisor must be started by run: descriptor %d: %w", fd, err)
		}
		syscall.CloseOnExec(fd)
	}
	request := os.NewFile(requestFD, "rotari-request")
	response := os.NewFile(responseFD, "rotari-response")
	return pipeConn{Reader: request, Writer: response, closers: []io.Closer{request, response}}, nil
}

// Client is a run client's connection to the supervisor it started. It
// carries one run request.
type Client struct {
	conn    io.ReadWriteCloser
	decoder *json.Decoder
	// PID is the supervisor's PID, from its Ready message.
	PID int
}

// Connect waits for the Ready message on conn. It fails with
// ErrAlreadyRunning when another supervisor holds the project's lease, and
// closes conn on any failure.
func Connect(conn io.ReadWriteCloser, timeout time.Duration) (*Client, error) {
	client := &Client{conn: conn, decoder: json.NewDecoder(conn)}
	type result struct {
		response Response
		err      error
	}
	ready := make(chan result, 1)
	go func() {
		var response Response
		err := client.decoder.Decode(&response)
		ready <- result{response: response, err: err}
	}()
	var response Response
	select {
	case result := <-ready:
		if result.err != nil {
			_ = conn.Close()
			if errors.Is(result.err, io.EOF) || errors.Is(result.err, io.ErrUnexpectedEOF) {
				return nil, errNotReady
			}
			return nil, result.err
		}
		response = result.response
	case <-time.After(timeout):
		_ = conn.Close()
		return nil, os.ErrDeadlineExceeded
	}
	if !response.OK {
		_ = conn.Close()
		if response.Message == ErrAlreadyRunning.Error() {
			return nil, ErrAlreadyRunning
		}
		return nil, errors.New(response.Message)
	}
	client.PID = response.PID
	return client, nil
}

// Close closes the startup connection. After acceptance, closing this
// connection is normal and does not control or cancel the run.
func (client *Client) Close() error {
	return client.conn.Close()
}

// Send sends a request that is answered with one response, such as an
// asynchronous run, and closes the connection.
func (client *Client) Send(request Request) (Response, error) {
	defer client.conn.Close()
	if err := json.NewEncoder(client.conn).Encode(request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := client.decoder.Decode(&response); err != nil {
		return Response{}, err
	}
	return response, nil
}

// AcceptRun sends a synchronous run request and reads its startup acceptance
// (or rejection). Subsequent progress and control use the shared filesystem
// follower, not the startup pipe.
func (client *Client) AcceptRun(request Request) (Response, error) {
	if err := json.NewEncoder(client.conn).Encode(request); err != nil {
		return Response{}, err
	}
	var response Response
	if err := client.decoder.Decode(&response); err != nil {
		return Response{}, err
	}
	if response.OK && !response.Accepted {
		return Response{}, errors.New("server response did not accept a run")
	}
	return response, nil
}

// Start starts command as a supervisor and returns the connection to it once
// it holds its project's lease. The supervisor is detached from the caller's
// terminal session but inherits the caller's working directory and
// environment, which its run's jobs then inherit in turn. Start never reuses
// a running supervisor: it fails with ErrAlreadyRunning when another one
// holds the lease. logPath names the log that explains a failed start.
func Start(command []string, logPath string) (*Client, error) {
	child := exec.Command(command[0], command[1:]...)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to detach server stdio: %w", err)
	}
	defer devNull.Close()
	child.Stdin = devNull
	child.Stdout = devNull
	child.Stderr = devNull
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	clientConn, serverConn, err := Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create server pipes: %w", err)
	}
	server := serverConn.(pipeConn)
	child.ExtraFiles = []*os.File{server.Reader.(*os.File), server.Writer.(*os.File)}
	err = child.Start()
	// The child has its own copies; keeping these would hide its exit.
	_ = serverConn.Close()
	if err != nil {
		_ = clientConn.Close()
		return nil, fmt.Errorf("failed to start server: %w", err)
	}
	pid := child.Process.Pid
	go func() { _ = child.Wait() }()
	client, err := Connect(clientConn, readyTimeout)
	switch {
	case err == nil:
		return client, nil
	case errors.Is(err, ErrAlreadyRunning):
		return nil, err
	case errors.Is(err, errNotReady):
		return nil, fmt.Errorf("server exited before it became ready (pid=%d); see %s for the cause", pid, logPath)
	case errors.Is(err, os.ErrDeadlineExceeded):
		_ = child.Process.Kill()
		return nil, fmt.Errorf("server did not become ready (pid=%d); see %s for the cause", pid, logPath)
	default:
		return nil, fmt.Errorf("server failed to start (pid=%d): %w; see %s", pid, err, logPath)
	}
}

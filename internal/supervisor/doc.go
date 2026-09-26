// Package supervisor performs the project work behind the background
// server's requests for one base directory: adding jobs, cancelling,
// suspending, and resuming them, and starting synchronous and asynchronous
// runs.
//
// It implements server.Operations. internal/server owns the transport, the
// request protocol, and the server's lifetime; this package decides what each
// request does, using internal/queueops, internal/jobcontrol, and
// internal/projectrun. Messages it returns are plain text that the client
// colors; it must not parse flags or depend on terminal output.
package supervisor

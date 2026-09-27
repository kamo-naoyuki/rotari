// Package supervisor performs the project work behind the background
// server's requests for one base directory: starting synchronous and
// asynchronous runs, and cancelling a synchronous run whose client
// disconnected. `cancel`, `suspend`, and `resume` do not go through it; they
// call internal/jobcontrol from the calling process.
//
// It implements server.Operations. internal/server owns the transport, the
// request protocol, and the server's lifetime; this package decides what each
// request does, using internal/jobcontrol and internal/projectrun. Messages it returns are plain text that the client
// colors; it must not parse flags or depend on terminal output.
package supervisor

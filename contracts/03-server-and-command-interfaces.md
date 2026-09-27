# Server and command interfaces

This file covers the server control surface, read projections, client lifecycle,
and CLI presentation. Web-specific UI and asset behavior live in
[05-web-assets-and-static-export.md](05-web-assets-and-static-export.md).

Representative implementation and tests:

- [internal/server/serve.go](../internal/server/serve.go),
  [internal/server/client.go](../internal/server/client.go), and
  [internal/server/serve_test.go](../internal/server/serve_test.go) for the
  server lease, the pipe connection to the run client, request dispatch,
  active-run tracking, and the detach and disconnect protocol.
- [cmd/rotari/server.go](../cmd/rotari/server.go) and
  [cmd/rotari/server_test.go](../cmd/rotari/server_test.go) for server
  commands and the `Operations` that perform run work. The request handlers
  live in [internal/supervisor](../internal/supervisor/operations.go) and
  [cmd/rotari/run_command.go](../cmd/rotari/run_command.go). `cancel`,
  `suspend`, and `resume` do not use the server: the CLI
  ([cmd/rotari/job_control.go](../cmd/rotari/job_control.go)) and the Web UI
  call [internal/jobcontrol](../internal/jobcontrol/jobcontrol.go) in their
  own process.
- [cmd/rotari/show.go](../cmd/rotari/show.go) and
  [cmd/rotari/show_test.go](../cmd/rotari/show_test.go) for CLI projections.
- [cmd/rotari/wait.go](../cmd/rotari/wait.go) and
  [cmd/rotari/wait_test.go](../cmd/rotari/wait_test.go) for wait output and
  run selection.

## Server and read projections

- A server (the supervisor) serves one run of one project. `run` and `retry`
  start a new one for every run, as a child process that inherits their
  working directory and environment (RUN-3 in
  [02-run-lifecycle-and-execution.md](02-run-lifecycle-and-execution.md#run-lifecycle)),
  and never reuse a running one: a supervisor that finds the project's lease
  held reports it and exits, so `server.Start` fails, and a supervisor
  rejects a second run request. It stops when its run ends, when its client
  leaves without a run request, or after a minute without one. Its lease
  (`server.lock`) and PID file (`server.pid`) live in the project directory,
  so the runs of different projects have different supervisors. Durable
  behavior belongs in files, not memory. Covered by
  [internal/server/serve_test.go](../internal/server/serve_test.go) and
  [internal/server/client_test.go](../internal/server/client_test.go).
- The run client and its supervisor talk over two pipes the supervisor
  inherits as descriptors 3 and 4, not over a socket, so `run` needs no Unix
  socket and works in sandboxes that block them, whatever the length of the
  base directory. Only that client can reach the supervisor. The supervisor
  marks the descriptors close-on-exec, so jobs do not hold them, and its
  first message says whether it took the lease. Covered by
  [internal/server/client_test.go](../internal/server/client_test.go).
- A supervisor is running while its lease is locked. `server status`, `show`,
  and `server list` check the lock and read the PID file without contacting
  the supervisor; the check never creates the lease file, and a starting
  supervisor retries the lease briefly so a check does not make it fail.
  `server status` and `server shutdown` act on the supervisor of every
  project in the base directory that has one, and report `server is not
  running` when none does. `server shutdown` sends it `SIGTERM` and waits for
  the lease to be released. Shutting a supervisor down leaves its run
  interrupted with its jobs still running; `cancel` is how to stop a run.
  `server list` shows the supervisors registered in the master directory,
  one per project.
- A detached server discards stderr, so `runServer` writes a `start failed`
  event to `server.log` when it cannot start for any reason other than
  another server holding the lease, and sends the reason to its client;
  `server.Start` reports it with the log's path, as it does when the server
  exits or does not become ready.
- Every supervisor of a base directory writes lifecycle, request, and error
  events, each prefixed with its project, to `<basedir>/server.log`. Before an
  event would make the regular file exceed 1 MiB, it is truncated and the new
  event is written.
- The optional Python interface invokes the installed CLI with `subprocess` and
  must not implement queue or execution semantics itself.
- `wait --json` emits one `RunSummary` object per requested run, using NDJSON
  when multiple IDs are supplied.
- `wait` polls for the run's `summary.json`. While it is missing, `wait` also
  inspects the project state without removing a stale lock; when the run is no
  longer active (for example, interrupted because its supervisor exited), it
  prints how to inspect and recover the run and exits 1 instead of waiting
  forever. A malformed summary is treated as still being written.
- `show --json` emits one object with the resolved location, available run
  summary, and saved commands. JSON modes are additive; default CLI output
  remains human-facing.
- `show RUN` compares the current queue with that run's saved commands by job
  ID. Its changed count uses the same definition-field comparison as `diff`,
  including environment, stage, timeout, and retry settings; see
  [`cmd/rotari/show.go`](../cmd/rotari/show.go),
  [`internal/rundiff/rundiff.go`](../internal/rundiff/rundiff.go), and
  [`cmd/rotari/main_test.go`](../cmd/rotari/main_test.go).

## Client connection lifecycle

The server distinguishes client cancellation, detach, and run completion as
follows:

- A synchronous client disconnect, including Ctrl-C, requests cancellation and
  returns immediately with exit code 130. The server-side run continues in the
  background and then finalizes normally.
- Ctrl-D sends an explicit detach before disconnecting. The run is left
  uncancelled and completion cleanup moves to the background waiter.
- Ctrl-Z sends no rotari protocol message. The terminal suspends the client
  while the server-side run continues; a later EOF follows the normal
  disconnect path and requests cancellation.
- An async run executes inside its supervisor, like a sync run whose client
  detached at once: `run --async` returns after `=== Run started ===`, and the
  supervisor finishes the run in the background. Interrupted-run recovery is
  reserved for failures that bypass finalization, such as a killed
  supervisor.
- Completed sync and async runs decrement the active-run count immediately
  through `Server.BeginRun`/`Server.EndRun`. When it reaches zero, the server stops without
  waiting for the idle timeout.

## CLI presentation

- **CLI-1** `check --json` reports the same project state, run identifier,
  queue count, lock, and runnable result as the human-readable `check`
  output.
- **CLI-2** The human-readable `jobs` table keeps every column's visible start
  position aligned across rows, including when status values are colorized.
  ANSI escape sequences are presentation only and do not count toward a
  column's width. The implementation is in
  [`cmd/rotari/jobs.go`](../cmd/rotari/jobs.go), with the end-to-end check in
  [`conformance/03-interfaces/jobs_presentation_test.go`](../conformance/03-interfaces/jobs_presentation_test.go).

- CLI colors are semantic presentation, not machine-readable output. They are
  emitted only on TTY streams; redirected and piped output remains plain text.
- Red denotes errors and failed results; green denotes success; yellow denotes
  warnings, running/blocked state, retries, and recovery/cancellation; cyan
  denotes informational labels and suggested actions; white denotes values.
- Parsers and tests must rely on text, not ANSI sequences or color choice.
- `check --json` emits the same result as its text view. `queued` is a JSON
  number when known and `null` when an active or interrupted project's queue
  cannot be read; `run_id` is omitted when no run is associated with the state.
- `show` uses a lazy pager. `--no-pager` and non-TTY output go directly to
  stdout. On a TTY, output of at most 24 lines is direct; longer output uses
  `$PAGER`, defaulting to `less -R`. Pager failure falls back to stdout.

### Quiet output contract

- `--quiet` and `ROTARI_QUIET=true` are equivalent global defaults for all
  commands that support quiet output. Each command also accepts its own
  environment default (`ROTARI_ADD_QUIET`, `ROTARI_COPY_QUIET`,
  `ROTARI_CHANGE_QUIET`, `ROTARI_REMOVE_QUIET`, `ROTARI_RESET_QUIET`,
  `ROTARI_CHECK_QUIET` or `ROTARI_RUN_QUIET`), which overrides the global
  value. `retry` shares `ROTARI_RUN_QUIET`. Config files support
  root `quiet` and command-specific values such as `add.quiet` and `run.quiet`
  with the same precedence.
  The shared environment-variable mapping is defined in
  [`cmd/rotari/environment.go`](../cmd/rotari/environment.go) and
  [`cmd/rotari/cli_spec.go`](../cmd/rotari/cli_spec.go).
- For `run`, quiet is carried in the server request as `Request.Quiet`; the
  client suppresses successful progress and completion output. Job failures,
  startup errors, and other command errors remain visible, and synchronous
  `run` still returns a non-zero status when a job fails.
- The same success-silent/error-visible rule applies to queue-editing commands
  (`add`, `copy`, `change`, `remove`, `reset`, and `check`). The protocol field
  is defined in [`internal/server/protocol.go`](../internal/server/protocol.go),
  with client behavior covered by
  [`cmd/rotari/server_test.go`](../cmd/rotari/server_test.go).

# Server and command interfaces

This file covers the server control surface, read projections, client lifecycle,
and CLI presentation. Web-specific UI and asset behavior live in
[05-web-assets-and-static-export.md](05-web-assets-and-static-export.md).

Representative implementation and tests:

- [cmd/rotari/server.go](../../cmd/rotari/server.go) and
  [cmd/rotari/server_test.go](../../cmd/rotari/server_test.go) for server
  lifecycle and client requests.
- [cmd/rotari/show.go](../../cmd/rotari/show.go) and
  [cmd/rotari/show_test.go](../../cmd/rotari/show_test.go) for CLI projections.
- [cmd/rotari/wait.go](../../cmd/rotari/wait.go) and
  [cmd/rotari/wait_test.go](../../cmd/rotari/wait_test.go) for wait output and
  run selection.

## Server and read projections

- The server supervises one base directory and may stop when idle, so durable
  behavior belongs in files, not memory.
- The background server writes lifecycle, request, and error events to
  `<basedir>/server.log`. Before an event would make the regular file exceed
  1 MiB, it is truncated and the new event is written.
- The optional Python interface invokes the installed CLI with `subprocess` and
  must not implement queue or execution semantics itself.
- `wait --json` emits one `RunSummary` object per requested run, using NDJSON
  when multiple IDs are supplied.
- `show --json` emits one object with the resolved location, available run
  summary, and saved commands. JSON modes are additive; default CLI output
  remains human-facing.

## Client connection lifecycle

The server distinguishes client cancellation, detach, and worker completion as
follows:

- A synchronous client disconnect, including Ctrl-C, requests cancellation and
  returns immediately with exit code 130. The server-side run continues in the
  background and then finalizes normally.
- Ctrl-D sends an explicit detach before disconnecting. The run is left
  uncancelled and completion cleanup moves to the background waiter.
- Ctrl-Z sends no rotari protocol message. The terminal suspends the client
  while the server-side run continues; a later EOF follows the normal
  disconnect path and requests cancellation.
- The server monitors async workers. Worker exit decrements the active-run
  count, and interrupted-run recovery is reserved for failures that bypass
  finalization.
- Completed sync and async runs decrement the active-run count immediately
  through `beginRun`/`endRun`. When it reaches zero, the server stops without
  waiting for the idle timeout.

## CLI presentation

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

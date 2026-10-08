# Async runs, waits, and interruptions

Start a run asynchronously, wait for one or more runs, and learn what happens
when a client or supervisor stops unexpectedly. For job setup and queue control,
see [Defining and controlling jobs](RUNNING.md).

## Async runs

```sh
rotari run -p sweep --async
rotari wait sweep
```

To add a command and then start the queue asynchronously:

```sh
rotari add ./train.sh
rotari run -p sweep --async
```

Jobs run in the working directory and with the environment of the shell that
runs `rotari run`, sync or async, unless a job sets its own with
`--working-directory` or `--env`; see
[Workflow and execution environment](CONCEPTS.md#workflow-and-execution-environment).
Use `run --env=NONE` or `retry --env=NONE` to suppress ordinary caller
environment variables for that run. The default `--env=ALL` propagates them;
job `--env` values and rotari metadata still apply in either mode.

## Waiting for runs

The async start message prints commands for checking status and cancelling the
run. While the run is active, `wait` shows new job-start, retry, failure, and
progress-count messages emitted after it attaches, using the same renderer as
synchronous `run`. On attach it prints `=== Run attached ===` and a single
progress-count line with the current snapshot; it does not replay earlier
events. Before returning, it drains any remaining new events, prints the same
completion message as `run`, and returns the overall run exit code. A run
already finished when `wait` starts prints only its completion message. Older
runs without a progress journal remain waitable, but have no progress snapshot.

Multiple selected runs are monitored concurrently. Their text events use a
`[PROJECT/…ID]` label; only the label receives a run-specific blue/purple color,
with distinct colors while the eight-color palette permits. Single-run waits
have no label. Multiline events remain together, and JSON results remain
unlabelled and in selector order.

`wait --quiet` suppresses normal progress and completion output, but retains
job-failure diagnostics, early-failure reports, errors, and timeouts. It uses
the usual quiet default from `ROTARI_QUIET` and configuration (`quiet` or
`wait.quiet`); an explicit CLI value overrides those defaults. `wait --json`
prints only the JSON result on stdout, never text progress; `--quiet` does not
suppress that result. While attached, Ctrl-D stops waiting and leaves the run
running; Ctrl-C requests cancellation of the run and exits with status 130.
With multiple selectors, Ctrl-C requests cancellation of each selected run
still active. Reaching `--timeout` or returning on `--until-failure` does not
cancel a run; use `rotari cancel` to stop it explicitly.

Pass a project name, run name,
or run ID as a positional selector. Rotari checks them in that order, so a
project name wins over a run name and a run ID when the same string is used for
more than one kind of identifier. Use `--run-id/-r` to select a run explicitly.
Selecting a project by name, `--project-name/-p`, or `ROTARI_PROJECT_NAME`
waits for its active run, or returns its latest run's result immediately if
the run has already finished. This also works when a short async run finishes
before `wait` starts.
Pass multiple selectors to wait for independent async runs together:

```sh
rotari run -p sweep --async
rotari run -p eval --async
rotari wait sweep eval
```

To learn of a failure without waiting for the rest of a long run, pass
`--until-failure`. `wait` then also returns, with status 1, as soon as a job
of the run has failed with no retry left: it prints the failures grouped by
cause (see [Inspecting](INSPECT.md#inspect)) and the commands to keep waiting
or cancel. A failed attempt that the run will retry does not count. With
`--json`, it prints `{"run_id": ..., "status": "running", "failures": [...]}`
instead of a completed run's summary.

```sh
rotari wait sweep --until-failure
```

Waiting for a project that has not been created yet succeeds immediately and
does not create it, whether selected by name or `--project-name`. A missing
explicit `--run-id` (including `latest`) remains an error. A name that matches
neither a project nor a run is treated as an uncreated project unless it looks
like a run ID; use explicit run IDs when a missing run must be reported.

`--async` starts the run in a detached session (`setsid`), so it survives
terminal closure. Use `rotari wait` with a project, run name, or run ID from any
terminal, and `rotari cancel` to stop it. Without a selector or an explicit
project, `wait` scans the resolved basedir: it waits when exactly one project
is running, and lists the
running projects and run IDs and asks for a selector when several are running.
If the run stops without finishing, for example because its supervisor was
killed, `wait` reports the interrupted run and exits with status 1.

Coding agents should use `--async` with `wait`. A synchronous `run` requests
cancellation when its client disconnects, so an agent's command timeout that
kills the client cancels the run. `rotari guide` prints this and other
agent-facing rules.

## Interrupting a synchronous run

During a synchronous `rotari run`, the terminal keys behave as follows:

| Key | Effect |
| --- | --- |
| Ctrl-C | Requests cancellation and returns immediately with exit code 130. The supervisor cancels the remaining jobs and then finishes its normal cleanup (run summary, project metadata, and run-lock removal) in the background, so `run` on the same project may be rejected briefly. No `unlock` or `server shutdown` is needed. |
| Ctrl-D | Detaches the client without cancelling. The run continues as if it had been started with `--async`; follow it with `rotari wait -r RUN_ID` or `rotari show -r RUN_ID`. |
| Ctrl-Z | Only suspends the client through shell job control. The run continues, and `fg` resumes the progress view. Closing the terminal while the client is stopped disconnects it and requests cancellation, so use Ctrl-D or `--async` to leave the progress view. |

## Supervisor failure

The supervisor is not restarted automatically if the process crashes or is
killed. The run lock records its PID and host, and local jobs report their own
status through wrappers, so `rotari show -r RUN_ID` still sees results written
after the supervisor disappeared. `show` then reports the interrupted run.
After confirming jobs have stopped, use `unlock` to recover the run, then
`retry --run-id RUN_ID` to rerun its failed and unfinished jobs: the run took
its jobs from the queue when it started, so they are not queued again.
`reset` independently clears only the next queue; it does not change or recover
the interrupted run. `unlock` refuses a run whose supervisor is still alive on
this host; stop that one with `cancel`.

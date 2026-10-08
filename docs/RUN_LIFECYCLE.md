# Async runs, waits, and interruptions

Start runs asynchronously, wait for them, and handle interruptions. For job
setup and queue control, see [Defining and controlling jobs](JOB_MANAGEMENT.md).

## Async runs and waiting for runs

```sh
rotari add ./train.sh
rotari run -p sweep --async
rotari wait sweep
```

`wait` prints new progress after attaching, then the completion message and
run exit code. If the run already finished, it prints only the completion
message.

Select by project name, run name, or run ID. Positional names are checked in
that order; use `--run-id` to select an ID explicitly. A project selector waits
for its active run, or returns its latest result if finished.

```sh
rotari run -p sweep --async
rotari run -p eval --async
rotari wait sweep eval
```

Several selected runs are watched concurrently. Text events are labelled by
project and run ID; JSON results stay in selector order.

```sh
rotari wait  # wait for all active projects
```

With no selector, `wait` monitors every active project in the resolved basedir
concurrently; it errors if none are active. A project selector waits for that
project's active run, or returns its latest result.

Return as soon as a job fails with no retries left:

```sh
rotari wait sweep --until-failure
```

The command exits with status 1 and reports failures grouped by cause (see
[Inspecting](INSPECT.md#inspect)). An attempt that will be retried does not
count. Add `--json` for a running-status result with the failures.

Control output or stop waiting:

```sh
rotari wait sweep --quiet
rotari wait sweep --json
rotari cancel sweep
```

Quiet mode hides normal progress but keeps failures, errors, and timeouts. JSON
prints only the result; quiet does not suppress it. Ctrl-D detaches without
stopping the run; Ctrl-C cancels selected active runs. Neither `--timeout` nor
`--until-failure` cancels a run. Quiet defaults come from `ROTARI_QUIET` and
configuration; an explicit flag takes precedence.

```sh
rotari wait -p sweep          # wait for sweep's active or latest run
rotari wait -p typo-project   # error; does not create the project
rotari wait -r RUN_ID   # error if this run ID does not exist
```

The project must exist; a
typo is an error and does not create a project. `-r` names one specific run, so
an unknown ID is also an error. Older runs without a progress journal can be
waited on, but have no progress snapshot.
If a tool timeout may kill a command, start the run asynchronously:

```sh
rotari run -p sweep --async
rotari wait sweep
# If wait is stopped by a timeout, reconnect:
rotari wait sweep
```

Killing `wait` stops only monitoring; the async run continues. Killing a
synchronous `rotari run` requests cancellation of that run.

## Interrupting a synchronous run

```text
Ctrl-C  Cancel the run; client exits with status 130.
Ctrl-D  Detach; run continues. Follow with wait or show.
Ctrl-Z  Suspend the client only; use fg to resume its view.
```

After Ctrl-C, cleanup continues in the background, so starting another run for
the same project may briefly fail. Closing a terminal with the client stopped
by Ctrl-Z disconnects it and requests cancellation.

## Supervisor failure

The supervisor is not restarted after a crash. Inspect the run, confirm its
jobs have stopped, then unlock and retry unfinished work:

```sh
rotari show -r RUN_ID
rotari unlock -p sweep
rotari retry --run-id RUN_ID
```

The interrupted run's jobs are not put back in the queue. `reset` only clears
the next queue, and `unlock` refuses while the supervisor is still alive on
this host; cancel that run instead.

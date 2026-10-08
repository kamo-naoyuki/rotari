# Running jobs

Array and matrix jobs, dependencies, background runs, automatic retries, and
controlling queued and running jobs. To rerun failed jobs after a run, see
[Recovering failed runs](RECOVERING.md).

## Array and matrix jobs

Array jobs can be added with a numeric range or a comma-separated task list:

```sh
rotari add --array 1-10 -e local ./train.sh
rotari add --array 1-10 -e slurm ./train.sh
rotari add --array 1-10 -e sge ./train.sh
rotari add --array 1,3,4 -e slurm ./train.sh
```

Each task is tracked separately. Local and SSH execution starts one process per
task. Slurm, PBS, and LSF may submit a complete contiguous range as a native
scheduler array, while SGE always submits independent jobs to avoid relying on
fork-specific native array syntax. Sparse selections use independent jobs
where native arrays are not applicable. For each array task, rotari exposes:

- `ROTARI_ARRAY_TASK_ID`: current task number
- `ROTARI_ARRAY_FIRST`: first task number in the array
- `ROTARI_ARRAY_LAST`: last task number in the array
- `ROTARI_ARRAY_SIZE`: total number of tasks

Scheduler-backed native arrays also map the scheduler index variable into
these values, for example `SLURM_ARRAY_TASK_ID`, `PBS_ARRAY_INDEX`, or
`LSB_JOBINDEX`.

To register a matrix as independent jobs, repeat `--matrix` on `add`:

```sh
rotari add --job-name train \
  --matrix python=3.10,3.11 \
  --matrix cuda=cpu,cuda \
  ./train.sh
```

This registers the Cartesian product as four jobs named like
`train-python3.10-cudacpu`. Each job receives its values as ordinary
environment variables, such as `python=3.10` and `cuda=cpu`. Matrix jobs have
independent job IDs and can be combined with `--array`; the array is applied to
each matrix combination. `--depends-on` can name the matrix's `--job-name` (for
example `--depends-on train`) to wait for every combination. If `copy`,
`remove`, or `change` later touches only part of the matrix, such dependencies
are rewritten to the remaining combination names. Repeat
`--matrix-exclude KEY=VALUE[,KEY=VALUE...]` to omit matching combinations; all
assignments in a rule must match, and separate rules are alternatives. The
option requires `--matrix`, and workflow export retains the exclusion rules.
Workflow manifests accept the equivalent `matrix_exclude` list.

The Slurm and PBS executors are integration-tested in CI against a Slurm
container and an OpenPBS container. These tests do not certify compatibility
with every real cluster configuration. The LSF executor is covered by unit
tests using fake scheduler commands, but has not yet been tested against a
real LSF installation. SGE is covered by unit tests using fake commands and
can also be tested on demand through the Scheduler integration workflow's
`sge` choice. That job uses a digest-pinned, CentOS 7 Grid Engine image last
published in 2021; it is a compatibility smoke test, not certification for
every Grid Engine fork or a recommendation to use that image in production.

## Dependencies and stages

Use `--depends-on NAME` to define prerequisites.
Use a job's name as `NAME`; the job waits until that prerequisite succeeds.
Repeat the option to require multiple prerequisites.

For example, run `train.sh` only after `prepare.sh` completes successfully:

```sh
rotari add --job-name prepare ./prepare.sh
rotari add --job-name train --depends-on prepare ./train.sh
rotari run
```

For a barrier between batches of jobs, assign the jobs to a stage and depend on
the stage name. Jobs in a stage run concurrently; a dependent job starts only
after every job in the stage succeeds:

```sh
rotari add --stage prepare ./prepare-data.sh
rotari add --stage prepare ./prepare-config.sh
rotari add --job-name train --depends-on prepare ./train.sh
rotari run
```

`--depends-on` accepts either a job name or a stage name. A job name and stage
name cannot be the same within one queue. The name of an array job stands for
all of its tasks, like a stage name: `--depends-on train` waits until every
task of the array `train` succeeds.

Use `--depends-on-finished NAME` for a job that should run once its
prerequisites finish, whatever their result, like Slurm's `afterany`. It suits
aggregation and cleanup jobs that must still run when part of a sweep fails:

```sh
rotari add --stage sweep --matrix LR=0.1,0.01 python train.py
rotari add --job-name collect --depends-on-finished sweep python collect.py
rotari run
```

`collect` waits while a failed prerequisite still has `run --retry` attempts
left, and it also runs after a prerequisite is blocked or cancelled. A later
`run --failed` or `retry` that re-executes a prerequisite re-executes such
dependents too, so their output reflects the new results. A name cannot be
listed in both `--depends-on` and `--depends-on-finished` of one job. `show`
lists these prerequisites as `finished:NAME`. Use `change
--depends-on-finished` or `--clear-depends-on-finished` to edit them.

Use `rotari show --stage NAME` to list only the jobs in one stage of the
selected run or queue, and `rotari show --matrix NAME` for the jobs of one
matrix. The same `--stage` and `--matrix` select jobs for `change`, `remove`,
`copy`, `run`, and `retry`.

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

### Interrupting a synchronous run

During a synchronous `rotari run`, the terminal keys behave as follows:

| Key | Effect |
| --- | --- |
| Ctrl-C | Requests cancellation and returns immediately with exit code 130. The supervisor cancels the remaining jobs and then finishes its normal cleanup (run summary, project metadata, and run-lock removal) in the background, so `run` on the same project may be rejected briefly. No `unlock` or `server shutdown` is needed. |
| Ctrl-D | Detaches the client without cancelling. The run continues as if it had been started with `--async`; follow it with `rotari wait -r RUN_ID` or `rotari show -r RUN_ID`. |
| Ctrl-Z | Only suspends the client through shell job control. The run continues, and `fg` resumes the progress view. Closing the terminal while the client is stopped disconnects it and requests cancellation, so use Ctrl-D or `--async` to leave the progress view. |

### Supervisor failure

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

## Automatic retries

A failed job can be retried automatically within the same run. This is
separate from `rotari retry`, which starts a new run after one has finished;
see [Recovering failed runs](RECOVERING.md).

`run --retry N` retries failed jobs within the same run, up to N
additional attempts. The default is `0`; use `--retry -1` to retry indefinitely.
Jobs explicitly cancelled by the user are not retried by this option. For
example, these commands allow two additional attempts or retry indefinitely:

```sh
# Run the queue; failed jobs may be attempted twice more.
rotari run --retry 2

# Keep retrying failed jobs until they succeed or are cancelled.
rotari run --retry -1
```

Without `--retry`, each failed job is attempted only once.

A failed job is retried as soon as it fails; it does not wait for the other
jobs of the run. To retry only jobs that fail for transient reasons, such as a
flaky file system or network, give them their own limit when adding them. A job's
`--retry` replaces the run's for that job; `0` turns retries off even when the
run uses `--retry`:

```sh
rotari add --retry 3 ./download-data.sh
rotari add --retry 0 python evaluate.py
rotari change -p sweep --job-name download --retry 5
rotari change -p sweep --job-name download --clear-retry   # use the run's limit again
```

To change a setting for many jobs at once, select them by stage, by matrix, or
all together:

```sh
rotari change -p sweep --stage train --retry 0
rotari change -p sweep --matrix train --executor-option="-p gpu"
rotari change -p sweep --all --timeout 3h
```

To give a transient problem time to clear, or to keep many failed jobs from
retrying against a shared service at once, space the retries out. The first
retry waits `--retry-delay`, each further retry multiplies the wait by
`--retry-backoff`, and `--retry-max-delay` caps it:

```sh
# Wait 30s, 1m, 2m, then 5m between attempts.
rotari add --retry 4 --retry-delay 30s --retry-backoff 2 --retry-max-delay 5m ./download-data.sh
```

The delay settings apply to retries from `run --retry` as well as the job's
own `--retry`. Cancelling the run stops pending retries.

## Job timeouts

Stop a job that runs too long, for example one that hangs on a stalled file
system or collective operation:

```sh
rotari add --timeout 2h python train.py
rotari change -p sweep --job-name train --timeout 3h
rotari change -p sweep --job-name train --clear-timeout
```

The limit counts from when the job starts running, not from submission, so
time waiting in a scheduler queue is not included. When it runs out, rotari
sends SIGTERM to the job, gives it 30 seconds to exit (for example, to save a
checkpoint), then sends SIGKILL. The job fails with exit code 124 and the error
`timed out after 2h0m0s`, and its log ends with a matching `rotari:` line. A
timed-out job is an ordinary failure, so `run --retry`, `retry`, and
`--depends-on` treat it like any other failed job. The timeout works the same
way for every executor and is independent of scheduler walltime options such
as Slurm `--time`, which still apply.

## Queue and job control

Stop running jobs without stopping the supervisor:

```sh
rotari cancel -p sweep
rotari cancel -j ATTEMPT_ID
rotari cancel ATTEMPT_ID
rotari cancel RUN_ID
```

`--job-id/-j` is optional. Without it, all running jobs in the queue are
cancelled. With it, only the specified running jobs are cancelled, and the
option may be repeated. `--job-id/-j` cannot be used with `--wait`, nor combined
with a positional `JOB_ID`/`ATTEMPT_ID`/`RUN_ID`. An array job's ID selects
all of its unfinished tasks. A `RUN_ID` or `ATTEMPT_ID` must belong to the
project's active run; `rotari cancel` of an earlier run's ID fails instead of
cancelling the run that is active now. Without `-p`, a `JOB_ID` is looked for
in the active run of every project.

A cancelled job's result records the error `cancelled`, so
`--filter-failure-kind cancelled` selects it and failure summaries list it
under `cancelled` rather than by its exit code.

Whole-run cancel (no `--job-id/-j`) and, for `local`-executor jobs, `--job-id/-j`
cancel/suspend/resume all signal jobs by PID, which only means
something on the host that actually runs it; run these commands from that
host if it differs from wherever `cancel`/`suspend`/`resume` is invoked. See
the [FAQ](FAQ.md#client-control-and-job-cancellation) for what happens
when you can't.

Temporarily suspend and resume running jobs:

```sh
rotari suspend -p sweep
rotari suspend -j ATTEMPT_ID
rotari resume -j ATTEMPT_ID
rotari suspend ATTEMPT_ID
rotari resume RUN_ID
```

Without `--job-id/-j`, all currently running jobs are affected. Repeat `--job-id/-j`
to control selected jobs; job IDs select as for `cancel`, except that an array
job's ID selects only its running tasks. Local jobs use `SIGSTOP`/`SIGCONT`; Slurm jobs use
`scontrol suspend`/`scontrol resume`; SGE jobs use `qmod` suspend/unsuspend.

Remove jobs from the current queue without affecting saved run history:

```sh
rotari remove -p sweep --job-name train
rotari remove -p sweep -j JOB_ID -j OTHER_JOB_ID
rotari remove -p sweep --stage eval
```

Like `change`, `remove` edits the current queue and does not restore an empty
one; restore a run with `copy`, or pass `--run-id/-r ID` (or `latest`) to replace
the queue with that run's jobs first. Specify exactly one target selector:
`--job-name NAME`, one or more `--job-id/-j ID` options, `--stage STAGE`,
`--matrix NAME`, or `--all`, as for `change`. Removing a job that another queued
job depends on is rejected.

Delete saved run logs while keeping queued commands:

```sh
rotari delete -p sweep RUN_ID
rotari delete -p sweep --all
```

`delete` removes the run given as `RUN_ID` or `--run-id/-r`. Removing every
saved run of the project needs `--all`; without a run or `--all`, nothing is
deleted.

The commands affect the current queue and saved run history differently:

```mermaid
flowchart LR
  add([rotari add]) --> queue[(queue.json)]
  change([rotari change]) --> queue
  remove([rotari remove]) -->|remove selected jobs| queue

  queue --> run([rotari run])
  run --> active((running jobs))
  run --> history[(runs/<run-id>/)]
  run -->|empty after start| queue
  history --> copy([rotari copy])
  copy -->|all jobs| queue
  run -.->|--run-id/-r: copy, then select/carry forward| queue
  cancel([rotari cancel]) -->|stop selected/all| active
  suspend([rotari suspend]) -->|pause selected/all| active
  resume([rotari resume]) -->|continue selected/all| active
  delete([rotari delete]) -->|delete saved runs| history

  classDef edit fill:#e0e7ff,stroke:#4f46e5,color:#1e1b4b
  classDef control fill:#ccfbf1,stroke:#0f766e,color:#134e4a
  classDef destructive fill:#fee2e2,stroke:#dc2626,color:#7f1d1d
  class add,change,run,copy edit
  class suspend,resume control
  class remove,cancel,delete destructive

  subgraph legend[Legend]
    legendEdit[edit queue or run jobs]
    legendControl[pause or resume jobs]
    legendDestructive[destructive action]
  end
  class legendEdit edit
  class legendControl control
  class legendDestructive destructive
```

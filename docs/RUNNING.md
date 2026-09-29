# Running and recovering

Background runs, reruns and retries, and controlling queued and running jobs.

## Array and matrix jobs

Array jobs can be added with a numeric range or a comma-separated task list:

```sh
rotari add --array 1-10 -e local ./train.sh
rotari add --array 1-10 -e slurm ./train.sh
rotari add --array 1,3,4 -e slurm ./train.sh
```

Each task is tracked separately. Local and SSH execution starts one process per
task. Slurm, PBS, and LSF may submit a complete contiguous range as a native
scheduler array, while sparse selections use independent jobs where native
arrays are not applicable. For each array task, rotari exposes:

- `ROTARI_ARRAY_TASK_ID`: current task number
- `ROTARI_ARRAY_FIRST`: first task number in the array
- `ROTARI_ARRAY_LAST`: last task number in the array
- `ROTARI_ARRAY_SIZE`: total number of tasks

Scheduler-backed arrays also map the native index variable into these values,
for example `SLURM_ARRAY_TASK_ID`, `PBS_ARRAY_INDEX`, or `LSB_JOBINDEX`.

To register a matrix as independent jobs, repeat `--matrix` on `add`:

```sh
rotari add --job-name train \
  --matrix python=3.10,3.11 \
  --matrix cuda=cpu,cuda \
  -- ./train.sh
```

This registers the Cartesian product as four jobs named like
`train-python3.10-cudacpu`. Each job receives its values as ordinary
environment variables, such as `python=3.10` and `cuda=cpu`. Matrix jobs have
independent job IDs and can be combined with `--array`; the array is applied to
each matrix combination. `--depends-on` can name the matrix's `--job-name` (for
example `--depends-on train`) to wait for every combination. If `copy`,
`remove`, or `change` later touches only part of the matrix, such dependencies
are rewritten to the remaining combination names. `include` and `exclude`
customization is not supported by the version 1 workflow manifest.

The Slurm and PBS executors are integration-tested in CI against a Slurm
container and an OpenPBS container. These tests do not certify compatibility
with every real cluster configuration. The LSF executor is covered by unit
tests using fake scheduler commands, but has not yet been tested against a
real LSF installation.

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
run. `wait` returns the overall run exit code. Pass a project name, run name,
or run ID as a positional selector. Rotari checks them in that order, so a
project name wins over a run name and a run ID when the same string is used for
more than one kind of identifier. Use `--run-id/-r` to select a run explicitly.
Pass multiple selectors to wait for independent async runs together:

```sh
rotari run -p sweep --async
rotari run -p eval --async
rotari wait sweep eval
```

Waiting for a project that has not been created yet succeeds immediately and
does not create it, whether selected by name or `--project-name`. A missing
explicit `--run-id` (including `latest`) remains an error. A name that matches
neither a project nor a run is treated as an uncreated project unless it looks
like a run ID; use explicit run IDs when a missing run must be reported.

`--async` starts the run in a detached session (`setsid`), so it survives
terminal closure. Use `rotari wait` with a project, run name, or run ID from any
terminal, and `rotari cancel` to stop it. Without a selector, `wait` scans the
resolved basedir: it waits when exactly one project is running, and lists the
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
| Ctrl-C | Requests cancellation and returns immediately with exit code 130. The supervisor cancels the remaining jobs and then finishes its normal cleanup (run summary, queue clearing, and run-lock removal) in the background, so `run`, `add`, or `copy` on the same project may be rejected briefly. No `unlock` or `server shutdown` is needed. |
| Ctrl-D | Detaches the client without cancelling. The run continues as if it had been started with `--async`; follow it with `rotari wait -r RUN_ID` or `rotari show -r RUN_ID`. |
| Ctrl-Z | Only suspends the client through shell job control. The run continues, and `fg` resumes the progress view. Closing the terminal while the client is stopped disconnects it and requests cancellation, so use Ctrl-D or `--async` to leave the progress view. |

### Supervisor failure

The supervisor is not restarted automatically if the process crashes or is
killed. The run lock records its PID and host, and local jobs report their own
status through wrappers, so `rotari show -r RUN_ID` still sees results written
after the supervisor disappeared. `show` then reports the interrupted run.
After confirming jobs have stopped, use `unlock` to keep the queue or
`reset --recover` to discard it. Both refuse a run whose supervisor is still
alive on this host; stop that one with `cancel`.

## Recover and rerun
### run and retry

`run` can select jobs from the latest run, or from a saved run given by
`--run-id/-r`, and execute them as a new run while carrying forward everything
else. Result filters select which jobs in the queue are re-executed; jobs with
completed results that do not match are carried forward. To inspect or edit the
whole previous queue before selecting work, copy it first and apply the filter
when running.

```sh
rotari run -p sweep --failed
rotari run -p sweep --unfinished
rotari run -p sweep --success
rotari run -p sweep --failed --unfinished
rotari run -j ATTEMPT_ID
```
When a new queue has different job IDs, `run` can match its jobs to the
reference run by identity mode: `--match-by job-id`, `--match-by fingerprint`,
or the default `--match-by id-and-fingerprint`. The combined mode uses Job ID
first and fingerprint only for jobs that remain unmatched. Fingerprints are
calculated from the command, explicitly saved job inputs, and expanded array
or matrix parameters; they are recalculated for each comparison rather than
stored in queue or run files.

The two workflows establish source links differently. Copying a saved run
records an `Origin` for each copied job or task. A newly created queue has no
such links, so `--match-by fingerprint` can match its execution units to a
reference run. A fingerprint is a matching key, not a stored ID; an `Origin`
points to the source run and job, and (when available) the attempt. Result
filters use the source result to decide whether to execute a job or carry its
result forward.

`copy` copies jobs individually from a saved run into the current queue. Each
copied job records an `Origin` that points back to its source run, job, and
(when available) attempt, and preserves the source status. The copied queue
entry remains pending until a later `run` applies its selection. The source
job ID is kept unless it conflicts with an ID already in the queue.

```mermaid
flowchart LR
  subgraph SavedRun["Saved run: RUN_ID"]
    SourceA["Job A<br/>Job ID: job-a"]
    SourceB["Job B<br/>Job ID: job-b"]
  end

  subgraph CurrentQueue["Queue after rotari copy RUN_ID"]
    CopiedA["Copied job A<br/>Queue ID: job-a*"]
    OriginA["Origin A<br/>Run ID: RUN_ID<br/>Job ID: job-a<br/>Attempt ID: attempt-a"]
    StatusA["Status: success"]
    CopiedB["Copied job B<br/>Queue ID: job-b*"]
    OriginB["Origin B<br/>Run ID: RUN_ID<br/>Job ID: job-b<br/>Attempt ID: attempt-b"]
    StatusB["Status: failed"]
    CopiedA --> OriginA
    OriginA --> StatusA
    CopiedB --> OriginB
    OriginB --> StatusB
  end

  SourceA -->|"copy job A"| CopiedA
  SourceB -->|"copy job B"| CopiedB

  Note["* Source job ID is preserved unless it conflicts with an ID in the queue.<br/>Attempt ID is recorded when available."]

  classDef source fill:#f1f5f9,stroke:#64748b,color:#0f172a
  classDef queue fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef origin fill:#ccfbf1,stroke:#0f766e,color:#134e4a
  classDef note fill:#fef3c7,stroke:#d97706,color:#78350f
  classDef success fill:#dcfce7,stroke:#16a34a,color:#14532d
  classDef failed fill:#fee2e2,stroke:#dc2626,color:#7f1d1d
  class SourceA,SourceB source
  class CopiedA,CopiedB queue
  class OriginA,OriginB origin
  class StatusA success
  class StatusB failed
  class Note note
```

For example, to retry selected jobs from a saved run, copy its queue first:

```sh
rotari add -p sweep ...
rotari run -p sweep # Run finished with some failed jobs.
rotari copy -p sweep RUN_ID
rotari run -p sweep --failed --unfinished
```

When the copied queue is run, each `Origin` resolves its saved result. The
result filter determines which jobs execute and which completed results carry
forward:

```mermaid
flowchart LR
  SavedRun["Saved run<br/>queue + results"] --> Copy["rotari copy RUN_ID"]
  Copy --> CopiedQueue["Copy job definitions<br/>into current queue"]
  CopiedQueue --> Origin["Attach Origin per job / task<br/>source Run ID + Job ID + Attempt ID"]
  Origin --> Filter["run selection / result filter"]
  Filter -->|"selected"| Execute["Execute in new run"]
  Filter -->|"completed, not selected"| Carry["Carry result and output link"]
  Filter -->|"no completed result"| Unfinished["Remain unfinished"]

  classDef source fill:#f1f5f9,stroke:#64748b,color:#0f172a
  classDef queue fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef origin fill:#ccfbf1,stroke:#0f766e,color:#134e4a
  classDef filter fill:#fef3c7,stroke:#d97706,color:#78350f
  classDef execute fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef carried fill:#dcfce7,stroke:#16a34a,color:#14532d
  classDef unfinished fill:#e2e8f0,stroke:#64748b,color:#334155
  class SavedRun source
  class Copy,CopiedQueue queue
  class Origin origin
  class Filter filter
  class Execute execute
  class Carry carried
  class Unfinished unfinished
```

If you create a new queue whose job IDs differ from those in the latest run,
use fingerprint matching to find equivalent jobs:

```sh
rotari add -p sweep ...
rotari run -p sweep # Run finished with some failed jobs.
# Add the same commands again to create a new queue with different job IDs.
rotari add -p sweep ...
rotari run -p sweep --failed --unfinished --match-by fingerprint
```

```mermaid
flowchart LR
  NewQueue["New queue<br/>without Origin links"] --> Match["Match against reference run<br/>--match-by fingerprint<br/>(default: Job ID, then fingerprint)"]
  Reference["Reference run<br/>commands.json + results"] --> Match
  Match -->|"fingerprint match"| Origin["Create Origin link<br/>source Run ID + Job ID + Attempt ID"]
  Origin --> Filter["run selection / result filter"]
  Filter -->|"selected"| Execute["Execute in new run"]
  Filter -->|"completed, not selected"| Carry["Carry result and output link"]
  Filter -->|"no completed result"| Unfinished["Remain unfinished"]

  classDef source fill:#f1f5f9,stroke:#64748b,color:#0f172a
  classDef queue fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef match fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef origin fill:#ccfbf1,stroke:#0f766e,color:#134e4a
  classDef filter fill:#fef3c7,stroke:#d97706,color:#78350f
  classDef execute fill:#dbeafe,stroke:#2563eb,color:#1e3a8a
  classDef carried fill:#dcfce7,stroke:#16a34a,color:#14532d
  classDef unfinished fill:#e2e8f0,stroke:#64748b,color:#334155
  class NewQueue queue
  class Reference source
  class Match match
  class Origin origin
  class Filter filter
  class Execute execute
  class Carry carried
  class Unfinished unfinished
```

`retry` is `run --failed --unfinished` by default, but not an alias of it:
`--failed --unfinished` applies only when no result filter or job is given. It selects failed and
unfinished jobs from the reference run, copies them into the next run with
successful results carried forward, and executes that run. Given `--job-id/-j`,
it runs only those jobs instead:

```sh
rotari retry -p sweep
rotari retry -p sweep -j JOB_ID
```

`--retry N` is different: it retries failed jobs within the same run, up to N
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
rotari add --retry 3 -- ./download-data.sh
rotari add --retry 0 -- python evaluate.py
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
rotari add --retry 4 --retry-delay 30s --retry-backoff 2 --retry-max-delay 5m -- ./download-data.sh
```

The delay settings apply to retries from `run --retry` as well as the job's
own `--retry`. Cancelling the run stops pending retries.

The result filters select which jobs are actually re-executed:

| Option | Executed jobs |
| --- | --- |
| `--failed` | Finished jobs with a non-zero exit code. |
| `--unfinished` | Jobs without a completed result. |
| `--success` | Finished jobs with exit code zero. |
| `--failed --unfinished` | Failed or unfinished jobs. |

Result filters or repeated `--job-id/-j` select jobs to re-execute; a job
named with `--job-id/-j` or `--job-name` runs whatever its result, so the two
cannot be combined. Finished
non-matching jobs carry forward their previous result and output; jobs without
a result remain unfinished. Carried-forward jobs are not re-executed, but they
appear on the new run with a link to their original output, so the whole run
can be inspected in one place. Use `--failed --unfinished` to recover everything
that did not complete successfully.

`--stage STAGE` or `--matrix NAME` (the base job name given to `add --matrix`)
narrows the selection to one stage or matrix. Jobs outside it carry forward
like non-matching jobs. Alone, it re-executes every job in the stage or matrix;
with a result filter, only the matching jobs in it:

```sh
rotari retry -p sweep --stage train        # failed or unfinished jobs in stage train
rotari run -p sweep --matrix train         # every job of matrix train
rotari copy -p sweep --stage eval --failed # restore only failed jobs in stage eval
```

`--run-id/-r ID` selects a saved run as both the queue snapshot and filter
reference. A non-empty queue requires confirmation; add `--overwrite` to
replace it without asking:

```sh
rotari run -p sweep -r RUN_ID --failed
```

For array jobs, filters select matching tasks by default
(`--partial-array=true`); use `--partial-array=false` to re-execute every task
when any task matches.


### copy and change

Copy jobs from a previous run into the current queue without executing them:

```sh
rotari copy -p sweep
rotari run -p sweep --failed
```

Without a selector, `copy` restores every job from the latest run. Then apply
`--failed`, `--unfinished`, `--success`, or explicit job selectors to `run`.
This keeps the full queue available for inspection and editing before choosing
which jobs to execute. `copy --failed` and the other copy-side filters remain
available when only a subset should be restored. When the queue is empty,
`run --failed` restores the latest run automatically before selecting failed
jobs. `run --job-id/-j` and `--job-name` likewise run a job from a non-empty
queue, keeping edits made with `change`, and restore the latest run only when
the job is not queued.

`copy` keeps the source job ID unless it would collide with the destination
queue, and preserves dependencies between copied jobs. A non-empty queue
requires confirmation before replacement; use `--append` to add jobs or
`--overwrite` to replace it without asking. Selection options include
`--failed`, `--unfinished`, `--success`, `--stage`, `--matrix`, and repeated
`--job-id/-j`; a job ID or `--job-name` is not combined with a result filter.
Copied jobs
remain pending, with source run, status, and working-directory metadata kept
for later inspection.

Use `copy` when a selected job needs to be edited before it is run again. It
restores jobs into the current queue without executing them; then `change` can
modify their commands or options while preserving the saved run history:

```sh
rotari copy
rotari change --job-name train -e local
rotari change --job-name train --executor-option="-p gpu"
rotari change --job-name train --depends-on prepare -- ./train-v2.sh
rotari retry
```

A changed job keeps its previous result, even when its command, environment
(`--env`), or working directory changes. To run an edited job again with
`retry`, mark it with `change --status unfinished`: it then counts as
unfinished until it runs again, and the old result is never carried forward.

`change` requires exactly one target selector: `--job-id/-j ID` or
`--job-name NAME` for one job, or `--stage STAGE`, `--matrix NAME` (the base job
name given to `add --matrix`), or `--all` for every matching job. It also
requires at least one change, such as a new command, `--executor/-e`,
`--executor-option`, `--set-job-name`, or `--depends-on`. A new command and
`--set-job-name` need a single job. An array job is changed as a whole; its
tasks cannot be changed one by one. It replaces only the options specified,
keeps the job IDs, and edits the current queue. A run leaves the queue empty,
and `change` does not restore it on its own: restore a run with `copy` first, or
pass `--run-id/-r ID` (or `latest`) to replace the queue with that run's jobs
before the change.

### Job timeouts

Stop a job that runs too long, for example one that hangs on a stalled file
system or collective operation:

```sh
rotari add --timeout 2h -- python train.py
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
`scontrol suspend`/`scontrol resume`.

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

  classDef edit fill:#1d4ed8,stroke:#1e3a8a,color:#ffffff
  classDef control fill:#0f766e,stroke:#115e59,color:#ffffff
  classDef destructive fill:#b91c1c,stroke:#7f1d1d,color:#ffffff
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

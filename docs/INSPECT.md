# Inspecting runs and jobs

Checking status and logs, run readiness, and execution history.

## Inspect

Choose a command by what you want to see:

| Purpose | Commands |
| --- | --- |
| Get a quick overview of the current context and active runs | `info` |
| Find objects in a list | `jobs`, `runs`, `projects`, `basedirs` |
| Inspect a selected object | `show` |
| Trace execution history or compare runs | `lineage` |

## Overview: info

Start with `rotari info` for a compact snapshot of the current context: 

```sh
rotari info
```

The resolved masterdir, basedir and project, visible config files, running
supervisors, run locks, and active or interrupted runs. Each run row also
shows the count of jobs with final results, including carried results, plus a
best-effort process check for unfinished local jobs. Remote and scheduler jobs
are shown as unverified; `info` does not contact their hosts or schedulers.

```sh
rotari info --json
```

Add `--json` for structured output. Use the lists below to find other jobs,
runs, projects, or state directories, and `show` to inspect details.

## Lists: jobs, runs, projects, basedirs

The four list commands differ in what each row represents:

| Command | Lists |
| --- | --- |
| `rotari jobs` | Unfinished jobs of active, interrupted, or incomplete runs, and recently finished jobs |
| `rotari runs` | Active, interrupted, and recently finished runs |
| `rotari projects` | Projects, with their latest run status |
| `rotari basedirs` | Known state directories (not job working directories) |

Use these lists to find a project, run, or job, then use `show` for its details.
The `runs` list shows run `STATUS` separately from `CLIENT`. `CLIENT` describes
current aggregate CLI attachment: synchronous `run`/`retry` and `wait` each
hold an independent session, and the run is attached while any verified live
session remains. Launch mode and the initiating client's last detach reason
remain recorded as history; Web and MCP readers do not count as attachments.

### Reading run and job states

Run lifecycle and job execution are independent. A run can be `interrupted`
while a job still reports `running (recorded)`, or while some jobs already have
terminal results. Job phases are the latest persisted observations, not live
executor probes: `running (recorded)` does not prove that the process is still
alive. `waiting (recorded)` and `suspended (recorded)` have the same limitation.
When state is missing or cannot be interpreted, Rotari displays `unknown`
rather than claiming the job has not started. Completed jobs show `success`,
`failed`, `cancelled`, or `blocked`; results carried from another run are marked
`(carried)`.

Client state describes only the initiating run/retry progress client, not
`wait`, Web, or MCP readers. If the supervisor cannot be verified, the current
connection is `unknown`; where available, the label retains the last recorded
detach reason. A finalized client record shows `async (completed)` or
`sync (completed)` and retains recorded detach/cancel history; this describes
history, not a live connection. A finished run without a finalized client
record still shows `unknown`. These classifications are best-effort and do not
contact an executor or scheduler.

### Scope and time window

The lists cover state directories known to the master registry, not every
directory on disk. `basedirs` also prints the resolved master directory.
Use `--basedir DIR` on `jobs`, `runs`, or `projects` to inspect one state
directory.

For `jobs` and `runs`, finished entries default to the last day. Unfinished
attempts of active, interrupted, or incomplete runs, and active or interrupted
runs, are always included. Their job states are persisted observations: for example,
`running (recorded)` does not assert that an executor is still alive. Jobs
without an attempt are not listed by `jobs`.

```sh
rotari jobs --since 7d
rotari runs --since 7d
rotari projects --basedir DIR
```

When `jobs` finds nothing, it names the state directories it searched and the
time window. To choose its displayed fields, use `--format`; see the
[CLI reference](CLI_REFERENCE.md#rotari-jobs).

The project list shows each project's last run and its result, such as
`failed 7/15` when 7 of its 15 jobs failed. Its suggested commands name the
basedir (`-b BASEDIR`) when a listed project lies outside the default state
directory. To inspect a project in another state directory, use
`rotari show -b DIR -p PROJECT`.

## Details: show

Use `show` to inspect a project's current run or pending queue, or a specific
run, job, or attempt.

```sh
rotari show -p sweep # the project's current run or queue
rotari show RUN_ID # one run
rotari show JOB_ID # one job from the resolved run or queue
rotari show ATTEMPT_ID # one execution attempt, including its logs
```

### Selecting the target

With no selector, `show` resolves one project from explicit options,
configuration/environment defaults, or sole-project discovery, then displays
its current run or queue. If more than one project remains, it exits with a
hint to use `rotari projects`; it does not silently switch to a list view.

You can also select a job by name with `rotari show JOB_NAME`. Passing
`--run-id latest` selects the latest settled run even when the project has a
non-empty queue or an active run; an unknown job ID fails instead of showing
the queue.

When the selected run is active or interrupted, a non-empty next queue is
shown separately after the run; `check` reports its queued count.

### Filtering jobs and inspecting failures

```sh
rotari show -p sweep --failed
rotari show -p sweep --stage train
rotari show -p sweep --matrix train
```

`--matrix` names the matrix by its base job name. For other result selections
and per-job conditions, see the [CLI reference](CLI_REFERENCE.md#rotari-show).

For a run with failed jobs, `show` prints a `Failure summary:` line before
the job table, with the `rotari lineage` command that prints only the run's
summary and failure causes. It also ends its job table with
`Failures by cause:`, one entry per cause with the number of jobs, their exit
codes, the jobs (array tasks as `train[3,7,11]`, at most ten), an example
line, the command that shows the first job, and the diagnosis rule's
suggestion:

```text
Failures by cause:
  42 CUDA/GPU memory exhausted (exit 1): sweep[7,14,21,28,35,42,49,56,63,70] +32 more
    e.g. torch.OutOfMemoryError: CUDA out of memory (task 7)
    show: rotari show -j att_20261002-124301-d7741cfe-32ab8e9d1-7-0
    fix: Reduce batch size or model memory use, select a GPU with more free memory, and check for other processes using the GPU.
  1 timeout (exit 124): slow
    e.g. timed out after 5s
```

Each job is classified on its own result: a block by a failed dependency, a
cancellation, or a timeout comes first, then the job's latest saved rule
diagnosis, and otherwise its failure kind (`oom`, `signal`, or `error`; see
`--filter-failure-kind`). The most frequent cause is listed first. With result
selections or filters, only the listed jobs are grouped. `show --run-id RUN_ID
--json` carries the same groups, with every job ID, as `failures`; so do
`lineage RUN_ID --json` and the Web UI's run summary.

### Logs

Showing one job or attempt includes its logs. To view logs for several jobs,
select a stream, or explicitly follow output:

```sh
rotari show -p sweep --logs # logs for every job in the selected run
rotari show -p sweep --failed-logs --stream stderr # failed jobs' separate stderr
rotari show -p sweep --job-id JOB_ID --stream stdout --follow
```

`show --logs --failed` (or `--logs --filter-result failed`) selects the same
failed jobs as `--failed-logs`, including output carried from an older run.

When a run is active and output is a terminal, `show JOB_ID` and
`show ATTEMPT_ID` follow the selected attempt's log automatically. The default
merged log mode follows the combined `output` file, where stdout and stderr
cannot be distinguished. `--log-mode separate` stores them independently, and
`--stream` chooses which one to follow. Output buffered by the job itself
appears only after that program flushes it.

When output is a terminal, log views (including `--job-id/-j`) longer than 24
lines open in `$PAGER` (or `less -R` by default). Use `--no-pager` to print
directly; piped and redirected output is always printed directly.

An older `ATTEMPT_ID` shows that attempt's own status, timestamps, and logs.
Logs are merged by default; use `add --log-mode separate` when adding a job to
preserve stdout and stderr independently. Repeat `add --output FILE` and
`add --error FILE` to add external destinations independently. If `--error` is
omitted, stderr follows the `--output` destinations too. Destinations append
by default; `--open-mode truncate` truncates them before execution.
The run's saved hosts and diagnoses belong to the latest attempt, so they are
not shown for an older one.

### JSON and reports

Use `--json` for structured data, or `--report` for an AI-ready Markdown report:

```sh
rotari show -p sweep --run-id latest --job-id JOB_ID --json
rotari show ATTEMPT_ID --report
rotari show RUN_ID --report
```

The run/job JSON view includes the resolved run and project, a `jobs` array
with each job's definition, a `finished` flag, and a `result` when available.
For a running job, `finished` is false and no `result` is present. With
`--failed`, the run JSON keeps only the failed jobs' summary results and
failure groups (the `commands` snapshot stays whole), and the job and array
JSON keeps only the failed jobs, as the job table does. When a next queue is
shown, `commands` remains the run snapshot and `next_queue` contains the
queued work.

A report's log section shows, for a job whose saved diagnosis cites a line
found in its log, the lines around that evidence and the last 20 lines, with
the skipped lines marked as `[... N lines omitted ...]`. Otherwise it shows
the last 100 lines. Either way it keeps at most 12000 characters.

### Artifacts

Rotari records candidate files and directories referenced by a job definition.
Discovery is best-effort and static: it does not run the job, verify that a
path exists, or determine whether it is an input or output. In particular, it
may not find paths that the job constructs at runtime. Recording candidates
does not affect job execution. Declare runtime-generated paths explicitly with
repeatable `rotari add --artifact PATH`; paths can use
`$ROTARI_ARRAY_TASK_ID`, `$ROTARI_JOB_DIR`, and the job's `--env` and matrix
variables, so each array task or matrix member can have its own artifact:

```sh
rotari add --array 0-9 --artifact 'results/$ROTARI_ARRAY_TASK_ID/plot.png' python train.py
```

Automatic discovery looks for likely paths in command arguments, environment
and matrix values, output destinations, and referenced configuration or script
files. Paths in configuration and scripts are inspected without running the
job; dynamically generated paths and unreadable files may be missed. For SSH
and scheduler jobs, discovery reads files available on the host running
`rotari run`.

`rotari show -j JOB` lists the first 20 of them after the command, each with
what is at the path now on the host running `show` (`file`, `directory`,
`other`, `missing`, or `unknown` for a relative path whose directory was
never known), the path relative to the job's working directory, and where it
was found:

```text
Artifacts: relative to /work/exp
  file       train.py  (argument)
  file       conf/a.yaml  (--config)
  directory  results  (a.yaml: out_dir)
  missing    out/3.log  (> (run.sh:2:15))
```

`missing` means only that the path is absent on the host running `show`; an
SSH or scheduler job's files may be on another host. Use
`rotari show -j ATTEMPT_ID --artifacts` to list all recorded candidates and
discovery notes instead of logs; `show -j JOB --json` includes them as
`artifacts`. Older runs from before artifact recording show `(not recorded)`.
A carried job shows candidates from the attempt that produced its result.

In the live Web UI, the Artifacts button opens the listing for the attempt
shown by Output. Files under the job's working directory can be previewed in
supported formats (including images, audio/video, NumPy arrays, CSV/TSV, and
text) or downloaded; directories show their immediate contents. Other paths
are listed but not opened unless allowed by `rotari web --artifact-root DIR`.
A static export records the listing as of export time. It includes no file
contents by default; `--static-artifact-contents` copies previewable files
(up to 10 MiB each and 100 MiB total) so they can be previewed there. Anyone
who can access the Web UI or an export with copied contents may be able to read
those files; see [Web UI security](OPERATIONS.md#security-model) before sharing.

### Interrupted runs

If a runner exits before finalizing its run, `show` reports the interrupted run
and blocks `run` until you acknowledge it. First confirm
that all jobs have stopped:

```sh
rotari show -p sweep
```

Then execute the `rotari unlock` command printed by `show` (for example,
`rotari unlock -r RUN_ID`). The run took its jobs from the queue when it
started, so the queue does not bring them back; rerun the ones that failed or
did not finish from the run itself, as `unlock` suggests:

```sh
rotari retry -p sweep --run-id RUN_ID
```

Without an explicit run ID, `unlock` is also safe to call when the project
does not exist or has no lock/interrupted run: it succeeds without changing
state. With `--run-id`, a missing or mismatched run remains an error.

Use `retry` or result filters when the interrupted run contains completed jobs.
`rotari reset` clears only the queue for the next run, even while a run is
active or interrupted; it does not recover the run. If the project does not
exist yet, `reset` initializes an empty project, so it can safely start a
batch-building script. Use `unlock` only after confirming interrupted jobs
have stopped.

## Run lineage

Use `lineage` as the run history and comparison view. With no run IDs, it
shows the whole sequence oldest first:

```sh
rotari lineage -p sweep
rotari lineage -p sweep --json
```

With one run ID it shows that run's summary. With two run IDs it compares
them. With three or more run IDs it shows a job-by-run result grid:

```sh
rotari lineage -p sweep RUN_ID
rotari lineage -p sweep RUN_A RUN_B
rotari lineage -p sweep --json RUN_A RUN_B
rotari lineage -p sweep RUN_A RUN_B RUN_C
```

`lineage` summarizes jobs that were fixed, are still failing, or newly fail,
with each run's failure cause as `show` groups it (the `CAUSE` column, and
`from_cause`, `to_cause`, and `cause_changed` in JSON), so a job that still
fails for a different reason stands out; jobs
added or removed; jobs whose command, executor, executor options, environment,
working directory, stage, or dependencies changed; and jobs whose result was
carried forward instead of re-executed, marked `(carried)` in the `RESULT`
column. The tasks of an array that read the same share one row, such as
`train[1,2,4]`, and so do the tasks listed under a shared definition change.
Jobs whose result and definition did not change are hidden, and the line that
counts them names them.
Jobs are matched by origin when the
origin points to the compared run; otherwise named jobs are matched by name.
The one-run summary also counts failed jobs by diagnosis, groups them by
cause as `show` does, and reports their source-run origins, including `new`
for jobs without an origin.

## Check run readiness

To check whether a project can start its queued run without changing any
state:

```sh
rotari check sweep
```

`check` reports whether the project is ready to run, together with its project,
queue, and lock state. It exits with status 0 when the queued run can start and
status 1 otherwise. Pass `--json` for machine-readable output, or `--deep` to
also check executables and local working directories on the current host. Both
`check` and `reset` accept the project name as an optional positional argument;
do not combine it with `--project-name`.

A project that does not exist yet is reported as `state=empty` with zero queued
jobs and exit status 1; `check` does not create the project.

The command is read-only and does not reserve the project or remove a stale
lock. `run` and `reset` repeat the applicable checks before changing state, so
they remain safe if the project changes after `check` returns. Inconsistent
saved state is reported instead of starting or recovering a run.

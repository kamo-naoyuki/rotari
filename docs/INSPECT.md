# Inspecting and diagnosing

Checking status and logs, checking run readiness, and diagnosing failed jobs.

## Inspect

Use `jobs` to inspect the current project activity and recent execution history across the selected basedir.

```sh
rotari jobs # list running and recently finished jobs across projects; good for a quick status scan
rotari jobs --since 7d # include finished jobs from the last seven days
rotari jobs --all-basedirs # list jobs across basedirs known to the master registry
rotari jobs --all-basedirs --format "%s %b %p %a %n %c %t %e" # choose displayed fields
```

When `jobs` finds nothing, it names the state directory it searched and the
time window, and suggests `--all-basedirs` to search every registered state
directory.

Use `show` to inspect a project's runs and pending queue, or a specific run/job.

```sh
rotari show # list projects across registered basedirs
rotari show --basedirs # print the resolved master directory and state directories
rotari show -p sweep # list the project's runs and current queue, if non-empty
rotari show -p sweep --failed # list failed jobs in the selected run
rotari show -p sweep --stage train # list only the jobs in stage train of the selected run or queue
rotari show -p sweep --matrix train # list only the jobs of matrix train, named by its base job name
rotari show ATTEMPT_ID # show one job attempt in detail: status, executor, command, stdout, and stderr
rotari show JOB_ID --stream stderr # show only the selected job's stderr
rotari show JOB_ID # show a job from the resolved run or queue
rotari show JOB_NAME # show a job by name
rotari show -p sweep --logs # print each job's configured log for every job in the selected run
rotari show -p sweep --failed-logs --stream stderr # print stderr for failed jobs whose logs are separate
rotari show -p sweep --job-id JOB_ID --stream stdout --follow # follow only stdout
rotari show ATTEMPT_ID --report # print an AI-ready Markdown report for one attempt
rotari show RUN_ID --report # describe the whole run and include recent logs
rotari show -p sweep --run-id latest --job-id JOB_ID --json # one run job (or all tasks of an array) as JSON
```

The project list shows each project's last run and its result, such as
`failed 7/15` when 7 of its 15 jobs failed. Its suggested commands name the
basedir (`-b BASEDIR`) when a listed project lies outside the default state
directory; with `ROTARI_BASEDIR` set, `show` lists only that directory, so use
`rotari show -b DIR` for another one.

The run/job JSON view includes the resolved run and project, a `jobs` array
with each job's definition, a `finished` flag, and a `result` when available.
For a running job, `finished` is false and no `result` is present. Passing
`--run-id latest` selects the latest saved run even when the project has a
non-empty queue; an unknown job ID fails instead of showing the queue.

For a run with failed or blocked jobs, `show` ends its job table with
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

A report's log section shows, for a job whose saved diagnosis cites a line
found in its log, the lines around that evidence and the last 20 lines, with
the skipped lines marked as `[... N lines omitted ...]`. Otherwise it shows
the last 100 lines. Either way it keeps at most 12000 characters.

An older `ATTEMPT_ID` shows that attempt's own status, timestamps, and logs.
Logs are merged by default; use `add --log-mode separate` when adding a job to
preserve stdout and stderr independently. Repeat `add --output FILE` and
`add --error FILE` to add external destinations independently. If `--error` is
omitted, stderr follows the `--output` destinations too. Destinations append
by default; `--open-mode truncate` truncates them before execution.
The run's saved hosts and diagnoses belong to the latest attempt, so they are
not shown for an older one.

If a runner exits before finalizing its run, `show` reports the interrupted run
and blocks `add`, `copy`, and `run` until you acknowledge it. First confirm
that all jobs have stopped:

```sh
rotari show -p sweep
```

To keep the retained queue for the next run, execute the `rotari unlock`
command printed by `show` (for example, `rotari unlock -r RUN_ID`). To
discard the queue while preserving the interrupted run's history, use:

```sh
rotari reset --recover
```

Without an explicit run ID, `unlock` is also safe to call when the project
does not exist or has no lock/interrupted run: it succeeds without changing
state. With `--run-id`, a missing or mismatched run remains an error.

Use `retry` or result filters when the interrupted run contains completed jobs.
Outside an interrupted run, `rotari reset` simply discards the current queue.
If the project does not exist yet, `reset` (also with `--recover`) initializes
an empty project, so it can safely start a batch-building script.
When output is a terminal, log views (including `--job-id/-j`) longer than 24
lines open in `$PAGER` (or `less -R` by default). Use `--no-pager` to print
directly; piped and redirected output is always printed directly.

### Run lineage

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
carried forward instead of re-executed. Jobs are matched by origin when the
origin points to the compared run; otherwise named jobs are matched by name.
The one-run summary also counts failed jobs by diagnosis, groups them by
cause as `show` does, and reports their source-run origins, including `new`
for jobs without an origin.

### Check run readiness

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

## Diagnosis
**Experimental:** The `diagnose` command/API is an early feature. Its command
options, prompts, supported providers, and response format may change in future
releases.

### LLM error diagnosis
See the [LLM diagnosis guide](LLM_DIAGNOSIS.md) for setup,
provider details, configuration, and execution examples.

### Local rule-based error diagnosis

For common, recognizable failures, rotari runs local rule-based diagnosis when
a failed job is finalized. The saved analysis is informational only: it never
changes job status, retries, dependencies, or scheduler control. View it with:

```sh
rotari show ATTEMPT_ID
```

Every finalized failed job records a recognized diagnosis, an explicit no-match
result, or an analysis-unavailable result when its output cannot be read.
The Web UI shows a `Diagnosis` button beside every job's log button and enables
it when a finalized failed job has saved analysis. Saved analysis is not
updated when the rules change; `show`, reports, and the Web UI note when it was
produced by earlier rules.

To check a saved job manually, run:

```sh
rotari diagnose -j ATTEMPT_ID --rules
```

The `diagnose` command/API is experimental. With `--rules`, it sends nothing
over the network and needs no API key or model. It checks the scheduler error
and recorded output against the documented [local diagnosis
rules](LOCAL_DIAGNOSIS.md), which cover common scheduler, GPU,
distributed-compute, Python, filesystem, network, and HTTP failures.

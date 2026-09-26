# Inspecting and diagnosing

Checking status and logs, checking run readiness, and diagnosing failed jobs.

## Inspect

Use `jobs` to inspect the current project activity and recent execution history across the selected basedir.

```sh
rotari jobs # list running and recently finished jobs across projects; good for a quick status scan
rotari jobs --since 7d # include finished jobs from the last seven days
rotari jobs --all # list jobs across basedirs known to the master registry
rotari jobs --all --format "%s %b %p %a %n %c %t %e" # choose displayed fields
```

Use `show` to inspect a project's runs and pending queue, or a specific run/job.

```sh
rotari show # list projects across known basedirs
rotari show --basedirs # print the resolved master directory and state directories
rotari show -p sweep # list the project's runs and current queue, if non-empty
rotari show -p sweep --failed # list failed jobs in the selected run
rotari show -p sweep --stage train # list only the jobs in stage train of the selected run or queue
rotari show -p sweep --matrix train # list only the jobs of matrix train, named by its base job name
rotari show ATTEMPT_ID # show one job attempt in detail: status, executor, command, and saved output path
rotari show JOB_ID # show a job from the resolved run or queue
rotari show JOB_NAME # show a job by name
rotari show -p sweep --logs # print output logs for every job in the selected run
rotari show -p sweep --failed-logs # print only the logs for failed jobs in the selected project/run
rotari show ATTEMPT_ID --report # print an AI-ready Markdown report for one attempt
rotari show RUN_ID --report # describe the whole run and include recent logs
```

An older `ATTEMPT_ID` shows that attempt's own status, timestamps, and output.
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

Use `retry` or result filters when the interrupted run contains completed jobs.
Outside an interrupted run, `rotari reset` simply discards the current queue.
When output is a terminal, log views (including `--job-id/-j`) longer than 24
lines open in `$PAGER` (or `less -R` by default). Use `--no-pager` to print
directly; piped and redirected output is always printed directly.

### Compare runs

Use `diff` after a fix-and-rerun cycle to see what changed and whether it
worked:

```sh
rotari diff -p sweep            # the latest run against the one before it
rotari diff -p sweep RUN_ID     # RUN_ID against the run before it
rotari diff RUN_A RUN_B         # two specific runs of one project
rotari diff -p sweep --json     # machine-readable comparison
```

To see the whole sequence of runs of a project, oldest first, with each run's
result counts and what changed since the run before it:

```sh
rotari show -p sweep --lineage
rotari show -p sweep --lineage --json
```

`diff` summarizes jobs that were fixed, are still failing, or newly fail; jobs
added or removed; jobs whose command, executor, executor options, environment,
working directory, stage, or dependencies changed; and jobs whose result was
carried forward instead of re-executed. Jobs are matched by name, or by job ID
when they have none. Jobs whose result and definition did not change are
hidden unless `--all` is given; `--json` always lists every job.

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

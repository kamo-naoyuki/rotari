# Inspecting runs and jobs

Checking status and logs, run readiness, and execution history.

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
rotari show # inspect the configured project, or list projects when none is selected
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

Workspace defaults from cwd `.rotari.toml` also select the project and basedir.
They are not inherited from parent directories. Run/attempt IDs still resolve
their own locations through the registry unless an explicit location selector
conflicts; see [Configuration](CONFIGURATION.md#workspace-defaults-and-initialization).

When a run is active and output is a terminal, `show JOB_ID` and
`show ATTEMPT_ID` follow the selected attempt's log automatically. The default
merged log mode follows the combined `output` file, where stdout and stderr
cannot be distinguished. `--log-mode separate` stores them independently, and
`--stream` chooses which one to follow. Output buffered by the job itself
appears only after that program flushes it.

When the selected run is active or interrupted, a non-empty next queue is
shown separately after the run. In JSON, `commands` remains the run snapshot
and `next_queue` contains the queued work; `check` reports its queued count.

`show --logs --failed` (or `--logs --filter-result failed`) selects the same
failed jobs as `--failed-logs`, including output carried from an older run.

The project list shows each project's last run and its result, such as
`failed 7/15` when 7 of its 15 jobs failed. Its suggested commands name the
basedir (`-b BASEDIR`) when a listed project lies outside the default state
directory; with `ROTARI_BASEDIR` set, `show` lists only that directory, so use
`rotari show -b DIR` for another one.

The run/job JSON view includes the resolved run and project, a `jobs` array
with each job's definition, a `finished` flag, and a `result` when available.
For a running job, `finished` is false and no `result` is present. With
`--failed`, the run JSON keeps only the failed jobs' summary results and
failure groups (the `commands` snapshot stays whole), and the job and array
JSON keeps only the failed jobs, as the job table does. Passing
`--run-id latest` selects the latest settled run even when the project has a
non-empty queue or an active run; an unknown job ID fails instead of showing
the queue.

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

### Artifacts

Each attempt also records the files and directories its job definition
refers to, in `artifacts.json` in the attempt directory: paths found in the
command's arguments (such as `train.py` or `--config conf/run.yaml`), in
`--env` and matrix values (a value that looks like a path, or any value of a
variable whose name ends in `_DIR`, `_PATH`, or `_FILE`), bare names after
options such as `--output results` or `--save-dir ckpt` (but not a format name
such as `--output png`), in
`--output`/`--error`, and in the YAML, JSON, or TOML files those name.
Relative paths are resolved on the job's working directory. Files a job
builds in code that discovery cannot see can be declared with
`rotari add --artifact PATH` (repeatable; `change --artifact` replaces and
`change --clear-artifacts` removes the declarations). A declared path may use
`$ROTARI_ARRAY_TASK_ID`, `$ROTARI_JOB_DIR`, and the job's own `--env` and
matrix variables, so each array task or matrix member records its own file:

```sh
rotari add --array 0-9 --artifact 'results/$ROTARI_ARRAY_TASK_ID/plot.png' python train.py
```

These are candidates, found without running anything: rotari does not check that they
exist or tell inputs from outputs, and finding them never affects the job.
Shell code given to `bash -c` (or `sh`, `dash`, `zsh`) is parsed without
running it: its commands' arguments and literal redirection targets such as
`> out/log.txt` are recorded, and `$ROTARI_ARRAY_TASK_ID`, `$ROTARI_JOB_DIR`,
and the job's own `--env` and matrix variables are filled in, so each array
task records its own paths. Shell scripts the job runs, such as
`bash run.sh` or a `.sh` argument, are read and inspected the same way.
Other variables, `$(...)`, and globs are not evaluated. A Python file the
job names, such as `train.py`, is read without running it: the `default` of
each `argparse` `add_argument` and the plain string literals in it are
recorded, but not paths the code builds with f-strings, `.format`, or
`os.path.join`. Code given to `python -c` and the text given to `echo` or
`printf` are not searched. Configuration
files are read on the host that runs `rotari run`, also for SSH and scheduler
jobs; a file that host cannot read is skipped.

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

`missing` only means the path is not there for this host: an SSH job's files
may exist on its own host. `rotari show -j ATTEMPT_ID --artifacts` lists all
of them, with notes on files discovery could not read, instead of the logs,
and `show -j JOB --json` includes the same listing as `artifacts`. In the
Web UI, a job row's Artifacts button shows the same listing for the attempt
its Output button shows, also in a static export, where the listing
describes the files as they were when the export was made.

In the live Web UI, a listed file or directory under the job's working
directory can be opened: images are shown, audio and video play, NumPy
`.npy` and `.npz` files show each array's dtype, shape, and first values,
CSV and TSV files show as tables, logs
and `.txt` files from the end, other text from the start, each loading more
on demand, and a directory as its immediate children, 200 at a time. Any
file can be downloaded. Opening a file or directory automatically scrolls
to its preview below the listing; loading more content does not move the view.
A path outside the working directory, or a symlink
leading out of it, is listed but not opened; add a directory with
`rotari web --artifact-root DIR` to allow it. A static export contains no
file contents unless it is made with `--static-artifact-contents`, which
copies previewable files from the jobs' working directories (up to 10 MiB
each, 100 MiB in all) so previews work in the export; anyone who can read the
export can then read those files. A carried
job shows the candidates of the attempt that produced its result. Runs from
before this record existed show `(not recorded)`.

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

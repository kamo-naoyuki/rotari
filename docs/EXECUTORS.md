# Executors and schedulers

Running jobs locally, over SSH, or on Slurm, PBS, or LSF, and defining array and matrix jobs.

## Scheduler

Each job can choose its execution backend and backend-specific options:

```sh
rotari add -p sweep ./prepare-data.sh
rotari add -p sweep \
  -e slurm \
  --executor-option="-p gpu --gres=gpu:1 --cpus-per-task=8" \
  ./train.sh
rotari run -p sweep --local-concurrency 4 --batch-concurrency 8
```

Local jobs and scheduler-backed jobs may be mixed in the same queue. Use
`--local-concurrency` for local jobs and `--batch-concurrency` as the default
submission limit for non-local execution backends. Use `--ssh-concurrency`,
`--slurm-concurrency`, `--pbs-concurrency`, or `--lsf-concurrency` for
backend-specific limits.
`--batch-concurrency` only limits how many jobs rotari submits and tracks
concurrently. It does not change the scheduler's own queue priority or
execution limits; after submission, the scheduler decides whether each job is
`pending`, `running`, or in another state.
`--executor-option` is the common dispatch option list. Use `--ssh-options`,
`--slurm-options`, `--pbs-options`, or `--lsf-options` for backend-specific
options. Backend-specific settings take precedence over common dispatch
settings, while job-specific executor options take precedence over both.

Use `--env KEY=VALUE` with `add` to save environment variables on a job. They
are exported for every executor, including local, SSH, Slurm, PBS, and LSF, and
are preserved when the job is copied or retried. `rotari change --env KEY=VALUE`
replaces the job's saved environment; repeat it for multiple variables, or use
`--clear-env` to remove them. Rotari's own `ROTARI_*` context variables take
precedence over a same-named user value.

### Job timeouts

`add --timeout` is enforced by rotari's job wrapper on the node that runs the
job, for local, SSH, Slurm, PBS, and LSF jobs alike, and counts only running
time. On schedulers it does not set or replace walltime options such as Slurm
`--time`, PBS `-l walltime`, or LSF `-W`; keep those when the scheduler needs
them. See [Job timeouts](RUNNING.md#job-timeouts).

### SSH executor

For `ssh`, the first `--executor-option` is the destination and subsequent
options go to `ssh`; `--working-directory` is a remote directory and can be
changed later with `rotari change`. Rotari records output, exit status, and
host locally. The remote host needs Linux `/proc`, `setsid`, and standard
command-line tools. Each remote job runs in its own process group;
cancellation reconnects over SSH and sends `SIGTERM` only when the recorded PID
still has the same process start time, so a reused PID is never signalled.

```sh
rotari add -p sweep \
  -e ssh \
  --executor-option="user@gpu-01" \
  --executor-option="-p 2222" \
  --working-directory=/work/sweep \
  --env DATASET=dev \
  --env CUDA_VISIBLE_DEVICES=0 \
  ./train.sh
rotari run -p sweep
```

### Array and matrix jobs

Array jobs can be added with a numeric range or a comma-separated task list:

```sh
rotari add --array 1-10 -e local ./train.sh
rotari add --array 1-10 -e slurm ./train.sh
rotari add --array 1,3,4 -e slurm ./train.sh
```

Each task is tracked separately. Local execution starts one process per task;
Slurm submits native scheduler arrays for both complete ranges and sparse task
lists. PBS and LSF submit native arrays when the complete range is selected;
sparse task lists are submitted as independent jobs for those executors. For
each array task, rotari exposes:

- `ROTARI_ARRAY_TASK_ID`: current task number
- `ROTARI_ARRAY_FIRST`: first task number in the array
- `ROTARI_ARRAY_LAST`: last task number in the array
- `ROTARI_ARRAY_SIZE`: total number of tasks

Scheduler-backed arrays also map the native index variable into these values,
for example `SLURM_ARRAY_TASK_ID`, `PBS_ARRAY_INDEX`, or `LSB_JOBINDEX`.

The Slurm and PBS executors are integration-tested in CI against a Slurm
container and an OpenPBS container. These tests do not certify compatibility
with every real cluster configuration. The LSF executor is covered by unit
tests using fake scheduler commands, but has not yet been tested against a
real LSF installation.

To register a matrix as independent jobs, repeat `--matrix` on `add`:

```sh
rotari add --job-name train \
  --matrix python=3.10,3.11 \
  --matrix cuda=cpu,cuda \
  -- ./train.sh
```

This registers the Cartesian product as four jobs named like
`train-python3.10-cudacpu`. Each job receives its values as
ordinary environment variables, such as `python=3.10` and `cuda=cpu`.
Matrix jobs have independent job IDs and can be combined with `--array`; the
array is applied to each matrix combination. `--depends-on` can name the
matrix's `--job-name` (for example `--depends-on train`) to wait for every
combination. If `copy`, `remove`, or `change` later touches only part of the
matrix, such dependencies are rewritten to the remaining combination names. `include` and `exclude`
customization is not supported by the version 1 workflow manifest.

# Executors and schedulers

Running jobs locally, over SSH, or on Slurm, PBS, LSF, or Sun Grid Engine.

## Choosing an executor

Select the executor with `--executor EXECUTOR` (short form `-e EXECUTOR`).
It accepts these values:

| Value | Runs the job | Submission command |
| --- | --- | --- |
| `local` | on the machine running rotari | `/bin/sh` |
| `ssh` | on a remote host over SSH | `ssh` |
| `slurm` | as a Slurm job | `sbatch` |
| `pbs` | as a PBS job | `qsub` |
| `lsf` | as an LSF job | `bsub` |
| `sge` | as a Sun Grid Engine job | `qsub` |

Any other value is rejected. The option is accepted by:

* `rotari add`, to set the job's executor;
* `rotari change`, to replace a job's executor;
* `rotari run` and `rotari retry`, to set the executor for jobs that do not
  have their own.

A job's own executor takes precedence over the `run` / `retry` value. When
neither is set, the job runs with `local`. `ROTARI_EXECUTOR` and the
`executor` [configuration](CONFIGURATION.md) key supply the default for
`--executor`.

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
`--slurm-concurrency`, `--pbs-concurrency`, `--lsf-concurrency`, or
`--sge-concurrency` for backend-specific limits.
`--batch-concurrency` only limits how many jobs rotari submits and tracks
concurrently. It does not change the scheduler's own queue priority or
execution limits; after submission, the scheduler decides whether each job is
`pending`, `running`, or in another state.
`--executor-option` is the common dispatch option list. Use `--ssh-options`,
`--slurm-options`, `--pbs-options`, `--lsf-options`, or `--sge-options` for backend-specific
options. Backend-specific settings take precedence over common dispatch
settings, while job-specific executor options take precedence over both.
The list that takes precedence replaces the others rather than adding to
them: a job with its own `--executor-option` gets none of the run's
`--slurm-options`.

Each option value is split into words by shell quoting rules, so these two
jobs pass the same three options to `sbatch`:

```sh
rotari add -e slurm --executor-option="-p gpu --gres=gpu:1 --cpus-per-task=8" ./train.sh
rotari add -e slurm --executor-option=-p --executor-option=gpu \
  --executor-option=--gres=gpu:1 --executor-option=--cpus-per-task=8 ./train.sh
```

Quote a word that contains spaces inside the value, as in
`--executor-option="--comment='lr sweep'"`. The same splitting applies to
the backend-specific options, `ROTARI_EXECUTOR_OPTIONS`, and configuration
values.

Array and matrix jobs are run semantics rather than executor features. The
Slurm, PBS, and LSF executors may optimize a selected contiguous array range
by submitting it as one native scheduler array. SGE, local, and SSH use
independent jobs. See [Array and matrix jobs](RUNNING.md#array-and-matrix-jobs).
For an individual Slurm job, the default scheduler job name is
`rotari show ATTEMPT_ID (COMMAND)`; long command previews are truncated so
the name stays within 128 bytes. Native Slurm arrays keep the shared name
`rotari-array`.

Use `--env KEY=VALUE` with `add` to save environment variables on a job. They
are exported for every executor, including local, SSH, Slurm, PBS, LSF, and SGE, and
are preserved when the job is copied or retried. `rotari change --env KEY=VALUE`
replaces the job's saved environment; repeat it for multiple variables, or use
`--clear-env` to remove them. Rotari's own `ROTARI_*` context variables take
precedence over a same-named user value.

`run` and `retry` default to `--env=ALL`, which propagates the caller's
environment using each executor's native mechanism. Pass `--env=NONE` to
suppress caller variables while retaining job `--env` values and rotari
metadata. Slurm uses `--export=ALL|NONE`, PBS uses `qsub -V` for ALL, and LSF
uses `bsub -env all|none`; SGE uses `qsub -V` for ALL and omits it for NONE.
Rotari compensates for documented LSF exclusions.
`PWD` is set to the job's effective working directory. ALL can propagate
secrets and is not a secret-management facility.

When `add --output` or `add --error` is used with SSH, Slurm, PBS, LSF, or SGE,
`rotari` must be available on the execution host's `PATH` so the job wrapper
can stream logs live to those destinations. Jobs without external destinations
do not invoke this helper.

### Concurrency and scheduler load

`--batch-concurrency` (or `--<executor>-concurrency`) limits how many jobs
rotari keeps submitted to a scheduler at once. When one finishes, the next
ready job is submitted immediately. rotari checks each job's own status file
and spaces its `squeue`, `qstat`, `bjobs`, and accounting queries at least
200ms apart, so waiting on many jobs does not flood the scheduler.

### Job timeouts

`add --timeout` is enforced by rotari's job wrapper on the node that runs the
job, for local, SSH, Slurm, PBS, LSF, and SGE jobs alike, and counts only running
time. On schedulers it does not set or replace walltime options such as Slurm
`--time`, PBS `-l walltime`, or LSF `-W`; keep those when the scheduler needs
them. See [Job timeouts](RUNNING.md#job-timeouts).

### GPU training on Slurm

This job asks Slurm for one GPU on the `gpu` partition and runs a Python
script in a conda environment named `train`:

```sh
cd ~/exp
rotari add -p train \
  -e slurm \
  --executor-option="--partition=gpu --gres=gpu:1 --cpus-per-task=8 --mem=32G --time=12:00:00" \
  --job-name train \
  conda run -n train --no-capture-output python train.py --config configs/base.yaml
rotari run -p train
```

The job starts in `~/exp`, the directory `rotari run` was started from (see
[Workflow and execution environment](CONCEPTS.md#workflow-and-execution-environment)),
so that directory must be on a file system the compute node shares.

Alternatively, activate the environment in the shell that runs `rotari run`:
the default `--env=ALL` exports its `PATH` to the job. With `--env=NONE`, the
job's `PATH` is `/usr/local/bin:/usr/bin:/bin`. Without
`--no-capture-output`, `conda run` holds the output until the job exits, so
`rotari show --follow` sees none while it runs.

### SSH executor

For `ssh`, the first `--executor-option` value is the destination (for example,
`user@gpu-01`); subsequent values are passed to `ssh`. `--working-directory`
specifies a directory on the remote host and can be changed later with
`rotari change`.

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

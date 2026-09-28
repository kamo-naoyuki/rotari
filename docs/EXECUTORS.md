# Executors and schedulers

Running jobs locally, over SSH, or on Slurm, PBS, or LSF.

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

Array and matrix jobs are run semantics rather than executor features. The
Slurm, PBS, and LSF executors may optimize a selected contiguous array range
by submitting it as one native scheduler array; other selections and local or
SSH execution use independent jobs. See [Array and matrix jobs](RUNNING.md#array-and-matrix-jobs).

Use `--env KEY=VALUE` with `add` to save environment variables on a job. They
are exported for every executor, including local, SSH, Slurm, PBS, and LSF, and
are preserved when the job is copied or retried. `rotari change --env KEY=VALUE`
replaces the job's saved environment; repeat it for multiple variables, or use
`--clear-env` to remove them. Rotari's own `ROTARI_*` context variables take
precedence over a same-named user value.

`run` and `retry` default to `--env=ALL`, which propagates the caller's
environment using each executor's native mechanism. Pass `--env=NONE` to
suppress caller variables while retaining job `--env` values and rotari
metadata. Slurm uses `--export=ALL|NONE`, PBS uses `qsub -V` for ALL, and LSF
uses `bsub -env all|none`; rotari compensates for documented LSF exclusions.
`PWD` is set to the job's effective working directory. ALL can propagate
secrets and is not a secret-management facility.

When `add --output` or `add --error` is used with SSH, Slurm, PBS, or LSF,
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

CI also tests execution and remote cancellation over a real, loopback-only
OpenSSH server with a disposable key. These tests verify SSH transport and
authentication in addition to the ordinary executor tests that replace the
`ssh` command with a local stub.

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

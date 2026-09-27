# Comparison with nearby tools

This note compares rotari with the tools it most directly replaces: shell
background jobs, GNU Parallel, pueue, task-spooler, submitit, and hand-written
scheduler submissions. It is based on clones of
[pueue](https://github.com/Nukesor/pueue) (2026-09-09),
[task-spooler](https://github.com/justanhduc/task-spooler) (the GPU fork,
2024-01-19), and [submitit](https://github.com/facebookincubator/submitit)
(2026-01-14), taken on 2026-09-27, and on the
[GNU Parallel manual](https://www.gnu.org/software/parallel/man.html).
Workflow engines are compared in
[DAGU_COMPARISON.md](DAGU_COMPARISON.md).

## Summary

Rotari belongs with these tools, not with workflow engines. Like them, it runs
commands you already have, in the environment you already have, with no
workflow definition to write. What it adds is the batch as a unit with a
history: a queue of jobs is run as a run, and fixing and rerunning part of it
creates the next run, so the batch can be corrected step by step while every
earlier result stays inspectable.

| | Unit of work | Where it runs | Rerunning failures | Batch history |
| --- | --- | --- | --- | --- |
| Shell `&` + `wait` | a process | local | by hand | none |
| GNU Parallel | one command template over inputs | local, SSH | `--retry-failed` reruns the joblog's commands unchanged | one joblog per invocation |
| pueue | a task in a daemon's queue | one machine | `restart --all-failed`, optionally `--edit` | task list until cleaned |
| task-spooler | a task in a per-user server's queue | one machine, GPU-aware | add the command again | finished-task list, capped |
| submitit | a Python function call | Slurm, local | resubmit from Python | job folders |
| `sbatch --array`, `queue.pl` | a script and an index | one scheduler | resubmit chosen indices | scheduler accounting |
| rotari | a job in a queue, run as a batch | local, SSH, Slurm, PBS, LSF | `retry` starts a new run of failed and unfinished jobs, after `change` if needed | every run, with `diff` between runs |

## Tool by tool

### Shell background jobs

`cmd &` and `wait`, `nohup`, `xargs -P`, and terminal multiplexers are what
most batches start with. They need nothing installed and inherit the shell's
directory and environment. They keep no status: which jobs failed has to be
reconstructed from exit codes and logs the script chose to write, and a rerun
means editing the script to skip what already succeeded.

### GNU Parallel

GNU Parallel runs one command template over many inputs, locally or on SSH
hosts (`--sshlogin`), and is the most capable of these tools at spreading one
kind of task. `--joblog` records each job's exit status, `--retries` retries a
job, possibly on another host, and `--retry-failed` reruns the failed jobs
using the commands recorded in the joblog, ignoring the command line.
`--resume-failed` reruns them with the arguments from the command line.
There are no dependencies between jobs and no cluster scheduler support.
Local jobs inherit the shell's environment; remote jobs start in the login
directory unless `--workdir` is given, with variables carried by `--env`.

Choose it when the batch is one command over a list of inputs and the
machines are reachable by SSH.

### pueue

pueue keeps a persistent queue of shell commands in a daemon (`pueued`) on one
machine. It has groups with their own parallelism, dependencies (`add
--after`), delays, pause and resume, and restart of failed tasks, optionally
editing a task's command or directory first (`restart --edit`). Each task
records the working directory and a copy of the environment at `add` time, so
a restart runs where and with what it was added. Its README states that it is
built for human interaction and not for hundreds of tasks.

Choose it for a personal, long-lived queue of heterogeneous commands on one
machine.

### task-spooler

`ts` queues commands in a per-user server started on demand, runs a set number
at once, and in the GPU fork allocates free GPUs (`--gpus`) and sets
`CUDA_VISIBLE_DEVICES`. Dependencies are `-D` (after the given jobs end) and
`-W` (after they succeed). A job runs in the process of the `ts` call that
queued it, so it has that call's directory and environment. There is no
command to rerun failed jobs, and the server keeps a capped list of finished
jobs rather than a persistent history.

Choose it for a GPU workstation shared by a few experiments at a time.

### submitit

submitit submits Python functions, not commands, to Slurm (or runs them
locally) through a `concurrent.futures`-style interface, and returns their
results and exceptions to the caller. It handles preemption and
checkpointing. Commands need wrapping in `CommandFunction`, and rerunning
failures is up to the calling Python code.

Choose it when the experiment driver is a Python program and the jobs are
functions in it.

### Hand-written scheduler submissions

A loop over `sbatch`, an array job, or Kaldi's `queue.pl` uses the scheduler
directly. The scheduler knows each job's state, but the batch as a whole
exists only in the script: finding the failed indices, resubmitting them, and
keeping track of what changed between attempts is manual, and the script is
tied to one scheduler.

## Where rotari differs

- **The run is the unit of history.** Every `run` or `retry` snapshots the
  queue and records each job's result, so `show` and `diff` answer which jobs
  a fix repaired, which still fail, and which definitions changed. The other
  tools track individual tasks or one invocation.
- **Fixing is part of the loop.** `change` edits jobs, `copy` brings jobs of an
  earlier run back into the queue, and `retry` reruns only failed and
  unfinished jobs while carrying successful ones forward. A failed result can
  also be accepted after review.
- **One queue, several backends.** The same queue runs locally, over SSH, or on
  Slurm, PBS, or LSF, with array and matrix jobs, and status and logs look the
  same on each.
- **The environment binds at run time.** pueue and task-spooler bind a task's
  directory and environment when it is added; rotari takes them from the shell
  that runs `rotari run` or `rotari retry`, so a batch can be rerun after
  `cd` or activating another environment without editing it. See
  [Workflow and execution environment](CONCEPTS.md#workflow-and-execution-environment);
  how far this reaches on remote executors is still open.
- **No resident daemon.** Each run has its own supervisor process for its
  lifetime; state is files.

Rotari does less than these tools in places: it has no GPU allocation like
task-spooler, no group-level queues like pueue, and no input-driven fan-out as
flexible as GNU Parallel's replacement strings.

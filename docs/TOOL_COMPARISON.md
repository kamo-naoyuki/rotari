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
workflow definition to write. What it adds is a history: a script of
`rotari add` lines stays the definition of the batch, and each time it runs,
rotari keeps that run's commands, every job's status, and its logs, on
whichever backend it ran, and can rerun only the jobs that failed.

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

- **The run is the unit of history.** Every run keeps its commands and each
  job's status and logs, so `show` answers what ran and what failed long
  after the terminal is gone, and `diff` compares two runs. The other tools
  track individual tasks or one invocation.
- **Rerunning only what failed.** Across many hosts, a correct command can
  still fail because one node misbehaved. `run --retry N` retries a failed
  job within the run, and `rotari retry` starts a new run of only the failed
  and unfinished jobs, carrying the successful ones forward. GNU Parallel's
  `--retry-failed` is the closest, for one invocation's joblog.
- **Editing in rotari is optional.** `change`, `copy`, and manifests edit a
  batch inside rotari, but most batches are simply rerun from their script.
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

Rotari does less than these tools in places: it has no group-level queues
like pueue and no input-driven fan-out as flexible as GNU Parallel's
replacement strings. Resource allocation, such as task-spooler's GPU
assignment, is out of scope on purpose: it belongs to the scheduler or other
middleware below rotari, and rotari only limits how many jobs it runs or
submits at once.

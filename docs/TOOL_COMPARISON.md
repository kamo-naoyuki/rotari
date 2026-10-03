# Comparison with other tools

This note compares rotari first with the tools it most directly replaces:
shell background jobs, GNU Parallel, pueue, task-spooler, submitit, and
hand-written scheduler submissions. It then compares it with workflow
engines, which are often mentioned alongside it but do a different job. The
first part is based on clones of
[pueue](https://github.com/Nukesor/pueue) (2026-09-09),
[task-spooler](https://github.com/justanhduc/task-spooler) (the GPU fork,
2024-01-19), and [submitit](https://github.com/facebookincubator/submitit)
(2026-01-14), taken on 2026-09-27, and on the
[GNU Parallel manual](https://www.gnu.org/software/parallel/man.html).

## Summary

Rotari belongs with these tools, while also offering a small manifest format
and a Python client for constructing and inspecting batches. Like them, it
runs commands you already have, in the environment you already have. What it
adds is a history: each run keeps its commands, every job's status, and its
logs, on whichever backend it ran. A batch can be built from shell commands,
a YAML/JSON/TOML manifest, or Python, then retried, edited, or reconciled
without turning rotari into a workflow engine.

| | Unit of work | Where it runs | Rerunning failures | Batch history |
| --- | --- | --- | --- | --- |
| Shell `&` + `wait` | a process | local | by hand | none |
| GNU Parallel | one command template over inputs | local, SSH | `--retry-failed` reruns the joblog's commands unchanged | one joblog per invocation |
| pueue | a task in a daemon's queue | one machine | `restart --all-failed`, optionally `--edit` | task list until cleaned |
| task-spooler | a task in a per-user server's queue | one machine, GPU-aware | add the command again | finished-task list, capped |
| submitit | a Python function call | Slurm, local | resubmit from Python | job folders |
| `sbatch --array`, `queue.pl` | a script and an index | one scheduler | resubmit chosen indices | scheduler accounting |
| rotari | a job in a queue, run as a batch | local, SSH, Slurm, PBS, LSF, SGE | `run --retry`, `retry`, or manifest/fingerprint matching can reuse or rerun selected work | every run, with lineage across related runs |

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
machine. Rotari also provides persistent run history, cross-backend
execution, and a web interface, but pueue remains the simpler choice when a
daemon queue is the main requirement.

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
functions in it. Rotari's Python client is different: it controls the
`rotari` executable and submits command argument lists, so it preserves the
same command-based model as the CLI rather than serializing Python functions
or closures.

### Hand-written scheduler submissions

A loop over `sbatch`, an array job, or Kaldi's `queue.pl` uses the scheduler
directly. The scheduler knows each job's state, but the batch as a whole
exists only in the script: finding the failed indices, resubmitting them, and
keeping track of what changed between attempts is manual, and the script is
tied to one scheduler.

## Where rotari differs

- **The run is the unit of history.** Every run keeps its commands and each
  job's status and logs, so `show` answers what ran and what failed long
  after the terminal is gone, and `lineage` follows related runs and their
  carried-forward results. The other tools track individual tasks or one
  invocation.
- **Rerunning and reconciling work.** Across many hosts, a correct command
  can still fail because one node misbehaved. `run --retry N` retries a failed
  job within the run, and `rotari retry` starts a new run of only the failed
  and unfinished jobs. Fingerprint matching can carry successful results into
  a newly built queue, while exported manifests can accept, edit, remove, or
  rerun individual jobs. GNU Parallel's `--retry-failed` is the closest
  simpler analogue, for one invocation's joblog.
- **Editing in rotari is optional.** `change`, `copy`, manifests, and the
  Python client can edit or construct a batch inside rotari, but a shell
  script remains a natural way to build one.
- **One queue, several backends.** The same queue runs locally, over SSH, or on
  Slurm, PBS, LSF, or SGE, with array and matrix jobs, and status and logs look the
  same on each.
- **The environment binds at run time.** pueue and task-spooler bind a task's
  directory and environment when it is added; rotari takes the caller's
  environment at `run` or `retry` time by default, while retaining saved job
  variables and rotari metadata. `--env=NONE` suppresses caller variables.
  The setting is mapped to the native mechanisms of SSH, Slurm, PBS, LSF, and SGE;
  saved `--env KEY=VALUE` variables work on every executor.
- **Operations beyond the CLI.** `rotari web` provides history, logs, job
  control, and multiple-basedir monitoring. Failed jobs are
  diagnosed with local rules, and notifications can deliver webhook or
  browser alerts.
- **No resident daemon.** Each run has its own supervisor process for its
  lifetime; state is files.

Rotari does less than these tools in places: it has no group-level queues
like pueue and no input-driven fan-out as flexible as GNU Parallel's
replacement strings. Resource allocation, such as task-spooler's GPU
assignment, is out of scope on purpose: it belongs to the scheduler or other
middleware below rotari, and rotari only limits how many jobs it runs or
submits at once.

## Workflow engines

Workflow engines start from a definition of the workflow, written in a DSL,
YAML, or Python, and run it the same way each time. Rotari now accepts
manifests too, but its manifest describes jobs and their execution settings,
not a file-driven pipeline. Most workflow engines also define
where and with what each step runs, and several start runs by themselves from
schedules or events. This is the right design for pipelines that are shared,
reproduced, or operated, and it is what rotari leaves out.

| | Workflow defined as | Runs start from | Where steps run | Environment | Rerunning failures |
| --- | --- | --- | --- | --- | --- |
| [Snakemake](https://snakemake.github.io/) | rules with input and output files (Python-based DSL) | the CLI | local; Slurm, LSF, SGE, Kubernetes, and cloud batch services through executor plugins | per-rule conda environments or containers, or the calling shell | reruns jobs whose outputs are missing or out of date |
| [Nextflow](https://www.nextflow.io/) | processes and channels (Groovy-based DSL) | the CLI | local, Slurm, PBS, LSF, SGE, and other HPC schedulers, Kubernetes, and cloud batch services | per-process containers or conda environments | `-resume` reuses cached task results |
| [Dagu](https://dagu.sh/) | a YAML DAG | the CLI, Web UI, API, cron, and event triggers | local, SSH, containers, Kubernetes, distributed workers | the working directory, variables, and container in the YAML | step retry policies and `dagu retry` of a run |
| [Airflow](https://airflow.apache.org/), [Prefect](https://www.prefect.io/), [Dagster](https://dagster.io/) | Python code | a scheduler, sensors, the UI, or the API | workers on the configured infrastructure | each task's operator or deployment settings | task retries and rerunning from the failed task |
| rotari | shell commands, YAML/JSON/TOML manifests, or Python command lists | the user, from the CLI, Python, or API | local, SSH, Slurm, PBS, LSF, SGE | the shell that runs `rotari run`, or explicit job/run settings | `run --retry`, `retry`, fingerprint matching, and manifest reconciliation |

What separates rotari from all of them:

- **A batch definition, not a pipeline engine.** A shell script, manifest, or
  Python program can build the batch. Rotari supports job dependencies,
  arrays, matrices, retries, and reconciliation, but it does not provide
  file-based dependency discovery, conditional workflow logic, or general
  output passing between jobs.
- **No workflow triggers.** There is no built-in scheduler, cron, or sensor;
  every run is started by a person or an agent through the CLI, Python client,
  or API. The Web UI can inspect and control runs, but does not turn rotari
  into an event-driven workflow engine.
- **No service to operate.** There is no resident server or database, and
  on a cluster the site's scheduler does the placement and resource
  allocation.

Choose a workflow engine when the pipeline itself is the product: shared with
others, rerun on new data, or operated on a schedule. Rotari is not a step
toward one; it is for batches that people run themselves from scripts. Among
them:

- **Snakemake** when the pipeline is driven by files and its structure is
  worth formalizing.
- **Nextflow** for pipelines that are shared, reproduced, and run at scale,
  with containers, clusters, or cloud.
- **Dagu** for a lightweight, single-binary workflow engine with a Web UI,
  cron, and event triggers, if you are happy to describe workflows in YAML.
  If that is what you want, it is likely a better choice than rotari.
- **Airflow, Prefect, or Dagster** for production pipelines that run on a
  schedule and need monitoring.

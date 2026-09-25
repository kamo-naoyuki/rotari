# Dagu comparison and direction

This note compares rotari with [Dagu](https://dagu.sh/), based on a clone of
the Dagu source tree taken on 2026-09-25, and sets the direction rotari should
take in response.

## Summary

rotari and Dagu share a problem space: running existing commands and scripts
repeatedly while managing dependencies, logs, retries, and history. They start
from different places.

- Dagu is a workflow engine. A YAML DAG defines the workflow and its execution
  environment, and the project includes a scheduler, Web UI, REST API, MCP
  server, and distributed workers.
- rotari is an execution manager for batches of experiments that run in an
  environment that already exists: a workstation, SSH hosts, or a shared
  Slurm, PBS, or LSF cluster. Queues are built from the CLI, not from a
  workflow file.

Dagu is a much larger project (roughly 320k lines of non-test Go against
rotari's 21k, and years of history). rotari will not win by matching its
feature list. Dagu fits workflows that are defined once and then operated;
rotari fits the stage before that, when a batch is still being changed. It
should aim to be clearly better on three related axes:

1. **The fix-and-retry loop** for batches of experiment jobs. This is rotari's
   core: what it does.
2. **Depth on shared HPC schedulers.** This is where the loop runs.
3. **Operability by coding agents** such as Claude Code or Codex. This is who
   runs the loop besides a human.

AI integration is treated as a way to strengthen these axes, not as a
separate feature area to compete in. Containers are not a first-class concept.

## What Dagu provides

### The YAML DAG is the center

Dagu loads YAML, normalizes it into an IR, and runs it through its runtime and
executors. The CLI, REST API, Web UI, and scheduler all launch the same
workflow definition.

`dagu exec -- <command>` runs a one-off command without a YAML file, but it
creates a single-step run. There is no CLI path for building a multi-job queue
incrementally, which is rotari's normal starting point.

`dagu retry --run-id ID [--step NAME] [--downstream]` re-runs a previous run,
optionally from one step. Fixing a failed step means editing the YAML
definition; per-job command changes that keep the history of the original
command are not a core concept.

### Execution environments are part of the workflow

Dagu steps can run as:

- plain shell commands
- Docker or Podman containers, or `exec` into an existing container
- Kubernetes Jobs
- remote commands over SSH
- built-in actions such as HTTP, SQL, S3, Redis, mail, and git

Container and Kubernetes settings (image, volumes, network, resources, service
account, node selectors, tolerations, and so on) are part of the step
definition. Custom executors can be registered through the public
`dagu.RegisterExecutor` API.

The clone contains no Slurm, PBS, or LSF executor. Dagu can drive a cluster
through SSH or plain commands, but it does not track `sbatch`, `qsub`, or
`bsub` job IDs as scheduler state.

### The control plane ships in one binary

Dagu uses file-backed state with no external database or message broker, and
still includes:

- a scheduler with cron, timezones, catch-up, and overlap policies
- queues, retries, webhooks, notifications, and human tasks
- an HTTP API, SSE, Web UI, and MCP server
- a coordinator with gRPC-connected distributed workers and label-based routing
- Helm charts for standalone and distributed Kubernetes deployments

### AI integration puts agents inside the workflow

Dagu's AI features come in three layers:

1. **`chat.completion`**: an LLM call as an ordinary step. Earlier step outputs
   are templated into the prompt and the reply becomes a step output.
2. **`harness.run`**: an external coding agent CLI (Claude Code, Codex, Gemini
   CLI, Cursor, and others) run as a step, with provider fallback, logs, exit
   codes, and retries handled like any other step. The agent can run on the
   host or in a container.
3. **`type: agent` DAGs**: `steps` becomes a catalog of allowed actions and
   `tasks` describes the goal. An LLM picks one action per turn, observes the
   result (including failures), and continues until the goal is met. It can
   pause for `human.task` approval and resume with the same conversation.

The implementation is built for real operation: ordered model fallback,
retries on transient provider errors that do not consume agent turns, limits
on tool-result size, compaction of old observations, and persisted
conversation state across suspension.

The design principle worth learning from is that agent activity flows through
the same step execution, logs, history, and approval machinery as everything
else, and that agents can only choose actions the author declared.

## Where rotari differs

### Existing environments come first

In research environments the execution environment usually exists already:

- host modules, conda environments, and CUDA installations
- shared filesystems such as NFS, Lustre, or GPFS
- shared batch schedulers such as Slurm, PBS, or LSF
- large collections of existing shell scripts and research CLIs

Redefining that environment as container images or Kubernetes resources to
satisfy a workflow engine is often more work than it saves. rotari adds
queueing, parallelism, logs, status, retries, and run history on top of the
environment as it is.

### The queue is built from the CLI

`rotari add` puts commands on the current queue and `rotari run` runs them.
Manifests exist for exporting and importing a queue, but they are not the
starting point. This matters for humans who already have shell scripts, and it
matters even more for coding agents, which are good at issuing CLI commands and
worse at writing and maintaining workflow files correctly.

### Failures are fixed per job and history is kept

When some jobs fail, `rotari change` updates only those jobs' commands or
executor options and a rerun leaves successful jobs alone. The previous
attempts stay in the run history.

### Runs are versions of the experiment

Dagu and Airflow also have runs, but there a run is one execution of a fixed
definition, and the definition's history lives separately in YAML or code under
version control. In rotari, a run is a snapshot of the queue as it was edited
at that moment (`runs/<run-id>/commands.json`). Commands change between runs,
and each carried-forward job records which run and attempt it came from
(`JobOrigin`). The sequence of runs is therefore the version history of the
experiment itself.

### Positioning

```text
Dagu
  YAML workflow
      + execution environment
      + scheduler / triggers
      + Web UI / control plane
      + optional distributed workers
      + agents running inside the workflow

rotari
  existing commands and shell scripts
      + current host or shared HPC environment
      + queue / parallel execution
      + fix-and-retry with preserved history
      + a CLI that humans and coding agents drive directly
```

## Direction

### Axis 1: the fix-and-retry loop for batches

#### Runs as the history of the loop

The data for treating runs as experiment versions already exists: per-run
command snapshots, per-job `JobOrigin`, and a queue-versus-run change count in
`show`. What is missing is a way to look at it.

- List a project's run lineage, similar to `git log`: run name, time, success
  and failure counts, jobs added, removed, or changed since the previous run,
  and jobs carried forward.
- Compare two runs (for example `rotari diff RUN_A RUN_B`): per job, changes in
  command, executor options, and environment, together with status changes
  such as failed to succeeded. This shows which change made a job pass.
- Summarize a run: what it fixed, what still fails, what newly fails, and how
  long it took, plus scheduler resource usage once Axis 2 collects it.
- Show the run lineage as a timeline in the Web UI, next to the matrix grid and
  per-job attempt history.

Run comparison is also the foundation for AI Phase 2: once rotari can diff
runs and attempts, diagnosis can use those differences as evidence.

#### Batch-level failures

Experiment runs often come as array or matrix jobs where many jobs fail for a
few reasons. Dagu's agents reason about one step at a time; rotari should
reason about the batch.

- Group failed jobs by cause. Rule-based diagnosis runs first; an LLM, when
  configured, summarizes each group and proposes a fix.
- Let one reviewed change apply to a whole group, with `--dry-run` showing the
  affected jobs and the resulting retry set.
- Compare a failed attempt with a successful attempt of the same job, or with
  successful siblings in the same matrix, and use the differences in command,
  environment, node, and resources as diagnosis evidence.

### Axis 2: depth on shared HPC schedulers

This is the axis Dagu does not cover and is unlikely to prioritize. rotari
already submits and tracks Slurm, PBS, and LSF jobs and tests them in CI.
Deepen that:

- Collect scheduler accounting beyond state and exit code. Today Slurm polling
  reads only `State,ExitCode` from `sacct`; add peak memory, elapsed time,
  requested resources, node, and scheduler-reported reasons such as
  `OUT_OF_MEMORY` or `TIMEOUT`, with equivalent data from PBS and LSF where
  available.
- Record that data per attempt so it can be displayed, compared, and used by
  diagnosis.
- Handle scheduler-specific retry cases explicitly, such as preemption,
  node failure, and requeue.
- Make migration from Kaldi/ESPnet-style `run.pl` and `queue.pl` scripts
  straightforward.

### Axis 3: operability by coding agents

Instead of running agents inside rotari, make rotari the tool an agent uses to
run experiments. Because queues are built from the CLI, an agent can use
rotari without writing workflow files.

- Provide `--json` output consistently across inspection and mutation commands
  (it already exists on many of them) with a stable schema.
- Give exit codes stable, documented meanings.
- Make blocking and polling straightforward (`rotari wait`), so an agent does
  not need its own sleep loops.
- Ship a short agent-facing usage guide (for example, an AGENTS.md snippet or
  an agent skill) describing the safe workflow: add, run, wait, show, diagnose,
  change, retry.
- Keep destructive operations explicit so an agent cannot silently discard
  history.
- Work inside agent sandboxes. The run server's socket lives at
  `<basedir>/server.sock`, which fails when the path exceeds the 108-byte Unix
  socket limit (common under deep agent workspaces) and would fail entirely
  where a sandbox blocks Unix sockets. `run` and job control need a path that
  works there.
- Survive agent command timeouts. Synchronous `run` treats client disconnect
  as cancellation, so the agent guide must use `run --async` and `wait`.

A representative demo: a coding agent submits a 100-job sweep to Slurm, waits,
finds that 12 jobs ran out of memory, raises their memory request with one
reviewed change, and retries only those jobs.

### AI roadmap

Each phase is useful on its own and feeds the three axes above.

#### Phase 1: machine-readable diagnosis

- Define a structured diagnosis result: `cause`, `evidence`,
  `suggested_action`, `confidence`, and `retryable`.
- Emit the same structure from rule-based and LLM diagnosis.
- Keep the Markdown view, and expose JSON through the CLI and API.
- Keep the existing redaction, log-tail limits, and the rule that API keys are
  never stored in state.

#### Phase 2: scheduler-aware and batch-aware evidence

- Feed the per-attempt scheduler accounting from Axis 2 into diagnosis.
- Group failures across a run by cause, as described in Axis 1.
- Include run, attempt, and sibling-job differences as evidence, built on the
  run comparison from Axis 1.

#### Phase 3: proposals

- Add a read-only proposal command, for example
  `rotari propose --run-id RUN_ID [--job-id JOB_ID | --group GROUP]`.
- A proposal lists the targets, the before and after values, the reason, and
  the expected risk. It never changes the queue.
- Resource changes are expressed as executor options and must pass the
  existing executor validation.

#### Phase 4: approval and application

- Apply an approved proposal through the same validation as `rotari change`.
- `--dry-run` shows the changed queue and the retry set.
- Record the proposal, the original values, who applied it, and when, in the
  run history.

#### Phase 5: bounded closed-loop retry

- Only when explicitly enabled, apply a proposal and retry once.
- Limit the number of attempts, the fields that may change, and the executor
  options that may be set.
- Stop when the same failure repeats rather than letting the model iterate
  without bound.
- Link every decision, change, and result to the attempt history.

## Out of scope

These Dagu features are strong, but they do not serve rotari's chosen axes:

- **First-class containers and Kubernetes.** HPC sites typically use
  Apptainer/Singularity, and `rotari add -- apptainer exec image.sif ...`
  already works without rotari knowing about containers.
- **Coding agents as a special job type** (Dagu's `harness.run`). Running
  `rotari add -- claude -p ...` already covers this; rotari focuses on being
  driven by agents instead.
- **Autonomous agent DAGs** with a YAML goal and action catalog, and free
  selection or parallel execution of arbitrary actions.
- **A coordinator with resident distributed workers.** Shared clusters already
  have a scheduler.
- **Unapproved automatic changes** to commands, dependencies, or resource
  requests.
- **Cron-style scheduling and event triggers** as a core feature.

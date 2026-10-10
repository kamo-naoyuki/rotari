# Plan: rotari as a Shared Experiment Record for Agents and Humans

**Created:** 2026-10-10
**Status:** Phase 1 trial 1 done ([report](trial-2026-10-10-reconstruction.md)); Phase 2 not started
**Related:** [Agent-facing MCP interface](../2026-10-02-mcp-agent-interface/plan.md)

## Purpose

An agent that runs experiments by calling scripts directly leaves no record
unless it decides to keep one: which commands ran, with which settings and
code, why, and with what result. Running them through rotari records this
without the agent having to think about it. The goal is to make rotari a
platform where an agent and a human share one experiment environment: either
can see what the other ran, why, and what came of it, and continue from there.

rotari is worth using only if it is clearly more convenient than letting the
agent run the experiment directly. That has two sides:

- **Cost to the agent:** using rotari takes no more effort than running the
  scripts itself. The [MCP interface plan](../2026-10-02-mcp-agent-interface/plan.md)
  and its agent trials measured this side (calls and output size).
- **Value of the record:** after the agent's work, a human can tell from
  rotari alone what was done, why, with which code, and which result was
  best; and an agent can pick up a human's experiment the same way. No trial
  has measured this side yet.

rotari has no users yet, and the user has not run it in practice, so the
evidence for this plan comes from trials, not from operations.

## What a run records today

Checked at `ba2b937b` by running `rotari add --env LR=0.1 -- sh train.sh`
and `rotari run --run-name try1` in an empty state directory.

Recorded:

- the command snapshot (`commands.json`), with each job's argv, its own
  environment values, name, and settings;
- per attempt: the command, logs (`output`), exit status, submit and finish
  times, the host, and diagnoses;
- the run's name, working directory, hostname, launch process, load, and a
  snapshot of rotari's own merged configuration (`context.json`, `configs/`);
- run lineage: carried and retried jobs and their origins, compared by
  `lineage`;
- the files a command names, as path candidates (`artifacts.json`, here
  `train.sh`).

Not recorded:

- **the code that ran:** the content or a hash of `train.sh`, and the git
  commit or uncommitted changes of the working directory. An agent edits
  scripts between runs, so two runs with the same command may have run
  different code, and the record cannot tell them apart;
- **why a run was made:** a run has a name, but nowhere to say why it exists
  (for example, "raise the timeout of `train[12]` and retry"), or what its
  author concluded from it;
- **who made it:** an agent's runs and a human's runs look the same.

## Hypotheses

Each is to be confirmed or rejected by the trial below before any code
changes.

1. **Code version.** Without the code that ran, a human cannot tell which
   change produced which result. This may be the largest gap.
2. **Intent.** Without the reason for a run and the conclusion drawn from it,
   a human sees a sequence of runs but not the agent's reasoning.
3. **Readability of an agent's session.** A human cannot follow an agent's
   chain of runs (what changed between them and which was best) in one view
   of the Web UI or `lineage`.
4. **Choosing rotari.** An agent that is told rotari is available, but not
   told to use it, runs the scripts directly. Earlier trials always said
   "using rotari".
5. **Handoff.** An agent can continue a human's experiment from rotari alone
   (scenario s2 of the
   [zero-information trial](../2026-10-02-mcp-agent-interface/agent-trial-2026-10-10-zero-info.md)
   suggests yes), and a human can continue an agent's.

## Approach

### Phase 1: reconstruction trial

Measure the value of the record before changing rotari.

1. **Experiment.** A headless agent, set up like the zero-information trial
   (isolated state and config directories, a logging `rotari` wrapper),
   is given an experiment that needs code changes between runs. For example:
   tune a training script, fix a bug that makes some settings fail, and
   report the best setting. The agent's transcript is kept, but only as the
   answer key.
2. **Reconstruction.** A second agent, and the user where possible, is given
   only rotari (CLI and Web UI) on the same state directory, and answers a
   fixed list of questions:
   - Which runs were made, in what order, and why?
   - What changed between consecutive runs: commands, settings, code?
   - Which code produced the best result, and could it be run again?
   - Which failures happened, and what was done about each?
   - What did the agent conclude?
3. **Scoring.** Compare each answer with the transcript: correct, partly
   correct, or unanswerable from rotari. Each unanswerable question points to
   a gap.
4. **Choice.** Run the experiment twice: once telling the agent to use
   rotari, once only telling it rotari is installed (hypothesis 4).

Hypothesis 5's other direction, an agent continuing a human's experiment, is
covered by the zero-information trial's s2 and is not repeated unless
Phase 2 changes it.

Trial 1 ([report](trial-2026-10-10-reconstruction.md)) confirmed hypotheses
1 and 2, partly confirmed 3 (comparison fails across projects), and rejected
4 for its task: an agent told only that rotari is installed used it for every
run. No question about code or intent could be answered from the record
except by forensic inference that a real project would not support.

### Phase 2: close the gaps the trial finds

Decide the work from the trial's unanswerable questions, in order of how many
questions each would answer. Candidates, if the hypotheses hold:

- record the code that ran: a hash of each file the command names, and the
  git commit and dirty state of the working directory; whether to copy file
  contents is a separate decision (size, secrets);
- a run note: a reason given at `run` / `retry` and a conclusion added later,
  shown in `show`, `lineage`, `runs`, and the Web UI;
- marking whether a run was started by an agent;
- a session view: a project's runs in order, with what changed between them.

Each candidate is planned in detail only after the trial confirms it is
needed.

## Decisions

- **Evaluation axis.** Work is chosen by whether it makes the record answer
  the reconstruction questions. Agent efficiency (calls, output size) stays a
  constraint, not the goal.
- **Trial before code.** No Phase 2 work starts before Phase 1 has scored
  the current record.
- **Separate plan.** This plan replaces the MCP interface plan as the place
  where new agent-related work is chosen. That plan's deferred M8 items
  (liveness, interrupted-run guidance) are not carried over unless the trial
  calls for them.

## Non-goals

- Changes to MCP tools; the user cannot connect MCP servers, so agents use
  the CLI.
- A `stuck` verdict or other judgments that the agent should make from facts.
- Recording an agent's whole transcript in rotari.

## Validation

- Phase 1 is done when the trial report gives, for each reconstruction
  question, the score with and without the instruction to use rotari, and
  lists the gaps.
- Each Phase 2 change is validated by rerunning the reconstruction trial and
  showing that the questions it targets become answerable, plus the usual
  package tests, conformance rows for user-visible behavior, and
  `scripts/check.sh`.

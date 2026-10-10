# Plan: rotari as a Shared Experiment Record for Agents and Humans

**Created:** 2026-10-10
**Status:** Phase 1 trial 1 done ([report](trial-2026-10-10-reconstruction.md)); Phase 2: source revisions (RUN-15) and run notes (RUN-16) done; [trial 2](trial-2026-10-10-reconstruction-2.md) answered Q2, Q3, Q5, and Q6 from the record
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

### Phase 3: one run report for people and agents

Notes alone do not let a person follow a run: they are short messages that
assume the writing agent's conversation, shown as plain text. The existing
Markdown report (`show RUN_ID --report`, the Web `Report` button) already
gathers each job's conditions, diagnosis, and log excerpt for an AI. Extend
that report into the run's record for both readers instead of adding a
second one:

```
# rotari run report
- Project, run ID, status, start and finish
- Source: the revision each repository was at (RUN-15)
## Notes               the run's notes, Markdown as written
## Jobs                one row per job: name, matrix and env values,
                       status, exit code, last log line
## Job: NAME           as today, plus
### Notes              that job's notes
### Command / Diagnosis / Log
```

The summary (header, notes, job table) comes first, so a reader can stop
there; the per-job evidence follows.

Who fills what:

| Part | Filled by |
|---|---|
| Job table: name, matrix and `--env` values, status, exit code, last log line | rotari |
| Source revision, and whether the code changed since the run before | rotari |
| Conditions the table cannot show, such as what changed in a config file | the agent, in the run note |
| A job's own condition or reading | the agent, in the job note |
| Results gathered into a Markdown table, one row per job name | the agent, in the run note |
| Conclusion and next step | the agent, in the run note |

rotari does not read config files: it knows a file's path (artifacts) and,
through the source revision, that the repository changed, not what the change
means. Those differences are the agent's to write.

Decided with the user:

1. Every job appears in the job table with its last log line, which for a
   failed job is often its error. The per-job log sections stay limited to jobs that failed or may not have
   finished.
2. Order is summary first, details after, so a long run's report can be read
   from the top.
3. The Web renders the report as Markdown, escaping all text, since notes
   are agent-written. A small renderer in the page does this without a new
   dependency.
4. The report is described as the run's record, no longer as "AI-ready".

The run notes' place on the Web run page is this report; the header list
stays only until the rendered report exists.

After seeing the first cut, the user noted that a log line is a result only
for programs like the trial's `train.py`, which print their final metric last.
So the agent gathers the results: the agent guide asks it to put them, as a
Markdown table with one row per job name, in its concluding run note, where a
reader can check each row against rotari's job table. The job table dropped
its first-log-line column, which helped only programs that print their
configuration first. If agent-gathered results prove unreliable, a job could
write a declared results file for rotari to tabulate; that needs the job's
cooperation and waits for a trial to ask for it.

[Trial 3](trial-2026-10-11-reconstruction-3.md) confirmed it: both agents,
including the one told only that rotari is installed, wrote a results table
in their concluding notes, every number matching the logs, and both readers
used it. Its open finding is that both agents ran uncommitted edits, so the
code of their later runs is again inferred.

## Decisions

- **Record the commit ID, not file hashes or diffs.** A hash or a diff of the
  files a command names cannot cover installed packages, data outside the
  repository, or imported modules, so it would suggest more than it records.
  A commit ID is unambiguous about what it covers. Git records `HEAD` and
  whether tracked files had uncommitted changes; a comparison of such runs
  says the code is unknown rather than unchanged.
- **jj is snapshotted.** For a jj repository, rotari snapshots the working
  copy before reading `@`, so the recorded commit ID covers edits nobody
  committed. This adds one jj operation per run, which the user accepted for
  the exactness. The user is considering jj.
- **Run notes.** A reason for a run and a conclusion are an agent-specific
  feature the user wants; they answer Q2 and Q6, which no automatic record
  can. A note is on a run or on one job attempt, given at `run --note` /
  `retry --note` or added later with `rotari note`, and only ever appended.
  Notes on job definitions (`add --note`) are left out until a trial needs
  them: they would travel with the job through retry and copy and raise the
  question of whether a changed note is a definition change.
- **A run preview's revision guards its plan.** Trial 2 showed an agent
  dropping a previewed `--filter-diagnosis` from the start; the project
  revision still matched, so other jobs ran than previewed. The user agreed to
  make `run`/`retry --dry-run` print `PROJECT_REVISION.PLAN_HASH` and refuse a
  start whose own plan differs. A bare project revision from `check` still
  guards only the state, so earlier scripts keep working.
- **No author on notes.** Whether a person or an agent wrote a note is not
  recorded. Detecting it (`CLAUDECODE=1`) covers only one agent and mislabels
  a person typing in its terminal; the text usually tells, and a later trial
  can show whether it is needed. The user was unsure and accepted leaving it
  out for now.

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

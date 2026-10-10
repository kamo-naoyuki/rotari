# Reconstruction Trial 1

**Date:** 2026-10-10
**Plan:** [plan.md](plan.md), Phase 1
**rotari:** built at `e4d097aa`

## Setup

Built with [trial-fixture.sh](trial-fixture.sh) and run with
[trial-run.sh](trial-run.sh). Each variant has its own state and config
directories, a `rotari` wrapper that logs calls, and a git repository holding
one commit of a toy `train.py`.

The toy script is built so that the best result needs code changes:

- batch size 48 crashes with an `IndexError` (a batching bug);
- learning rate 0.1 diverges to NaN unless the learning rate warms up;
- after the bug fix and a warmup, `lr=0.1, bs=48` is best (0.899).

Two headless `claude -p` agents ran the experiment, with Bash, Read, Edit,
Write, Glob, and Grep and no settings:

| Variant | Instruction about rotari |
| --- | --- |
| rA | "Run the experiments with rotari." |
| rB | "A job runner called rotari is installed." |

The task was otherwise the same: find the best learning rate (0.001, 0.01,
0.1) and batch size (32, 48), fix a bug in `train.py` if a run crashes
because of one, add a one-epoch linear warmup and check whether it helps,
and report the best configuration.

A second agent per variant was then given only rotari and the work directory,
was told not to run or change anything, and answered seven questions
(Q1-Q7 in [trial-run.sh](trial-run.sh)). Its answers were scored against the
experiment agent's transcript. Transcripts and call logs were kept outside
the directories the agents could see; the reconstruction agents' tool calls
show that they read nothing outside the work directory.

## What the experiment agents did

Both completed the task and found `lr=0.1, bs=48` with warmup at 0.899.

| | rA | rB |
| --- | --- | --- |
| rotari calls | 30 | 24 |
| runs | 4 in 2 projects (`sweep`, `sweep-warmup`) | 4 in 2 projects (`sweep`, `warmup`) |
| experiments outside rotari | none | none |
| code changes | batching fix; `--warmup-epochs` with **default 1** | batching fix; `--warmup` flag, off by default |
| git commits | none | none |
| `--run-name` or other notes | none | none |
| cost | 23 turns, $0.55 | 19 turns, $0.45 |

Both followed the same path: a first run that failed entirely (below), a
corrected run, a fix to `batches()` and a `retry` of the `IndexError` jobs,
then the warmup change and a new project for the warmup sweep.

## Scores

Score per question: **yes** (correct from the record), **inferred**
(correct, but only by reasoning that a real project would not support),
or **no** (unanswerable).

| Question | rA | rB | How it was answered |
| --- | --- | --- | --- |
| Q1 runs, order, commands | yes | yes | `runs`, `show`, `lineage`. Neither could tell which command started run 3 (a `retry` filtered by diagnosis). |
| Q2 why each run | inferred | inferred | From the sequence of failures. rotari stores no reason. |
| Q3 what changed, including code | inferred | inferred | Commands: `lineage A B`. Code: traceback line numbers matched against `git show HEAD:train.py`, file modification times against run times, and arithmetic on the toy script's fixed accuracies. |
| Q4 failures and fixes | yes | yes | `lineage RUN` failure groups and logs. Fixes inferred as in Q3. |
| Q5 best result, its code, rerun | inferred | inferred | Config and result: yes. Code: "the current uncommitted working tree, if nobody edits it". Neither could say the code was byte-for-byte what ran. |
| Q6 the agent's conclusion | no | no | Nothing records it. |
| Q7 experiments outside rotari | no | no | Nothing can show what did not go through rotari. |

The reconstruction agents were thorough, but the "inferred" answers depend
on things a real project lacks: a deterministic script whose accuracies can
be recomputed by hand, tracebacks that pin line numbers, and only one person
editing within seconds of each run.

## Findings

Ordered by how much of the record each would make answerable.

1. **The code that ran is not recorded (Q3, Q5).** Both agents edited
   `train.py` between runs and committed nothing. rotari recorded `train.py`
   as a path (`artifacts.json`) but not its content, hash, or the
   repository's commit and changes. Two runs with the same command (runs 2
   and 3 of each variant) ran different code, and the record shows "definition
   changed 0" between them. In rA, the warmup change has a default of 1
   epoch, so rerunning run 2's command today runs warmup and gives different
   numbers; nothing in the record warns of that.
2. **Nothing records why a run was made or what was concluded (Q2, Q6).**
   Neither agent used `--run-name`, and rotari has nowhere for a reason or a
   conclusion. The agents' final messages held both, and both are lost once
   the conversation ends. Recording a note will only help if agents write
   one; the guide does not ask for it.
3. **A session split over projects cannot be compared (Q3).** Both agents
   put the warmup sweep in a new project, and `lineage RUN_A RUN_B` across
   projects fails with "run not found", so the warmup runs could not be
   compared with the runs they followed.
4. **How a run was started is not shown (Q1).** Run 3 came from
   `retry --filter-diagnosis 'Python key or index error'`; the record shows
   which jobs it executed and carried but not the selection.
5. **Agent cost: commands do not run in a shell, and the guide says they
   do.** Both agents wrote `--lr '$LR'` for a matrix value, and their first
   run failed in all six jobs with `invalid float value: '$LR'`. The guide
   opens with "rotari queues shell commands", and its matrix example passes
   the value only through the environment. rA also tried `change -r RUN` to
   fix the command, which failed. This is not a record gap, but it cost each
   agent one run and several calls.

Hypotheses from the plan:

- H1 (code version): confirmed, finding 1.
- H2 (intent): confirmed, finding 2.
- H3 (readability of a session): partly; within a project, `lineage` gave
  both agents the sequence quickly, but across projects it fails (finding 3).
- H4 (choosing rotari): rejected for this task. rB, told only that rotari is
  installed, used it for every run and ran nothing directly. One sample.
- H5 (handoff): not tested in this trial.

## Next

Phase 2 candidates in order: record the code that ran (finding 1), a run
note that agents are asked to write (finding 2), comparison across projects
(finding 3). Finding 5 is a guide fix and can be made separately.

The transcripts and fixtures are in the session's scratchpad, not in the
repository.

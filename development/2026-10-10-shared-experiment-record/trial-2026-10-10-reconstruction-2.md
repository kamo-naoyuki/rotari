# Reconstruction Trial 2

**Date:** 2026-10-10
**Plan:** [plan.md](plan.md), Phase 2
**rotari:** built at `6e727399`, with source revisions (RUN-15) and run notes
(RUN-16)
**Baseline:** [trial 1](trial-2026-10-10-reconstruction.md)

## Setup

The same as trial 1: [trial-fixture.sh](trial-fixture.sh) and
[trial-run.sh](trial-run.sh), the same task, and the same two variants (rA
told to use rotari, rB told only that it is installed). Only rotari changed,
including the agent guide, which now asks for a note when a run starts and
after its results are read, and mentions that `lineage` compares the code
each run used.

## What the experiment agents did

Both completed the task and found `lr=0.1, bs=48` with warmup at 0.899 again.

| | trial 1 rA | trial 2 rA | trial 1 rB | trial 2 rB |
| --- | --- | --- | --- | --- |
| rotari calls | 30 | 29 | 24 | 26 |
| runs | 4 in 2 projects | 4 in 1 project | 4 in 2 projects | 4 in 2 projects |
| notes when a run started | - | 4 of 4 | - | 4 of 4 |
| notes added afterwards | - | 4 | - | 4 |
| git commits of their changes | none | 2, before the runs that used them | none | 2, the warmup one 3 s after its run |
| cost | $0.55 | $0.52 | $0.45 | $0.44 |

- Every run has a reason, in the agents' words: "Baseline sweep … unmodified
  train.py", "Retry BS48 jobs after fixing partial-batch IndexError", "Same
  6-config sweep with linear LR warmup; expect lr=0.1 to stop diverging".
- The notes added afterwards hold the conclusions: "Invalid run: $LR/$BS
  were passed literally", "lr=0.1 jobs: genuine divergence, not a code bug",
  "Warmup helps everywhere … Best: lr=0.1, bs=48 … 0.8990".
- Both agents committed their fixes, which neither did in trial 1. rA
  committed each change before the run that used it. rB ran the warmup sweep
  on uncommitted edits, said so in the run's note, and committed 3 seconds
  after the run finished.
- Both first runs still failed on `--lr '$LR'` without a shell (trial 1
  finding 5, not yet fixed).

## Scores

As in trial 1: **yes** from the record, **inferred** by reasoning a real
project would not support, **no** unanswerable.

| Question | trial 1 (rA / rB) | trial 2 rA | trial 2 rB | How it was answered in trial 2 |
| --- | --- | --- | --- | --- |
| Q1 runs, order, commands | yes / yes | yes | yes | `runs`, `show`, `lineage`; the `Source:` commit per run in the run table. How run 3 was started is still not recorded. |
| Q2 why each run | inferred / inferred | **yes** | **yes** | The run notes, quoted. |
| Q3 what changed, including code | inferred / inferred | **yes** | **yes**, but run 4 inferred | `lineage A B` `Source: changed`, then `git diff` between the recorded commits. rB's run 4 ran on uncommitted edits, so the code was matched to the later commit by recomputing the results. |
| Q4 failures and fixes | yes / yes | yes | yes | Failure groups, logs, and the notes, which state which failures were bugs. |
| Q5 best result, its code, rerun | inferred / inferred | **yes** | inferred | rA: commit `a764556`, the current HEAD. rB: "4f038df plus uncommitted changes". |
| Q6 the agent's conclusion | no / no | **yes** | **yes** | The notes added after the runs. |
| Q7 experiments outside rotari | no / no | no | no | Unchanged: nothing records what did not go through rotari. |

Q2, Q3, Q5, and Q6 moved from inferred or unanswerable to answered from the
record, except where the agent ran uncommitted code.

## Findings

1. **A preview and the run started after it can differ, and the record then
   misleads.** rA previewed `retry --filter-diagnosis 'Python key or index
   error' --dry-run` ("would execute 2 of 6 jobs"), then started
   `retry --async --if-revision REV --note "Retry BS48 jobs …"` without the
   filter. `--if-revision` guards only the project's state, so the start
   succeeded and executed all 4 failed jobs, rerunning the two lr=0.1 jobs
   that its note did not mention. The reconstruction agent noticed: "The run
   also re-ran the two LR0.1 jobs … The record doesn't say whether the agent
   meant that." A replay of rA's command sequence showed this was the
   omitted option, not a selection bug: the same retry with the filter
   executes 2 jobs, before and after the source and notes changes. Proposal:
   make the revision printed by a run preview also cover the planned jobs, so
   a start whose plan differs from the preview fails; or record and show the
   selection a run was started with.
2. **A clean source is not shown as clean.** A git revision without
   uncommitted changes prints only the commit (`git a764556 …`), and rA's
   reconstruction agent concluded that "rotari records only the commit ID,
   not whether there were uncommitted edits", which is wrong: dirty trees are
   marked. Proposal: print the clean state explicitly, for example
   `git a764556c3e1f (clean)`.
3. **Uncommitted code still blocks Q5.** rB ran its best configuration on
   uncommitted edits. The record says so, which is correct, but the exact
   code is gone. jj would have recorded it; for git, the guide could ask
   agents to commit before a run whose results they will report.
4. **Still open from trial 1:** a sweep split over projects cannot be
   compared (rB, finding 3), and commands do not run in a shell while the
   guide says "shell commands" (both agents, finding 5).

## Next

Findings 2 and 5 are small fixes. Finding 1 is a design decision for the
user: whether a preview's revision should guard the plan as well as the
state. A further trial should give the human side of the record a turn:
the user reads an agent's runs in the Web UI or with `lineage` without the
transcript.

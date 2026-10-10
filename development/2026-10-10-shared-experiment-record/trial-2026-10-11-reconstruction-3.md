# Reconstruction Trial 3

**Date:** 2026-10-11
**Plan:** [plan.md](plan.md), Phase 3
**rotari:** built at `56fee209`: the run report as the run's record (sources,
notes, job table), rendered on the Web, and the agent guide asking for a
Markdown table of results in the concluding run note
**Baseline:** [trial 2](trial-2026-10-10-reconstruction-2.md)

## Question

Do agents gather a run's results into its concluding note, as the guide now
asks, and does that make the record easier to read?

## Setup

The same as trials 1 and 2: [trial-fixture.sh](trial-fixture.sh),
[trial-run.sh](trial-run.sh), the same task, rA told to use rotari, rB told
only that it is installed.

## What the experiment agents did

Both found `lr=0.1, bs=48` with warmup at 0.899 again.

| | trial 2 rA | trial 3 rA | trial 2 rB | trial 3 rB |
| --- | --- | --- | --- | --- |
| rotari calls | 29 | 40 | 26 | 41 |
| runs | 4 in 1 project | 3 in 2 projects | 4 in 2 projects | 3 in 2 projects |
| notes when a run started | 4 of 4 | 3 of 3 | 4 of 4 | 3 of 3 |
| notes added afterwards | 4 | 3 | 4 | 2 |
| results tables in notes | - | 2 | - | 2 |
| runs on uncommitted code | 0 | 2 of 3 | 1 | 2 of 3 |
| cost | $0.52 | $0.44 | $0.44 | $0.40 |

- **Results tables: yes, in both variants.** Each agent's last two runs
  end with a note holding a Markdown table, one row per job name, of
  `val_acc` or of warmup against the baseline, followed by the conclusion.
  Every number matches the job's log. rB, told only that rotari is
  installed, did the same, so the agents follow the guide whenever they read
  it.
- Both first runs used `sh -c` for the matrix values, so no run failed on
  `$LR` passed literally (CLI-25's warning and the guide's example).
- Both used `retry --filter-diagnosis ... --dry-run`, then `--if-revision`
  with the plan revision, with no refusal.
- **Code was less exact than in trial 2.** rA committed each change only
  after the run that used it had started (the fix 12 s into run 2, warmup
  after run 3); rB never committed. Runs 2 and 3 of both record
  `(uncommitted changes)`. In trial 2, rA had committed before each run.
  The guide does not ask for a commit, so this varies between agents.

## Scores

| Question | trial 2 (rA / rB) | trial 3 rA | trial 3 rB | How it was answered in trial 3 |
| --- | --- | --- | --- | --- |
| Q1 runs, order, commands | yes / yes | yes | yes | `runs`, `lineage`, `show --report`, `export`. The rotari commands that started each run are still not recorded. |
| Q2 why each run | yes / yes | yes | yes | The first note of each run. |
| Q3 what changed, including code | yes / yes, run 4 inferred | inferred | inferred | Commands from `lineage A B`. Code: runs 2 and 3 ran uncommitted edits; rA matched them to later commits, rB to `train.py`'s mtime. |
| Q4 failures and fixes | yes / yes | yes | yes | Reports, logs, notes. |
| Q5 best result, its code, rerun | yes / inferred | inferred | inferred | The result from the concluding note's table and the log; the code only as "plus uncommitted changes". |
| Q6 the agent's conclusion | yes / yes | yes | yes | The concluding notes, tables included; rB's answer reproduced the table. |
| Q7 experiments outside rotari | no / no | no | no | Unchanged. |

Q6 is answered with the numbers, not only a sentence: both reconstructing
agents quoted or reproduced the results table and checked it against the
logs. Q3 and Q5 fell back to inferred, not because rotari changed but
because both agents ran edits before committing them.

## Findings

1. **The results table works.** Both agents wrote one without being told
   more than the guide says, and both readers used it. No change needed.
2. **Carried jobs had no log in the report.** A retry's job table showed `-`
   for each carried job's last log line, and a carried failure had no log
   excerpt: the report never followed the carry. Fixed in `26a9eb22`
   (WEB-8); other places that follow carries on their own are recorded in
   [ISSUES.md](../ISSUES.md).
3. **Uncommitted code makes Q3 and Q5 inferred again.** The source record
   says `(uncommitted changes)` but not what they were. This is the largest
   remaining gap and is the user's decision: ask agents to commit before a
   run, warn at `run` when the tree is dirty, record the uncommitted diff of
   tracked files, or rely on jj's snapshot.
4. **Comparisons across projects still fail.** Both agents put the warmup
   sweep in a new project, and both readers found `lineage RUN2 RUN3` fails
   with "run not found" (trial 1 finding 3).
5. The reconstructing rA agent read every run through `show --report`, which
   is now the main reading path.

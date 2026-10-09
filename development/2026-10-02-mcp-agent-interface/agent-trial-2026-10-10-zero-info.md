# Agent Trial With No Information About rotari

**Date:** 2026-10-10
**Plan:** [plan.md](plan.md)
**Baseline:** the [CLI agent trials](agent-trial-2026-10-03-cli-agent.md)

## Setup

The earlier CLI trials told the agent to learn rotari from `rotari guide` and
`--help`, and they all used one scenario: diagnosing a failed run. This trial
told the agent only that a job runner called `rotari` is installed, and it
used three scenarios, two of them new.

- Fixtures were built at `c8925b8e` with
  [agent-trial-zero-info-fixture.sh](agent-trial-zero-info-fixture.sh). Each
  scenario has its own state and config directory. Its `rotari` is a wrapper
  that logs each call and the size of its output.
- Each agent was a headless `claude -p` session, run in the scenario's `work/`
  directory outside the repository. It had `--setting-sources ""` and Bash,
  Read, Edit, Write, Glob, and Grep. It had no web access and no subagents.
  The wrapper's directory was put first on its PATH, and nothing else was set.

| Scenario | Task given to the agent |
| --- | --- |
| s1, new sweep | "`train.sh` reads LR and SEED. Try LR = 0.001, 0.01, 0.1 with seeds 1, 2, 3 using rotari, then tell me which LR is best and whether anything failed and why." The task with LR=0.1 and SEED=2 fails with NaN. |
| s2, inherited failure | "A colleague ran an experiment with rotari from this directory, and part of it failed. Find out what and why, fix what can reasonably be fixed, rerun only what needs it, and don't edit the scripts." This is the fixture from the earlier trials, with one state directory. |
| s3, stuck job | "I started rendering jobs with rotari a few minutes ago. One frame seems stuck. Deal with it so the rest finish and the video is encoded." An async run is in progress: frame[5] of 8 sleeps for 100000s, and encode depends on all frames finishing. |

## Result

All three agents completed their tasks correctly. No call failed on a path or
on the wrong project.

| | s1 | s2 | s3 |
| --- | --- | --- | --- |
| rotari calls | 10 | 17 | 15 |
| output | 27 KB | 41 KB | 44 KB |
| failed calls | 0 (a nonzero `wait` exit is the run's result) | 1 | 1 |
| wall time | 26 s | 75 s | 49 s |

- s1 added a 3×3 `--matrix`, ran `check` and then `run --async` with
  `wait`, and read the NaN failure with `lineage` and `show --report`. It got
  the accuracies by grepping `show -r RUN --logs`.
- s2 used `projects`, `runs`, `lineage`, and `export` to find that only the
  `train[12]` timeout was a settings problem. It ran `change -r ... --timeout
  2m --dry-run` and then `--if-revision`, then `retry -j 36dc1772e-12
  --dry-run` and then `--async --if-revision`, `wait`, and `lineage OLD NEW`.
- s3 used `info`, `show`, `show -j ATTEMPT --logs`, and `cancel ATTEMPT_ID`,
  then waited for encode, and said that frame 5 is missing from the video.

Each agent started with `rotari --help` and then `rotari guide`, about 18 KB
together. The rest of the loop was short, because `lineage` gave each agent
the cause and the next command.

## Findings

Ordered by how much each would help an agent.

1. **`rotari --help` costs 10.4 KB before the agent reaches the guide.** All
   three agents read it first. It tells them to run `rotari guide`, but it
   prints the full flag synopsis of every command first, with each filter
   flag repeated for `cancel`, `suspend`, `resume`, and the other commands.
   Proposal: make top-level help a one-line-per-command index plus the guide
   pointer, and leave each command's synopsis to `COMMAND --help`.
2. **There is no way to see that a running job is stuck.** The `show` table
   has SUBMITTED but no elapsed time and no time of last output. Its status,
   `running (recorded)`, does not say whether the process is still making
   progress. The s3 agent decided frame 5 was stuck from the `sleep 100000`
   in its command text, which a real job would not show. Proposal: give
   running jobs an elapsed time and a last-output age (for example, "running
   12m, quiet 11m"), and say so in `show -j ATTEMPT`.
3. **The `wait` and `run` completion message grows with every failure,
   including carried ones.** After s2's retry, the message listed all six
   failed jobs in seven-line blocks, although five were carried from the
   previous run and did not execute. It did not mark them as carried, and
   their hints combine the new run ID with the old attempt ID. Proposal: end
   with the `lineage` summary (counts, causes, executed or carried), and
   point to `lineage RUN_ID` instead of listing every job.
4. **A cancelled job reads as a failure with no diagnosis.** In s3, the `wait`
   summary said "Failed: 1" and "Diagnosis: no_match 1" for the job the agent
   had just cancelled. `lineage` correctly groups it as "cancelled", and the
   `show` table says `cancelled`, so only the summary lines disagree.
   Proposal: count cancellations separately in the summary and the
   diagnosis line.
5. **Hints carry long absolute `--basedir` paths, against the guide.** The
   `retry:` lines in `lineage`, "Rerun failed jobs" in `wait`, and "Failure
   summary" in `show` all add `--basedir '/abs/path' --project-name NAME`. In
   these trials that was the default state directory, so the options added
   about 150 characters to each line and changed nothing. The guide says that
   ID-based hints work without `--basedir`. Proposal: leave out `--basedir`
   and `--project-name` when they equal what the current directory resolves
   to.
6. **The `show` table is wide.** It repeats the full COMMAND on every row,
   along with DEPENDS ON, EXECUTOR, and SUBMITTED. Ten s3 jobs took 5.5 to
   6.8 KB per `show`. s3 read it three times and cut it with `grep` and `awk`.
   Proposal: shorten COMMAND in the table, or leave it out when every task
   shares it, as the tasks of an array do.
7. **Collecting a result across a sweep needs grep.** s1 grepped
   `show -r RUN --logs` for `final_acc=`. The jobs are ordered by job ID, a
   hash, and the matrix values appear only in the job name. Proposal: give
   `show --logs` a `--tail N` option and sort the jobs by name, so that the
   last line of each job lines up with its matrix values.
8. **`cancel --wait` with a job selection is rejected, and the help does not
   say so.** `--wait` is described as "wait until cancellation is complete".
   The error, "--wait may not be used with a job selection", does not say what
   to do instead. Proposal: support `--wait` for job cancels, or state the
   restriction in the help and have the error suggest `wait`.
9. **`change -j ARRAY_TASK` is rejected without the command to use.** The
   error "36dc1772e-12 is a task of array job 36dc1772e; select the array job
   instead" was enough for s2, but it does not say that the change will apply
   to all 12 tasks. Proposal: print the corrected command and the number of
   tasks it affects.
10. **Smaller points:**
    - The `retry` progress started at `0/15` and then read `1/3`.
    - "Retry source: current queue; latest run RUN has 0 failed or unfinished
      job(s) not included" is hard to parse.
    - The completion message's "Show output" hint
      (`show --run-id R --job-id ATT`) differs from the progress hint
      (`show -j ATT`).

## Fixture note

The first build of these fixtures did not set `XDG_CONFIG_HOME`, so the
fixture runs read the developer's real
`~/.config/rotari/notifications.toml`, and its webhook may have received the
failed fixture run. [agent-trial-fixture.sh](agent-trial-fixture.sh) has the
same gap. The new fixture script sets `XDG_CONFIG_HOME` for every rotari
call.

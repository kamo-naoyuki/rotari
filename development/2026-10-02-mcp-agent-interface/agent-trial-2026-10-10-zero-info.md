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

## Second run, after the fixes

Four findings were fixed:

- `25e6dd82`: printed commands name `--basedir` only when the state directory
  is not the implicit one (finding 5, CLI-22).
- `6fec04d1`: the completion message of `run`, `retry`, and `wait` groups
  failures by cause as `lineage` does, marks carried failures, and no longer
  reports a cancelled job as `no_match` (findings 3 and 4, CLI-14).
- `d262d620`: top-level `--help` is a one-line index of the commands, 2.4 KB
  instead of 10.4 KB (finding 1).

New agents got the same three tasks on fresh fixtures, built at `d262d620`.
All three completed them again.

| | s1 first | s1 second | s2 first | s2 second | s3 first | s3 second |
| --- | --- | --- | --- | --- | --- | --- |
| rotari calls | 10 | 11 | 17 | 18 | 15 | 13 |
| output | 27 KB | 20 KB | 41 KB | 40 KB | 44 KB | 34 KB |

- `rotari --help` with `rotari guide` fell from 18 KB to 10 KB.
- The s2 agent spent the saving on `change --help` and `retry --help`
  (9.6 KB) before changing anything, which the first agent had not read.
- s2's `wait` after its retry printed 2.5 KB instead of 3.0 KB, and the
  carried failures were marked as carried.
- In s3, `wait` grouped frame[5] as "1 cancelled" with no diagnosis line.

The remaining findings reappeared:

- s1 found the accuracies by grepping the run directory under the state
  directory, outside rotari, after looking for a log option in `show --help`
  (finding 7).
- s2 tried `change -j` with an array task first (finding 9).
- s3 tried `cancel --wait` with a job selection (finding 8), and read the
  wide `show -r` table three times (finding 6).

## Third run, after the remaining fixes

The remaining findings were addressed:

- `33efb8dd`: the `show` run table and `show -j` give each job's elapsed
  time and, for a running job, how long ago it last wrote output
  (`12m 03s, quiet 11m 58s`), and the table's columns fit their contents
  (findings 2 and 6, CLI-23).
- `60b215fd`: `show --tail N`, and `show --logs` in definition order
  (finding 7, CLI-24). This also stopped `show --logs` from listing the
  run's `configs` directory as a running job.
- `b3d7df65`: `cancel --wait`'s help and error say that it waits for a
  whole-run cancel and point to `rotari wait` (finding 8). The rejection
  itself is SEL-12 and was kept.
- `bc94db72`: selecting an array task names the array job to select and how
  many tasks that covers (finding 9).
- `c4c8d011` and `07a5254e`: the failed-job hint uses `show -j ATTEMPT`, and
  the first progress count is out of the executed jobs (finding 10).

New agents got the same tasks on fixtures built at `07a5254e`. All three
completed them.

| | s1 | s2 | s3 |
| --- | --- | --- | --- |
| first run | 10 calls, 27 KB | 17 calls, 41 KB | 15 calls, 44 KB |
| second run | 11 calls, 20 KB | 18 calls, 40 KB | 13 calls, 34 KB |
| third run | 9 calls, 13 KB | 18 calls, 35 KB | 12 calls, 24 KB |

- s3 found the stuck frame from `rotari jobs` and `show`, cancelled it with
  `cancel -j`, and did not try `cancel --wait`.
- s2 tried `change -j` with the array task again. The new error led it to the
  array job in one call.
- s1 still grepped the run directory for the accuracies, because the guide did
  not mention `--tail`. `62c49b94` added it to the guide's rule about reading
  summaries before logs. A further s1 agent, on a fixture built at
  `62c49b94`, read the results with `show -r RUN --logs --tail 3`, with no
  search of the state directory.

What remains is recorded in [ISSUES.md](../ISSUES.md).

## Fourth run, 2026-10-20

New agents got the same three tasks on fresh fixtures built at `93863df6`,
from a clean worktree. Since the third run, rotari had gained run and attempt
notes (`0c6eb957`), and the guide now asks agents to leave them and to gather
a run's results into its concluding note as a Markdown table (`d79c77a3`).
`wait` without a selector now follows only the current shell's runs
(`11b67bc5`). The `claude` CLI was version 2.1.296. All three agents completed
their tasks.

| | s1 | s2 | s3 |
| --- | --- | --- | --- |
| third run | 9 calls, 13 KB | 18 calls, 35 KB | 12 calls, 24 KB |
| fourth run | 12 calls, 28 KB | 18 calls, 39 KB | 14 calls, 24 KB |
| failed calls | 1 (`wait`, the run's result) | 1, plus `wait` | 1, plus `wait` |
| wall time | 38 s | 95 s | 62 s |

- All three agents left notes without being told to. s1 and s2 each put a
  results table in the run note and the cause of a failure in an attempt
  note. s3 noted why it cancelled `frame[5]`. `lineage RUN` showed the notes
  before the counts.
- s1 ran a 3×3 `--matrix`, `check`, `run --async --note`, and `wait`, and
  read the accuracies with `show -r RUN --logs --tail 3`. It did not search
  the state directory.
- s2 again tried `change -j` with an array task first. The error named the
  array job and its 12 tasks, and the next call worked. It retried only
  `train[12]` with `--dry-run` and then `--if-revision`. Both evals reran as
  dependents, and the agent explained why.
- s3 cancelled the frame with `cancel -p render -j ATTEMPT --wait`, which now
  works with a job selection. It ended by pointing to the `retry:` line from
  `wait`, `retry -p render --filter-failure-kind cancelled --dry-run`, which
  previews `frame[5]` and `encode`.

Output size rose for s1 and s2, for two reasons:

- `rotari --help` and `rotari guide` take 12 KB, against 10 KB in the second
  run, because the guide's rule about notes added about 2 KB.
- Command help is now the largest single cost. s1 read `add --help` and
  `run --help` (9.7 KB) before adding anything. s2 read `change --help` and
  `retry --help` (10.4 KB), as in the second and third runs.

### Findings

1. **A failure that matches no rule has no example line.** s1's group read
   `1 error (exit 1): train-LR0.1-SEED2` with `show:` and `retry:` lines but
   no `e.g.` line, although the job's last line was
   `RuntimeError: loss became NaN`. `runlineage.classifyFailure` takes the
   evidence of a plain error from `result.Error`, which is empty for a job
   that only exited nonzero. The agent needed `show -j ATTEMPT` to learn the
   cause. Proposal: use the last line of the job's error output as the
   evidence when no rule matched. Recorded in ISSUES.md.
2. **The "Job failed" progress block does not name the job.** `wait` printed
   `Name: train-LR0.1-SEED3` for a running job, but its "Job failed" block
   for `train-LR0.1-SEED2` gave only the job ID, attempt ID, and command
   (`supervisor.runObserver`). For a matrix, every member has the same
   command, so the block does not say which member failed. Recorded in
   ISSUES.md.
3. **The s3 fixture cannot test quiet time.** M8 asks that the first `jobs`
   or `show` single out `frame[5]` by its quiet time. The agent arrived 25 s
   after the run started, when frames 3 to 8 all read `35s, quiet 34s`,
   because every frame prints once and then sleeps. The agent again chose
   `frame[5]` from its command text and log line. To test quiet time, the
   healthy frames should print every few seconds, and the agent should
   start a few minutes after the run, as its task says.
4. **Smaller points:**
   - s3 tried `rotari status` first. The error listed the subcommands, and
     the next call was `rotari guide`.
   - `show -r` and `show -j` still print the absolute `Base directory`,
     `Queue`, `Directory`, and `Output directory` paths, about 500 bytes per
     call in these fixtures.

# Agent Trial With a Real Agent and the CLI

**Date:** 2026-10-03
**Plan:** [plan.md](plan.md)
**Baseline:** the scripted trials, such as the [M7 trial](agent-trial-2026-10-03-m7.md)

## Setup

The user cannot connect MCP servers in their environment, so this trial used the CLI.

- A fresh fixture from [agent-trial-fixture.sh](agent-trial-fixture.sh), built at `3d6be9f`. `rotari` on PATH was a wrapper that logged each call and the size of its output.
- A general-purpose Claude subagent had no context from the session. It was told only to source the fixture's `env.sh` and to do the engineer's request: "My training project `exp` had failures in its last run. Find out what failed and why, fix what can reasonably be fixed, rerun only what needs rerunning, wait for that run to finish, and tell me what is fixed and what still fails and why."
- It could learn rotari only from `rotari guide` and `--help`. It could not read the source or the state files. It could edit the job scripts.

## Result

The agent completed the task:

- It found labA's `exp` among three basedirs, with a healthy `exp` in labB.
- It grouped the causes with `lineage` and read the evidence with `show -j ... --report`.
- It fixed the one honest failure, `train[12]`'s 5s timeout, with `change -r RUN --job-name train --timeout 60s`, using `--dry-run` and then `--if-revision`.
- It reran with `retry --dry-run` and then `retry --async --if-revision`, waited with `wait --json`, and compared with `lineage OLD NEW`.
- It left the OOM, unknown optimizer, and missing test split unfixed, with reasons, instead of editing the scripts' forced errors.

The rotari calls, as the wrapper logged them:

| Call | Output | Note |
| --- | --- | --- |
| `rotari guide` | 46.6 KB | read first; 754 lines, most of them the full command reference |
| `jobs --all-basedirs --since 720h` | 41.3 KB | 333 rows; the 32 `exp` rows were lost among labC's 300 `sweep` rows |
| `rotari --help` | 9.9 KB | |
| `show -r RUN` | 7.2 KB | |
| `change --help`, `retry --help` | 10.0 KB | on stderr, exit 1 |
| the other 21 calls | about 15 KB | the diagnose, fix, retry, and compare loop |

In all, 27 calls returned about 140 KB. The loop itself took about 15 KB.

## Findings

1. **`rotari guide` is too long for its purpose.** It is meant for agents and is read first, but it carries the whole command reference.
2. **`COMMAND --help` exits 1 and prints a bare flag dump.**
   - There is no description of the command or its positional arguments.
   - `retry --help` prints "Usage of run" with `run`'s flags, including flags that `retry` does not take.
   - `change -r` is described as "run ID to use when restoring a batch", which does not say that it loads that run into the queue.
3. **A missing project gives no lead.** `show -p exp` in the default basedir said the project does not exist, but did not name the registered basedirs that have an `exp`. The agent spent about six calls finding labA.
4. **`lineage OLD NEW` repeats a definition change for each array task.** Its "timeout (carried)" reads like a timeout failure, though it means the timeout setting changed on a carried job.
5. **Smaller points:**
   - The `--async` start message has no trailing newline, and its cancel hint lacks the run ID.
   - `retry --dry-run` does not say why a passed job executes (its upstream reruns).
   - The queue table shows neither the timeout nor the source of a queue restored by `change -r`.

The diagnose, fix, retry, and compare loop worked as designed. The cost is in finding the way in.

## Second run, after the fixes

The first four findings were fixed in:

- `b99bf19`: a shorter `rotari guide`, and `COMMAND --help` as a reference that exits 0.
- `7e8169c`: a missing project's error names the registered state directories that have it.
- `a459e0d`: `lineage` groups the tasks of an array that read the same.

Finding 2's retry-specific part, that `retry`'s help omits run options it takes, is recorded in ISSUES.md (`07c22fa`). It overlaps with the CLI option interaction work.

A new subagent got the same task and rules on a fresh fixture, built at `a459e0d`. It completed the task again:

- It raised the timeout and reran only `train[12]` and the dependent eval jobs with `retry -j`.
- It reported which failures it left and why, as the first agent did.
- It asked to confirm that labA was the intended `exp`.

| | First run | Second run |
| --- | --- | --- |
| rotari calls | 27 | 23 |
| output | about 140 KB | about 45 KB |
| `rotari guide` | 46.6 KB | 5.6 KB |
| calls to find the project | about 6 | 3; the error named labA and labB |

New findings:

1. **A `--basedir` with `..` failed with "invalid state path".** The agent passed `-b ../labA` from the job directory, and `lineage` and `show` failed without saying why. In a reproduction, `add` with such a path created the directory, then failed. Fixed by resolving state and master directories to absolute paths (RES-22).
2. **Still open:**
   - `retry --dry-run` does not say why it adds a dependent job.
   - `lineage` counts carried jobs with a definition change as "changed" and does not name its hidden jobs.
   - The queue table shows neither the timeout nor the source of a queue restored by `change -r`, and its `DEPENDS ON` column is very wide.
   - `wait --json` exits 1 for a failed run, which reads like a failure of `wait`.

## Third run

After the second run, three more of its findings were fixed:

- `9b66c8e`: relative state directories, including `..`.
- `d5e4359`: the async start message ends its line, and its hints name the run.
- `62e644a`: run previews mark a dependent job with `depends_on_rerun=NAME`.

A new subagent got the same task and rules on a fresh fixture, built at `62e644a`. It completed the task:

- It raised the `train` timeout with `change --dry-run`, then `--if-revision`.
- It reran `train[12]` with `run -j ... --dry-run`, then `--async --if-revision`.
- It left the same three failures, with reasons, and asked to confirm labA.

| | First | Second | Third |
| --- | --- | --- | --- |
| rotari calls | 27 | 23 | 21 |
| output | about 140 KB | about 45 KB | about 57 KB |
| failed calls from wrong paths or projects | 4 | 3 | 1 |

The third agent read more help (`--help`, `run --help`; 16.5 KB), which accounts for the larger output. No call failed on a path, and the only failed calls were the planned project lookup and `--async --dry-run`.

New findings:

1. **The guide says to start runs with `--async` and to preview first, but `run --async --dry-run` is rejected.** The agent had to drop `--async` to preview.
2. **`change --dry-run` prints only the job IDs it changes, not the fields.** The agent could not see "timeout 5s -> 60s" until `lineage`.
3. **Hints without `--basedir` looked broken.** The agent assumed that hints such as `rotari show -j ATTEMPT_ID` and `rotari change -r RUN_ID ...` need `--basedir`. They do not: run and attempt IDs locate their state directory, as checked after the trial. The guide does not say so.
4. **`run --help` does not say that `--job-id` also executes dependent jobs.** The dry run showed it.
5. **Still open from before:**
   - `lineage` does not name the jobs it hides, so a rerun and passed `eval-splitval` was invisible in the comparison.
   - `wait --json` results have no job names.
6. **Smaller points:**
   - `show --report` says "Executor: default" where the table says `local`.
   - The timeout failure group has no "fix:" suggestion.

## Fourth run: diagnosis and preview only

After the third run, these were fixed:

- `c1347d1`: the `--async --dry-run` refusal says how to preview and start.
- `2df0c80`: the guide says that run and attempt IDs locate their state directory, and `--job-id` help says that dependents execute.
- `04e58e6`: `change` output names the fields it changes.

The fourth run was tried twice, on fixtures built at `7cc208d` and at `bf4d645`. Both agents stopped at their first state-changing command, `rotari change ... --if-revision`. Claude Code's auto-mode classifier denied it as "Modify Shared Resources". Neither agent worked around the denial, and the session did not run the command for them.

Allow rules in `.claude/settings.local.json` for the fixture's `env.sh`, for `rotari`, and for writes under `/tmp` did not change the outcome. The rerun, wait, and comparison steps were not measured. The first three runs completed them.

The diagnosis and preview steps used the fixes as intended:

- `-b ../labA` worked as given.
- `lineage RUN_ID`, `export -r RUN_ID`, and `show -j ATTEMPT_ID` ran without `--basedir`.
- `change --dry-run` printed `timeout=5s->60s` before applying.
- Each agent reached the planned fix in 11 to 13 calls, with about 36 KB of output.

New findings:

1. `change -r RUN --dry-run` names the changed fields but not that the queue is first replaced with the run's jobs.
2. The missing-project error lists the state directories that have the project, but not their last run's result, which is what tells them apart.
3. The `--job-id` help does not say whether `depends_on_finished` dependents count, and there is no way to rerun one task without its dependents.
4. Repeated from earlier runs:
   - `show --report` says "Executor: default" where the table says `local`.
   - The `show` run table is very wide, because of `DEPENDS ON`.

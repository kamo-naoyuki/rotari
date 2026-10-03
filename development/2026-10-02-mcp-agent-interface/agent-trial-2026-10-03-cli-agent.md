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

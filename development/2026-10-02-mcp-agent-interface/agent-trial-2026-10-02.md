# Agent Trial: Triage and Fix Loop with the Existing CLI

**Date:** 2026-10-02
**Plan:** [plan.md](plan.md)

## Purpose

Find out what an agent actually needs from rotari by having an agent (Claude Code, with a shell) perform the two most common requests using only the current CLI, before designing any agent-specific interface:

1. "The `exp` run failed. Take a look." (triage)
2. "I fixed it; rerun and tell me whether it improved." (fix loop)

The agent was not told any paths. Every call's output size was recorded because output size is the agent's main cost.

## Fixture

An isolated `XDG_STATE_HOME`, with the binary built from `2cfdaae`. [agent-trial-fixture.sh](agent-trial-fixture.sh) recreates it:

- `labA/exp`: `prep` -> `train` (array 1-12, stage `training`, `--timeout 5s`) -> `eval` (matrix `split=val,test`, `--depends-on-finished training`). Failures with distinct causes: CUDA OOM (tasks 3, 7, 11, exit 1), `ValueError` (tasks 5, 9, exit 2), timeout (task 12, exit 124), `KeyError` (`eval-splittest`, exit 3).
- `labB/exp`: a same-named, healthy project in a second basedir.
- `labC/sweep`: a 300-task array with 84 failures in three causes (OOM 42, `ValueError` 24, `FileNotFoundError` 18), to measure scale.

## Triage (labA, 15 jobs)

| Step | Command | Exit | Size | Outcome |
| --- | --- | --- | --- | --- |
| 1 | `show` | 0 | 578 B | Lists both basedirs' `exp` with last run IDs. No result column, so it cannot tell which project failed. |
| 2 | `show --basedirs` | 0 | 533 B | Basedirs only; adds nothing to step 1. |
| 3 | `jobs` | 0 | 44 B | "No running or recently finished jobs found": it only looks at the default basedir, and says nothing about that scope. |
| 4 | `show -p exp` | 1 | 163 B | Fails. This is exactly the hint printed by step 1 (`rotari show -p PROJECT`), which does not work when the project is not in the default basedir. |
| 5 | `show RUN_ID` | 0 | 6.0 KB | Usable. Status, exit codes per task, next-step hints. |
| 6 | `show RUN_ID --json` | 1 | 104 B | Rejected: "a run name selector cannot be combined with ... JSON". The positional form must be rewritten as `-r RUN_ID`. |
| 9 | `show -r RUN_ID --json` | 0 | 5.4 KB | Good: each result carries rule diagnoses (name, evidence, suggestion). Lacks job name, task index, and stage; only IDs. |
| 10 | `show -r RUN_ID --report` | 0 | 14.8 KB | Includes successful jobs in full. |
| 13 | `... --report --failed` | 0 | 12.0 KB | Failed jobs only. 240 of 466 lines are uninformative progress lines from the log tail. |
| 14 | `... --json --failed` | 0 | 5.4 KB | Byte-identical to step 9: `--failed` is silently ignored with `--json`. |
| 12 | `diagnose -r RUN_ID` | 1 | 310 B | Needs a job; there is no run-level diagnosis summary. |

The timed-out task reports "Error: timed out after 5s" and, in the same report, "Diagnosis: No known rule matched". rotari knows the cause but the diagnosis says it does not.

Reaching a correct triage took about 6 useful calls and about 25 KB of output for 15 jobs. The answer itself (four causes, which tasks, what to do) fits in about 500 bytes.

## Fix loop (labA)

| Step | Command | Exit | Outcome |
| --- | --- | --- | --- |
| 15 | `change -r RUN -j TASK_ID --timeout 40s` | 1 | "is a task of array job ...; select the array job instead". A good, actionable error. |
| 16 | `retry -r RUN --async` | 0 | Started a new run. The agent (I) proceeded although step 15 failed, so the retry still used the 5 s timeout. Nothing in the retry output shows the effective per-job settings. |
| 17 | `wait -r RUN2 --timeout 60s --json` | 1 | Returns the summary at the end only; there is no way to return at the first failure or to ask "what changed since I last looked". |
| 19-20 | `lineage -r ...` | 1 | Wrong guess at the flags; 14 lines of usage per mistake. |
| 21 | `lineage RUN1 RUN2` | 0 | Excellent: fixed 5, still failing 2, newly failing 0, keyed by job name. |

`lineage` comparison already answers "did it improve?". What it lacks is whether a still-failing job fails for the same reason: `train[12]` was still a timeout because of the step 15 mistake, and `train[11]` was still OOM, but the comparison cannot tell an unchanged failure from a different one.

## Scale (labC, 300 tasks, 84 failures)

| Command | Size |
| --- | --- |
| `show -r RUN` | 75.7 KB |
| `show -r RUN --json` | 67.5 KB |
| `show -r RUN --failed-logs` | 94.7 KB |
| `show -r RUN --report --failed` | 108.4 KB |

The useful answer is three groups (OOM 42, `ValueError` 24, `FileNotFoundError` 18) with their task lists. With a shell, I could get it by piping the JSON through a script without reading it, because the JSON already carries per-job diagnoses. An agent without a shell (an MCP-only client) would have to read the 108 KB report, roughly 27k tokens.

## Other observations

- Depending on an array job by its `--job-name` fails for `--depends-on` and `--depends-on-finished`, from a plain or matrix dependent. `add` accepts the dependency; only `check` or `run` rejects it. A stage works as a workaround.
- One mistaken flag prints the full usage (about 50 lines for `add`), which is expensive for an agent and hides the one line that matters.
- Absolute paths appear in almost every human-oriented output and in the JSON, often several times per screen.

## Conclusions for the plan

What already works and should not be rebuilt for agents:

- run-ID resolution without paths, `show -r --json` with per-job diagnoses, `lineage` comparison by job name, actionable errors such as step 15's.

What agents need that does not exist:

1. **Failure grouping.** One call returning failures grouped by cause (diagnosis rule, or exit code plus a normalized error line when no rule matches), with counts, task lists, one representative excerpt per group, and the IDs needed to drill down. This is the largest gap: about 100 KB versus under 1 KB for the same decision. It also helps humans in `show` and the Web UI.
2. **Relevant excerpts instead of log tails.** Prefer stderr and the lines around the diagnosis evidence over the last N lines.
3. **Cause-aware comparison.** `lineage` should say whether a still-failing job fails for the same cause.
4. **Incremental waiting.** Return at the first new failure, and answer "what changed since cursor X" for long runs.
5. **Cross-basedir discovery with results.** `show` with no arguments should show each project's last result so the agent can see where to look, and its hints must work for a project outside the default basedir. `jobs` should state its basedir scope.
6. **Effective settings in run output.** `retry` and `run` should make it possible to confirm which per-job settings (timeout, retry) a run used.

Bugs found are recorded in [ISSUES.md](../ISSUES.md).

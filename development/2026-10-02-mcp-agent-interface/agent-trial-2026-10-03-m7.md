# Agent Trial After M7: Stopping a Hang and Recovering a Crash Through MCP

**Date:** 2026-10-03
**Plan:** [plan.md](plan.md)
**Baseline:** [M6 trial](agent-trial-2026-10-03-m6.md), which fixed and reran a project

## Setup

These runs used fresh fixtures from [agent-trial-fixture.sh](agent-trial-fixture.sh). [agent-trial-mcp.py](agent-trial-mcp.py) `control` acts as an agent that changes rotari state only through MCP tools. The environment makes task 12 of labA wait 600 seconds for a data server that never answers.

The agent:

1. Drops the 5-second timeout from the exported manifest, imports it, and starts a retry.
2. Waits with `until_failure`, then waits again until the wait times out.
3. Previews a cancel and cancels the jobs it lists, then waits for the run to settle.
4. Starts another retry. The environment kills its supervisor (`rotari __server`) and leaves the hanging job running.
5. Waits; the run comes back interrupted.
6. Previews a reset, and if jobs may still be running, cancels them and previews again.
7. Resets with `recover_interrupted`, and checks the project.

The first pass ran on `026722a` and found the defects below. The final pass ran on `418bbcf`.

## Calls (final pass)

| Call | Response | Used for |
| --- | --- | --- |
| `rotari_wait_run` (`until_failure`, 20 s) | 1.5 KB | `failure` while running: 5 failed |
| `rotari_wait_run` (10 s) | 1.5 KB | `timeout`: the run still runs |
| `rotari_preview_job_control` (cancel) | 238 B | 10 job IDs |
| `rotari_cancel` | 148 B | cancel requested |
| `rotari_wait_run` | 2.1 KB | `settled`: `train[12]` and both `eval` jobs cancelled |
| `rotari_wait_run` after the crash | 1.5 KB | `settled`, `state: interrupted` |
| `rotari_preview_reset` | 325 B | `1 of 6 job(s) appear to still be running`, `jobs_may_be_running: true` |
| `rotari_preview_job_control`, `rotari_cancel` | 386 B | the hanging job stopped, through the interrupted run's stale lock |
| `rotari_preview_reset` | 291 B | `all 6 job(s) report having finished` |
| `rotari_reset` | 291 B | recovered; the project is `empty` |

No job process remained after the scenario.

## Findings

- **Fixed: a cancelled pending job started anyway** (`64df260`).
  - In the first pass, both `eval` jobs were cancelled while they waited for training, and both still ran.
  - Only the single scheduler-job path checked the `cancelled` marker. Local jobs and native array tasks did not.
  - `Dispatcher.cancelledBeforeStart` now guards every submission path.
  - Contract CAN-6. The CLI's `rotari cancel JOB_ID` had the same defect.
- **Fixed: reset never warned that an interrupted run's jobs might still run** (`418bbcf`).
  - In the first pass, `rotari_preview_reset` returned no detail, and the reset recovered the run while its hanging job kept running.
  - The scan looked for `command.json` in each job directory instead of in the latest attempt.
  - Contract SAFE-7. The CLI's `reset` message had the same defect.
- **Open: carried jobs read as running while their run is active.** This is recorded in [ISSUES.md](../ISSUES.md).
  - During the run, the summary reported `succeeded 0, unfinished 10`, although 7 results were carried.
  - The cancel preview listed the 7 carried jobs with the 3 that were still unfinished, and the agent cancelled all 10. That did no harm, because carried jobs are not dispatched, but the preview misled.
  - `rotari show` has the same view.
  - A fix needs the run to record its carried results at start, which is new persistent state.
- **Usage: job control works on an interrupted run.** `rotari_cancel` with the interrupted run's ID reached its still-running job through the stale lock. So an agent can stop the jobs before recovering, without leaving MCP.
- **Limit: the scan reads files only.** A job killed without writing its status still appears to be running. The warning says "appear to" for this reason.

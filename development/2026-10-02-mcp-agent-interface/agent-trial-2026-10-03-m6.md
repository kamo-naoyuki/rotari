# Agent Trial After M6: An MCP-Only Agent That Fixes and Reruns

**Date:** 2026-10-03
**Plan:** [plan.md](plan.md)
**Baseline:** [M4 trial](agent-trial-2026-10-03-m4.md), the read-only path

## Setup

These runs used fresh fixtures from [agent-trial-fixture.sh](agent-trial-fixture.sh). [agent-trial-mcp.py](agent-trial-mcp.py) `write` acts as an agent that changes rotari state only through MCP tools, and changes files with its own tools. It is given no paths and no run IDs. In labA it:

1. Reads the last run's failures.
2. Fixes the OOM in `train.sh`.
3. Gives the timed-out training job more time by editing the exported manifest.
4. Imports the edited manifest.
5. Previews and starts a retry.
6. Follows the run until it is no longer running.
7. Compares the new run with the previous one.

The first pass ran on `dcc0899` and found the defects below. The final pass ran on `1efb12c`. The `retry` description was clarified afterwards, in `66aff94`.

## Calls (final pass)

| Call | Response | Used for |
| --- | --- | --- |
| `tools/list` | 17.2 KB, once per session | ten tool schemas |
| `rotari_list_projects` | 750 B | labA's `basedir_ref` and last run |
| `rotari_run_summary` | 2.3 KB | `state: finished`; OOM 3, value error 2, timeout 1, key error 1 |
| `rotari_export_run` | 1.6 KB | the manifest to edit; it held no redacted value, so it could be imported |
| `rotari_preview_import` | 3.3 KB | the plan and revision |
| `rotari_import` | 3.3 KB | the same plan, applied |
| `rotari_preview_run` (`retry`) | 444 B | 8 of 15 jobs execute; 7 results carried |
| `rotari_start_run` | 56 B | the run ID |
| `rotari_run_summary` x 31 | 232 B each, 7.2 KB in all | polled once a second until `state` was `finished` |
| `rotari_compare_runs` | 4.0 KB | 4 fixed (OOM and timeout), 3 still failing for unchanged causes, 7 carried |

The agent finished the task through MCP in 10 distinct tool calls and about 23 KB of results, 7 KB of it polling. Each write was applied at the revision its preview returned. The run executed exactly the 8 jobs the preview listed. The read scenario on the same fixture still answered in about 14 KB, of which `rotari_run_summary` on labC's 84 failures took 7.3 KB.

## Findings

- **Fixed: a run preview left out the tasks of an array that runs whole** (`69b6de6`).
  - In the first pass, a plain run of the imported queue was previewed as 3 of 15 jobs, but the run executed all 15.
  - `run.PlanRerun` marked such an array by its command ID, and both the CLI and MCP listings read the plan by job ID.
  - The plan is now keyed by job ID, and the supervisor's start message counts jobs on both sides.
  - Conformance: `TestRunPreviewListsTheTasksOfAWholeArray` (CLI-7, MCP-1).
- **Fixed: an agent could not follow the run it started** (`1efb12c`).
  - `rotari_run_summary` failed right after `rotari_start_run`, because the run had not written its jobs yet. Its error held the run's absolute path.
  - The summary had no field that tells a running run from a finished one.
  - It now reports `state` through `project.RunPhaseOf`, the rule `rotari wait` uses.
  - Every tool's errors replace registered state directories with `BASEDIR`.
  - Contract MCP-3.
- **Open: a project that has only been added cannot be reached through MCP.** A basedir is registered only by a run or an import. This is recorded in [ISSUES.md](../ISSUES.md).
- **Usage: `retry` after an import reruns only the failures.**
  - It plans from the imported queue's source results; it does not copy from the last run.
  - The first pass used a plain run and re-executed the 8 jobs that had already passed.
  - The `retry` description now says this.
- **Cost: following a run is polling.**
  - A 30-second run took 31 calls.
  - A bounded wait tool, like `rotari wait --timeout` (optionally until the first failure), would replace the polling with one or a few calls. This belongs to M7's progress inspection.
- **Cost: write results repeat the plan.**
  - `rotari_preview_import` and `rotari_import` each return the full plan with every task's source attempt, 3.3 KB here.
  - `rotari_compare_runs` lists unchanged jobs.
  - `tools/list` grew to 17 KB for ten tools.
  - Output size limits are still an open decision.
- **The redaction rule held.** labA's manifest has no environment values or working directories, so its view could be edited and imported. A manifest with them would be refused, and the agent would need the CLI's `rotari export`.

## After the fixes (`cc8ca55`)

The basedir registration (`c24dc25`) and `rotari_wait_run` (`cc8ca55`) followed from the findings above. The script now follows the run with `rotari_wait_run`, calling it again while it returns `timeout`. A fresh pass on `cc8ca55`:

- `rotari_wait_run` returned once, after the run settled. It used 1 call and 1.4 KB, where polling took 31 calls and 7.2 KB.
- The other calls and sizes were as above. The task took 10 tool calls and about 17 KB of results.
- `tools/list` grew to 20.6 KB for eleven tools, which makes schema size the largest fixed cost of a session.

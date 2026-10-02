# Agent Trial After M4: An MCP-Only Agent

**Date:** 2026-10-03
**Plan:** [plan.md](plan.md)
**Baseline:** [M3 trial](agent-trial-2026-10-02-m3.md), the same path with the CLI

## Setup

These runs used the fixtures from [agent-trial-fixture.sh](agent-trial-fixture.sh) and their later runs from the M2 and M3 trials, with the binaries from `bf47bd5`. [agent-trial-mcp.py](agent-trial-mcp.py) acts as an agent that can only call MCP tools. It speaks JSON-RPC to `rotari-mcp` over stdio and is given no paths and no run IDs. It picks the project whose last run has the most failures and follows the IDs that each response returns.

## Calls

| Call | Response | Used for |
| --- | --- | --- |
| `tools/list` | 7.1 KB, once per session | the four tool schemas |
| `rotari_list_projects` | 747 B | three projects in three basedirs, two named `exp`, with last run, status, and failed/total counts |
| `rotari_run_summary` (worst run) | 1.1 KB | two causes, with an example and attempt each |
| `rotari_get_job_info` (first job of the first cause) | 928 B | one job's report |
| `rotari_compare_runs` (`labA`, previous run by default) | 3.2 KB | fixed 5, still failing 2; `train[11]` OOM to OOM, `train[12]` timeout to timeout |

The four tool calls took about 6 KB. The first CLI trial needed about 100 KB for the same decisions, and the M3 CLI path about 4 KB. No call needed a path, and no response contained an absolute path. Same-named projects were told apart by `basedir_ref` and `basedir_name`.

## Findings

- The M4 done criterion holds. An MCP-only agent triaged and compared runs in four calls without being given a path. The `exp` projects in two basedirs were never confused.
- The worst project in this fixture was the run cancelled in the M3 trial. Its stopped tasks were grouped as `signal`, not as cancelled: a whole-run cancel records exit 143 without a cancellation error. This is recorded in `ISSUES.md`, because it also affects `--filter-failure-kind cancelled`.
- Tool schemas cost 7 KB once per session; the outputs embed `runlineage` types with every field. Trimming schemas is possible later if clients load many tools.
- `rotari_compare_runs` defaults to the previous run of the same project. That spared the agent a run listing, because the project list gives only the last run.

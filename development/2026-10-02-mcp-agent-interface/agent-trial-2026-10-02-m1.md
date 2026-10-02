# Agent Trial After M1: Failure Grouping

**Date:** 2026-10-02
**Plan:** [plan.md](plan.md)
**Baseline:** [first trial](agent-trial-2026-10-02.md)

## Setup

The fixtures were built with [agent-trial-fixture.sh](agent-trial-fixture.sh), and the commands were run with the binary from `3f59a01`. This note measures the triage step only: the fix loop is unchanged by M1.

## Output sizes

`labA` has 15 jobs with 7 failures in four causes. `labC` has 300 tasks with 84 failures in three causes.

| Command | `labA` before | `labA` after | `labC` before | `labC` after |
| --- | --- | --- | --- | --- |
| `show -r RUN` | 6.0 KB | 7.1 KB | 75.7 KB | 77.0 KB |
| `show -r RUN --json` | 5.4 KB | 7.2 KB | 67.5 KB | 73.8 KB |
| `lineage RUN` | not tried | 1.4 KB | not tried | 1.5 KB |
| `lineage RUN --json` | not tried | 3.1 KB | not tried | 11.7 KB |
| `show -r RUN --report --failed` | 12.0 KB | unchanged | 108.4 KB | unchanged |

Before M1, `lineage RUN` already printed diagnosis counts (about 240 bytes for `labC`) but no tasks, examples, or suggestions. The first trial did not find it.

## What `lineage RUN` now answers for `labC`

```text
Failures by cause:
  42 CUDA/GPU memory exhausted (exit 1): sweep[7,14,21,28,35,42,49,56,63,70] +32 more
    e.g. torch.OutOfMemoryError: CUDA out of memory (task 7)
    show: rotari show -j att_20261002-124301-d7741cfe-32ab8e9d1-7-0
    fix: Reduce batch size or model memory use, ...
  24 Python type or value error (exit 2): sweep[11,22,33,44,55,66,88,99,110,121] +14 more
  18 File or directory not found (exit 2): sweep[13,26,39,52,65,78,104,117,130,156] +8 more
```

For `labA`, the timed-out task is now its own `timeout` group instead of an unexplained `no_match`.

## Findings

- With one call to `lineage RUN`, triage takes 1.5 KB for `labC` instead of about 100 KB. The done criterion holds for `lineage RUN`.
- `show -r RUN` contains the same groups after its job table, so its size grows with the run. An agent that starts with `show` still reads the whole table first. This is a discoverability problem, now an M3 item.
- The JSON `failures` field lists every member ID (6.9 KB for `labC`). That is enough to drill down or retry by ID without a second listing call.
- The fix loop's remaining gaps are unchanged: whether a still-failing job fails for the same cause (M2), and incremental waiting (M3).

# Agent Trial After M2: Fix Loop

**Date:** 2026-10-02
**Plan:** [plan.md](plan.md)
**Baseline:** [first trial](agent-trial-2026-10-02.md), [M1 trial](agent-trial-2026-10-02-m1.md)

## Setup

These measurements used the fixtures from [agent-trial-fixture.sh](agent-trial-fixture.sh) and the binary from `94cd80c`. The fix loop repeated the first trial's partial fix:
- the `ValueError` and two of the three OOM tasks were fixed;
- `train[11]` still runs out of memory;
- `train[12]` still times out, because the attempted timeout change was rejected for an array task;
- the `eval` split was fixed.

The loop ran `retry -r RUN` and then `lineage RUN RUN2`.

## Comparison

```text
Summary: fixed 5, still failing 2, newly failing 0, added 0, removed 0, changed 0, carried 7, cause changed 0

JOB             FROM        TO          RESULT          CHANGES  CAUSE
train[3]        failed      success     fixed           -        CUDA/GPU memory exhausted -> -
...
train[11]       failed      failed      still failing   -        CUDA/GPU memory exhausted
train[12]       failed      failed      still failing   -        timeout
eval-splittest  failed      success     fixed           -        Python key or index error -> -
```

In the first trial, the comparison said only "still failing". Now one call shows that `train[12]` still fails for the same reason, a timeout, so the attempted timeout change did not take effect.

## Report

`show -j ATTEMPT_ID --report` for the timed-out task now reads:

```text
### Diagnosis
- Job timeout reached
  Evidence: timed out after 5s
  Next: rotari stopped the job when its --timeout expired. ...

### Log (lines around the diagnosis evidence and the last 20 lines, at most 12000 characters)
[... 22 lines omitted ...]
```

`show -r RUN --report --failed` changed from 108.4 KB to 97.3 KB for `labC` and from 12.0 KB to 10.3 KB for `labA`. Each fixture log is about 40 lines, so the evidence window covers most of it. The excerpt only matters for long logs.

## Findings

- With failure groups (M1) and cause-aware comparison, triage and "did it improve, and why not" take one `lineage` call each.
- `rotari show ATTEMPT_ID --report`, as given in `docs/INSPECT.md`, fails; `show -j ATTEMPT_ID --report` works. This is recorded in `ISSUES.md` with the same positional-argument bug for run IDs.
- Waiting is still all-or-nothing (`wait` returns at the end of the run). That is M3.

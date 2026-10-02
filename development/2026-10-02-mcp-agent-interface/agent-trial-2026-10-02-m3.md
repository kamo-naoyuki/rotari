# Agent Trial After M3: Discovery and Early Failure

**Date:** 2026-10-02
**Plan:** [plan.md](plan.md)
**Baseline:** [first trial](agent-trial-2026-10-02.md)

## Setup

The fixtures were built with [agent-trial-fixture.sh](agent-trial-fixture.sh). The commands were run with the binary from `594b6c4`, with no paths given, as in the first trial.

## Discovery

The first trial needed four calls before it found the failed project, and one of them followed a hint that failed. Now `rotari show` with no arguments answers in one call:

```text
BASEDIR      PROJECT  QUEUED  RUNS  STATE  LAST RUN                  LAST RESULT
.../labA     exp      0       2     idle   20261002-133244-3980f25a  failed 2/15
.../labB     exp      0       1     idle   20261002-124256-2afcadc6  finished
.../labC     sweep    0       1     idle   20261002-124301-d7741cfe  failed 84/300

To show runs in a project:
  rotari show -b BASEDIR -p PROJECT
To summarize a run's failures by cause:
  rotari lineage -b BASEDIR RUN_ID
```

Both hints work as printed (contract CLI-5). `rotari jobs` with nothing to show now says which state directory and window it searched, and suggests `--all-basedirs`.

## Early failure

A four-task array was started with `--async`. Task 2 fails immediately, and the other tasks sleep for 20 s. `rotari wait --until-failure` returned with status 1 after 1 s. It printed the failure group (`1 Python type or value error (exit 2): slow[2]`), the evidence line, the attempt to inspect, and the commands to keep waiting or cancel. Without the option, `wait` returns after 20 s.

## Agent entry point

`rotari guide` now leads with:
- `lineage RUN_ID` for failures by cause;
- `show -j ATTEMPT_ID --report` for one job;
- `lineage RUN_ID NEW_RUN_ID` after a retry;
- `wait --until-failure`.

It no longer suggests `show --failed-logs`, which was 95 KB for the 300-task run, or the positional `show ATTEMPT_ID --report`, which fails.

## Resulting path for an agent with a shell

| Step | Command | Size (`labC`) |
| --- | --- | --- |
| Find where to look | `rotari show` | 0.9 KB |
| Why it failed | `rotari lineage -b BASEDIR RUN_ID` | 1.5 KB |
| One job's evidence | `rotari show -j ATTEMPT_ID --report` | 1.4 KB |
| Did the fix help | `rotari lineage RUN_ID NEW_RUN_ID` | grows with the changed jobs only |

The first trial needed about 100 KB of output for the same decisions.

## Remaining

- `rotari show RUN_ID|ATTEMPT_ID --json|--report` (positional) and `jobs --since 7d` still fail as documented; both are in `ISSUES.md`.
- An MCP-only client has not been tried; that is M4.

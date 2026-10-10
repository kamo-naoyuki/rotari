# Plan: Review Follow-ups for Run, Wait, and Status Views

Created: 2026-10-10 (work started 2026-10-09)
Status: Review findings fixed; follow-up items below remain open

## Purpose

On 2026-10-09 the user asked for a review of the previous days' changes:
bugs, unexpected behavior, and usability problems. The review exercised the
built binary in isolated master and base directories, including pseudo-terminal
Ctrl-D/Ctrl-C/disconnect tests, and read the new attachment, `info`, `wait`,
and list code. This directory records the fixes that followed and what is
still open. History is in [work-log.md](work-log.md).

## Decisions

- Implicit `wait` (no selector), like a shell's `wait`, follows the active
  runs that its own parent process (shell, script, or program) started,
  whether or not another client is attached; `--all` follows every active
  run. Each run's context records the launching process (host, PID, start
  time). This replaced skipping runs another client was attached to, which
  let a second script's `wait` return 0 while its run still ran, and treated
  a synchronous `run` differently from `run --async` plus `wait`, which users
  should not need to tell apart. Scoping by terminal session was rejected
  because two scripts started from one terminal share it; scoping by parent
  process has the same limits as a shell's `wait` (Makefile lines and
  subshells), and those scripts name the project or run instead. When no run
  of the scope is active but others are, `wait` says so and points to
  `--all`. It warns on stderr about interrupted runs of the same scope and
  keeps its exit code (RES-16, CLI-19).
- `wait` without `-p` ignoring `ROTARI_PROJECT_NAME` is intended: aggregate
  commands ignore implicit project defaults, as `jobs` and `runs` do.
- A job rotari has not dispatched is `pending` everywhere, including the
  `info` count. A job a scheduler accepted but has not started remains
  `waiting (recorded)`. "Not dispatched" is decided by positive evidence: the
  job's directory is absent from an existing run directory, because executors
  create it before starting a job. Unreadable attempt state stays `unknown`.
- Every view labels a job through `jobstatus.DisplayLabel`, including
  `success (accepted)` and `(carried)`; reports use the same labels as `show`.
- A whole-run cancel (any entry point: `cancel`, Ctrl-C, disconnect policy,
  Web, MCP) records the run's status as `cancelled` when the run did not
  succeed. Its exit code stays 1, and notifications treat it as a failure, to
  stay on the safe side for accidental cancels. Cancelling selected jobs leaves
  a run `failed` (CAN-8). Runs recorded before this change stay `failed`.
- Run notifications decide success by exit code 0, for both webhooks and
  browser notifications.
- Short options are limited to the target selectors used across commands
  (`-b`, `-p`, `-r`, `-j`, `-e`); `-o` was removed. Machine-readable output is
  `--json` on `info`, `check`, `show`, `lineage`, `wait`, `import`, `runs`, and
  `jobs`; `config` and `export` keep `--format` for their file formats.
- Option errors are one line naming the option as typed (`--long`, `-s`),
  followed by a pointer to `rotari COMMAND --help`.

## Remaining work

### `wait` and cancel

1. Done: `cancel --wait` with a job selection waits until each selected job
   has stopped, including a local job's whole process group (CAN-3). It
   exposed that a local job's cancel is recorded before its process exits,
   now open in ISSUES.md.
2. `TestSynchronousRunInterruptCancelsAcceptedRun` timed out once on
   2026-10-10 waiting for the run to finalize after Ctrl-C. Thirty repeats on
   2026-10-10 after the `cancelled` change passed. Open in ISSUES.md; investigate
   with full logs on recurrence.
3. `TestRunClientDisconnectDetachesByDefaultAndCanCancel/default_detaches`:
   implicit `wait` did not find the detached run once on 2026-10-09. Open in
   ISSUES.md; cause not established.

### Status and wording (from the same agent trial)

4. Done: the diagnosis summary skips failures whose cause rotari recorded
   (blocked, cancelled, timeout), and text summaries (`lineage`, the Web run
   page) omit `no_match`; JSON keeps it. The run/wait completion message had
   already dropped its `Diagnosis:` line in favor of failure groups.
5. Done: the "Retry source" notice is composed once by
   `projectrun.SourceNotice` and names the queue, the latest run, and the
   left-out jobs in plain sentences.
6. Done: hints built outside `cmd/rotari` follow CLI-22. `project` prints
   recovery commands through a location renderer that CLI commands install
   (`SetCommandLocation`), and the CLI sends its rendering to the supervisor
   for the retry source notice. Refusal messages use one command per line.

### Not started elsewhere

7. Supervisor restart and run recovery:
   [2026-10-09-supervisor-restart](../2026-10-09-supervisor-restart/plan.md)
   is deferred.
8. Multi-host attachment validation remains partial:
   [2026-10-09-unified-run-attachment](../2026-10-09-unified-run-attachment/plan.md).

### Older flaky tests

Four other timing failures from 2026-10-07 to 2026-10-09 stay open in
ISSUES.md, to be investigated with full logs if they recur.

## Validation practice used

Each fix added a test first and confirmed it failed for the reported reason;
tests added after a fix were run against the pre-fix commit in a temporary
worktree. While another thread edited the same working tree, commits staged
only this work's hunks, and `scripts/check.sh` ran on a worktree of the
committed state, with `node_modules` linked so the Web DOM tests could run.

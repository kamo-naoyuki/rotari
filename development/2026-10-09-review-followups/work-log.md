# Work Log: Review Follow-ups

Entries group cohesive changes. Times are Git commit times (JST, +0900).

## Review findings recorded

- `a00890c8` 2026-10-09 22:59:53

**Change:** Added the review's open findings to `development/ISSUES.md`.
**Reason:** The user asked to fix the clear bugs first and record the rest.
**Plan impact:** Opened the items this log later resolves or reclassifies.
**Validation:** Documentation only.
**Remaining:** See [plan.md](plan.md).

## Bugs found in the review

- `d86c9a2e` 2026-10-09 22:57:04 — run Ctrl-D/Ctrl-C outcome printed once
- `220cb470` 2026-10-09 22:58:24 — `show` STATUS column alignment
- `38f1489a` 2026-10-09 22:59:19 — `runs` start time and name before a summary

**Change:** `cmd/rotari/wait.go` and `run_command.go` stopped printing the
detach/cancel outcome a second time without a newline; `show`'s job table
padded the plain status before coloring; `runs` read an unfinished run's
name and start time from its run lock. Added a pseudo-terminal conformance
test (`TestRunTerminalControlsPrintOutcomeOnce`, CLI-19), a `show` alignment
test, and a `runs` row test.
**Reason:** Reproduced with the built binary: duplicated outcome lines, a
10-column header over 24-byte rows, and `-` for running runs' STARTED.
**Plan impact:** None beyond the fixes.
**Validation:** Each new test failed before its fix; `go test ./cmd/rotari`,
the touched conformance packages, and `scripts/check.sh` passed.
**Remaining:** None.

## Implicit wait and attachment wording

- `2f9bb144` 2026-10-09 23:27:33 — warn about interrupted runs in implicit `wait`
- `d43edbe1` 2026-10-09 23:28:18 — describe CLIENT attachment consistently
- `41d90be9` 2026-10-10 03:34:21 — wait for the orphaned job in that test

**Change:** Implicit `wait` prints a stderr warning, with inspect and recover
commands, for each interrupted run and keeps its exit code; RES-16, RUNNING.md,
INSPECT.md, and the implicit-location design note were updated. The
conformance test now waits for its orphaned job in a cleanup that runs before
the test directory is removed.
**Reason:** Implicit `wait` exited 0 silently beside an interrupted run. The
user decided against a non-zero exit. INSPECT.md contradicted itself about
whether `wait` sessions count as attachment. The test was recorded in
ISSUES.md as failing cleanup because the orphaned job still wrote.
**Plan impact:** Decided that skipping runs attached by another `wait` is
intended.
**Validation:** `TestWaitWithoutSelectorWarnsAboutInterruptedRuns` failed
before the fix; the cleanup failure did not reproduce in 15 local repeats, so
its cause is inferred from the fixture.
**Remaining:** None.

## Pending jobs and shared status labels

- `ecae3d66` 2026-10-09 23:40:13 — report jobs that have not started
- `37e5ca97` 2026-10-09 23:59:33 — share lineage status between CLI and Web
- `8ff89298` 2026-10-10 02:16:33 — label report jobs as `show` does
- `75194ad5` 2026-10-10 02:23:02 — label accepted jobs alike in every view
- `80d4d36f` 2026-10-10 03:49:00 — call undispatched jobs `pending`
- `a7f6bc9a` 2026-10-10 04:33:07 — list pending jobs next to their run

**Change:** `jobstatus` reports a job whose directory is absent from an
existing run as pending (`StatusPending`); `jobs` lists such jobs without an
attempt ID and `info` counts them, with jobs awaiting a retry, as pending.
`runview.LineageStatus` and `jobstatus.DisplayLabel` became the single
classification and label used by `show`, `jobs`, reports, the Web API and
history search. `joblist.Sort` places pending rows after their run's newest
row. Contracts DUR-5, CLI-21, and the Web search note, INSPECT.md, and
ARCHITECTURE.md were updated.
**Reason:** Pending jobs were `unknown` in `show` and the Web, absent from
`jobs`, and uncounted in `info`. The Web loader and reports re-implemented
status rules, and the accepted label was composed differently in four places.
**Plan impact:** Reversed the 2026-10-08 decision never to emit a not-started
status, using positive evidence instead; the user chose `pending` over
`not started`.
**Validation:** New conformance tests `TestNotStartedJobsAgreeAcrossViews`
(now asserting `pending`), `TestReportLabelsJobsAsShowDoes`, and
`TestAcceptedJobLabelAgreesAcrossViews` failed on the pre-fix commits in
temporary worktrees; `scripts/check.sh` passed on each committed state.
**Remaining:** None.

## Attachment cleanup

- `d5788547` 2026-10-10 00:24:04

**Change:** `attachment.ForgetRun`, called by `delete`, removes a deleted
run's marker and unheld session records; session scans no longer create lock
files. Added DUR-9 and `TestDeleteRemovesRunAttachmentState`.
**Reason:** `.rotari-attachments/<RUN_ID>.enabled` outlived deleted runs.
**Plan impact:** None.
**Validation:** The conformance and scan tests failed before the fix;
`scripts/check.sh` passed.
**Remaining:** None.

## CLI usability

- `0e394bc9` 2026-10-10 00:46:09 — recovery commands on their own lines
- `304f96c7` 2026-10-10 00:48:33 — `projects` columns sized to content
- `3452707f` 2026-10-10 00:51:04 — one control hint for multi-run `wait`
- `9aee9250` 2026-10-10 01:11:07 — failed counts in `info`
- `637bb405` 2026-10-10 01:13:39, `d9e095a6` 2026-10-10 01:18:21 — `runs --json`
- `c8925b8e` 2026-10-10 01:39:15 — one-line option errors with a help pointer
- `abd944b6` 2026-10-10 01:54:46 — remove `-o`
- `fa511cea` 2026-10-10 01:58:33 — `jobs --json`
- `69f5d1f1` 2026-10-10 02:06:22 — warn before recovery while jobs may run

**Change:** The listed CLI changes, with regenerated CLI reference, Python
CLI, golden outputs, and flag-pair inventory where options changed; CLI-21
and INSPECT.md updated; `project.UnconfirmedStopWarning` shared by `wait`'s
hint and `EnsureIdle`.
**Reason:** Usability findings from the review; `-o` came from Slurm's
`squeue -o` and was attached to every `--format` by the global short-flag
map, against the project's short-option policy.
**Plan impact:** Decided the short-option and `--json` conventions in
[plan.md](plan.md).
**Validation:** Each change added a failing test first. One full check failed
after `runs --json` because the flag-pair inventory pinned the old schema;
`d9e095a6` updated it and the rerun passed. A commit made at 02:06 briefly
included another thread's staged deletion; it was recreated before any push
without it.
**Remaining:** None.

## Run cancellation and notifications

- `6ce72e8c` 2026-10-10 03:20:36 — webhook run events chosen by exit code
- `0b017509` 2026-10-10 03:33:01 — record a cancelled run as `cancelled`

**Change:** Webhook settings decide run success by exit code 0. A run whose
whole-run cancel was requested and that did not succeed is recorded as
`cancelled` with exit code 1 (`run.BuildRunSummary` from the project's
`cancelling` phase); `runview`, the completion title, browser notifications,
and history search follow it. Added CAN-8, `TestWholeRunCancelRecordsCancelledRun`,
and notification and JavaScript outcome tests; RUNNING.md, INSPECT.md, and
NOTIFICATIONS.md were updated.
**Reason:** Cancelled runs were indistinguishable from failed ones; the
webhook compared run status with `"success"`, which a run never has, so every
run used `run_failure`; browser notifications would have reported a cancelled
run as a success.
**Plan impact:** Decided the cancel semantics in [plan.md](plan.md).
**Validation:** New tests failed before the fixes; `scripts/check.sh` passed
on a worktree of the change; thirty repeats of the Ctrl-C conformance tests
passed afterwards.
**Remaining:** Items 2 and 3 in [plan.md](plan.md).

## Plan and work log

- `283f8211` 2026-10-10 04:37:15

**Change:** Added this directory's `plan.md` and `work-log.md`.
**Reason:** The user asked for a note of the decisions and remaining work.
**Plan impact:** Created the plan.
**Validation:** `go test ./internal/doclinks` and pre-commit passed.
**Remaining:** See [plan.md](plan.md).

## Agent-trial leftovers: wording and diagnosis summaries

- `3ccb9528` 2026-10-10 04:39:53 — word the retry source notice once
- `3751a4ce` 2026-10-10 04:45:39 — keep recorded causes and `no_match` out of
  diagnosis summaries

**Change:** `projectrun.SourceNotice` now composes the notice that run or
retry used the non-empty queue, for both the supervisor and
`retry --dry-run`, naming the queue, the latest run, and the jobs it left out.
`runlineage.SummarizeDiagnoses` skips failures whose cause rotari recorded
(blocked, cancelled, timeout), using the failure groups' classification; the
`lineage` text and the Web run summary omit `no_match`, while JSON keeps it.
RUN-14's conformance test and CAN-5's contract and test were updated.
**Reason:** Items 5 and 4 of [plan.md](plan.md), from ISSUES.md's
zero-information agent trial leftovers; the user chose to skip recorded
causes and hide `no_match` in text.
**Plan impact:** Items 4 and 5 done.
**Validation:** The updated RUN-14 and CAN-5 conformance tests failed on the
pre-change commit in a temporary worktree and passed after; new unit, CLI
text, and Node-based JavaScript tests failed before their changes;
`go test` of the touched packages passed.
**Remaining:** Plan items 1–3 and 6–8.

## Implicit wait follows its own shell's runs

- `11b67bc5` 2026-10-10 05:18:17

**Change:** `run` and `retry` send the launching process (host, parent PID,
its start time) to the supervisor, which records it in the run context as
`launch_origin`. `wait` without a selector follows the active runs that its
own parent process started, attached or not, and registers an independent
session for each; `--all` follows every active run and rejects selectors.
When no run of the scope is active but others are, it says so on stderr and
points to `--all`; interrupted-run warnings use the same scope. Process start
times moved to `state.ProcessStart`, shared with attachment sessions; the
origin helpers are `attachment.CurrentLaunchOrigin` and `SameLaunchOrigin`.
RES-16, CLI-19, RUNNING.md, the implicit-location design note, the agent
guide (name the project, since each tool call is a new shell), and the
generated CLI reference, Python CLI, and schema golden were updated.
**Reason:** With two scripts waiting at once, the second implicit `wait` saw
the first one's attachment, skipped the run, and returned 0 while it ran.
The user did not want a synchronous `run` treated differently from
`run --async` plus `wait`, rejected terminal-session scoping because two
scripts in one terminal share a session, and chose parent-process scoping,
noting that Makefile recipes name the project or run anyway.
**Plan impact:** Replaced the earlier decision that implicit `wait` skips
attached runs; see [plan.md](plan.md).
**Validation:** `TestWaitWithoutSelectorWaitsForRunsThisProcessStarted`
failed before the change. Tests that pinned the old skipping were rewritten
for the new contract; the flag-pair harness learned `--all`. `go test` of the
touched packages, the Python tests, `scripts/check.sh --short`, and
`scripts/check.sh` on a worktree of the commit passed.
**Remaining:** None for this decision. Runs started before this change have
no recorded origin, so only `--all` or a selector waits for them.

## `cancel --wait` with a job selection

- `9e16f485` 2026-10-10 08:19:32

**Change:** `jobcontrol.Controller.Cancel` waits, with a job selection and
`--wait`, until each selected job has stopped (`finishJobCancelMessage`):
its latest attempt has ended and, for a local job, its process group is gone;
a job not dispatched yet counts once the cancel marks it. The CLI no longer
rejects the combination. `jobstatus` reads a job directory holding only the
cancel marker as pending. CAN-3, SEL-12, the `--wait` help, RUNNING.md, and
ARCHITECTURE.md were updated; tests that pinned the rejection now expect the
selected jobs to be cancelled.
**Reason:** Plan item 1: nothing could block until a cancelled job had
stopped. The new conformance test showed that a local job's wrapper records
`cancelled` before a command that traps SIGTERM exits, so the wait also
checks the process group; the broader issue is open in ISSUES.md. A job
cancelled before dispatch showed `unknown`, which the jobstatus change fixes.
**Plan impact:** Item 1 done.
**Validation:** `TestCancelJobWaitReturnsOnceTheJobStopped` failed before the
change; `go test` of the touched packages, the flag-pair and selector suites,
and `scripts/check.sh` on a worktree of the commit passed. During
`scripts/check.sh --short`, `TestSynchronousRunInterruptCancelsAcceptedRun`
timed out once under repository-wide load (recorded in ISSUES.md); it passed
20 isolated repeats and three full package runs.
**Remaining:** The commit also swept in another thread's uncommitted
single-lock-read change to `resolveActiveWaitTargets` in `cmd/rotari/wait.go`,
because the files were staged from `git status`. The change is correct and
passed the full check, so it was kept rather than rewriting history. Two
seconds after the commit the index was found reset to the previous commit's
content for the committed paths, apparently by another thread's tooling;
`git reset HEAD -- <paths>` restored it without touching the working tree.

## Hints built outside `cmd/rotari` follow CLI-22

- `81165b63` 2026-10-10 14:30:35

**Change:** `project.EnsureIdle` and `project.RerunCommand` print their
recovery commands through `commandLocation`, which defaults to
`ExplicitCommandLocation` and which CLI commands replace with `hintLocation`
through `SetCommandLocation` (not `__server`, `web`, or `mcp`). The run request
carries the CLI's rendering as `hint_location`, which the supervisor uses for
the retry source notice. Refusals list one command per line. CLI-22,
ARCHITECTURE.md, and an `unlock` test that pinned the old wording were
updated; ISSUES.md's agent-trial leftovers item is now empty and removed.
**Reason:** Plan item 6: those hints always named `--basedir`, because the
packages that build them cannot read the CLI's environment and configuration.
A typed error was rejected because many callers print errors through `%v`,
which would drop the type.
**Plan impact:** Item 6 done.
**Validation:** `TestRefusalAndSourceHintsNameOnlyANonImplicitBaseDir`
failed before the change for the implicit state directory and passed for
both cases after; `go test ./conformance/...` and the touched packages
passed; `scripts/check.sh` passed on a worktree of the commit. The commit
staged only its own twelve paths, and the index matched HEAD afterwards.
**Remaining:** None.

## A cancel is recorded only after the command exits

- `08120c58` 2026-10-10 15:54:38

**Change:** The status wrapper's `on_signal` (`internal/executor/wrapper.go`),
shared by local, SSH, and scheduler wrappers, forwards SIGTERM, waits for the
command, and writes `cancelled` only after it exits; after
`timeoutGraceSeconds` a grace timer records the cancel and kills the process
group. The forked job resets TERM and checks a `status.json.stopping` marker
before exec, and the trap takes the job's PID from `$!` when the signal
arrived during the fork. The grace timer and the timeout watchdog name their
sleep by `$!`. CAN-1, RUNNING.md, and ISSUES.md (moved to Resolved) were
updated.
**Reason:** The ISSUES.md item found while implementing `cancel JOB --wait`:
the wrapper recorded `cancelled` while a command that traps SIGTERM still
ran, and a command ignoring SIGTERM was never stopped.
**Plan impact:** None beyond the issue.
**Validation:** `TestLocalCancelWaitsForCommandToExit`,
`TestLocalCancelKillsCommandIgnoringTerm`, and
`TestCancelledJobsStopBeforeTheyAreRecorded` failed before the change (the
last verified on the pre-change commit after giving the command a marker
`JobProcesses` can see). Waiting exposed hidden races: under CPU load
`TestLocalJobCancelStopsCommandBeforeForegroundExec` took the 30-second
grace in 4 of 8 runs (the existing test only timed the wrapper, so the
orphaned command had gone unnoticed), and a stress script reproducing
`TestCLIFlagPairCancel`'s fixture timed out 2 of 32 times on an orphan timer
sleep. After the fixes, under load: 15–20 repeats of each executor cancel
and timeout test, 48 stress iterations without a timeout, the pair suite,
two `scripts/check.sh --short` runs, and `scripts/check.sh` on a worktree
of the commit passed.
**Remaining:** None.

# Work History: Agent-Facing MCP Interface

See [plan.md](plan.md) for current scope and status. Historical one-line notes did not preserve test results unless explicitly stated below; test files in diffs are not assumed to have passed.

## Explore the interface and prototype read-only inspection

**Commits:** `820acb4`; `d8a140f`; `d30dc7a`; `d8de696`.

**Change:** Added/reframed the agent-facing plan, explored master-directory-scoped discovery, and built an initial read-only MCP job-inspection prototype with command/package/docs/test changes.

**Reason:** Explore a typed agent interface with discovery limited to registered state.

**Plan impact:** Established the initial MCP direction and prototype; the agent trial later refined these assumptions.

**Validation:** Original one-line notes do not record test commands or results.

**Remaining:** Treat the initial raw-target model and command structure as superseded where the current plan differs; M0 remains next.

## Share inspection behavior and organize MCP commands

**Commits:** `854261d`; `35b7ba4`.

**Change:** Shared job inspection between MCP and terminal-facing code, grouped MCP server/agent entry points under `cmd/mcp`, and updated architecture/MCP docs.

**Reason:** Avoid duplicate behavior and clarify command/package ownership.

**Plan impact:** Advanced the thin-adapter direction and reorganized entry points; subsequent planning retired the duplicate agent command.

**Validation:** Historical notes do not preserve executed test commands or results.

**Remaining:** Continue with M0 and shared capabilities; do not assume the prototype command layout is final.

## Record the agent trial and reprioritize milestones

**Commits:** `167b6ee`; `c12fb5d`; `bc22864`.

**Change:** Explored a query/projection model, added a reproducible CLI agent-trial note and fixture, and rewrote the plan around observed information gaps. The current plan prioritizes compact shared summaries, treats CLI `--json` as the shell-agent interface, and keeps MCP a thin adapter.

**Reason:** The trial found that the CLI supports the basic fix loop; output volume and missing decision-ready summaries were the primary gaps.

**Plan impact:** Replaced generic query/projection with shared, task-shaped capabilities and M0-M7 milestones; recorded four CLI issues in `ISSUES.md`.

**Validation:** The fixture ran from scratch and reproduced 7/15 failures in `labA` and 84/300 in `labC`. The trial records calls, exit statuses, and output sizes. No Go tests ran for the fixture/docs change; `shellcheck` was unavailable. The plan rewrite was documentation-only and its note says referenced packages/commands were checked against the tree.

**Remaining:** Repeat the trial after each milestone; test an MCP-only client when read-only MCP tools exist. M0 is next.

## M0: Retire `rotari-agent` and locate MCP runs through the master registry

**Commits:** 2026-10-02 22:10:34 `dac2691`.

**Change:** Removed `cmd/mcp/agent` (`rotari-agent`) and its test. `rotari_get_job_info` in `internal/mcp` now takes only `run_id` and `job_id`; `NewServer` takes the master directory, and the tool finds the run's basedir and project through that directory's run registry. Added `resolve.RegisteredRun`, which has no fallback to a default or working-directory basedir, and moved the stale-run check into `requireRunDirectory`, shared with `resolve.ExistingRun`. `cmd/mcp/server` resolves the master directory with `state.ResolveMasterDir` and exits non-zero when it cannot start or fails. Updated `docs/MCP.md`, `docs/ARCHITECTURE.md`, and `plan.md`.

**Reason:** M0 of the plan. The prototype took a raw basedir path from the agent, `rotari-agent` duplicated `rotari show --report` and depended on the MCP package, and the server exited 0 on failure.

**Plan impact:** M0 is done. The "MCP configuration" decision now names the run registry as the only route to a run. Structured result fields beyond identity were deferred to M4, where they will come from the M1-M3 shared capabilities instead of a projection inside the adapter. M1 (failure grouping) is next.

**Validation:**
- New `TestRegisteredRunHasNoFallback` (`internal/resolve`) covers: a run in another master directory, a stale entry, and `/` and `\` in run IDs.
- Rewritten `internal/mcp` tests cover two same-named projects in different basedirs over MCP, an input schema without location fields, and unregistered, path-unsafe, and unknown run and job IDs.
- `go test ./internal/resolve ./internal/mcp ./internal/archtest` passed. `go test ./conformance/...` passed in all seven packages.
- `scripts/check.sh --short` and `scripts/check.sh` failed only in `TestWebJobsPageShowsRecentJobs`, which also fails on the parent commit and is already in `ISSUES.md`.
- The built `rotari-mcp` was run over stdio against the trial fixture. It returned the timed-out `labA` task's report from `run_id` and `job_id` alone, reported an unregistered run as a tool error, and exited 1 when no master directory could be resolved.
- `pre-commit` is not installed here and was not run; `gofmt -l` and `go vet` reported nothing.

**Remaining:** M1, failure grouping. `TestWebJobsPageShowsRecentJobs` remains open in `ISSUES.md`.

## M1: Group a run's failures by cause

**Commits:** 2026-10-02 22:26:28 `3f59a01`; 2026-10-02 22:26:36 `85a41aa`; 2026-10-02 22:27:16 `a254005`.

**Change:**
- Added `runlineage.FailureGroups` (`internal/runlineage/failures.go`). It classifies each failed or blocked job on its own result, in this order: a recorded block, cancellation, or timeout; then the latest saved rule diagnosis; then the remaining `model.FailureKinds` kind.
- Groups carry the count, the carried count, the exit codes, an example (job, attempt, evidence), the diagnosis suggestion, and every member.
- `runlineage.Job` gained the resolved `Result`, set by `runview.LoadRun` and by the Web loader's `buildLineageSummary`, which now also passes array task IDs and carried flags.
- `show` for a run prints `Failures by cause:` for the jobs its table lists, and `show --json` adds `failures`. `lineage RUN` prints and returns the same groups, and the Web API's `lineage_summary.failures` carries them. The run page shows them as `Failure causes`.
- The CLI text is in `cmd/rotari/failure_groups.go`.
- Added contract CLI-4 with `TestFailureGroupsAgreeAcrossViews`, and updated `docs/INSPECT.md` and `docs/ARCHITECTURE.md`.
- Recorded in `ISSUES.md` that conformance results can be stale from the Go test cache (`85a41aa`).
- Added the M1 trial note and updated `plan.md` (`a254005`).

**Reason:** M1 of the plan. The first trial needed about 100 KB of output to learn three causes in a 300-task run.

**Plan impact:**
- M1 is done. Failure grouping is owned by `runlineage`, beside the existing `SummarizeDiagnoses`, which resolved the owner open decision. The existing diagnosis counts are unchanged.
- `model.FailureKinds` replaced the planned error-line normalization.
- The 2 KB criterion is met by `lineage RUN` (1.5 KB for `labC`), but not by `show -r RUN` (77 KB, dominated by its job table). Making the compact view the obvious first call became an M3 item.
- Timeout diagnosis in reports stays in M2.

**Validation:**
- New tests:
  - `TestFailureGroupsClassifiesEachJobByCause`: distinct exit codes within one cause, a carried task, a timeout whose log also matched a rule, a signal, a cancellation, no match, and a blocked matrix member.
  - `TestFailureMemberLabels`
  - `TestShowAndLineageAgreeOnFailureGroups`: text, JSON, and `--success` filtering.
  - `TestBuildLineageSummaryGroupsFailuresByCause`
  - `TestWebRunPageShowsFailureCauses`, run in jsdom. It fails with the JS change reverted and passes with it.
  - `TestFailureGroupsAgreeAcrossViews`: the built binary and the Web API.
- `go test -count=1 ./conformance/...` passed in all seven packages; an earlier cached run had not re-tested four of them.
- `scripts/check.sh` failed only in the known `TestWebJobsPageShowsRecentJobs`.
- `prettier --check` passed for `web_app_core.js`. `pre-commit` is not installed.

**Remaining:** M2 (relevant excerpts, timeout diagnosis in reports, cause-aware `lineage` comparison). M3 now includes making the compact summary discoverable from `show`. The stale-cache conformance issue is open in `ISSUES.md`.

## M2: Relevant excerpts, timeout diagnosis, and cause-aware comparison

**Commits:** 2026-10-02 22:35:00 `0e805c2`; 2026-10-02 22:35:00 `d50b977`; 2026-10-02 22:35:01 `94cd80c`; 2026-10-02 22:35:01 `a00d427`; 2026-10-02 22:35:31 `02acad4`.

**Change:**
- `0e805c2`: added the "Job timeout reached" default rule (`internal/diagnose/default_rules.go`, `docs/LOCAL_DIAGNOSIS.md`). It matches only the wrapper's `rotari: job timed out after` line and the exact `timed out after DURATION` error.
- `d50b977`: `internal/report` picks log lines with `reportLogExcerpt`. When a saved diagnosis's evidence is in the log, the excerpt keeps the 20 lines before and 5 after the latest line with each evidence, plus the last 20 lines, and marks omitted lines. Otherwise it keeps the last 100 lines. The section heading names the selection. `docs/INSPECT.md` was updated.
- `94cd80c`: `runlineage.JobDiff` gained `from_cause`, `to_cause`, and `cause_changed`, and `Summary` gained `cause_changed`. Causes come from the new `runlineage.FailureCause`, which shares the classification with `FailureGroups`. The `lineage` comparison table gained a `CAUSE` column. `docs/INSPECT.md` was updated.
- `a00d427`: extended the `ISSUES.md` positional-argument entry to attempt IDs with `--report`.
- `02acad4`: updated the plan and added the M2 trial note.

**Reason:** M2 of the plan. The first trial's fix loop could not tell that a still-failing timeout was the same failure. Reports labelled rotari's own timeout as unexplained, and report logs were fixed tails.

**Plan impact:**
- M2 is done. M3 (discovery and progress) is next.
- Contract CLI-4 was deliberately not extended to comparisons, because no conformance test checks them.
- The rule change sets a new rules version, so earlier saved analyses show the "earlier rules" note.

**Validation:**
- New tests:
  - Rule cases for the timeout line and error, and lookalike cases (`request timed out after 30s`, a quoted rotari line) that must not match.
  - `TestReportLogExcerptKeepsEvidenceFarFromTheEnd`: two evidence windows, the tail, and the exact omitted counts.
  - `TestReportLogExcerptFallsBackToTheTail`
  - `TestCompareReportsFailureCauses`: changed, same, fixed, and newly failing causes.
  - `TestDiffCause`
- Existing `TestBuildIncludesDiagnosisAndBoundedLog` still passes, because its evidence is not in the log.
- The fix loop was rerun on the trial fixture with the new binary, as recorded in the M2 trial note.
- `go vet ./...` passed. `go test -count=1 ./conformance/...` passed in all seven packages.
- `scripts/check.sh` failed only in the known `TestWebJobsPageShowsRecentJobs`.
- `pre-commit` is not installed.

**Remaining:** M3. The open positional-argument issue (`show RUN_ID|ATTEMPT_ID --json|--report`) affects documented examples.

## M3: Discovery, a summary pointer, and early failure

**Commits:**
- Discovery: 2026-10-02 22:41:59 `5353a25`; 2026-10-02 22:43:04 `766bd04`; 2026-10-02 22:43:05 `0ee31dc`.
- Summary pointer: 2026-10-02 22:44:25 `427d327`.
- Early failure and guide: 2026-10-02 22:52:12 `198659d`; 2026-10-02 22:52:12 `594b6c4`.
- Plan and trial note: 2026-10-02 22:52:46 `14ad825`; 2026-10-02 22:53:00 `c892845`.

**Change:**
- `5353a25`: the `show` project list gained `LAST RESULT` (for example `failed 84/300`). Its hints add `-b BASEDIR` when a listed project lies outside the default state directory, and it suggests `rotari lineage RUN_ID`. Added contract CLI-5, `TestProjectListHintsWork`, and `TestShowProjectListHintsWorkForListedBaseDirs`. Updated the expectation of `TestCmdShowProjectsListsProjectSummaries`, which encoded the broken hint. Removed the resolved `ISSUES.md` entry.
- `0ee31dc`: when `jobs` finds nothing, it names the searched state directory or count, the project, and the window, and suggests `--all-basedirs`.
- `766bd04`: recorded in `ISSUES.md` that the documented `jobs --since 7d` is rejected.
- `427d327`: `show -r RUN` prints `Failure summary: rotari lineage --basedir ... --project-name ... RUN_ID` before its job table when jobs failed.
- `198659d`:
  - Added `wait --until-failure`. `runlineage.Job.Final` is set by `runview.LoadRun` from the run summary or `final_result.json`, and that recorded result supplies diagnoses before the summary exists.
  - Added contract RUN-6 with two conformance tests, and a Python client test for `until_failure`.
  - Regenerated the CLI reference, Python API docs, `generated_cli.py` (with ruff 0.14.10, as in CI), and the golden help and schema.
- `594b6c4`: `rotari guide` now leads with `lineage RUN_ID`, `show -j ATTEMPT_ID --report`, `lineage RUN NEW_RUN`, and `wait --until-failure`.

**Reason:** M3 of the plan. The first trial spent four calls finding the failed project, including one through a failing hint. It read a 77 KB table before the failure groups, and could not learn of an early failure until the run ended. Separately, `rotari guide`, the documented agent entry point, recommended the costly path and a command that fails.

**Plan impact:**
- M3 is done. M4 (read-only MCP tools) is next.
- Two M3 items were not built:
  - a cursor of changes since the last call, because returning at the first final failure covered the need;
  - effective per-job settings, because the comparison now exposes the original mistake and `show --json` already has the settings.

**Validation:**
- The hint tests and the RUN-6 retry test fail on the pre-change behavior. The hint tests were run in a temporary worktree at the previous commit. For the retry test, ignoring `Final` was injected by hand and then reverted. That test first passed even with `Final` ignored, so it now uses `--retry-delay 3s`.
- `go test -count=1 ./conformance/...` passed in all packages after updating the golden files. The golden diff is only the new flag.
- `scripts/check.sh` failed only in the known `TestWebJobsPageShowsRecentJobs`. Python tests passed (28).
- The fixture measurements are in the M3 trial note.
- `pre-commit` is not installed; ruff format was applied to the Python files.

**Remaining:** M4. The positional `show RUN_ID|ATTEMPT_ID --json|--report` failure and `jobs --since 7d` remain in `ISSUES.md`.

## Before M4: Fix the issues that affect verification and shared selection

**Commits:** 2026-10-02 22:56:04 `457b7fd`; 2026-10-02 22:59:32 `fddf05a`; 2026-10-02 23:03:07 `d64b4b6`; 2026-10-02 23:06:33 `b18399f`.

**Change:**
- `457b7fd`: `TestWebJobsPageShowsRecentJobs` matched the Job activity heading on one line, but the prettier-formatted template splits it. The test had never passed. It now uses a whitespace-tolerant pattern, and the `ISSUES.md` record was corrected: the link was never wrong.
- `fddf05a`: the conformance packages build rotari through `support.BuildRotari`. Every command that runs the binary calls `support.TrackBuildInputs`, which stats `cmd/`, `internal/`, `go.mod`, and `go.sum` during the test run, so Go's test cache reruns conformance when rotari's sources change.
- `b18399f`: the package boundary rule now allows the root `conformance` package to import `conformance/support`, and `contracts/00-overview.md` and `ARCHITECTURE.md` were updated. `fddf05a` had broken that rule; I committed it before running `scripts/check.sh`.
- `d64b4b6`: `show --json --failed` now applies the selection to the run, array, and job JSON views. The table and the JSON views share `selectsShownJob`, which gathers facts for `jobfilter.Filter.Selects`. Added `TestShowJSONAppliesFailedSelection`, two `TestSelectorTable` JSON rows, and a sentence in `contracts/06-selectors.md`.

**Reason:** M4 builds MCP tools on the shared selection and is verified by conformance runs and `scripts/check.sh`. The stale cache and the permanently failing test made those checks unreliable, and the ignored `--failed` would have carried into the MCP tools.

**Plan impact:** No milestone changed; these were prerequisites agreed before M4. `scripts/check.sh` passes for the first time in this plan's work.

**Validation:**
- The new selection tests fail on the previous commit, which was checked in a temporary worktree.
- The heading check fails when the link target changes, which was injected by hand and then reverted.
- Cache invalidation was checked by hand: unchanged sources stay `(cached)`, and editing `cmd/rotari/guide.go` or `internal/webui/assets/web_styles.css` reruns the packages.
- `go test -count=1 ./conformance/...` passed in all packages, and `scripts/check.sh` passed.

**Remaining:** In `ISSUES.md`: the positional `show RUN_ID|ATTEMPT_ID --json|--report` failure, array job names as dependencies, and `jobs --since 7d`. M4 is next.

## M4: Read-only MCP tools over the shared functions

**Commits:** 2026-10-03 01:02:06 `3ae47f4`; 2026-10-03 01:04:14 `0576d19`; 2026-10-03 01:09:42 `bf47bd5`; 2026-10-03 01:09:42 `f510d9e`; 2026-10-03 01:10:15 `dc40174`.

**Change:**
- `3ae47f4`: moved `projectRunsByStart` and the unused `previousRunID` from `cmd/rotari/lineage.go` into `runview` as `RunsByStart` and `PreviousRun`. Added `runview.Summary`, which `lineage RUN` now uses.
- `0576d19`: moved the project list's per-project facts into `project.Overviews` (and `CountRuns`). Added `basedirregistry.Discover`, which lists basedirs without the CLI's registry migration, and `basedirregistry.Ref`. CLI output is unchanged.
- `bf47bd5`: `internal/mcp/tools.go` adds `rotari_list_projects`, `rotari_run_summary`, and `rotari_compare_runs` beside `rotari_get_job_info`. `report.RedactPatterns` was exported for evidence lines. Updated `docs/MCP.md` and `docs/ARCHITECTURE.md`.
- `f510d9e`: recorded two issues found while testing: whole-run `cancel` results are classified as `signal`, and `internal/archtest` has stale cache passes.
- `dc40174`: added the MCP-only trial script and note, and updated the plan.

**Reason:** M4 of the plan: give agents without a shell the same compact information as the CLI path, through shared functions rather than a second implementation.

**Plan impact:**
- M4 is done.
- Decided:
  - no blocking `wait` tool;
  - the comparison tool defaults to the previous run, instead of adding a run-listing tool;
  - evidence uses pattern-only redaction, while per-run redaction stays in reports.
- The open decision on redaction ownership is narrowed but not closed.

**Validation:**
- New tests:
  - `TestRefNamesTheRecordWithoutThePath`
  - `TestDiscoverFallsBackToRunRecordsWithoutWriting`
  - `TestPreviousRunFollowsStartOrder`
  - `TestListProjectsNamesBaseDirsWithoutPaths`, which checks that no absolute path appears.
  - `TestRunSummaryGroupsFailuresAndRedactsEvidence`
  - `TestCompareRunsDefaultsToThePreviousRun`
  - `TestCompareRunsRejectsRunsOfDifferentProjects`, which also covers an unsafe `previous_run_id`.
  - The MCP schema test now checks that no tool takes a location.
- `scripts/check.sh` passed. `go test -count=1 ./internal/archtest ./conformance/...` passed.
- The MCP-only trial ran against the fixture over stdio.

**Remaining:**
- M5, which requires extracting `check` and `export` from `cmd/rotari`.
- Open issues that affect agents: the positional `show RUN_ID|ATTEMPT_ID --json|--report` failure, cancelled jobs classified as `signal`, and archtest's stale cache.

## After M4: Clear the open issues found during this work

**Commits:** 2026-10-03 01:43:41 `a9beda4`; 2026-10-03 01:45:45 `967838e`; 2026-10-03 01:46:42 `05ad304`; 2026-10-03 01:49:36 `84fbc2e`; 2026-10-03 01:53:03 `3373408`.

**Change:**
- `a9beda4`: `show` with an exact positional selector (attempt ID, run ID, `latest`, project) now takes log, follow, JSON, and report options like its option form. A run name, job ID, or job name selector still rejects them, with a clearer message. Contract SEL-10 changed: it had excluded output options for every positional, while the docs and the agent guide used `show RUN_ID --report`.
- `967838e`: a job stopped by `cancel` records the error `cancelled` (`model.CancelledError`, set in the runner's final-result hook). `--filter-failure-kind cancelled` and failure groups now read such jobs as cancelled, not as `signal` or `error`. Added contract CAN-5.
- `05ad304`: `internal/archtest` stats the module's Go sources during its test, so its cached result follows source changes.
- `84fbc2e`: a named array job's name stands for all of its tasks in `--depends-on` and `--depends-on-finished`, as stage and matrix names already did. Added contract RUN-7.
- `3373408`: `jobs --since` and the Web jobs page accept whole days such as `7d`. Added contract CLI-6, and regenerated the CLI reference and Python metadata.

**Reason:** These issues were recorded during M0-M4. Three affect agents directly: documented commands failed, and cancellations were misclassified in failure groups.

**Plan impact:** No milestone changed. `ISSUES.md` has no open items. The agent guide's `show -j ATTEMPT_ID --report` form keeps working, and the positional form now works too.

**Validation:**
- Each fix's new tests failed on the previous commit before the fix: the SEL-10 rows, `TestCancelledJobsReadAsCancelled` for both kinds of cancel, `TestArrayNameDependsOnEveryTask`, and `TestJobsWindowAcceptsDays`.
- The archtest cache was checked by hand: it reran after editing or adding a Go file, and stayed cached otherwise.
- `scripts/check.sh` passed. `go test -count=1 ./conformance/...` passed in all packages, and the generator `--check` steps passed.

**Remaining:** M5 (`check` and `export` as read-only operations), which needs their extraction from `cmd/rotari`.

## M5: Expose `check`; move `export` to M6

**Commits:** 2026-10-03 01:58:25 `515a956`; 2026-10-03 01:58:26 `229fd78`; 2026-10-03 01:59:14 `48ab65f`.

**Change:**
- `515a956`: moved `rotari check`'s logic into `projectrun.Runner.Check`. The `--deep` host checks are passed in as an optional function. The CLI output is unchanged.
- `229fd78`:
  - Added the MCP tool `rotari_check_project` (`basedir_ref` and `project` in; state, runnability, queue size, and lock out), and `basedirregistry.Find` to resolve a `basedir_ref`.
  - Error messages replace the basedir path with `BASEDIR`.
  - The MCP schema test now allows `basedir_ref` and a project name, while still rejecting inputs that take a path.
  - Updated `docs/MCP.md` and `ARCHITECTURE.md`.
- `48ab65f`: updated the plan and added a check call to the MCP trial script.

**Reason:** M5 of the plan. `check` lets an MCP-only agent tell a person whether a queued run can start.

**Plan impact:**
- M5 is done for `check`.
- `export` moved to M6, with its extraction from `cmd/rotari`. A run's manifest carries environment values, absolute working directories, and commands, which the principles keep out of default results. A redacted manifest could not be imported, and import is `export`'s main use.
- M6 must therefore decide what an exported manifest may carry through MCP.

**Validation:**
- New tests:
  - `TestFindResolvesARef`
  - `TestCheckProjectReportsReadinessWithoutPaths`, which covers empty, ready, an invalid executor, an unknown ref, an unknown project, and an unsafe project, and checks that no error contains the basedir path.
- The existing `rotari check` tests and `TestCheckJSONMatchesText` (CLI-1) pass after the move.
- `go test -count=1 ./internal/archtest` passed.
- The MCP trial script's check call returned `empty` in 135 bytes.

**Remaining:** M6 needs design decisions before implementation; see the plan's open decisions.

## M6 (CLI part): Preview and guarded apply for every state-changing command

**Commits:** 2026-10-03 02:08:46 `ce00901`; 2026-10-03 02:17:13 `ac747a5`; 2026-10-03 02:19:35 `d639434`; 2026-10-03 02:21:27 `0f4a779`; 2026-10-03 02:29:30 `d04ab62`; 2026-10-03 02:29:30 `dcf747d`.

**Change:**
- `ce00901`:
  - Added `project.Revision`, a hash of `queue.json` and `meta.json`.
  - Added `project.Guard{DryRun, IfRevision, Report}`, `EditGuarded`, `EditQueueGuarded`, and `CheckRevision`. The revision is compared under the state lock, and a stale one returns `ErrRevisionChanged`.
- `ac747a5`: `queueops.Editor.Guard` routes add, change, remove, copy, and delete through the guard. `import`'s own `--dry-run` moved onto it, and its plan gained `revision`. The CLI flags come from one list, `guardedCommands`, and are command-line only. `check` (text, JSON, and the MCP tool) reports the revision. Added contract CLI-7 with a table test that gives every command the same preview, apply, and stale calls.
- `d639434`: `gc` now removes orphan entries by default, and `--dry-run` lists them. The cached `gc.json` plan and `--apply` are gone. The race check moved to a `RemoveOrphan` test. This is an incompatible change, which the user allowed.
- `0f4a779`: `reset` is guarded on both paths. The idle path uses `EditQueueGuarded`, and the interrupted path uses the new `RecoverInterruptedGuarded`. A dry run needs no `--recover` and creates nothing.
- `d04ab62`: fixed a SEL-10 row that still expected `gc`'s old output. `d639434` was committed after package tests only, without the conformance suite.
- `dcf747d`:
  - Supervisor run planning moved into `projectrun.Runner.PlanRun`.
  - `run --dry-run` and `retry --dry-run` plan with it, using the queue a copy would leave (from the copy's own dry run).
  - `--if-revision` guards the copy, and the run request carries the resulting revision, which the supervisor checks again before `Begin`.

**Reason:** M6 design decision 2: every change is previewed, then applied only at the previewed revision. The user chose `--dry-run` and `--if-revision` as the names, approved making `gc` consistent, and allowed incompatible changes.

**Plan impact:**
- The CLI half of M6 is done.
- Remaining for M6:
  - MCP tools for import preview and apply, and for starting a run.
  - Extracting `export` and deciding what an exported manifest may carry.
- Starting a run from MCP needs a supervisor process, which `rotari-mcp` cannot start by itself. This is an open decision.

**Validation:**
- Each step ran its package tests.
- New tests:
  - `TestEditQueueGuardedPreviewsAndChecksTheRevision`
  - `TestEditGuardedPassesDryRunAndChecksTheRevision`
  - `TestRecoverInterruptedGuardedPreviewsAndChecksTheRevision`
  - `TestRemoveOrphanKeepsChangedAndReappearedEntries`
  - `TestCmdGCRemovesOnlyOrphansAndDryRunKeepsThem`
  - `TestGuardedCommandsPreviewAndCheckTheRevision`: seven commands.
  - `TestRunPreviewMatchesTheRun`: a retry executes exactly the previewed jobs.
- Before the last two commits: `scripts/check.sh` passed, `go test -count=1 ./conformance/...` passed in all packages, the generator `--check` steps passed, and the Python tests passed (28).
- Lesson: run the conformance suite before every commit that changes CLI output.

**Remaining:** The MCP side of M6.

## M6 (MCP part): `rotari mcp`, previewed writes, and a redacted export

**Commits:** 2026-10-03 02:37:24 `e502d5e`; 2026-10-03 02:40:12 `e96b37f`; 2026-10-03 02:42:24 `3f644f5`; 2026-10-03 02:49:32 `136827e`; 2026-10-03 02:54:48 `cb83f70`; 2026-10-03 02:54:48 `024b50f`.

**Change:**
- `e502d5e`: the MCP server moved into the binary as `rotari mcp`, so it can start a supervisor. `rotari-mcp` (`cmd/mcp`) is removed; the trial script runs `rotari mcp`.
- `e96b37f`: workflow import and run sources moved from `cmd/rotari` into `internal/workflowstate`.
- `3f644f5`: `projectrun.RunSource` decides once whether a run copies from the last run first; `run`, `retry`, and their previews use it. An async run start returns the run ID in `server.Response.RunID`.
- `136827e`: MCP tools `rotari_preview_import`, `rotari_import`, `rotari_preview_run`, and `rotari_start_run`. Previews are read-only and return the revision; writes require `if_revision` and are refused at another one. Contract MCP-1.
- `cb83f70`: the settled-run check and loading of `rotari export` moved into `workflowstate.RequireSettledRun` and `LoadSettledRun`.
- `024b50f`: `rotari_export_run` returns the manifest with environment values and executor options replaced by `[REDACTED]` and paths redacted by pattern. The MCP import tools refuse a manifest that still holds a placeholder. Contract MCP-2.

**Reason:** M6 decisions 1 to 4, approved by the user: per-tool permission is the approval, every write is previewed and applied at a revision, MCP export is a redacted view, and import and run start come first. The user chose to integrate the server into the binary (option 1).

**Plan impact:**
- M6 is done for import, export, and run start. Run start was brought forward from M7.
- MCP queue edits (`add`, `change`, `remove`, `copy`, `delete`) are not exposed yet.
- The open decision on how MCP starts a run is closed.

**Validation:**
- New tests: `TestExportRunRedactsAndImportRefusesTheRedactedView`, `TestExportRunRefusesAnActiveRun`, the write tool tests in `internal/mcp/write_test.go`, and the conformance tests `TestMCPWritesApplyOnlyAtThePreviewedRevision` (MCP-1) and `TestMCPExportIsARedactedViewThatImportRefuses` (MCP-2), which run the built binary over stdio.
- Before `024b50f`: `scripts/check.sh` passed, and `go test -count=1 ./conformance/... ./internal/archtest ./internal/doclinks` passed.
- The agent trial has not been repeated with the write tools yet.

**Remaining:** An agent trial with the write tools; MCP queue edits or M7.

## M6 agent trial: an MCP-only agent fixes and reruns a project

**Commits:** 2026-10-03 03:03:13 `69b6de6`; 2026-10-03 03:10:02 `1efb12c`; 2026-10-03 03:13:34 `66aff94`; 2026-10-03 03:13:34 `be5a54c`.

**Change:**
- `69b6de6`:
  - `run.PlanRerun` now expands an array that runs whole into its tasks, so `run.Plan.Execute` is keyed by job ID for every reader. Before, `run --dry-run` and `rotari_preview_run` left those tasks out.
  - The run no longer expands the plan itself; `ExpandArrayPlan` is unexported.
  - The supervisor's start message counts the queue's jobs, not its commands.
  - Five tests that asserted the command-ID key were updated to the task keys.
- `1efb12c`:
  - Added `project.RunPhaseOf` (running, interrupted, finished, ended); `rotari wait`'s three state checks use it.
  - `rotari_run_summary` reports it as `state`, and a running run that has not written its jobs yet is running, not an error.
  - Every MCP tool is added through `addTool`, which replaces registered basedirs with `BASEDIR` in errors. The per-tool `hidePath` calls are removed. Contract MCP-3.
- `66aff94`: the `retry` input of the MCP run tools, and `docs/MCP.md`, say that a retry plans from the queue's own results when it has any, such as after an import.
- `be5a54c`:
  - The trial script gained a `write` scenario.
  - Added the trial note [agent-trial-2026-10-03-m6.md](agent-trial-2026-10-03-m6.md), and the plan's status and next step.
  - Recorded in ISSUES.md that `add` does not register the basedir.

**Reason:** the plan repeats the agent trial after each milestone. The write tools were tried by an agent that may change rotari state only through MCP.

**Plan impact:**
- M6 is confirmed by the trial. The trial needed no MCP queue edit tool.
- Next candidate: a bounded wait tool, from M7's progress inspection, because following a run took 31 polls.
- The output-size decision now also covers import plans and comparisons.

**Validation:**
- `TestRunPreviewListsTheTasksOfAWholeArray` fails on `dcc0899`: the dry run listed only the plain job.
- `TestRunSummaryFollowsAStartingRunAndHidesPaths` fails on `69b6de6`, with its `state` assertion removed because that commit has no `state` field. The starting run returned an error, and the abandoned run's error held the absolute path.
- New `TestRunPhaseOf`: six phases.
- Before `69b6de6` and before `1efb12c`: `scripts/check.sh` passed, and `go test -count=1 ./conformance/... ./internal/archtest ./internal/doclinks` passed.
- Before `66aff94`: `go test ./internal/mcp ./internal/doclinks` passed.
- The final trial pass on `1efb12c` completed. Its numbers are in the note.

**Remaining:**
- The ISSUES.md entry on basedir registration by `add`.
- A bounded wait tool.
- Output sizes of import plans, comparisons, and tool schemas.

## After the M6 trial: basedir registration by `add` and a bounded wait tool

**Commits:** 2026-10-03 03:24:12 `c24dc25`; 2026-10-03 10:51:05 `cc8ca55`.

**Change:**
- `c24dc25`:
  - `add` registered the basedir only when an option was invalid. Applied `add` and `copy` now register through `queueops.Editor.RegisterBaseDir`, and a dry run registers nothing; `copy` used to register even on a dry run.
  - `add --dry-run` of a new project failed while locking a directory that a dry run does not create. `project.CreateQueueGuarded` now previews and creates a project for `add` and `import`; it replaces import's own copy of that case.
  - The ISSUES.md entry moved to Resolved, and the contract's registry note names the implementation and test.
- `cc8ca55`:
  - Added `rotari_wait_run`: it waits for at most `timeout_seconds` (1 to 300, default 30) until the run settles or, with `until_failure`, until a job has failed with no retry left. It returns the run summary and the reason it returned. A cancelled call stops waiting.
  - `rotari wait --until-failure`'s failure check moved into `runview.FinalFailureGroups`, which both share.
  - `addTool` passes the request context to the tools.
  - Contract MCP-3 was extended.

**Reason:**
- Two findings of the [M6 trial](agent-trial-2026-10-03-m6.md):
  - A project that had only been added was unreachable through MCP.
  - Following a run took 31 polls.
- The registry note in contract 01 already required registration when a project is created.

**Plan impact:**
- The waiting part of M7's progress inspection is done.
- The next decision is between trimming outputs and schemas (20.6 KB for eleven tools) and the destructive rest of M7.

**Validation:**
- `TestCommandsThatCreateAProjectRegisterItsBasedir` fails on `0d783c3` for both causes:
  - As written, `add --dry-run` exits 1.
  - Without the dry-run step, `add did not register`.
- New unit tests:
  - `TestCreateQueueGuardedPreviewsANewProjectWithoutCreatingIt`
  - `TestAddRegistersTheBaseDirOnlyWhenApplied`
  - three `rotari_wait_run` tests
- New conformance test: `TestMCPWaitReturnsOnTheFirstFinalFailure`.
- Before each commit, `scripts/check.sh` passed, and `go test -count=1 ./conformance/... ./internal/archtest ./internal/doclinks` passed.
- A fresh trial pass on `cc8ca55` followed the run in one `rotari_wait_run` call.

**Remaining:**
- Output sizes of import plans, comparisons, and tool schemas.
- The destructive rest of M7.

## Summarized MCP import plans and comparisons

**Commits:** 2026-10-03 11:09:27 `7922a08`.

**Change:**
- Added `workflowstate.Plan.Summary`, which keeps each job's status, counts an array's tasks by status, and counts statuses over the plan. It drops commands and source attempts.
- `rotari_preview_import` and `rotari_import` return the summary. `detail` adds the full plan.
- Added `runlineage.JobDiff.Notable`: the rule for which jobs a comparison lists, which `rotari lineage` used inline. `rotari_compare_runs` lists only notable jobs and reports the rest in `hidden_unchanged`; `all_jobs` lists every job.
- Updated `docs/MCP.md`.

**Reason:** the [M6 trial](agent-trial-2026-10-03-m6.md) found that import results and comparisons grew with the number of jobs. The user agreed to trim them.

**Plan impact:**
- Import results fell from 3.3 KB to 401 B. The trial's comparison fell only from 4.0 KB to 3.7 KB, because its array tasks all carry a definition change.
- Measured the model-facing size of the tool definitions: 7.4 KB of 23.3 KB. This corrects the earlier claim that schemas are the largest fixed cost; output schemas were left as they are.
- The rest of M7 is next, with the user's go-ahead.

**Validation:**
- New tests: `TestPlanSummaryCountsTasksInsteadOfTheirArray`, `TestNotableListsChangedResultsAndDefinitions`, `TestCompareRunsCountsUnchangedJobsUnlessAllAreAsked`, and a `detail` check in `TestImportToolsPreviewThenApplyAtTheRevision`.
- `go vet ./...` passed.
- `scripts/check.sh` and `go test ./conformance/...` failed only in `TestCLIFlagPairEdits` and `TestCLIFlagPairEditSamples`:
  - Both come from another thread's uncommitted edits to `conformance/03-interfaces/flag_pair_*`.
  - `TestCLIFlagPairEdits` fails the same way at `cc8ca55` without this change, with only those files copied into a worktree.
  - The rest of `conformance/03-interfaces` (run with `-skip 'TestCLIFlagPairEdit'`) passed, as did the other conformance packages, `internal/archtest`, and `internal/doclinks`.
- A fresh trial pass on `7922a08` gave the sizes above.

**Remaining:**
- Failure groups name every member.
- Comparisons repeat a definition change per array task.
- The destructive rest of M7.

## M7: MCP job control and reset

**Commits:** 2026-10-03 11:16:59 `a7d6637`; 2026-10-03 11:19:27 `b7e51d1`; 2026-10-03 11:23:13 `8ef98d3`.

**Change:**
- `a7d6637`:
  - Added `rotari_preview_job_control`, which lists the unfinished jobs of a running run that an operation reaches, through `jobcontrol.Controller.Select`.
  - Added `rotari_cancel`, `rotari_suspend`, and `rotari_resume`, which call `Controller.Cancel` and `Control` with the run ID. A run that is no longer active is refused.
  - The states each operation reaches moved from `cmd/rotari` into `jobcontrol.States`.
  - Contract MCP-4.
- `b7e51d1`:
  - Added `project.Reset`, which clears an idle queue or recovers a confirmed interrupted run, and refuses a running project.
  - `rotari reset` keeps its confirmation and cancellation wait and calls it.
  - `resetQueueCommands`, with its own copy of the new-project dry run, is removed in favour of `CreateQueueGuarded`.
- `8ef98d3`: added `rotari_preview_reset` and `rotari_reset`, revision-guarded, with `recover_interrupted`. Contract MCP-5.

**Reason:**
- M7 of the plan; the user said to go on after being asked for a go-ahead for the destructive tools.
- `gc` was left CLI-only: its subject is stale registry paths, which the MCP principles keep out of results, and an agent has no decision to make there.

**Plan impact:**
- M7 is done.
- The server has 17 tools: 11.3 KB of model-facing definitions, plus 1 KB of instructions.
- Next: an agent trial of job control and reset.

**Validation:**
- New tests:
  - `TestResetByProjectState` (idle; interrupted, confirmed and unconfirmed; running; each as a dry run and applied)
  - conformance `TestMCPJobControlActsOnlyOnThePreviewedRunningRun` (MCP-4)
  - conformance `TestMCPResetRecoversOnlyAConfirmedInterruptedRun` (MCP-5)
- The existing reset and cancel conformance tests passed unchanged.
- `go vet ./...` passed.
- `scripts/check.sh` failed only in `TestCLIFlagPairEdits`, which belongs to another thread's uncommitted `conformance/03-interfaces/flag_pair_*` edits (see the previous entry).
- `go test -count=1 ./conformance/... ./internal/archtest ./internal/doclinks -skip 'TestCLIFlagPairEdit'` passed.

**Remaining:**
- An agent trial of these tools.
- Failure groups and comparisons that grow with the number of jobs.

## M7 agent trial: stopping a hang and recovering a crash

**Commits:** 2026-10-03 11:42:20 `64df260`; 2026-10-03 11:51:42 `418bbcf`.

**Change:**
- `64df260`: `Dispatcher.cancelledBeforeStart` checks the `cancelled` marker on every path that submits jobs. A native array is submitted without its cancelled tasks, falling back to single jobs when the scheduler cannot take the array that is left. Contract CAN-6.
- `418bbcf`:
  - `state.LatestAttemptDirs` replaces `ListRunJobDirs` and returns each job's latest attempt directory. The interrupted-run scan behind reset's warning now finds the jobs.
  - Added `support.KillSupervisors`, which kills only a run's supervisor, as a crash would.
  - Contract SAFE-7.
- The trial script gained a `control` scenario. Added the trial note [agent-trial-2026-10-03-m7.md](agent-trial-2026-10-03-m7.md).
- ISSUES.md records that carried jobs read as running during their run.

**Reason:** the plan repeats the agent trial after each milestone. The M7 tools were tried by an agent that may change rotari state only through MCP, on a hanging job and a killed supervisor.

**Plan impact:**
- M7 is confirmed by the trial.
- Both defects were in shared code, so the CLI's `cancel JOB_ID` and `reset` are fixed too.
- The carried-job view needs a design decision, because it adds persistent run state.

**Validation:**
- `TestDispatcherNeverSubmitsACancelledJob` fails on `026722a` in its local and two array cases; the batch case already passed.
- `TestCancelledPendingJobNeverStarts` failed before the fix, with the cancelled job's marker file present.
- `TestInterruptedResetWarnsAboutRunningJobs` failed before the fix, with no job detail in the message.
- New `TestScanInterruptedRunJobsReadsLatestAttempts`; `TestLatestAttemptDirsIncludesOnlyJobSnapshots` replaces the flat-layout test.
- Before each commit, `scripts/check.sh` and `go test -count=1 ./conformance/... ./internal/archtest ./internal/doclinks` passed.
- `64df260` was amended before any further work to drop two `contracts/README.md` hunks that belong to another thread's uncommitted work.
- The final trial pass on `418bbcf` left no job process behind.

**Remaining:**
- The ISSUES.md entry on carried jobs.
- Output sizes of failure groups and comparisons.

## Carried results recorded at the start of a run

**Commits:** 2026-10-03 12:20:43 `12b5395`.

**Change:**
- Before it dispatches any job, a run writes the results it carries to `carried.json` (`state.CarriedResultsFileName`), a `model.RunSummary` holding only those results; `CheckRunVersions` covers it.
- Added `jobstatus.RecordedResults`, which returns the summary's results, or before the summary the carried ones. `show` (run, job, and JSON views), `joblist`, `runview`, and the Web loader use it.
- `runlineage.IsCarried` takes whether the job has a recorded result.
- `jobcontrol` skips carried jobs in selections, whole-run cancels, and array expansion, and refuses a cancel that names one.
- Contracts: added DUR-7, updated the DUR-5 wording, and added the state layout entry.
- The ISSUES.md entry moved to Resolved.

**Reason:** the M7 trial's open finding. The user chose option A: record carried results at run start, not create job directories for carried jobs or write an early summary.

**Plan impact:** no open ISSUES.md item remains from this plan. The remaining work is output size.

**Validation:**
- `TestCarriedJobsReadAsCarriedDuringTheRun` (DUR-7) fails on `40a95e0` for `show`, `lineage`, the Web API, the MCP run summary, and the cancel preview. Its `jobs` check already passed and stays as a regression check.
- New `TestRecordedResultsPrefersTheSummaryToCarriedResults` and `TestIsCarriedNeedsARecordedResult`.
- The first full check failed in five `internal/web` loader tests: the loader had decided by `summary.json` on disk instead of by the summary it is given. It now uses the given summary's results when it has any.
- After that fix, `scripts/check.sh` and `go test -count=1 ./conformance/... ./internal/archtest ./internal/doclinks` passed.
- While staging, a hand-built `development/ISSUES.md` had dropped another thread's committed open item. It was rebuilt from HEAD before the commit.
- A fresh `control` trial pass on `12b5395` gave the counts above.

**Remaining:** output sizes of failure groups and comparisons.

## Limits on MCP summaries and comparisons

**Commits:** 2026-10-03 12:40:33 `82b1769`.

**Change:**
- Added `runlineage.LimitMembers` and `FailureGroup.JobsOmitted`.
- `rotari_run_summary` and `rotari_wait_run` list the first 10 jobs of each failure group, and take `all_jobs` to list all.
- `rotari_compare_runs` lists at most 20 changed jobs, keeping jobs whose result changed first, and reports the jobs left out, whose only change is their definition, as `hidden_changed`.
- Updated `docs/MCP.md`.

**Reason:** the remaining output costs named in the plan. A failure group named every member, and a comparison listed every changed definition.

**Plan impact:**
- labC's run summary in the read scenario fell from 7.3 KB to 3.7 KB.
- The write scenario's comparison stayed at 3.7 KB, because its 15 changed jobs are under the limit.
- Collapsing a definition change repeated over an array's tasks stays open.

**Validation:**
- New tests:
  - `TestLimitMembersKeepsTheFirstJobsAndCountsTheRest`
  - `TestLimitComparedJobsKeepsResultChangesFirstInOrder`, whose result changes sit beyond the limit, so priority is what keeps them
  - `TestRunSummaryListsTenJobsPerGroupUnlessAllAreAsked`
- The tool fixture now numbers results past 9 with `strconv.Itoa`; it had built IDs from runes.
- `scripts/check.sh` and `go test -count=1 ./conformance/... ./internal/archtest ./internal/doclinks` passed.
- Fresh read and write trial passes on `82b1769` gave the sizes above.

**Remaining:** none planned; see the plan's open decisions.

## Trials with a real agent and the CLI

**Commits:** 2026-10-03 13:01:38 `f47cc93`; 2026-10-03 13:21:01 `b99bf19`; 2026-10-03 13:32:06 `07c22fa`; 2026-10-03 13:44:31 `7e8169c`; 2026-10-03 13:59:05 `a459e0d`; 2026-10-03 14:12:51 `9b66c8e`.

**Change:**
- `f47cc93`: the trial note [agent-trial-2026-10-03-cli-agent.md](agent-trial-2026-10-03-cli-agent.md). A Claude subagent with no session context used only `rotari guide` and `--help`, and a wrapper logged its calls.
- `b99bf19`:
  - `rotari guide` keeps its rules and the common options, and indexes the commands in one line each (46.6 KB to 5.4 KB).
  - `writeCommandHelp` renders every `COMMAND --help` from `cliCommandSpecs`: description, usage, subcommands, and options with their effective defaults, `--filter-*` under their own heading, on stdout with exit 0.
  - The `change` and `remove` descriptions say what `--run-id` does.
- `07c22fa`: ISSUES.md records that `retry`'s spec omits `run` options it takes. A first attempt to reject them was reverted: the contract and `TestSelectorTable` make `retry --failed` and the others valid. The spec fix overlaps with the CLI option interaction work.
- `7e8169c`: `resolve.RegisteredProjectBaseDirs`. A missing project's error names the registered state directories that have it, and `jobs` points to `--all-basedirs`. Contract CLI-11.
- `a459e0d`: `lineage` comparisons join the tasks of an array that read the same into one row, and mark carried results in the `RESULT` column.
- `9b66c8e`: `state.ResolveBaseDir` and `ResolveMasterDir` return absolute paths, so `--basedir ../lab` works. Contract RES-22.

**Reason:** the user cannot connect MCP servers, so real agents use the CLI. The first trial's findings drove these changes, and a second trial measured them.

**Plan impact:**
- With a real agent, the task fell from 27 calls and about 140 KB to 23 calls and about 45 KB.
- Finding the project fell from about 6 calls to 3.
- Smaller findings remain in the trial note.

**Validation:**
- Each fix's new test fails on the commit before it:
  - `TestCommandHelpCoversEveryOptionAndExitsZero` is new behavior.
  - `TestMissingProjectNamesWhereItIs` fails for `show`, `lineage`, and `jobs`.
  - `TestRunDiffGroupsArrayTasksThatReadTheSame` fails.
  - `TestRelativeStateDirectoriesResolveAgainstTheWorkingDirectory` failed before the fix on `add -b ../lab`.
- `scripts/check.sh` and the conformance suite passed before each commit, except as noted below.
- Before `9b66c8e`, a script truncated the working `contracts/README.md` (it opened the file for writing before reading it). It was restored from HEAD plus this change's row. The other thread had committed its README edits at 14:01:57 (`09d19f3`), before the truncation, and has since added its own SAFE-4 entry to the working file.
- Before `9b66c8e`, `TestContractStatus` and two flag-pair unlock tests failed only because of the other thread's uncommitted `flag_pair_unlock_test.go`, which was in progress. The suite otherwise passed.
- The commits from `7e8169c` on were made through a separate index, because the other thread had staged its own files.

**Remaining:**
- `retry`'s spec (ISSUES.md).
- The trial's open points: dry-run reasons, `lineage` hidden-job names, the queue table's timeout and source columns, and `wait`'s exit code wording.

## CLI agent trials three and four, and their fixes

**Commits:** 2026-10-03 15:48:30 `d5e4359`; 2026-10-03 16:00:46 `62e644a`; 2026-10-03 16:05:50 `58af6b6`; 2026-10-03 16:41:22 `c1347d1`; 2026-10-03 16:55:00 `2df0c80`; 2026-10-03 16:55:00 `04e58e6`; 2026-10-03 17:11:28 `bf4d645`; 2026-10-03 17:32:48 `61f8ce3`; 2026-10-03 17:43:10 `903bb6b`; 2026-10-03 17:43:10 `80fac3c`; 2026-10-03 17:43:11 `e483723`; 2026-10-03 18:27:40 `28a544d`; 2026-10-03 18:34:18 `6eb4ec4`; 2026-10-03 18:34:19 `2048258`; 2026-10-03 18:40:01 `0a33bcf`.

**Change:**
- `d5e4359`:
  - The `--async` start message ends its last line, suggests `rotari wait ... --run-id RUN_ID`, and its cancel hint names the run.
  - Contract CAN-7, checked by running the hints as printed.
- `62e644a`: `run.Plan.RerunDependencies` records why a job the selection left out executes. Previews print `depends_on_rerun=NAME`, and `rotari_preview_run` returns it.
- `58af6b6`, `61f8ce3`: the third and fourth trials in [agent-trial-2026-10-03-cli-agent.md](agent-trial-2026-10-03-cli-agent.md).
- `c1347d1`:
  - The `--async --dry-run` refusal, which the user chose to keep, says how to preview and then start.
  - The guide shows that sequence.
  - CLI-8.
- `2df0c80`:
  - The guide says that run and attempt IDs locate their state directory.
  - The `--job-id` help says that dependents execute.
- `04e58e6`: `change` output names the changed fields through `runlineage.SpecChanges`, such as `timeout=5s->60s`. CLI-7.
- `bf4d645`: ignore `.claude/settings.local.json`, which the user had the session create for the fourth trial's permissions.
- `903bb6b`: `change -r` and `remove -r` begin their output with the restore of the queue from the run. CLI-7.
- `80fac3c`: the missing-project error gives each candidate state directory's last run. CLI-11.
- `e483723`: the `--job-id` help names both kinds of dependency.
- `28a544d`: `state.ReadAttemptExecutor`. The report and the run table show the executor an attempt ran on, instead of "default" or the queue's default. DUR-5.
- `6eb4ec4`: `lineage` names the jobs it hides.
- `2048258`: run summary results carry the job's `name`, an optional field. DUR-5.
- `0a33bcf`: `model.FormatDependencies` joins an array's tasks into ranges, such as `finished:train[1-12]`.

**Reason:** the third and fourth CLI agent trials.

**Plan impact:**
- The third run took 21 calls.
- The fourth run measured only diagnosis and preview. Claude Code's auto-mode classifier denied its first write, and allow rules did not change that.
- `retry`'s spec stays in ISSUES.md, because it overlaps with the CLI option interaction work.
- Rerunning one array task without its dependents was noted, not built.

**Validation:**
- Each behavior change has a test that fails before it: conformance where the binary shows it, unit tests for `run`, `model`, `state`, and `lineage` output.
- Before each commit, the conformance suite, `internal/archtest`, `internal/doclinks`, and `scripts/check.sh` passed. The only exceptions were failures in the CLI option interaction work's in-progress tests, which passed when rerun:
  - `TestCLIFlagPairFiles` and `TestCLIFlagPairMCP`; `TestCLIFlagPairMCP` passed four times in a row.
  - `TestContractStatus` rows for that work's CLI-12.
- Commits were made through a separate index, because that work had files staged and edited in the same tree.

**Remaining:**
- `retry`'s spec (ISSUES.md).
- A way to rerun one array task without its dependents.
- A fifth trial, once writes can be allowed for the agent.

## The fifth CLI agent trial and its fixes

**Commits:** 2026-10-03 21:58:59 `989b452`; 2026-10-03 22:10:19 `5f58781`; 2026-10-03 22:19:35 `b4d61ec`; 2026-10-03 23:22:51 `b59af78`; 2026-10-03 23:35:22 `ff5b5c3`; 2026-10-03 23:35:22 `7f2f333`; 2026-10-03 23:50:56 `e02286e`; 2026-10-03 23:51:08 `f7dac7c`; 2026-10-04 00:27:46 `06429d3`; 2026-10-04 00:27:55 `3e687a6`; 2026-10-04 00:53:17 `a78b4aa`.

**Change:**
- `989b452`: the plan's decision that dependents rerun with their dependency. The user chose not to add an option to skip them, since nobody would use an option for that edge case.
- `5f58781`:
  - `retry`'s help and schema list every option it takes. `runCommandFlags(retry)` gives the options whose meaning differs, such as `--run-id` and `--failed`, their retry wording.
  - An earlier attempt rejected those options in `retry`. It was reverted because the contract and `TestSelectorTable` say `retry` takes them.
  - The pinned flag-pair inventory and the coverage document were updated for `retry`'s new options.
- `b4d61ec`: `runview.LoadRun` reads a running run without `commands.json` as having no jobs yet, through `project.RunPhaseOf`, instead of failing. This fixes the async start race that the CLI option interaction work recorded in ISSUES.md. The MCP tool's special case for it was removed.
- `b59af78`: the fifth trial in [agent-trial-2026-10-03-cli-agent.md](agent-trial-2026-10-03-cli-agent.md). Its fixture lived in the repository's ignored `.example-state-agent6/`, where the agent's writes were allowed.
- `ff5b5c3`: the timeout, cancelled, and blocked failure groups get a `fix:` line, from `runlineage.recordedCauseSuggestions`.
- `7f2f333`:
  - The `--run-id` help of `retry` says that it copies the latest run only into an empty queue and otherwise keeps the queued jobs, such as ones changed with `change -r`.
  - The `run` and `retry` descriptions and the result filters say that dependents execute.
  - The guide's loop uses `retry --dry-run`, then `--async --if-revision`.
- `e02286e`: `lineage`'s summary says "definition changed N" instead of "changed N", which read like re-execution next to carried results.
- `f7dac7c`: each failure group in `lineage` and `show` prints a `retry:` command that previews the rerun of that cause, with `--filter-diagnosis` or `--filter-failure-kind` and `--dry-run`. It is printed only for the project's last run, once it has finished.
- `06429d3`: the guide stops recommending `--json` to agents. `--json` is the Python client's full-detail output, and the user did not want it read as an agent format. In the trial's run, `wait --json` printed 4.4 KB and the text 2.3 KB.
- `3e687a6`: ISSUES.md recorded that `wait` repeated the Summary, Diagnosis, and Origin lines of `run`'s completion message. The other thread fixed it in `43cfbf5`, which builds `wait`'s text from the same completion message as `run`.
- `a78b4aa`:
  - Command help lists the per-executor options, such as `--slurm-concurrency`, once per kind under "Executor options", naming every executor's option.
  - It states a default or the choices once.
  - The generated CLI and Python API references list each option's choices from the schema.
  - `retry --help` fell from 6.6 KB to 5.4 KB.

**Reason:** the fifth CLI agent trial, the first to complete since the auto-mode denials. It took 16 calls and about 41 KB.

**Plan impact:**
- All six findings of the fifth trial are addressed. For finding 5, the user chose not to add an output option for agents, and the guide's advice was changed instead.
- The remaining large outputs are command help and the `show -r` table.

**Validation:**
- Tests by commit:
  - `5f58781`: `guide_test.go` and the flag-pair inventory.
  - `b4d61ec`: `internal/runview`.
  - `ff5b5c3`: `internal/runlineage`.
  - `e02286e`: `lineage_diff_test.go`.
  - `f7dac7c`: the conformance failure-group test.
  - `a78b4aa`: `TestCommandHelpStatesEachNoteOnce`, which fails on the commit before it, as checked in a temporary worktree.
  - `7f2f333` changes only help and guide wording and has no test of its own.
- `b4d61ec`'s race did not reproduce in 40 async starts before the fix. Its unit test builds the state of a run that has started but has not written `commands.json`.
- Before each commit, `scripts/check.sh`, the conformance suite, and `internal/archtest` passed. For `a78b4aa`, the Python tests passed too. The exceptions were failures in the CLI option interaction work's in-progress tests (`TestCLIFlagPairFiles`, `TestCLIFlagPairMCP`, `TestContractStatus`), which passed when rerun or after that work committed.
- Commits were made through a separate index, because that work had files staged and edited in the same tree.

**Remaining:**
- The `show -r` table is wide because of `DEPENDS ON`.
- Whether to remove `.example-state-agent6/` and `.claude/settings.local.json`, both ignored by git.

## 2026-10-10: zero-information CLI agent trial

- `bebb6f74` (2026-10-10 01:44:07 +0900): [agent-trial-2026-10-10-zero-info.md](agent-trial-2026-10-10-zero-info.md) and [agent-trial-zero-info-fixture.sh](agent-trial-zero-info-fixture.sh).

**Change:**
- A trial report and a fixture script for three scenarios: a new matrix sweep, an inherited failed run, and a stuck job in a live async run.
- No code changed.

**Reason:** the user asked for an agent to use rotari with no prior information, and for proposals from what it ran into. The earlier trials told the agent to read `rotari guide` and covered only diagnosis.

**Plan impact:**
- All three headless `claude -p` agents completed their tasks in 10 to 17 calls with about 27 to 44 KB of output. They found `rotari guide` through the pointer in `rotari --help`.
- The report ranks ten proposals. The top four are a shorter top-level `--help`, elapsed and last-output time for running jobs, a completion message that summarizes failures and marks carried ones as `lineage` does, and summary lines that count a cancelled job as cancelled.
- No proposal has been accepted or implemented.

**Validation:**
- The agents' rotari calls were logged by the fixture's wrapper. Their transcripts were read in full.
- The cancelled-job display and the `cancel --wait` error source (`cmd/rotari/job_control.go`) were checked by hand.
- No tests were run, because no code changed.

**Remaining:**
- The ten proposals in the report.
- [agent-trial-fixture.sh](agent-trial-fixture.sh) does not set `XDG_CONFIG_HOME`, so its runs read the developer's notification config.

## 2026-10-10: fixes from the zero-information agent trial

- `25e6dd82` (2026-10-10 02:00:25 +0900): printed commands name `--basedir` only when it is not implicit.
- `6fec04d1` (2026-10-10 02:12:14 +0900): the completion message groups failures by cause.
- `d262d620` (2026-10-10 02:16:55 +0900): top-level help is a one-line command index.

**Change:**
- `25e6dd82`:
  - `cmd/rotari/hint_location.go` (`hintLocation`, `runHintLocation`) decides once whether a printed command needs `--basedir`, from `ROTARI_BASEDIR`, the configured basedir, and the default.
  - The hints in `lineage`, `show`, `wait`, `run`, and the running-project error use it.
  - Hints that name a run drop the location when it is implicit.
  - New contract CLI-22, with `TestHintLocationOmitsImplicitBaseDir` and the conformance test `TestPrintedHintsNameOnlyANonImplicitBaseDir`.
- `6fec04d1`:
  - The completion message moved from `internal/supervisor/messages.go` to `cmd/rotari/run_completion.go`. The supervisor's copy was never read: the client hangs up after acceptance and prints the message through the follower that `wait` uses.
  - The message prints the summary, a `Carried:` line, the origins, the failures by cause from `writeFailureGroups` with `failureRetryHints`, and `Inspect run:`.
  - It drops the `Success:`/`Failed:` lines, the `Diagnosis:` lines, and the per-job blocks.
  - CLI-14 was updated, with the conformance test `TestCompletionMessageGroupsFailuresByCause`.
- `d262d620`: `printUsage` lists each command's name and description. `TestTopLevelUsageIndexesCommandsWithoutOptions` was added, and the help golden file was regenerated.

**Reason:** findings 1, 3, 4, and 5 of [agent-trial-2026-10-10-zero-info.md](agent-trial-2026-10-10-zero-info.md), which the user chose to implement.

**Plan impact:**
- In the second run, `--help` plus `guide` fell from 18 KB to 10 KB, and s1 and s3 used 20 to 25 percent less output.
- s2 used the saving to read command help before changing state.
- Findings 2 and 6 to 10 remain open.

**Validation:**
- `TestCompletionMessageGroupsFailuresByCause` failed on `25e6dd82`, the commit before the fix, for every condition it checks. This was run in a temporary worktree.
- Before each commit, `go test ./cmd/rotari` and `scripts/check.sh --short` passed, and so did `go test ./conformance/...` for `25e6dd82` and `6fec04d1`. For `d262d620`, the conformance golden test was run before the commit, and after it the full `scripts/check.sh`, with the race detector, and `go test ./conformance/...` passed.
- `6fec04d1` was tested in a separate worktree while another thread's uncommitted test in `cmd/rotari/wait_test.go` did not compile.
- Three new headless agents completed the trial again, as recorded in the trial report.

**Remaining:**
- Findings 2 and 6 to 10 of the trial report.
- `internal/project/inspect.go` and `internal/supervisor/run.go` still print `--basedir` in errors, because they cannot see the CLI configuration.
- `lineage`'s `Diagnoses:` section still counts a cancelled job as `no_match`.

## 2026-10-10: remaining fixes from the zero-information agent trial

- `b3d7df65` (2026-10-10 03:26:54 +0900): `cancel --wait` help and error.
- `bc94db72` (2026-10-10 03:34:22 +0900): the array-task selection error names the array job and its task count.
- `33efb8dd` (2026-10-10 03:45:20 +0900): elapsed and quiet time in `show`, and a run table sized to its contents.
- `60b215fd` (2026-10-10 04:03:57 +0900): `show --tail`, and `show --logs` in definition order.
- `c4c8d011` (2026-10-10 04:09:35 +0900): the failed-job progress hint uses the attempt ID.
- `07a5254e` (2026-10-10 04:09:40 +0900): the first progress count is out of the executed jobs.
- `62c49b94` (2026-10-10 04:12:20 +0900): the guide points to `show --logs --tail`.

**Change:**
- `cmd/rotari`:
  - `show.go` gained `showJobElapsed`, `printShowJobTable`, `printExecutedJobLogs`, `lastLines`, and the `--tail` option.
  - `job_control.go` and `cli_spec.go` reword `cancel --wait`.
  - `assets/agent_guide.md` mentions `--tail`.
- `internal/jobstatus.LastOutputAt` reads the latest write to a job's logs.
- `internal/model.arrayTaskError` reworded the array-task error.
- `internal/supervisor/run.go`:
  - the failed-job hint;
  - the run-started event's `Total`.
- New contracts CLI-23 and CLI-24, a sentence in CLI-19, and conformance tests:
  - `TestShowReportsElapsedAndQuietTime`;
  - `TestShowLogsTailInDefinitionOrder`;
  - `TestRetryProgressCountsOnlyExecutedJobs`.
- `show` in the flag-pair inventory grew to 39 options, with a `tail` sample.
- The generated CLI reference, environment reference, Python CLI, and schema golden file were regenerated.

**Reason:** findings 2 and 6 to 10 of [agent-trial-2026-10-10-zero-info.md](agent-trial-2026-10-10-zero-info.md), after the user asked to continue.

**Plan impact:**
- Every finding of the trial is addressed or recorded in ISSUES.md.
- For finding 8, the SEL-12 rejection was kept; supporting `--wait` for job cancels would be a contract change.
- The third run used 13, 35, and 24 KB, against 27, 41, and 44 KB in the first.

**Validation:**
- Each commit's changes were applied to a clean worktree at the previous commit, because another thread was editing the shared tree. `go vet ./...` and `go test ./...`, including `conformance/...`, passed there.
- The generators' `--check` modes passed for `60b215fd`.
- These tests failed on the commit before their fix:
  - `TestRetryProgressCountsOnlyExecutedJobs` printed `0/3` before `07a5254e`.
  - `TestShowLogsTailInDefinitionOrder` fails before `60b215fd`, at `--tail`.
- The `configs` listing was reproduced separately with a build of `bc94db72`.
- `scripts/check.sh` with the race detector was not rerun after these commits.

**Remaining:** the ISSUES.md entry "Leftovers from the 2026-10-10 zero-information agent trial".

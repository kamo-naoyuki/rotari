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

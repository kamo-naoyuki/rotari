# Rotari design contracts

This file is the entry point for the behavior rotari must keep: the state
layout, resolution and fallback rules, run semantics, locking, and path rules.
The details live in the split pages in this directory, and
cross-cutting rules remain here as the high-level index.

These are contracts, not a tour of the code. For which process and package
does what, and how a command flows through the code, read
[ARCHITECTURE.md](../docs/ARCHITECTURE.md) first. User-facing behavior belongs in
[../README.md](../README.md) and the user guides it links to under `docs/`.

When a behavior contract changes, update the relevant contract note in the
same change. Include links to the representative implementation and tests so
future contributors and coding agents can move from the contract to the code
quickly.
[`internal/doclinks`](../internal/doclinks/links_test.go) fails when one of
these links, or any relative link in the root Markdown files, `contracts/`,
and `docs/`, names a missing file or heading.

Conformance tests mirror these documents under
[`conformance/`](../conformance/). The mapping is recorded in
[`conformance/layout.json`](../conformance/layout.json) and checked by
`TestConformanceLayout`; `TestContractStatus` also checks IDs after Markdown
files are split into subdirectories.

## Split notes

- [00-overview.md](00-overview.md): overview, system model,
  and core contracts.
- [01-resolution-and-config.md](01-resolution-and-config.md):
  path resolution, config precedence, shell completion, and registry behavior.
- [02-run-lifecycle-and-execution.md](02-run-lifecycle-and-execution.md):
  run creation, retries, filtered reruns, executor orchestration, and
  external integrations.
- [03-server-and-command-interfaces.md](03-server-and-command-interfaces.md):
  server projections, command interfaces, and client lifecycle.
- [04-coordination-and-safety.md](04-coordination-and-safety.md):
  locks, durability, recovery, and shared-state safety rules.
- [05-web-assets-and-static-export.md](05-web-assets-and-static-export.md):
  Web UI assets, static export, and embedded app boundaries.
- [06-selectors.md](06-selectors.md): how each command
  resolves project, run, and job selectors, known deviations, and the
  selector test fixture.

## Cross-cutting rules to keep in sync

[`conformance/`](../conformance/) tests these rules from outside the code: it
builds `cmd/rotari`, runs it with an isolated environment, and uses the Web
API, importing only the standard library. Its tests pass unchanged across
package-level refactoring, so a failure there means user-visible behavior
changed.

- Keep the filesystem as the source of truth; registry and in-memory state are
  only indexes or coordination helpers. The main persistence boundary is
  [`internal/state/store.go`](../internal/state/store.go), with coverage in
  [`internal/state/store_test.go`](../internal/state/store_test.go).
- Preserve the same fallback and resolution contracts across CLI, server, and
  web paths. Job result and timestamp resolution lives in
  [`internal/jobstatus`](../internal/jobstatus/); the renderers start with [`cmd/rotari/show.go`](../cmd/rotari/show.go),
  [`internal/report/report.go`](../internal/report/report.go), and
  [`internal/webui/webui.go`](../internal/webui/webui.go); representative tests are in
  [`cmd/rotari/show_test.go`](../cmd/rotari/show_test.go),
  [`internal/report/report_test.go`](../internal/report/report_test.go), and
  [`internal/webui/webui_test.go`](../internal/webui/webui_test.go);
  [`conformance/04-coordination/status_views_test.go`](../conformance/04-coordination/status_views_test.go) checks that
  `show --json`, `jobs`, and the Web API agree on a finished run.
- Treat path elements as arbitrary strings and reject unsafe separators before
  filesystem access. The shared boundary is
  [`internal/state/paths.go`](../internal/state/paths.go), with path safety
  checks in [`cmd/rotari/main_test.go`](../cmd/rotari/main_test.go) and, through
  the built binary and the Web API, in
  [`conformance/06-selectors/paths_test.go`](../conformance/06-selectors/paths_test.go).
- Update user-facing docs and relevant tests whenever a behavior contract
  changes. The user-facing entry points are [`README.md`](../README.md), the
  user guides it links to under `docs/`, and [`docs/FAQ.md`](../docs/FAQ.md); keep the relevant package tests alongside the
  implementation change.

## Contract status

A rule in the pages of this directory gets an ID by starting with
`**PREFIX-N**`. The prefix names the section that defines it:

| Prefix | Section |
| --- | --- |
| `CORE` | "Core design contracts" in [00-overview.md](00-overview.md#core-design-contracts) |
| `RES` | "Resolution rules" in [01-resolution-and-config.md](01-resolution-and-config.md#resolution-rules) |
| `DUR` | "Job execution durability" in [04-coordination-and-safety.md](04-coordination-and-safety.md#job-execution-durability) |
| `CAN` | "Cancellation" in [02-run-lifecycle-and-execution.md](02-run-lifecycle-and-execution.md#cancellation) |
| `LOG` | "Job logs" in [02-run-lifecycle-and-execution.md](02-run-lifecycle-and-execution.md#job-logs) |
| `RUN` | "Run lifecycle" in [02-run-lifecycle-and-execution.md](02-run-lifecycle-and-execution.md#run-lifecycle) |
| `CLI` | "CLI presentation" in [03-server-and-command-interfaces.md](03-server-and-command-interfaces.md#cli-presentation) |
| `COORD` | "Shared-state coordination" in [04-coordination-and-safety.md](04-coordination-and-safety.md#shared-state-coordination) |
| `STATE` | "State load and write contracts" in [04-coordination-and-safety.md](04-coordination-and-safety.md#state-load-and-write-contracts) |
| `SAFE` | "Concurrency and safety" in [04-coordination-and-safety.md](04-coordination-and-safety.md#concurrency-and-safety) |
| `SEL` | [06-selectors.md](06-selectors.md): complete IDs, what each command reads, the job selector and job control tables, selector combinations, and positional arguments |
| `WEB` | [05-web-assets-and-static-export.md](05-web-assets-and-static-export.md): Web API and UI behavior |

Other sections have no IDs yet; give a rule one, with the next free number,
when a conformance test starts checking it. Never renumber or reuse an ID;
when a rule is removed, remove its row.

Each ID has one row below. A status is `conformance` when the listed tests in
[conformance/](../conformance/) check the whole rule, `partial` when they check
part of it, `pending` when no conformance test checks it yet (package tests
may), and `deviation` when rotari knowingly breaks it; a test of a deviation skips
the failing cases with `knownDeviation(t, "ID")` until the fix removes it. A test declares what it checks
with `covers(t, "ID")`, and `TestContractStatus` in
[conformance/contracts_test.go](../conformance/contracts_test.go) fails when
the IDs, this table, and those calls disagree.

| ID | Rule | Status | Conformance tests |
| --- | --- | --- | --- |
| CORE-1 | The filesystem is the source of truth; registries and memory are recoverable indexes | partial | `TestRunFilesRemainAuthoritativeWithoutRegistryEntry` |
| CORE-2 | The supervisor coordinates but is not the authority for project or run state | partial | `TestPersistedRunStateIsReadableAfterServerShutdown` |
| CORE-3 | One mutable queue per project, edited only while idle and snapshotted by a run | partial | `TestFilteredRerunCarriesCompletedResults`, `TestRunningProjectRejectsChanges` |
| CORE-4 | Idle projects are queue-first; running and interrupted projects are run-first | partial | `TestProjectStates` |
| CORE-5 | At most one active run and runner per project | conformance | `TestRunningProjectRejectsChanges`, `TestUnlockRefusesLiveRun` |
| CORE-6 | Completed runs are immutable; reruns change only their destination run | partial | `TestFilteredRerunCarriesCompletedResults` |
| CORE-7 | Executors run jobs; run semantics stay in the shared execution path | partial | `TestRunRetrySucceedsWithinOneRun` |
| RES-1 | Base directory resolution order | conformance | `TestBaseDirResolutionOrder` |
| RES-2 | Project resolution order and the single-project default | partial | `TestProjectResolutionOrder` |
| RES-3 | Missing projects fail except `check` reports empty, `unlock`/`wait` without run ID no-op, and `add`, `import`, `reset` create one | partial | `TestMissingProjectIsAnError`, `TestCheckMissingProjectIsEmptyWithoutCreatingIt`, `TestResetMissingProjectCreatesEmptyQueue`, `TestUnlockMissingProjectIsNoOp`, `TestWaitMissingProjectIsNoOp` |
| RES-4 | `check` and `reset` take an optional positional project | conformance | `TestPositionalProject` |
| RES-5 | `jobs` takes an optional positional project that overrides defaults | partial | `TestPositionalProject` |
| RES-6 | `export TARGET [FILE]` names a project or saved run | conformance | `TestExportResolvesProjectAndRunTargets` |
| RES-7 | `unlock` derives the run from the lock or interrupted metadata | conformance | `TestUnlockDerivesInterruptedRun` |
| RES-8 | `show --basedirs` lists registered state directories | conformance | `TestShowBasedirsListsKnownStateDirectories` |
| RES-9 | Project names and job IDs are single path elements | conformance | `TestCLIRejectsUnsafePathElements`, `TestWebAPIRejectsUnsafePathElements` |
| RES-10 | Unsafe path elements are rejected before filesystem access, locally and remotely | partial | `TestCLIRejectsUnsafePathElements`, `TestWebAPIRejectsUnsafePathElements` |
| RES-11 | Stored times are UTC RFC3339; displayed times follow `TZ` | partial | `TestDisplayTimesFollowTZ` |
| RES-12 | `--run-id` is exact except the reserved `latest` | partial | `TestLatestRunID` |
| RES-13 | A run ID or attempt ID alone resolves base directory, project, and run | partial | `TestRunIDAloneResolvesLocation` |
| RES-14 | Explicit location options win; conflicts with the registry fail | partial | `TestExplicitLocationMustMatchRegistry` |
| RES-15 | History consumers fall back to `last_run_id`, then the newest run | conformance | `TestHistoryUsesLastRunThenNewestRun` |
| RES-16 | `wait` selector resolution, missing-project no-op, and single-active-project scan | partial | `TestWaitResolvesActiveAndFinishedSelectors`, `TestWaitReturnsCompletedRunExitCode`, `TestWaitMissingProjectIsNoOp` |
| RES-17 | Run lookup applies to history commands only | partial | `TestStateCreatingCommandsDoNotResolveRunIDs` |
| RES-18 | `cancel`, `suspend`, and `resume` merge selectors and require the active run | conformance | `TestJobControlSelectors` |
| RES-19 | `wait` resolves multiple run IDs independently | conformance | `TestWaitResolvesRunIDsIndependently` |
| RES-20 | Shell completion follows the location rules | partial | `TestCompletionScriptsExposeDynamicCompletion` |
| RES-21 | Missing state directories give no completion candidates | conformance | `TestCompletionMissingStateDirectoryHasNoCandidates` |
| DUR-1 | Every executor runs jobs through the self-reporting wrapper | partial | `TestJobOutlivesKilledSupervisor` |
| DUR-2 | The wrapper records status independently of its launcher | partial | `TestJobOutlivesKilledSupervisor` |
| DUR-3 | An orphaned local job still records its own status | conformance | `TestJobOutlivesKilledSupervisor` |
| DUR-4 | Supervisors are not restarted; crash detection is file-backed | conformance | `TestJobOutlivesKilledSupervisor` |
| DUR-5 | `show`, `jobs`, reports, and the Web UI share one status fallback chain | partial | `TestCLIAndWebAgreeOnJobResults`, `TestCLIShowSelectedOlderAttemptIgnoresLatestSummary`, `TestStatusFallbackChainAgreesAcrossViews` |
| DUR-6 | Recovery does not kill or reconcile leftover jobs | partial | `TestRecoveryLeavesJobsRunning` |
| SAFE-1 | `check` and `show` report a project as idle, running, or interrupted; a killed coordinator leaves it interrupted | conformance | `TestControlFromAnotherHost`, `TestProjectStates` |
| SAFE-2 | A running project rejects the commands that would change it, so no second runner starts | conformance | `TestRunningProjectRejectsChanges` |
| SAFE-3 | An interrupted project rejects them, naming the run and how to inspect and recover it | conformance | `TestInterruptedProjectNeedsRecovery` |
| SAFE-4 | `unlock` recovers an interrupted run, no-ops without one, and refuses one whose coordinator is alive | conformance | `TestControlFromAnotherHost`, `TestInterruptedProjectNeedsRecovery`, `TestUnlockRefusesLiveRun`, `TestUnlockWithoutInterruptedRunIsNoOp` |
| SAFE-5 | `reset` discards the queue, rejects a running project, and needs confirmation for an interrupted one | conformance | `TestResetOfInterruptedProject`, `TestRunningProjectRejectsChanges` |
| SAFE-6 | Commands ask for confirmation only on a terminal, otherwise naming the option that confirms | conformance | `TestCopyIntoQueueWithoutTerminal`, `TestResetOfInterruptedProject` |
| COORD-1 | Controlling a local job from another host fails, naming that host | conformance | `TestControlFromAnotherHost` |
| COORD-2 | Cancelling a run whose coordinator is on another host fails, naming that host | conformance | `TestControlFromAnotherHost` |
| COORD-3 | A lock from another host keeps the project locked until `unlock` | conformance | `TestControlFromAnotherHost` |
| COORD-4 | A missing scheduler command fails the job with an error naming it | partial | `TestMissingSchedulerCommand` |
| COORD-5 | `ROTARI_PRIVATE_STATE` makes new paths owner-only; the static export stays publishable | partial | `TestPrivateStateModes` |
| STATE-1 | Reading state from a newer rotari fails asking to upgrade, and leaves the file; the Web UI marks such a run unreadable | conformance | `TestNewerStateVersionIsRejected`, `TestWebShowsNewerRunAsUnreadable` |
| STATE-2 | State without `state_version` reads as version 1 | conformance | `TestUnversionedStateIsVersionOne` |
| STATE-3 | Reading history never rewrites it | conformance | `TestReadingHistoryDoesNotRewriteIt` |
| STATE-4 | Malformed load samples are skipped | conformance | `TestMalformedLoadSamplesAreSkipped` |
| CAN-1 | A whole-run cancel stops every running job of the run, from any caller | conformance | `TestWholeRunCancelFinishesRun` |
| CAN-2 | A cancelled run finishes with a summary and leaves the project idle, not interrupted | conformance | `TestWholeRunCancelFinishesRun` |
| CAN-3 | `cancel --wait` returns once the run has finished and exits 0 | conformance | `TestCancelWaitReturnsAfterRunFinishes` |
| CAN-4 | Cancelling one job stops only that job, which the run does not retry | conformance | `TestCancelJobStopsOnlyThatJob` |
| LOG-1 | Attempts use the selected internal merge or separate log mode, independently of external destinations | conformance | `TestJobStreamsPersistSeparately` |
| LOG-2 | Repeatable `--output`/`--error` sinks; absent `--error`, stderr follows `--output`; independent of internal log mode | conformance | `TestExternalLogDestinations` |
| LOG-3 | Destination parents are created on the execution host; setup errors fail before the command starts | conformance | `TestExternalLogDestinations` |
| LOG-4 | External sinks append by default; truncate initializes each destination once; shared streams have no guaranteed interleaving | conformance | `TestExternalLogDestinations` |
| RUN-1 | A filtered rerun executes the selected jobs, carries completed results outside the selection into the new run, and leaves the source run unchanged | conformance | `TestFilteredRerunCarriesCompletedResults` |
| RUN-2 | A run-level retry limit retries a failed job within the same run until it succeeds or the limit is exhausted; a successful retry makes the run successful | conformance | `TestRunRetrySucceedsWithinOneRun` |
| RUN-3 | Executor working-directory defaults are portable; run `--env=ALL|NONE` controls caller environment propagation and job overrides consistently | partial | `TestRunUsesCallersDirectoryAndEnvironment` |
| RUN-4 | New-run fingerprint matching prioritizes Job ID/Origin, matches remaining expanded jobs by fingerprint and occurrence, and treats count mismatches as new work | conformance | `TestFingerprintMatchingIgnoresNonInputMetadata`, `TestFingerprintMatchingNormalizesDirectoryAndIgnoresArrayRange`, `TestFingerprintMatchingPreservesArrayTasksAndMatrixLeaves`, `TestFingerprintMatchingPrioritizesIDsAndQueueOccurrence`, `TestFingerprintMatchingRejectsChangedCommand`, `TestFingerprintMatchingRejectsChangedExplicitInputs`, `TestFingerprintMatchingRejectsChangedMatrixValue`, `TestFingerprintMatchingTreatsMissingHistoryAsNewWork`, `TestFingerprintMatchingUsesIDsAndRejectsCountMismatches` |
| RUN-5 | Imported manifest jobs without source provenance execute as new work without a previous-run lookup | conformance | `TestImportedWorkflowRunsFreshJobs` |
| RUN-6 | `wait --until-failure` returns at a job's final failure, not at a failure the run retries | conformance | `TestWaitUntilFailureIgnoresAFailureTheRunRetries`, `TestWaitUntilFailureReturnsAtAFinalFailure` |
| CLI-1 | `check --json` reports the same project state, run identifier, queue count, lock, and runnable result as the human-readable `check` output | partial | `TestCheckJSONMatchesText` |
| CLI-2 | Human-readable `jobs` columns keep their visible start positions aligned across rows; ANSI color sequences do not count toward column width | conformance | `TestJobsTableKeepsVisibleColumnsAligned` |
| CLI-3 | All command options shared by CLI, environment, and config use the same source precedence | conformance | `TestCLIOptionPrecedence` |
| CLI-4 | `show`, `lineage RUN`, and the Web API group a run's failed and blocked jobs by the same causes | conformance | `TestFailureGroupsAgreeAcrossViews` |
| CLI-5 | The project list shows each project's last result, and its suggested commands work for every listed project | conformance | `TestProjectListHintsWork` |
| SEL-1 | A run ID or attempt ID alone resolves its location in every command | partial | `TestPositionalArguments`, `TestSelectorTable` |
| SEL-2 | Each command reads the run or queue its row names | partial | `TestSelectorTable` |
| SEL-3 | `show` resolves each selector form as its column says | conformance | `TestSelectorTable` |
| SEL-4 | `copy` resolves each selector form as its column says | conformance | `TestSelectorTable` |
| SEL-5 | `run` and `retry` resolve each selector form as their column says | conformance | `TestRunJobIDRunsEditedQueue`, `TestSelectorTable` |
| SEL-6 | `change` resolves each selector form as its column says | conformance | `TestSelectorTable` |
| SEL-7 | `remove` resolves each selector form as its column says | conformance | `TestSelectorTable` |
| SEL-8 | Selectors combine by kind, with the listed exclusions | conformance | `TestSelectorTable` |
| SEL-9 | `cancel`, `suspend`, and `resume` resolve each form as the job control table says | conformance | `TestJobControlSelectors` |
| SEL-10 | Each command takes its positional arguments with their meaning and exclusions | conformance | `TestPositionalArguments` |
| SEL-11 | The `--filter-*` options select jobs as the Filters section says | conformance | `TestSelectorTable` |
| SEL-12 | `--job-name` and filters choose unfinished jobs for `cancel`, `suspend`, and `resume`, confirmed before acting | partial | `TestJobControlSelectors` |
| WEB-1 | Run timelines start at the actual run start, show carried results at start, and include timestamped events for executed jobs with origins, without origin-time rewinds | conformance | `TestFilteredRerunCarriesCompletedResults`, `TestBlockedOriginJobsDoNotRewindWebTimeline`, `TestRunTimelineStartsAtActualRunStart` |
| WEB-2 | Live history search covers user-selected basedir/project/run scopes with validated, read-only hierarchical filters and pagination | partial | `TestHistorySearchAcrossProjects` |

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
| `MCP` | "MCP tools" in [03-server-and-command-interfaces.md](03-server-and-command-interfaces.md#mcp-tools) |
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
| CORE-3 | One mutable queue per project, taken by a run when it starts and editable in every project state | partial | `TestFilteredRerunCarriesCompletedResults`, `TestQueueEditsBesideAnActiveRun`, `TestRunningProjectRejectsSecondRunAndDelete` |
| CORE-4 | A project view is queue-first when idle and run-first when running or interrupted; a job selector looks in the queue first in every state | partial | `TestProjectStates` |
| CORE-5 | At most one active run and runner per project | conformance | `TestRunningProjectRejectsSecondRunAndDelete`, `TestUnlockRefusesLiveRun` |
| CORE-6 | Completed runs are immutable; reruns change only their destination run | partial | `TestFilteredRerunCarriesCompletedResults` |
| CORE-7 | Executors run jobs; run semantics stay in the shared execution path | partial | `TestRunRetrySucceedsWithinOneRun` |
| RES-1 | Base directory resolution order | conformance | `TestBaseDirResolutionOrder` |
| RES-2 | Project resolution order and the single-project default for project-scoped commands | partial | `TestProjectResolutionOrder` |
| RES-3 | Missing projects fail except `check` reports empty, `unlock` without run ID no-ops, and `add`, `import`, `reset` create one | partial | `TestMissingProjectIsAnError`, `TestCheckMissingProjectIsEmptyWithoutCreatingIt`, `TestResetMissingProjectCreatesEmptyQueue`, `TestUnlockMissingProjectIsNoOp`, `TestWaitMissingProjectIsAnError` |
| RES-4 | `check` and `reset` take an optional positional project | conformance | `TestPositionalProject` |
| RES-5 | `jobs` takes an optional positional project that overrides defaults | partial | `TestPositionalProject` |
| RES-6 | `export TARGET [FILE]` names a project or saved run | conformance | `TestExportResolvesProjectAndRunTargets` |
| RES-7 | `unlock` derives the run from the lock or interrupted metadata | conformance | `TestUnlockDerivesInterruptedRun` |
| RES-8 | `basedirs` lists registered state directories | conformance | `TestBasedirsListsKnownStateDirectories`, `TestCommandsThatCreateAProjectRegisterItsBasedir` |
| RES-9 | Project names and job IDs are single path elements | conformance | `TestCLIRejectsUnsafePathElements`, `TestWebAPIRejectsUnsafePathElements` |
| RES-10 | Unsafe path elements are rejected before filesystem access, locally and remotely | partial | `TestCLIRejectsUnsafePathElements`, `TestWebAPIRejectsUnsafePathElements` |
| RES-11 | Stored times are UTC RFC3339; displayed times follow `TZ` | partial | `TestDisplayTimesFollowTZ` |
| RES-12 | `--run-id` is exact except the reserved `latest` | conformance | `TestLatestRunID`, `TestLatestRunIDSkipsActiveRun` |
| RES-13 | A run ID or attempt ID alone resolves base directory, project, and run | partial | `TestRunIDAloneResolvesLocation` |
| RES-14 | Explicit location options win; conflicts with the registry fail | partial | `TestExplicitLocationMustMatchRegistry` |
| RES-15 | History consumers fall back to `last_run_id`, then the newest run | conformance | `TestHistoryUsesLastRunThenNewestRun` |
| RES-16 | `wait` selector resolution, missing-target errors, attached-run warnings, and implicit waiting for the runs this process started (or every active run with `--all`) regardless of `ROTARI_PROJECT_NAME` | partial | `TestWaitMissingProjectIsAnError`, `TestWaitResolvesActiveAndFinishedSelectors`, `TestWaitReturnsCompletedRunExitCode`, `TestWaitWithoutActiveRunsIsNoOp`, `TestWaitWithoutSelectorWaitsAllProjectsDespiteProjectEnv`, `TestWaitWithoutSelectorWarnsAboutInterruptedRuns`, `TestWaitWarnsForAttachedRunTargetsAndFollowsThemImplicitly`, `TestWaitWithoutSelectorWaitsForRunsThisProcessStarted` |
| RES-17 | Run lookup applies to history commands only | partial | `TestStateCreatingCommandsDoNotResolveRunIDs` |
| RES-18 | `cancel`, `suspend`, and `resume` merge selectors and require the active run | conformance | `TestJobControlSelectors` |
| RES-19 | `wait` resolves multiple run IDs independently | conformance | `TestWaitResolvesRunIDsIndependently` |
| RES-20 | Shell completion follows the location rules | partial | `TestCompletionScriptsExposeDynamicCompletion` |
| RES-21 | Missing state directories give no completion candidates | conformance | `TestCompletionMissingStateDirectoryHasNoCandidates` |
| RES-22 | Relative state and master directories resolve against the working directory | conformance | `TestRelativeStateDirectoriesResolveAgainstTheWorkingDirectory` |
| RES-23 | Cwd-only workspace discovery, staged location selection, scope merge and location constraints | partial | `TestWorkspaceInitAndCWDOnlyDiscovery`, `TestMergedFileConfigSnapshotAndScopeValidation`, `TestWebWorkspaceSourceSelectionAndRunSnapshot` |
| RES-24 | Init writes a cwd config template with relative basedir and project defaults without replacing settings or creating state | partial | `TestWorkspaceInitAndCWDOnlyDiscovery`, `TestWorkspaceInitDefaultTemplate` |
| RES-25 | Runs save immutable canonical merged file values, separate from runtime overrides and source metadata | partial | `TestMergedFileConfigSnapshotAndScopeValidation`, `TestWebWorkspaceSourceSelectionAndRunSnapshot` |
| RES-26 | List commands ignore implicit location defaults; jobs/runs/projects scan known basedirs by default, and show requires a uniquely selected detail target | conformance | `TestAggregateCommandsIgnoreImplicitLocationDefaults` |
| DUR-1 | Every executor runs jobs through the self-reporting wrapper | partial | `TestJobOutlivesKilledSupervisor` |
| DUR-2 | The wrapper records status independently of its launcher | partial | `TestJobOutlivesKilledSupervisor` |
| DUR-3 | An orphaned local job still records its own status | conformance | `TestJobOutlivesKilledSupervisor` |
| DUR-4 | Supervisors are not restarted; crash detection is file-backed | conformance | `TestJobOutlivesKilledSupervisor` |
| DUR-5 | `show`, `jobs`, reports, and the Web UI share one status fallback chain | partial | `TestCLIAndWebAgreeOnJobResults`, `TestCLIShowSelectedOlderAttemptIgnoresLatestSummary`, `TestNotStartedJobsAgreeAcrossViews`, `TestReportLabelsJobsAsShowDoes`, `TestAcceptedJobLabelAgreesAcrossViews`, `TestReportNamesTheExecutorTheAttemptRanOn`, `TestRunResultsCarryJobNames`, `TestStatusFallbackChainAgreesAcrossViews` |
| DUR-6 | Recovery does not kill or reconcile leftover jobs | partial | `TestRecoveryLeavesJobsRunning` |
| DUR-7 | A run records its carried results at start, and every view and job control reads carried jobs from them during the run | conformance | `TestCarriedJobsReadAsCarriedDuringTheRun` |
| DUR-8 | Run lifecycle, best-effort job execution state, and initiating-client connection history are separate shared display dimensions | conformance | `TestCLIAndWebAgreeOnJobResults`, `TestJobOutlivesKilledSupervisor`, `TestAsyncStartHintsWork` |
| DUR-9 | `delete` removes the deleted runs' client-attachment state and leaves other runs' alone | conformance | `TestDeleteRemovesRunAttachmentState` |
| SAFE-1 | `check` and `show` report a project as idle, running, or interrupted; a killed coordinator leaves it interrupted | conformance | `TestControlFromAnotherHost`, `TestProjectStates` |
| SAFE-2 | A running project rejects `run`, `retry`, and `delete`, so no second run or in-run retry starts; queue edits still target the next run | conformance | `TestRunningProjectRejectsSecondRunAndDelete`, `TestResetClearsQueueBesideActiveRun` |
| SAFE-3 | An interrupted project rejects `run` and `delete`, naming the run and how to inspect, recover, and rerun it | conformance | `TestInterruptedProjectNeedsRecovery` |
| SAFE-4 | `unlock` recovers an interrupted run, no-ops without one, and refuses one whose coordinator is alive | conformance | `TestCLIFlagPairUnlock`, `TestCLIFlagPairUnlockSamples`, `TestCLIFlagPairUnlockSafety`, `TestControlFromAnotherHost`, `TestInterruptedProjectNeedsRecovery`, `TestUnlockRefusesLiveRun`, `TestUnlockWithoutInterruptedRunIsNoOp` |
| SAFE-5 | `reset` clears only the next queue, preserves run history, and does not change an active or interrupted run | conformance | `TestResetOfInterruptedProject`, `TestResetClearsQueueBesideActiveRun` |
| SAFE-6 | Commands ask for confirmation only on a terminal, otherwise naming the option that confirms | conformance | `TestCopyIntoQueueWithoutTerminal` |
| SAFE-7 | Unlock warns when an interrupted run's jobs appear to still be running; MCP preview reports the same | conformance | `TestUnlockWarnsAboutRunningJobs`, `TestMCPUnlockRecoversAnInterruptedRun` |
| SAFE-8 | Queue edits, including reset, apply beside a running or interrupted run and keep its phase; `copy` rejects that run | conformance | `TestQueueEditsBesideAnActiveRun`, `TestResetClearsQueueBesideActiveRun`, `TestResetOfInterruptedProject` |
| SAFE-9 | Removed reset recovery flag and environment variable fail with an unlock hint | conformance | `TestResetRejectsRemovedRecoveryOptions` |
| SAFE-10 | `show` displays an active/interrupted run and a non-empty next queue separately | conformance | `TestShowActiveRunIncludesNextQueue` |
| COORD-1 | Controlling a local job from another host fails, naming that host | conformance | `TestControlFromAnotherHost` |
| COORD-2 | Cancelling a run whose coordinator is on another host fails, naming that host | conformance | `TestControlFromAnotherHost` |
| COORD-3 | A lock from another host keeps the project locked until `unlock` | conformance | `TestControlFromAnotherHost` |
| COORD-4 | A missing scheduler command fails the job with an error naming it | partial | `TestMissingSchedulerCommand` |
| COORD-5 | `ROTARI_PRIVATE_STATE` makes new paths owner-only; the static export stays publishable | partial | `TestPrivateStateModes` |
| COORD-6 | Suspend/resume preflight every selected target; errors after partial scheduler actions identify already-acted jobs | partial | `TestSuspendAndResumePreflightAllSelectedJobs` |
| STATE-1 | Reading state from a newer rotari fails asking to upgrade, and leaves the file; the Web UI marks such a run unreadable | conformance | `TestNewerStateVersionIsRejected`, `TestWebShowsNewerRunAsUnreadable`, `TestJobNameLookupReportsSnapshotErrors` |
| STATE-2 | State without `state_version` reads as version 1 | conformance | `TestUnversionedStateIsVersionOne` |
| STATE-3 | Reading history never rewrites it | conformance | `TestReadingHistoryDoesNotRewriteIt` |
| STATE-4 | Malformed load samples are skipped | conformance | `TestMalformedLoadSamplesAreSkipped` |
| CAN-1 | A whole-run cancel stops every running job of the run, from any caller | conformance | `TestWholeRunCancelFinishesRun` |
| CAN-2 | A cancelled run finishes with a summary and leaves the project idle, not interrupted | conformance | `TestWholeRunCancelFinishesRun` |
| CAN-3 | `cancel --wait` returns once the run has finished and exits 0 | conformance | `TestCancelWaitReturnsAfterRunFinishes` |
| CAN-4 | Cancelling one job stops only that job, which the run does not retry | conformance | `TestCancelJobStopsOnlyThatJob` |
| CAN-5 | A job stopped by a cancel records a cancelled result, read as the `cancelled` failure kind | conformance | `TestCancelledJobsReadAsCancelled` |
| CAN-6 | A job cancelled before it starts never starts, on every submission path | conformance | `TestCancelledPendingJobNeverStarts` |
| CAN-7 | The async start message ends its line, and its wait and cancel hints work as printed for that run | conformance | `TestAsyncStartHintsWork` |
| CAN-8 | A whole-run cancel records the run as `cancelled` with exit code 1 in every view; cancelling selected jobs leaves it `failed` | conformance | `TestWholeRunCancelRecordsCancelledRun` |
| LOG-1 | Attempts use the selected internal merge or separate log mode, independently of external destinations | conformance | `TestJobStreamsPersistSeparately` |
| LOG-2 | Repeatable `--output`/`--error` sinks; absent `--error`, stderr follows `--output`; independent of internal log mode | conformance | `TestExternalLogDestinations` |
| LOG-3 | Destination parents are created on the execution host; setup errors fail before the command starts | conformance | `TestExternalLogDestinations` |
| LOG-4 | External sinks append by default; truncate initializes each destination once; shared streams have no guaranteed interleaving | conformance | `TestExternalLogDestinations` |
| RUN-1 | A filtered rerun executes the selected jobs, carries completed results outside the selection into the new run, and leaves the source run unchanged | conformance | `TestFilteredRerunCarriesCompletedResults` |
| RUN-2 | A run-level retry limit retries a failed job within the same run until it succeeds or the limit is exhausted; a successful retry makes the run successful | conformance | `TestRunRetrySucceedsWithinOneRun` |
| RUN-3 | Executor working-directory defaults are portable; run `--env=ALL&#124;NONE` controls caller environment propagation and job overrides consistently | partial | `TestRunUsesCallersDirectoryAndEnvironment` |
| RUN-4 | New-run fingerprint matching prioritizes Job ID/Origin, matches remaining expanded jobs by fingerprint and occurrence, and treats count mismatches as new work | conformance | `TestFingerprintMatchingIgnoresNonInputMetadata`, `TestFingerprintMatchingNormalizesDirectoryAndIgnoresArrayRange`, `TestFingerprintMatchingPreservesArrayTasksAndMatrixLeaves`, `TestFingerprintMatchingPrioritizesIDsAndQueueOccurrence`, `TestFingerprintMatchingRejectsChangedCommand`, `TestFingerprintMatchingRejectsChangedExplicitInputs`, `TestFingerprintMatchingRejectsChangedMatrixValue`, `TestFingerprintMatchingTreatsMissingHistoryAsNewWork`, `TestFingerprintMatchingUsesIDsAndRejectsCountMismatches` |
| RUN-5 | Imported manifest jobs without source provenance execute as new work without a previous-run lookup | conformance | `TestImportedWorkflowRunsFreshJobs` |
| RUN-6 | `wait --until-failure` returns at a job's final failure, not at a failure the run retries | conformance | `TestCLIFlagPairWaitJSONAndEarlyFailure`, `TestWaitUntilFailureIgnoresAFailureTheRunRetries`, `TestWaitUntilFailureReturnsAtAFinalFailure` |
| RUN-7 | A named array job's name in a dependency stands for all of its tasks | conformance | `TestArrayNameDependsOnEveryTask` |
| RUN-8 | Manifest and CLI matrix exclusions omit matching combinations and survive queue/run export/import | conformance | `TestWorkflowMatrixExclusionExportImport`, `TestMatrixExclusionRejectsRepeatedDimension` |
| RUN-9 | Each submitted attempt records its statically discovered artifact candidates in `artifacts.json` without affecting execution | conformance | `TestArtifactCandidateExamples`, `TestDeclaredArtifactsFollowTasksAndEdits`, `TestShellVariablesDifferPerArrayTask`, `TestStartedAttemptRecordsArtifactCandidates` |
| RUN-10 | An imported manifest's edited status applies to the job or leaf it is written on; a matrix or array job has no status of its own, so unlisted leaves keep their source result and an edited job-level status is rejected | conformance | `TestImportedArrayWideningExecutesNewTasks`, `TestImportedGroupStatusKeepsUnlistedLeafResults` |
| RUN-11 | Export and import select the latest timestamped command snapshot; ties go to the last listed run, not the random run ID suffix | conformance | `TestWorkflowExportUsesListedRunOrderForTimestampTies` |
| RUN-12 | A run that ended without a summary, such as an interrupted run after unlock, can be the source of a filtered rerun; finished jobs keep their results and the rest are unfinished | conformance | `TestRerunOfAnInterruptedRun` |
| RUN-13 | Saved-run starts build their snapshot without changing the next queue and reject `--overwrite` | conformance | `TestRetryFromSavedRunLeavesNextQueueUntouched`, `TestMCPRetryFromSavedRunKeepsNextQueue` |
| RUN-14 | A non-empty queue retry reports failed/unfinished latest-run jobs it omits without changing selection | conformance | `TestRetryReportsFailedJobsOmittedByNonEmptyQueue`, `TestMCPRetryReportsFailedJobsOmittedByQueue` |
| CLI-1 | `check --json` reports the same project state, run identifier, queue count, lock, and runnable result as the human-readable `check` output | partial | `TestCLIFlagPairCheckObservability`, `TestCheckJSONMatchesText` |
| CLI-2 | Human-readable `jobs` columns keep their visible start positions aligned across rows; ANSI color sequences do not count toward column width | conformance | `TestJobsTableKeepsVisibleColumnsAligned` |
| CLI-3 | All command options shared by CLI, environment, and config use the same source precedence | conformance | `TestCLIOptionPrecedence` |
| CLI-4 | `show`, `lineage RUN`, and the Web API group a run's failed and blocked jobs by the same causes | conformance | `TestFailureGroupRetryHintsWork`, `TestFailureGroupsAgreeAcrossViews` |
| CLI-5 | `projects` lists each project's last result, and its suggested commands work for every listed project | conformance | `TestProjectListHintsWork` |
| CLI-6 | `jobs --since`, `runs --since`, and the Web jobs page take a Go duration or whole days such as `7d`; CLI listings default to `1d` and keep active work visible | conformance | `TestJobsWindowAcceptsDays`, `TestRunsWindowFiltersSettledHistoryButKeepsActiveRun` |
| CLI-7 | Commands that change a project take `--dry-run` and `--if-revision`, applying only at the previewed revision | conformance | `TestCLIFlagPairImportObservability`, `TestChangePreviewNamesTheFieldsItChanges`, `TestGuardedCommandsPreviewAndCheckTheRevision`, `TestRunPreviewListsTheTasksOfAWholeArray`, `TestRunPreviewMatchesTheRun`, `TestRunPreviewSaysWhyADependentJobExecutes`, `TestRunRestoringEditsSayTheyReplaceTheQueue` |
| CLI-8 | `run` and `retry` reject `--async` with `--dry-run`, which does not start a run | conformance | `TestAsyncDryRunRefusalShowsTheWayToPreview`, `TestCLIFlagPairAsyncDryRunIsRejected` |
| CLI-9 | `run` and `retry` use consistent selector combinations in dry-run plans | partial | `TestCLIFlagPairRunSelectionEffects` |
| CLI-10 | A supplied `--run-name` is visible in a dry-run preview | conformance | `TestCLIFlagPairRunNameInPreview` |
| CLI-11 | A missing project's error lists local projects and the registered state directories that have it | conformance | `TestMissingProjectNamesWhereItIs`, `TestMissingTargetDiagnostics` |
| CLI-12 | `wait --json` reports completed and early-failure runs as structured JSON | conformance | `TestCLIFlagPairWaitJSONAndEarlyFailure`, `TestCLIFlagPairWaitSamples` |
| CLI-14 | Text `wait` prints the same single, newline-terminated completion message as `run`, which groups failures by cause as `lineage` does | conformance | `TestWaitPrintsSameCompletionMessageAsRun`, `TestCompletionMessageGroupsFailuresByCause` |
| CLI-15 | Single-value CLI options reject duplicate occurrences while repeatable options remain repeatable | conformance | `TestCLIRejectsRepeatedSingleValueOption` |
| CLI-16 | `show` of one job lists its attempt's artifact candidates, `--artifacts` lists all of them, and `--json` carries them | conformance | `TestShowListsArtifactCandidates` |
| CLI-17 | Command help survives configuration errors with a warning, without bypassing configuration errors for execution | conformance | `TestCommandHelpSurvivesConfigurationErrors`, `TestConfigurationErrorsStillPreventExecution` |
| CLI-18 | `add` warns on duplicate fingerprints involving added units, including quiet and dry-run, without rejecting the jobs | conformance | `TestAddWarnsOnDuplicateFingerprints` |
| CLI-19 | Sync `run`/`retry` and `wait` share file-backed attachment, progress, cancellation, and disconnect behavior; every client, implicit waiters included, has an independent session | conformance | `TestWaitLiveProgress`, `TestRetryProgressCountsOnlyExecutedJobs`, `TestWaitQuietAndJSON`, `TestWaitInterruptCancelsRun`, `TestWaitInterruptCancelsAllSelectedRuns`, `TestSynchronousRunInterruptCancelsAcceptedRun`, `TestWaitWarnsForAttachedRunTargetsAndFollowsThemImplicitly`, `TestWaitAttachmentIsSharedAndImplicitWaitFollowsIt`, `TestRunClientDisconnectDetachesByDefaultAndCanCancel`, `TestWaitClientDisconnectDetachesByDefaultAndCanCancel`, `TestWaitJSONDisconnectWithSIGKILLCancelsRun`, `TestRunTerminalControlsPrintOutcomeOnce`, `TestWaitWithoutSelectorWaitsForRunsThisProcessStarted` |
| CLI-20 | Every command option maps to a CLI-default environment variable unless explicitly command-line-only | conformance | `TestCLIFlagEnvironmentCoverage` |
| CLI-21 | `info` reports context and active run state, marks uncreated projects, reports finished, failed, and pending jobs, best-effort checks local unfinished job process groups, and keeps coloring to terminal text output | conformance | `TestInfoReportsContextAndLeavesStaleLockUntouched`, `TestInfoCountsFailedJobsOfActiveRuns`, `TestNotStartedJobsAgreeAcrossViews` |
| CLI-22 | Printed commands name `--basedir` only when the state directory is not the one a command started in the same place would use, and work as printed | conformance | `TestPrintedHintsNameOnlyANonImplicitBaseDir` |
| CLI-23 | `show` reports each job's elapsed time, and for a running job how long ago it last wrote output | conformance | `TestShowReportsElapsedAndQuietTime` |
| CLI-24 | `show --logs` lists a run's jobs in definition order and only its jobs, and `--tail N` prints the last N lines of each log | conformance | `TestShowLogsTailInDefinitionOrder` |
| MCP-1 | MCP tools that change a project preview first and apply only at the previewed revision | conformance | `TestMCPWritesApplyOnlyAtThePreviewedRevision`, `TestRunPreviewListsTheTasksOfAWholeArray` |
| MCP-2 | `rotari_export_run` is a redacted view that the MCP import tools refuse | conformance | `TestMCPExportIsARedactedViewThatImportRefuses` |
| MCP-3 | `rotari_run_summary` and `rotari_wait_run` report the run's state as `wait` decides it, from right after the start, and tool errors name no state directory | conformance | `TestMCPWritesApplyOnlyAtThePreviewedRevision`, `TestMCPWaitReturnsOnTheFirstFinalFailure` |
| MCP-4 | MCP job control previews the jobs it reaches and acts only on the named running run | conformance | `TestMCPJobControlActsOnlyOnThePreviewedRunningRun` |
| MCP-5 | MCP reset previews and applies at the revision, clearing only the queue and leaving runs untouched | conformance | `TestMCPResetOnlyClearsTheQueue` |
| MCP-6 | MCP stdin EOF ends the session cleanly, even with responses in flight; malformed input remains an error | conformance | `TestMCPStdinEOF` |
| MCP-7 | MCP unlock previews the interrupted run, applies at the revision, refuses a live run, and keeps the queue | conformance | `TestMCPUnlockRecoversAnInterruptedRun` |
| SEL-1 | A run ID or attempt ID alone resolves its location in every command | partial | `TestPositionalArguments`, `TestSelectorTable` |
| SEL-2 | Each command reads the run or queue its row names | partial | `TestJobSelectorLooksInTheQueueBesideAnActiveRun`, `TestSelectorTable` |
| SEL-3 | `show` resolves each selector form as its column says and names missing positional selectors | conformance | `TestJobSelectorLooksInTheQueueBesideAnActiveRun`, `TestMissingTargetDiagnostics`, `TestSelectorTable` |
| SEL-4 | `copy` resolves each selector form as its column says | conformance | `TestCLIFlagPairCopyObservability`, `TestSelectorTable` |
| SEL-5 | `run` and `retry` resolve each selector form as their column says | conformance | `TestCLIFlagPairRunSelectionEffects`, `TestRunJobIDRunsEditedQueue`, `TestSelectorTable` |
| SEL-6 | `change` resolves each selector form as its column says | conformance | `TestSelectorTable` |
| SEL-7 | `remove` resolves each selector form as its column says | conformance | `TestCLIFlagPairMutationObservability`, `TestSelectorTable` |
| SEL-8 | Selectors combine by kind, with the listed exclusions | conformance | `TestSelectorTable` |
| SEL-9 | `cancel`, `suspend`, and `resume` resolve each form as the job control table says | conformance | `TestJobControlSelectors` |
| SEL-10 | Each command takes its positional arguments with their meaning and exclusions | conformance | `TestPositionalArguments` |
| SEL-11 | The `--filter-*` options select jobs as the Filters section says | conformance | `TestCLIFlagPairObservability`, `TestSelectorTable`, `TestShowLogResultSelection` |
| SEL-12 | `--job-name` and filters choose unfinished jobs for `cancel`, `suspend`, and `resume`, confirmed before acting | partial | `TestCLIFlagPairCancel`, `TestCLIFlagPairResume`, `TestCLIFlagPairSuspend`, `TestJobControlSelectors` |
| WEB-1 | Run timelines start at the actual run start, show carried results at start, and include timestamped events for executed jobs with origins, without origin-time rewinds | conformance | `TestFilteredRerunCarriesCompletedResults`, `TestBlockedOriginJobsDoNotRewindWebTimeline`, `TestRunTimelineStartsAtActualRunStart` |
| WEB-2 | Live history search covers user-selected basedir/project/run scopes with validated, read-only hierarchical filters and pagination, including recorded unfinished job states | partial | `TestHistorySearchAcrossProjects`, `TestHistorySearchFindsRecordedUnfinishedJobStatus` |
| WEB-3 | Static export rejects live-server-only web options instead of silently ignoring them | conformance | `TestCLIFlagPairWebStaticServerOptions` |
| WEB-4 | Static export applies `--notifications` to the initial browser-notification toggle | conformance | `TestCLIFlagPairWebNotificationsEffect` |
| WEB-5 | The Web UI and `/api/artifacts` show a job attempt's artifact listing as the CLI does, including in the static export | conformance | `TestWebShowsArtifactCandidates` |
| WEB-6 | The live server serves artifact content only for recorded entries under allowed roots, without symlink escapes, with safe headers and paged text and directories | conformance | `TestWebPreviewsArtifactsUnderAllowedRoots` |
| WEB-7 | `--static-artifact-contents` copies servable artifact contents into a static export within size limits | conformance | `TestStaticExportCopiesArtifactContents` |

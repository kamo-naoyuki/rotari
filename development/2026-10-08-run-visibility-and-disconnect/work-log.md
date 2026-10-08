# Work History: Run Visibility and Client Disconnect Behavior

See [plan.md](plan.md) for current scope and open lifecycle decisions.

## Separate collection views from detail inspection

**Commit:** 2026-10-08 03:09:18 +09:00 `a9538376`.

**Change:** Added `basedirs`, `projects`, and `runs` collection commands; retained `jobs` as the job-level listing. Removed `show --basedirs` and bare `show`'s aggregate fallback. `show` now resolves one project/run/job/attempt, showing the relevant run or queue and directing ambiguous project selection to `projects`. Split collection rendering from `show`, updated configuration scopes and shell completion, and made Python's generic command adapter inject only supported location options. Updated generated schema/references, README, user guides, contracts, golden files, and regression/conformance tests. Run listings preserve interrupted/running lifecycle state even when a summary exists and propagate unreadable/newer-version summaries.

**Reason:** Make list discovery predictable and reserve `show` for selected-target inspection before considering background client attachment/disconnect semantics.

**Plan impact:** Completed only the list-command and `show` phase. Ordinary project resolution applies to bare `show`; `runs` lists active/interrupted and saved history in the default non-config basedir, with explicit project filtering and `--all-basedirs`. Attachment state, disconnect policy, supervisor restart/recovery, and stronger orphan termination remain deferred. No runtime cancellation or detach behavior changed.

**Validation:** `go test ./cmd/rotari -count=1`; affected resolution/interface/selector conformance suites; CLI read flag-pair matrix; contract-status and golden checks; pre-commit on changed files; `scripts/check.sh --short`; final `scripts/check.sh` (go vet, all Go tests, race tests, exit 0); Python tests against a freshly built CLI (64 passed); generated CLI/Python API/README synchronization checks; strict MkDocs build; document link checks. Full-check logs were retained in the session's temporary directory.

**Remaining:** Decide attachment visibility and unexpected-client-disconnect policy separately, as listed in the plan. No compatibility alias for old list forms was added.

## Use selector shorthand in run hints

**Change:** Updated the `runs` detail hint to `rotari show -r RUN_ID` and the project failure-summary hint to `rotari lineage RUN_ID`; the run-ID registry resolves each location, so neither hint needs `--basedir`/`--project-name`. Updated executable hint tests.

**Reason:** Keep run inspection and lineage suggestions as short as the commands' run-ID resolution permits.

**Plan impact:** No lifecycle or list scope change; shorter command hints only.

**Validation:** `go test ./cmd/rotari -count=1` passed; `go test ./conformance/03-interfaces -run '^TestProjectListHintsWork$' -count=1` passed; focused run-hint tests and `git diff --check` passed.

**Remaining:** None.

## Make jobs and runs cross-basedir by default

**Change:** Aligned `jobs` and `runs` with the already-cross-basedir `projects` listing. All three now search registered basedirs plus the normal local default without an extra switch; `jobs` and `runs` accept `--basedir DIR` to narrow. Removed `--all-basedirs`. Set the shared job-list history window (including the Web jobs page) to `1d`; `runs --since` now filters only settled history and always retains running/interrupted/incomplete runs. The default multi-basedir `jobs` view includes a `BASEDIR` column.

**Reason:** Make all collection commands discover work consistently while preventing completed history from growing without bound.

**Plan impact:** Resolves the open listing-scope decision. `basedirs` remains the way to inspect the registry. Disconnect policy and client attachment visibility remain deferred.

**Validation:** Focused `cmd/rotari` tests, settings/completion tests, runs-window conformance, CLI flag-pair inventory, schema/golden generation, and documentation checks passed. Final `scripts/check.sh --short` and `scripts/check.sh` both passed (go vet, all Go tests, and race tests; combined validation command exited 0). Python tests against the freshly built CLI passed (64 tests), as did `TestContractStatus` and `TestGoldenOutputs` with `-count=1`. Final successful logs are retained as `rotari-cross-short-final.log` and `rotari-cross-full-final.log` in the session's temporary directory. Documentation and final diff checks were reviewed before commit; no full tests were rerun during commit cleanup.

**Remaining:** None for this listing change. Client attachment visibility and disconnect policy remain separate work items.

## Share live progress between run and wait

**Change:** Added `wait --quiet` and live text progress matching synchronous
`run`. Both execution modes record the existing observer's events in an optional
append-only progress journal regardless of the starting client's quiet setting.
`wait` uses an incremental reader and the shared CLI renderer, drains final events
before completion, and does not replay old progress for already-finished runs.
JSON remains result-only; quiet preserves failure diagnostics and requested JSON.
Journal I/O failures fall back to result-only waiting, and a job ID matching the
journal filename disables the optional journal rather than breaking execution.

**Reason:** `run --async` followed by `wait` should provide the same progress
information as a synchronous run rather than silently waiting until completion.

**Plan impact:** Adds a read-only progress projection, not a supervisor reconnect
or a change to cancellation/detach semantics. Ending or interrupting `wait` leaves
the run executing. Historical runs without a journal remain waitable.

**Validation:** The regression test failed before the CLI fix. Focused journal,
observer, CLI, public wait, and all 28 wait flag-pair tests passed, as did the full
CLI package and affected resolution/interface/selector conformance suites. `scripts/check.sh --short`
and `scripts/check.sh` passed (vet, full tests, and race tests). `git diff --check`
passed. `python3 -m pre_commit` on the changed files passed (the `pre-commit`
executable is not on PATH).

**Remaining:** None.

## Show only new wait progress

**Change:** `wait` now starts at the end of the existing progress journal and
prints an attach marker plus a single current progress-count snapshot, then only
events appended afterward. It still drains events produced while waiting and
before completion; partial records at the attach boundary are kept until
complete. Updated the contract and running guide to make the no-history-replay
behavior explicit.

**Reason:** Replaying every progress line since the beginning of a long-running
job is noisy when a user starts `wait` after `run --async` has already reported
the start and current activity.

**Plan impact:** Adjusts only the display cursor for `wait`; journaling,
completion, exit status, JSON, and cancellation behavior are unchanged.

**Validation:** Focused cursor tests and repeated live wait conformance passed;
related packages also passed with race detection and `-count=1`. Final
`scripts/check.sh` passed (vet, all tests, and race tests; exit 0).

**Remaining:** None.

## Add run controls to `wait`

**Change:** Active text waits now advertise Ctrl-D to stop waiting without
stopping the run and Ctrl-C to request cancellation. Ctrl-C cancels every
selected run that is still active, waits for local observers to stop, and exits
130. Quiet and JSON output remain free of interactive progress/control text.

**Reason:** Make async `run` followed by `wait` offer the same practical
detach/cancel controls as an attached synchronous run, while preserving the
important distinction that Ctrl-D only detaches the waiter.

**Plan impact:** Changes only the wait client's interaction with selected runs;
it does not change synchronous run disconnect behavior.

**Validation:** Single/multiple-run Ctrl-C cancellation, Ctrl-D detach, quiet,
JSON, and output-label tests passed. Related CLI/state/supervisor/interface/run
pair/lifecycle packages passed with race detection and `-count=1`; pre-commit
passed. Earlier full checks encountered edit-pair race and static-web timeouts;
their logs were retained. The final non-overlapping `scripts/check.sh` passed
(vet, all tests, and race tests; exit 0), recorded in
`rotari-wait-controls-serial-final.log` in the session temporary directory.

**Remaining:** None. Implementation changes were included in concurrent commit
`01701c58`; this follow-up synchronizes the wait contract and validation record.

## Display run, job, and client status separately

**Change:** Persisted the initiating client's sync/async mode and last connection
transition in each run. `runs` and `show` now display lifecycle and client
connection separately, distinguishing `async (detached)` from
`detached (Ctrl-D)` and reporting unverified supervisor/client state as
`unknown`. Added shared best-effort job execution labels for terminal results,
recorded waiting/running/suspended phases, carried results,
and unknown state. Exposed the same projection in run/job JSON and the Web API/UI.

**Reason:** A run can be interrupted while one or more jobs still execute or
later update their wrapper status. Persisted job observations provide useful
visibility without contacting executors, while separate lifecycle/client fields
avoid implying that `running` proves a process is alive or that a run is attached.

**Plan impact:** Implements the planned status dimensions and leaves
unexpected-disconnect behavior unchanged (detach by default, configurable
cancellation). No executor probing or run recovery was added.

**Validation:** Uncached CLI and focused conformance tests passed, including
wait controls/environment policies, selectors, async visibility, interrupted
wrapper updates, carried results, and contract/golden checks. Related package
tests and uncached race tests passed. The Web runtime test also failed for the
expected lifecycle/cache reasons against pre-fix `6b40a5ba` in a temporary
worktree, which was removed, and passed on the final implementation.

A full check and an additional uncached race check exposed concurrent JSON
encoder use by progress and the detached response. Response writes are now
serialized; sync-client tests passed with `-race -count=20`. The post-fix CLI
suite, focused conformance, and `scripts/check.sh --short` passed. Formatting,
JavaScript syntax, architecture, and document-link checks passed. Complete logs
and exit-code sidecars are retained under the session temporary directory as
`rotari-status-*.log` and `rotari-status-*.log.exit`.

The final `scripts/check.sh` completed successfully (vet, all Go tests, and
race tests; exit 0), logged as `rotari-status-full-serial-complete.log`.
The large edit-pair suite also passed uncached with race detection (512.787s),
logged as `rotari-status-editpairs-verbose-final.log`. Final review restored
unrelated editor overwrites in the running guide, architecture, contracts,
and async hints. The existing coarse recent-jobs list projection is recorded
separately in `development/ISSUES.md`; its behavior was not changed here.

**Remaining:** None for this status-visibility work. The pre-existing
`examples/basic.sh` edit is excluded from the commit.

## Detached-first client labels

**Change:** Active async clients now display `detached (async)` alongside
`detached (Ctrl-D)`. Both labels start with the connection state while keeping
the initiating client's detach reason distinguishable. Completed and unknown
history labels, persisted client fields, and detach/cancel behavior are unchanged.
The Web API now supplies `client_label` from the shared CLI formatter in both
lightweight summaries and full run details. Web rendering already prefers this
field; its fallback now uses the same detached-first labels.

**Plan impact:** Clarifies the existing client-status dimension; updates the
inspection guide and DUR-8 without changing run lifecycle semantics.

**Validation:** Before the implementation change, the shared formatter and Web
runtime tests failed on the old `async (detached)` wording. Focused uncached tests
passed for runview, CLI lists/show, Web projections, Web API and runtime rendering,
async-start conformance, and CLI/Web status agreement. Final formatting and full
validation results are recorded below after completion.

**Scope:** The pre-existing `examples/basic.sh` edit remains untouched and will
not be staged.

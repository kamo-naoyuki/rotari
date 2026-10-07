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

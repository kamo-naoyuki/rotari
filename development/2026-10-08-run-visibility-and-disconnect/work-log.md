# Work History: Run Visibility and Client Disconnect Behavior

See [plan.md](plan.md) for current scope and open lifecycle decisions.

## Separate collection views from detail inspection

**Commit:** 2026-10-08 03:09:18 +09:00 `a9538376`.

**Change:** Added `basedirs`, `projects`, and `runs` collection commands; retained `jobs` as the job-level listing. Removed `show --basedirs` and bare `show`'s aggregate fallback. `show` now resolves one project/run/job/attempt, showing the relevant run or queue and directing ambiguous project selection to `projects`. Split collection rendering from `show`, updated configuration scopes and shell completion, and made Python's generic command adapter inject only supported location options. Updated generated schema/references, README, user guides, contracts, golden files, and regression/conformance tests. Run listings preserve interrupted/running lifecycle state even when a summary exists and propagate unreadable/newer-version summaries.

**Reason:** Make list discovery predictable and reserve `show` for selected-target inspection before considering background client attachment/disconnect semantics.

**Plan impact:** Completed only the list-command and `show` phase. Ordinary project resolution applies to bare `show`; `runs` lists active/interrupted and saved history in the default non-config basedir, with explicit project filtering and `--all-basedirs`. Attachment state, disconnect policy, supervisor restart/recovery, and stronger orphan termination remain deferred. No runtime cancellation or detach behavior changed.

**Validation:** `go test ./cmd/rotari -count=1`; affected resolution/interface/selector conformance suites; CLI read flag-pair matrix; contract-status and golden checks; pre-commit on changed files; `scripts/check.sh --short`; final `scripts/check.sh` (go vet, all Go tests, race tests, exit 0); Python tests against a freshly built CLI (64 passed); generated CLI/Python API/README synchronization checks; strict MkDocs build; document link checks. Full-check logs were retained in the session's temporary directory.

**Remaining:** Decide attachment visibility and unexpected-client-disconnect policy separately, as listed in the plan. No compatibility alias for old list forms was added.

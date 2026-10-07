# Work log: Workspace configuration resolution

## 2026-10-07 16:57:38 +09:00 — Configuration primitives

Commit: `19fa824d` — Add scoped workspace configuration and immutable file values.

- Added cwd-only workspace discovery, explicit scope/path provenance,
  discovered-file errors, scope location-key restrictions, recursive map merge,
  replacing arrays/scalars, null-as-unspecified, and canonical TOML snapshots.
- Added independent file-config/source domain types. Configuration still
  depends only on permitted lower-level packages; no state/config cycle.
- Validation actually run: focused config/CLI workspace tests;
  `go test ./internal/config ./internal/model -count=1`; uncached architecture,
  notification and doc-link tests. Changed-file pre-commit passed after its
  expected first formatting pass. CLI/Web package checkpoints passed before
  the latest source-projection and documentation changes.
- New binary workspace/init/merge/snapshot conformance tests passed. Root
  conformance initially failed only because init changed help/schema golden
  output; those outputs were regenerated using the existing update workflow.
- Next: finish interface validation, generated metadata formatting, short/full
  checks including race detection, inspect diffs and commit remaining work.

## 2026-10-07 17:17:32 +09:00 — CLI, runs, Web, contracts and documentation

Commit: `fe1975a9` — Implement workspace defaults, merged run configs, and
source-specific Web editing.

- Added staged location/config loading, cwd-only init without state side
  effects, explicit-selector provenance for registry targets, metadata/help/
  completion/templates/inventory, and positional-project config loading.
- Run requests carry immutable canonical file values through sync/async,
  retry/saved-run and MCP paths. Sources and snapshot files are independent;
  notification files retain their separate first-file selection and snapshots.
- Current Web views choose real scope/path sources and save only validated
  targets atomically; run views keep saved read-only config, including legacy
  YAML/JSON. Startup cwd stays fixed across projects/basedirs. Static exports
  reuse source selection and reject writes.
- Added CLI/unit/JavaScript/binary/Web tests, contract IDs RES-23–25 and source
  metadata. Updated user docs, agent guide, generated schema/golden/Python CLI
  metadata, CLI reference and generated README section.
- Validation actually run: all related package tests; focused workspace,
  snapshot stability/deletion/failure ordering and Web chooser tests; resolution
  and Web/async binary conformance; root conformance; CLI flag-pair inventory;
  pre-commit; generation synchronization checks; Python client tests (29 passed).
- Broad go vet excluding scripts passed. The first broad test run exposed
  stale single-source show/UI fixture expectations and missing init inventory;
  these were corrected to the approved contract and their focused tests and
  complete affected package suites passed afterward.
- Both prescribed checks were attempted in main, but stopped in go vet on a
  concurrently edited unrelated scripts test's duplicate package declaration.
  No change was made to that work. Whole-repository validation is proceeding
  against this exact commit in a detached temporary worktree with existing
  node_modules reused. Main's branch/HEAD was not switched.
- Concurrent notification-document deletion and Slurm example changes were
  deliberately left unstaged; only this feature's new notification explanation
  was committed. Other sessions' commits were preserved.

## 2026-10-07 17:54:40 +09:00 — Selector observation compatibility

Commit: `2cf1aab7`.

- Change: excluded run-level `configs/` from the selector conformance helper's
  job-directory observations; production job selection was unchanged.
- Reason: every new run now has a config snapshot, even with no file settings.
  The old observer counted that metadata directory as an unknown `?configs`
  job, causing 41 table failures.
- Plan impact: resolved the config snapshot's interaction with existing
  selector contract tests without changing their expected job selections.
- Validation: reproduced the failures before the correction; the corrected
  selector table and full selector package passed uncached in short mode.
  `scripts/check.sh --short` passed in the detached `fe1975a9` worktree with
  this correction, including go vet and all short tests.
- Remaining: final full/race checks and cleanup of the validation worktree.

## 2026-10-07 18:11:37 +09:00 — Init template and symmetric defaults

Commit: `3abd9599`.

- Change: init shares the normal workspace TOML template generator and fills
  in only basedir/project-name. Other option assignments are commented out.
  Omitted arguments write `.rotari-state` and `default`. Existing-file errors
  now explain overwrite refusal and direct users to edit the settings, rather
  than exposing the temporary-file link operation.
- Reason: initialization should produce an editable full config template and
  symmetric location defaults rather than omit the project default.
- Plan impact: updated RES-24, its contract status mapping, user configuration
  guide, and init specification. Init still creates no state/project/registry.
- Validation: new default/template tests failed before implementation;
  focused init and config tests passed afterward, including file/symlink/
  directory refusal and binary init-default tests. Complete CLI and resolution
  package tests, root conformance, doc links, and focused init race tests passed.
  The contract-status check initially caught the missing new test name; its
  table was corrected and the check passed.
- Final validation: the subsequent full `scripts/check.sh` completed with
  exit code 0 and `all checks passed`, including go vet, all package and
  conformance tests, and go test -race. Some unchanged packages used Go's
  test cache; no cache-free whole-repository run is claimed. The earlier
  check overlapping test edits failed and is not the final validation result.
- Cleanup: removed the detached workspace validation worktree after verifying
  its selector observation correction matched the committed main version.
- Remaining: None.

# Plan: Matrix Exclusions in Workflow Manifests and CLI

**Created:** 2026-10-04

**Status:** Workflow-manifest and `rotari add --matrix-exclude` implementations
are complete. Focused unit, binary conformance, add flag-pair, and generated-doc
checks pass. Implemented in commits `a68f1cc` and `22f244f`; a clean full
repository check remains outstanding due the previously recorded Web fixture
snapshot failure.

## Purpose

Allow a workflow manifest to omit selected combinations from a matrix while
keeping the exclusion declaration when exporting the queue or a settled run.
The effective combinations—not the full Cartesian product—are the matrix
group's expected members.

## Scope and decisions

- Add **exclude only**. `include` and GitHub Actions-style row merging are
  non-goals; adding another matrix remains expressible as another job.
- Support both workflow manifests (`matrix_exclude`) and `rotari add`
  (`--matrix-exclude`). The CLI option is repeatable and each occurrence is
  one partial assignment rule in `KEY=VALUE[,KEY=VALUE...]` form.
- `--matrix-exclude` requires at least one `--matrix`; without it, `add` must
  fail with an error naming the required option. Do not silently ignore it.
- The CLI option is command-line-only; configuration and environment variables
  must not apply it to an unrelated `add` invocation.
- Proposed manifest syntax: an optional matrix-specific `matrix_exclude` list
  of partial dimension assignments, alongside the existing `matrix` field.
  For example:

  ```yaml
  matrix:
    SEED: [1, 2]
    MODEL: [small, large]
  matrix_exclude:
    - SEED: 2
      MODEL: large
  ```

  A rule excludes every generated combination matching all of its entries.
  Keys must name declared dimensions and values must belong to those
  dimensions. Rules must be non-empty and unique; overlapping rules are
  harmless. Reject a matrix when exclusions remove all
  combinations, rather than falling back to a non-matrix job.
- Keep matrix dimension declarations and exclusion rules in persisted matrix
  provenance for every group member. Export reproduces the declared rules,
  rather than inferring them from missing combinations. Existing manifests
  without `matrix_exclude` retain their current expansion and export behavior.
- JSON and TOML receive the equivalent optional `matrix_exclude` field. Keep the
  existing workflow manifest version: this is additive for existing manifests.
  Older rotari versions may reject queues containing excluded groups because
  their validator expects the full Cartesian product; they must not silently
  accept such a group as complete.
- Web matrix grids need no exclusion-specific API field: the UI already builds
  cells from the dimensions and actual members, so excluded combinations
  naturally render as empty cells. No distinct visual marker or interactive
  exclusion editor is in scope.

## Implementation approach

### 1. Define one effective-expansion rule

- Add a model representation for normalized exclusions and a shared expansion
  helper that returns the Cartesian combinations in existing order, minus any
  combination matched by an exclusion rule.
- Keep `ExpandMatrix` behavior for callers that do not pass exclusions.
- Normalize each rule against declared dimension order for persisted metadata
  and deterministic comparison. Reject unknown dimensions, undeclared values,
  empty rules, duplicate rules, and an empty effective matrix with actionable
  errors.

### 2. Compile and validate provenance

- Decode and validate `matrix_exclude` for YAML, JSON, and TOML manifests, then use
  the shared expansion helper in manifest compilation.
- Persist the normalized dimensions and exclusion rules with each expanded
  `MatrixSpec` member.
- Update matrix-group validation to require exactly the effective combination
  set, reject duplicates and extra/missing combinations, and ensure every
  member carries identical exclusion provenance.
- Update completeness checks used by queue mutations to count the effective
  combinations rather than multiplying dimension lengths. Partial copy/remove
  continues to clear provenance as it does today.
- Update workflow reconciliation's expected command count to use the same
  effective expansion, so instance matching and import planning agree with
  compilation.

### 3. Preserve rules through export/import

- Make queue and settled-run export emit the original normalized exclusion
  rules together with the matrix dimensions.
- Ensure run export still emits `instances` only for actual matrix/array
  members; excluded combinations must not be mistaken for unfinished leaves.
- Verify queue export/import and run export/import preserve both the effective
  member set and the exclusion declaration. Existing source reconciliation,
  array expansion, and dependencies on the matrix base name must continue to
  operate over actual members only. Re-importing a run after a group ID changes
  must still link carried matrix members through their source identity. When
  multiple run snapshots contain different matrix group generations, retain
  an older excluded member as a standalone job rather than a partial matrix
  group, and keep dependencies attached to the current group.

### 4. Contracts, documentation, and interface coverage

- Update `docs/WORKFLOW_MANIFESTS.md` with `matrix_exclude` syntax, partial-match semantics,
  validation errors, and export round-trip behavior; update the FAQ's matrix
  limitations if needed.
- Update the matrix behavior in
  `contracts/02-run-lifecycle-and-execution.md` and its status/representative
  test entry in `contracts/README.md`; document both CLI and manifest surfaces.
- Add binary-level conformance for manifest and CLI add/export behavior.
- Add `matrix-exclude` to the schema-driven `add` flag-pair inventory and
  samples. Supply a witness matrix when testing `matrix-exclude` alone, and
  separately check persisted matrix values/exclusions so an accepted but
  ignored option cannot pass a generic baseline comparison.
- Regenerate CLI references and goldens. Do not change Web rendering unless
  implementation evidence shows that an existing Web contract is violated.

## Tests and completion criteria

- Model tests cover stable ordering, partial matching, overlapping rules,
  malformed rules, all-combinations-excluded rejection, exact group
  validation, and provenance consistency.
- Workflow tests cover YAML/JSON/TOML decode and encode, compilation with and
  without arrays/dependencies, import's effective command count and instance
  matching, queue export round-trip, settled-run export round-trip, carried
  matrix re-import across group generations, and multi-run merge with an older
  excluded member.
- Queue mutation tests prove that a complete excluded matrix retains
  provenance and that removing/copying only some effective members clears it.
- Conformance verifies manifest and CLI add/import/export behavior, matrix-less
  CLI rejection, and the relevant contract ID through the built binary.
- CLI tests verify repeatable exclusions, array composition, no-matrix
  rejection, malformed/undeclared values, all-excluded rejection, and stored
  exclusion provenance. The add flag-pair inventory, valid sample strategy,
  and distinguishing effect witness include the new option.
- Generated CLI reference, schema/help goldens, Python CLI wrapper, and Python
  API documentation are regenerated and verified.
- Run focused model/workflow tests first, then affected package tests,
  conformance tests, formatting/vet, and the repository's full check before
  completion. Inspect the final diff and update this plan with decisions and
  status as implementation advances.

## Non-goals

- Adding matrix `include`, row merging, or a Web control for exclusions.
- Changing the order or naming of existing matrix combinations.
- Treating manually deleted combinations as declared exclusions; partial
  queue edits continue to drop matrix provenance.

## Validation results

- Focused uncached model, workflow, queueedit, queueops, and CLI tests passed.
- Uncached race tests for model, workflow, queueedit, queueops, doclinks, and
  CLI passed. The final binary conformance case passed normally and with the
  race detector, including both queue and settled-run export/import.
- Contract status, conformance layout, Markdown links, Go formatting, and
  diff whitespace checks passed. `pre-commit` was unavailable in PATH.
- Added repeatable command-line-only `rotari add --matrix-exclude`; supplying
  it without `--matrix` errors. The parser/add/array/config tests passed, as
  did the binary CLI/manifest export-import and no-matrix conformance case.
  The add flag-pair inventory now has 26 flags and 325 pairs; the complete
  `TestCLIFlagPairEdits` suite passed (1,545 pairs across add/change/copy/import,
  3,090 invocations). The dedicated exclusion-effect witness and all add pairs
  involving the new flag passed. CLI/Python/API docs and help/schema goldens
  were regenerated; generated CLI reference checks, README sync, and strict
  MkDocs build passed.
- After the CLI change, `go vet ./...`, uncached affected Go package tests,
  `TestWorkflowMatrixExclusionExportImport` (normal and race),
  `TestCLIFlagPairInventory`, `TestCLIFlagPairEditSamples`, and
  `TestCLIAddMatrixExclusionEffect` passed. The focused race run for
  `cmd/rotari`, model, workflow, queueedit, and queueops also passed.
- Full checks were attempted. Earlier runs completed all normal tests, but
  race validation failed once at the pairedits package's ten-minute timeout
  and once at a CLI vet dependency import error. The CLI race package passed
  on an uncached focused rerun; this does not establish the import error's
  cause. The later full run passed pairedits race in 535.561 seconds.
- The final full check exited 1 during normal tests at
  `TestCLIFlagPairWebStaticServerOptions/allow-control`: fixture snapshotting
  reported `lstat .../projects/pairs/server.pid: no such file or directory`.
  The script therefore did not reach its race phase. The full log is retained
  as `$TMPDIR/matrix-exclude-final-check.log`, with the pipeline preserving the
  command's exit code. See the static-Web fixture entry in
  [../ISSUES.md](../ISSUES.md). No unrelated production changes were made to
  turn these failures into passes.

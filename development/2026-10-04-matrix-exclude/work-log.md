# Matrix exclusion work log

## Implementation and export/import preservation

**Commits:**

- `a68f1cc` — 2026-10-04T02:24:21+09:00
- `fa42a91` — 2026-10-04T02:27:18+09:00
- `c5f650b` — 2026-10-04T02:28:06+09:00

**Change:** Added workflow job-level `matrix_exclude` for YAML, JSON, and
TOML. Model expansion validates and normalizes partial exclusion rules;
persisted matrix provenance, group validation, queue mutation completeness,
and workflow command counts use the remaining combinations. Queue/run export
retains the rules. Reconciliation follows carried matrix members across group
generations and reports excluded source members as removed. Multi-run merge
keeps an older excluded member as a standalone job without redirecting the
current group's dependencies. Added unit and binary conformance coverage,
RUN-8, documentation, and the final-check fixture issue. A commit retry added
duplicate tracking rows after the implementation was already committed; the
last commit removes only those duplicate rows.

**Reason:** Support sparse matrices without losing their declaration through
export/import. The matrix-specific field name avoids a generic job-level
`exclude` option.

**Plan impact:** Manifest implementation completed; at this commit the CLI
surface was still outstanding. Web exclusion is inferred from absent cells
in the existing dimensions-by-members grid and needs no additional rule field.

**Validation:** Focused uncached model/workflow/queueedit/queueops/CLI tests
passed. Uncached race tests for those packages passed; doclinks also passed
under race. `TestWorkflowMatrixExclusionExportImport` passed normally and
under race for queue and settled-run round trips. Formatting, diff whitespace,
contract status, conformance layout, and Markdown links passed. The carried
group-generation and historical-member merge regression tests were observed
failing before their fixes and passing afterward. `pre-commit` was unavailable.
Full checks were attempted but none completed cleanly: an earlier race run
hit pairedits' ten-minute timeout; another hit a CLI dependency import error
(the uncached CLI race rerun passed). The final full check exited 1 at the Web
fixture snapshot's disappearing `server.pid` and did not enter its race
phase. Its complete log is retained at
`$TMPDIR/matrix-exclude-final-check.log`. No full-check pass is claimed.

**Remaining at this commit:** CLI support was added in the follow-up entry
below. The static-Web fixture snapshot issue in [../ISSUES.md](../ISSUES.md)
needs separate investigation; a clean full repository check remains
outstanding.

## CLI `add --matrix-exclude`

**Commit:** `22f244f` — 2026-10-04T03:49:26+09:00

**Change:** Added repeatable, command-line-only
`rotari add --matrix-exclude KEY=VALUE[,KEY=VALUE...]`, reusing the shared
model parser and effective matrix expansion. The command rejects exclusions
without `--matrix`; exclusions must refer to declared dimensions/values and
leave at least one combination. Added unit, binary conformance, config
isolation, matrix/array, and flag-pair effect tests. Updated contracts, user
guides, agent guide, flag-pair inventory and coverage counts, help/schema
goldens, CLI reference, Python CLI metadata, and Python API docs. Updated the
plan to cover both workflow and CLI surfaces.

**Reason:** The earlier implementation mistakenly scoped exclusions to
manifests even though the matrix is created by `rotari add`; CLI parity was
part of the intended feature.

**Plan impact:** The existing RUN-8 contract now requires the manifest and CLI
surfaces to share behavior, and explicitly requires `--matrix` when an
exclusion is supplied.

**Validation:** `go test -count=1 ./cmd/rotari -run
'TestCommandLineOnlyFlagIgnoresConfigAndStaysOutOfTemplate|TestCmdAddMatrixExclusion'`,
the model/parser focused tests, `TestWorkflowMatrixExclusionExportImport`,
`TestCLIFlagPairInventory`, `TestCLIFlagPairEditSamples`, and the full
`TestCLIFlagPairEdits` suite passed. The latter covered 1,545 pairs / 3,090
invocations and took 259.561 seconds. `TestCLIAddMatrixExclusionEffect` and
all add pairs involving `matrix-exclude` passed. `TestContractStatus`,
`TestConformanceLayout`, `TestGoldenOutputs`, generated CLI reference checks,
README sync, strict MkDocs build, `gofmt`, and `git diff --check` passed. A
post-refactor uncached race run passed for CLI/model/workflow/queue packages,
and the matrix manifest+CLI binary conformance passed with race enabled. After
the final helper extraction, all 325 `add` flag pairs passed again, including
every pair involving `matrix-exclude`. `go vet` passed for the affected Go
packages. A fresh full `scripts/check.sh` was not rerun after this follow-up;
the prior full-check failure remains recorded above. `ruff` 0.16.9 was
installed in the selected system Python environment to apply the exact CI
generation format.

**Remaining:** A clean full repository check is outstanding; the recorded
static-Web fixture snapshot failure is unrelated to the matrix-exclusion
changes.

**CLI implementation commit:** Pending at the time this work-log entry was
written.

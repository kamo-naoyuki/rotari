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

**Plan impact:** Implementation is complete for workflow manifests; CLI
`add --matrix` and Web rendering remain unchanged as planned.

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

**Remaining:** The static-Web fixture snapshot issue in
[../ISSUES.md](../ISSUES.md) needs separate investigation. A clean full
repository check remains outstanding; no matrix-specific failures remain.
